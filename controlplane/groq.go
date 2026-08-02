// SPDX-License-Identifier: Apache-2.0
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// GroqClient talks to Groq's OpenAI-compatible chat completions API
// (https://api.groq.com/openai/v1/chat/completions). Groq, not a
// self-hosted model: this machine has no GPU, and a model capable of
// reliable tool-calling needs one to run at usable chat speed -- Groq's
// free tier gives real inference against a real hosted model without
// that infrastructure cost.
type GroqClient struct {
	apiKey     string
	model      string
	baseURL    string
	httpClient *http.Client
}

func NewGroqClient(apiKey, model string) *GroqClient {
	return newGroqClient(apiKey, model, "https://api.groq.com/openai/v1")
}

// newGroqClientWithBaseURL lets tests point this at a real httptest.Server
// standing in for Groq's API contract, the same pattern used for
// SupabaseAuthenticator in supabase_auth_test.go.
func newGroqClient(apiKey, model, baseURL string) *GroqClient {
	if model == "" {
		model = "llama-3.3-70b-versatile"
	}
	return &GroqClient{
		apiKey:     apiKey,
		model:      model,
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// ChatMessage mirrors the OpenAI-compatible message shape Groq expects.
type ChatMessage struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

type ToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// ToolDef is an OpenAI-compatible function-calling tool declaration.
type ToolDef struct {
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

type ToolFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type chatCompletionRequest struct {
	Model       string        `json:"model"`
	Messages    []ChatMessage `json:"messages"`
	Tools       []ToolDef     `json:"tools,omitempty"`
	ToolChoice  string        `json:"tool_choice,omitempty"`
	Temperature float64       `json:"temperature"`
}

type chatCompletionResponse struct {
	Choices []struct {
		Message      ChatMessage `json:"message"`
		FinishReason string      `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// Complete sends one real chat-completion request and returns the
// model's response message (which may itself be a tool-call request,
// not a final answer -- the caller drives the loop).
func (c *GroqClient) Complete(ctx context.Context, messages []ChatMessage, tools []ToolDef) (ChatMessage, error) {
	reqBody := chatCompletionRequest{
		Model:       c.model,
		Messages:    messages,
		Tools:       tools,
		Temperature: 0.2, // grounded answers, not creative ones
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return ChatMessage{}, fmt.Errorf("marshal groq request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return ChatMessage{}, fmt.Errorf("build groq request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return ChatMessage{}, fmt.Errorf("groq request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return ChatMessage{}, fmt.Errorf("read groq response: %w", err)
	}

	var parsed chatCompletionResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return ChatMessage{}, fmt.Errorf("decode groq response (status %d): %w", resp.StatusCode, err)
	}
	if parsed.Error != nil {
		return ChatMessage{}, fmt.Errorf("groq API error (status %d): %s", resp.StatusCode, parsed.Error.Message)
	}
	if resp.StatusCode != http.StatusOK {
		return ChatMessage{}, fmt.Errorf("groq API returned status %d: %s", resp.StatusCode, string(respBody))
	}
	if len(parsed.Choices) == 0 {
		return ChatMessage{}, fmt.Errorf("groq response had no choices")
	}
	return parsed.Choices[0].Message, nil
}
