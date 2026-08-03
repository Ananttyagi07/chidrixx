// SPDX-License-Identifier: Apache-2.0
package main

import "testing"

func TestAIEvalStatsWithNoEventsIsEmpty(t *testing.T) {
	s := testStore(t)
	tenantID := testTenant(t, s)

	stats, err := s.AIEvalStats(tenantID)
	if err != nil {
		t.Fatalf("AIEvalStats: %v", err)
	}
	if len(stats) != 0 {
		t.Fatalf("expected no real stats with no events recorded, got %+v", stats)
	}
}

func TestAIEvalStatsAggregatesRealEventsPerFeature(t *testing.T) {
	s := testStore(t)
	tenantID := testTenant(t, s)

	events := []AIEvalEvent{
		{Feature: "chat", LatencyMS: 100, Success: true, Rounds: 2, ToolCallCount: 1, ToolCallErrors: 0, PromptTokens: 50, CompletionTokens: 20},
		{Feature: "chat", LatencyMS: 300, Success: false, Rounds: 1, ToolCallCount: 1, ToolCallErrors: 1, PromptTokens: 40, CompletionTokens: 0, ErrorMessage: "boom"},
		{Feature: "anomaly_narrator", LatencyMS: 200, Success: true, Rounds: 1, PromptTokens: 30, CompletionTokens: 10},
	}
	for _, ev := range events {
		if err := s.RecordAIEvalEvent(tenantID, ev); err != nil {
			t.Fatalf("RecordAIEvalEvent: %v", err)
		}
	}

	stats, err := s.AIEvalStats(tenantID)
	if err != nil {
		t.Fatalf("AIEvalStats: %v", err)
	}
	if len(stats) != 2 {
		t.Fatalf("expected 2 real feature rows (chat, anomaly_narrator), got %d: %+v", len(stats), stats)
	}

	// Sorted alphabetically by feature: anomaly_narrator, chat.
	narrator, chat := stats[0], stats[1]
	if narrator.Feature != "anomaly_narrator" || chat.Feature != "chat" {
		t.Fatalf("unexpected feature ordering: %+v", stats)
	}

	if chat.TotalRequests != 2 || chat.SuccessCount != 1 {
		t.Fatalf("expected chat: 2 requests, 1 success, got %+v", chat)
	}
	if chat.SuccessRate != 0.5 {
		t.Fatalf("expected chat success rate 0.5, got %v", chat.SuccessRate)
	}
	if chat.AvgLatencyMS != 200 {
		t.Fatalf("expected chat avg latency 200ms ((100+300)/2), got %v", chat.AvgLatencyMS)
	}
	if chat.TotalToolCalls != 2 || chat.TotalToolCallErrors != 1 {
		t.Fatalf("expected chat: 2 tool calls, 1 error, got %+v", chat)
	}
	if chat.ToolSuccessRate == nil || *chat.ToolSuccessRate != 0.5 {
		t.Fatalf("expected chat tool success rate 0.5, got %+v", chat.ToolSuccessRate)
	}
	if chat.TotalPromptTokens != 90 || chat.TotalCompletionTokens != 20 {
		t.Fatalf("expected chat real token sums (90, 20), got %+v", chat)
	}

	if narrator.TotalRequests != 1 || narrator.SuccessCount != 1 || narrator.SuccessRate != 1 {
		t.Fatalf("expected narrator: 1 request, 1 success, rate 1.0, got %+v", narrator)
	}
	if narrator.ToolSuccessRate != nil {
		t.Fatalf("expected a nil tool success rate for a feature with zero real tool calls, got %v", *narrator.ToolSuccessRate)
	}
}

func TestAIEvalStatsIsolatesByTenant(t *testing.T) {
	s := testStore(t)
	tenantA := testTenant(t, s)
	tenantB := testTenant(t, s)

	if err := s.RecordAIEvalEvent(tenantA, AIEvalEvent{Feature: "chat", LatencyMS: 100, Success: true}); err != nil {
		t.Fatalf("RecordAIEvalEvent: %v", err)
	}

	statsB, err := s.AIEvalStats(tenantB)
	if err != nil {
		t.Fatalf("AIEvalStats(tenantB): %v", err)
	}
	if len(statsB) != 0 {
		t.Fatalf("tenant b's ai eval stats leaked tenant a's data: %+v", statsB)
	}
}
