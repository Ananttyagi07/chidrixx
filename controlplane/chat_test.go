// SPDX-License-Identifier: Apache-2.0
package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// fakeGroq stands in for the real Groq chat-completions API, the same
// discipline as fakeSupabase in supabase_auth_test.go: hitting the real
// service on every test run isn't practical (network, rate limits, a
// real key needed). It replays the given responses in order, one per
// request, and asserts the real auth header is present on each call --
// the one contract this code actually depends on.
func fakeGroq(t *testing.T, apiKey string, responses []chatCompletionResponse) *httptest.Server {
	t.Helper()
	call := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer "+apiKey {
			t.Errorf("expected Authorization: Bearer %s, got %q", apiKey, got)
		}
		if call >= len(responses) {
			t.Fatalf("fakeGroq received more requests (%d) than scripted responses (%d)", call+1, len(responses))
		}
		resp := responses[call]
		call++
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func toolCallResponse(id, name, args string) chatCompletionResponse {
	msg := ChatMessage{
		Role: "assistant",
		ToolCalls: []ToolCall{
			{ID: id, Type: "function"},
		},
	}
	msg.ToolCalls[0].Function.Name = name
	msg.ToolCalls[0].Function.Arguments = args
	return chatCompletionResponse{
		Choices: []struct {
			Message      ChatMessage `json:"message"`
			FinishReason string      `json:"finish_reason"`
		}{{Message: msg, FinishReason: "tool_calls"}},
	}
}

func contentResponse(content string) chatCompletionResponse {
	return chatCompletionResponse{
		Choices: []struct {
			Message      ChatMessage `json:"message"`
			FinishReason string      `json:"finish_reason"`
		}{{Message: ChatMessage{Role: "assistant", Content: content}, FinishReason: "stop"}},
	}
}

func TestParseLenientIntHandlesStringNumberAndMissing(t *testing.T) {
	cases := []struct {
		name     string
		raw      string
		fallback int
		want     int
	}{
		{"stringified number (the observed Groq quirk)", `{"limit":"5"}`, 10, 5},
		{"real JSON number", `{"limit":5}`, 10, 5},
		{"missing key falls back", `{}`, 10, 10},
		{"garbage string falls back", `{"limit":"not-a-number"}`, 10, 10},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var req map[string]json.RawMessage
			if err := json.Unmarshal([]byte(c.raw), &req); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			got := parseLenientInt(req["limit"], c.fallback)
			if got != c.want {
				t.Fatalf("parseLenientInt(%s) = %d, want %d", c.raw, got, c.want)
			}
		})
	}
}

func TestGroqClientCompleteReturnsPlainContent(t *testing.T) {
	srv := fakeGroq(t, "test-key", []chatCompletionResponse{contentResponse("hello from groq")})
	client := newGroqClient("test-key", "", srv.URL)

	msg, _, err := client.Complete(t.Context(), []ChatMessage{{Role: "user", Content: "hi"}}, nil)
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if msg.Content != "hello from groq" {
		t.Fatalf("unexpected content: %q", msg.Content)
	}
}

func TestGroqClientCompleteReturnsRealTokenUsage(t *testing.T) {
	resp := contentResponse("hello from groq")
	resp.Usage = &struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	}{PromptTokens: 42, CompletionTokens: 7}
	srv := fakeGroq(t, "test-key", []chatCompletionResponse{resp})
	client := newGroqClient("test-key", "", srv.URL)

	_, usage, err := client.Complete(t.Context(), []ChatMessage{{Role: "user", Content: "hi"}}, nil)
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if usage.PromptTokens != 42 || usage.CompletionTokens != 7 {
		t.Fatalf("expected real usage (42, 7) from the response, got %+v", usage)
	}
}

func TestGroqClientCompleteReturnsToolCall(t *testing.T) {
	srv := fakeGroq(t, "test-key", []chatCompletionResponse{toolCallResponse("call-1", "get_anomalies", "{}")})
	client := newGroqClient("test-key", "", srv.URL)

	msg, _, err := client.Complete(t.Context(), []ChatMessage{{Role: "user", Content: "why did my bill spike"}}, nil)
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if len(msg.ToolCalls) != 1 || msg.ToolCalls[0].Function.Name != "get_anomalies" {
		t.Fatalf("unexpected tool calls: %+v", msg.ToolCalls)
	}
}

func TestGroqClientCompleteSurfacesAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]string{"message": "rate limit exceeded"},
		})
	}))
	t.Cleanup(srv.Close)
	client := newGroqClient("test-key", "", srv.URL)

	_, _, err := client.Complete(t.Context(), []ChatMessage{{Role: "user", Content: "hi"}}, nil)
	if err == nil {
		t.Fatal("expected an error for a real rate-limit response, got nil")
	}
}

func TestRunChatLoopRetriesOnceOnATransientGroqError(t *testing.T) {
	s := testStore(t)
	tenantID := testTenant(t, s)
	call := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call++
		if call == 1 {
			// The real failure mode this is guarding against: Groq's
			// server-side schema validation rejecting a malformed
			// tool-call argument the model generated.
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]string{"message": "tool call validation failed"},
			})
			return
		}
		json.NewEncoder(w).Encode(contentResponse("recovered on retry"))
	}))
	t.Cleanup(srv.Close)
	client := newGroqClient("test-key", "", srv.URL)
	tools := buildChatTools(s, tenantID)

	result, err := runChatLoop(t.Context(), client, tools, toolDefs(tools), []ChatMessage{
		{Role: "user", Content: "hi"},
	})
	if err != nil {
		t.Fatalf("runChatLoop: %v", err)
	}
	if result.Reply != "recovered on retry" {
		t.Fatalf("unexpected reply: %q", result.Reply)
	}
	if call != 2 {
		t.Fatalf("expected exactly 2 calls (1 failure + 1 retry), got %d", call)
	}
}

func TestRunChatLoopCallsToolThenReturnsFinalAnswer(t *testing.T) {
	s := testStore(t)
	tenantID := testTenant(t, s)
	if err := s.Ingest(tenantID, "cluster-a", []Finding{
		{Source: "checkout/checkout-1", Destination: "redis/redis-master", PathClass: "cross_az", CostHighINR: 40, FixHint: "co-locate zones"},
	}); err != nil {
		t.Fatalf("Ingest: %v", err)
	}

	srv := fakeGroq(t, "test-key", []chatCompletionResponse{
		toolCallResponse("call-1", "get_top_fixes", `{"limit":5}`),
		contentResponse("Your top fix is checkout -> redis, costing ₹40."),
	})
	client := newGroqClient("test-key", "", srv.URL)
	tools := buildChatTools(s, tenantID)

	result, err := runChatLoop(t.Context(), client, tools, toolDefs(tools), []ChatMessage{
		{Role: "system", Content: chatSystemPrompt},
		{Role: "user", Content: "what should I fix first?"},
	})
	if err != nil {
		t.Fatalf("runChatLoop: %v", err)
	}
	if result.Reply != "Your top fix is checkout -> redis, costing ₹40." {
		t.Fatalf("unexpected reply: %q", result.Reply)
	}
	if result.ToolCallCount != 1 || result.ToolCallErrors != 0 {
		t.Fatalf("expected 1 real tool call with no errors, got %+v", result)
	}
}

func TestRunChatLoopStopsAfterMaxRoundsInsteadOfLoopingForever(t *testing.T) {
	s := testStore(t)
	tenantID := testTenant(t, s)

	// Script more tool-call-only responses than maxChatToolRounds -- the
	// loop must bail out with a real message, not hang or panic.
	responses := make([]chatCompletionResponse, maxChatToolRounds)
	for i := range responses {
		responses[i] = toolCallResponse("call", "get_anomalies", "{}")
	}
	srv := fakeGroq(t, "test-key", responses)
	client := newGroqClient("test-key", "", srv.URL)
	tools := buildChatTools(s, tenantID)

	result, err := runChatLoop(t.Context(), client, tools, toolDefs(tools), []ChatMessage{
		{Role: "user", Content: "loop forever"},
	})
	if err != nil {
		t.Fatalf("runChatLoop: %v", err)
	}
	if result.Reply == "" {
		t.Fatal("expected a real fallback message, not an empty reply")
	}
	if !result.HitRoundLimit {
		t.Fatal("expected HitRoundLimit to be true when the loop exhausts maxChatToolRounds")
	}
	if result.Rounds != maxChatToolRounds {
		t.Fatalf("expected Rounds = %d, got %d", maxChatToolRounds, result.Rounds)
	}
}

func TestRunChatLoopHandlesUnknownToolGracefully(t *testing.T) {
	s := testStore(t)
	tenantID := testTenant(t, s)

	srv := fakeGroq(t, "test-key", []chatCompletionResponse{
		toolCallResponse("call-1", "delete_everything", "{}"),
		contentResponse("I can't do that."),
	})
	client := newGroqClient("test-key", "", srv.URL)
	tools := buildChatTools(s, tenantID)

	result, err := runChatLoop(t.Context(), client, tools, toolDefs(tools), []ChatMessage{
		{Role: "user", Content: "delete everything"},
	})
	if err != nil {
		t.Fatalf("runChatLoop: %v", err)
	}
	if result.Reply != "I can't do that." {
		t.Fatalf("unexpected reply: %q", result.Reply)
	}
	if result.ToolCallCount != 1 || result.ToolCallErrors != 1 {
		t.Fatalf("expected 1 real tool call that errored (unknown tool), got %+v", result)
	}
}

func TestChatToolsAreIsolatedByTenant(t *testing.T) {
	s := testStore(t)
	tenantA := testTenant(t, s)
	tenantB := testTenant(t, s)

	if err := s.Ingest(tenantA, "cluster-a", []Finding{
		{Source: "checkout/checkout-1", Destination: "redis/redis-master", PathClass: "cross_az", CostHighINR: 40, FixHint: "co-locate zones"},
	}); err != nil {
		t.Fatalf("Ingest: %v", err)
	}

	toolsB := buildChatTools(s, tenantB)
	tool, ok := findTool(toolsB, "get_top_fixes")
	if !ok {
		t.Fatal("expected get_top_fixes tool to exist")
	}
	out, err := tool.call(json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("tool.call: %v", err)
	}
	var rows []FindingRow
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("decode tool output: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("tenant b's tool call saw tenant a's data: %s", out)
	}
}

func TestHandleChatReturns503WhenNotConfigured(t *testing.T) {
	store := testStore(t)
	tenantID := testTenant(t, store)

	body, _ := json.Marshal(chatRequest{Message: "hi"})
	req := withTenant(httptest.NewRequest(http.MethodPost, "/api/v1/chat", bytes.NewReader(body)), tenantID)
	rec := httptest.NewRecorder()
	handleChat(store, nil)(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503 when no GROQ_API_KEY is configured", rec.Code)
	}
}

func TestHandleChatRequiresMessage(t *testing.T) {
	store := testStore(t)
	tenantID := testTenant(t, store)
	srv := fakeGroq(t, "test-key", nil)
	client := newGroqClient("test-key", "", srv.URL)

	body, _ := json.Marshal(chatRequest{Message: ""})
	req := withTenant(httptest.NewRequest(http.MethodPost, "/api/v1/chat", bytes.NewReader(body)), tenantID)
	rec := httptest.NewRecorder()
	handleChat(store, client)(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for an empty message", rec.Code)
	}
}

func TestHandleChatEndToEndWithRealDataAndFakeGroq(t *testing.T) {
	store := testStore(t)
	tenantID := testTenant(t, store)
	if err := store.Ingest(tenantID, "cluster-a", []Finding{
		{Source: "checkout/checkout-1", Destination: "redis/redis-master", PathClass: "cross_az", CostHighINR: 40, FixHint: "co-locate zones"},
	}); err != nil {
		t.Fatalf("Ingest: %v", err)
	}

	srv := fakeGroq(t, "test-key", []chatCompletionResponse{
		toolCallResponse("call-1", "get_top_fixes", `{}`),
		contentResponse("checkout -> redis is your top real cost driver at ₹40."),
	})
	client := newGroqClient("test-key", "", srv.URL)

	body, _ := json.Marshal(chatRequest{Message: "what should I fix first?"})
	req := withTenant(httptest.NewRequest(http.MethodPost, "/api/v1/chat", bytes.NewReader(body)), tenantID)
	rec := httptest.NewRecorder()
	handleChat(store, client)(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	var got chatResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Reply != "checkout -> redis is your top real cost driver at ₹40." {
		t.Fatalf("unexpected reply: %q", got.Reply)
	}

	// The real point of aieval.go: this actual HTTP call must leave a
	// real observability record behind, not just a chat reply.
	stats, err := store.AIEvalStats(tenantID)
	if err != nil {
		t.Fatalf("AIEvalStats: %v", err)
	}
	if len(stats) != 1 || stats[0].Feature != "chat" || stats[0].TotalRequests != 1 || stats[0].SuccessCount != 1 {
		t.Fatalf("expected handleChat to record a real successful ai eval event, got %+v", stats)
	}
	if stats[0].TotalToolCalls != 1 {
		t.Fatalf("expected the real tool call to be counted, got %+v", stats[0])
	}
}

func TestHandleChatRecordsAFailedAIEvalEventOnGroqError(t *testing.T) {
	store := testStore(t)
	tenantID := testTenant(t, store)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"message": "rate limited"}})
	}))
	t.Cleanup(srv.Close)
	client := newGroqClient("test-key", "", srv.URL)

	body, _ := json.Marshal(chatRequest{Message: "hi"})
	req := withTenant(httptest.NewRequest(http.MethodPost, "/api/v1/chat", bytes.NewReader(body)), tenantID)
	rec := httptest.NewRecorder()
	handleChat(store, client)(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", rec.Code)
	}

	stats, err := store.AIEvalStats(tenantID)
	if err != nil {
		t.Fatalf("AIEvalStats: %v", err)
	}
	// completeWithRetry retries once, so this is really 2 real failed
	// Groq calls but still exactly 1 real recorded chat request/outcome.
	if len(stats) != 1 || stats[0].SuccessCount != 0 || stats[0].TotalRequests != 1 {
		t.Fatalf("expected 1 real recorded failed request, got %+v", stats)
	}
}
