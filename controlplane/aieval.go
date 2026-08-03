// SPDX-License-Identifier: Apache-2.0
package main

import (
	"fmt"
	"time"
)

// AIEvalEvent is one real, complete record of a single AI feature
// invocation -- not a sampled/estimated metric, the actual outcome of
// that one real request. "feature" is a small fixed set ("chat",
// "anomaly_narrator") so stats can be broken down per real AI surface
// rather than blended into one meaningless average.
type AIEvalEvent struct {
	Feature          string
	LatencyMS        int64
	Success          bool
	HitRoundLimit    bool
	Rounds           int
	ToolCallCount    int
	ToolCallErrors   int
	PromptTokens     int
	CompletionTokens int
	ErrorMessage     string
}

// RecordAIEvalEvent persists one real AI invocation's outcome. Called
// best-effort from the chat and anomaly-narrator handlers, the same
// pattern as RecordRecommendationsShown -- a telemetry-write failure
// must never take down the actual user-facing feature it's observing.
func (s *Store) RecordAIEvalEvent(tenantID int64, ev AIEvalEvent) error {
	_, err := s.db.Exec(`
		INSERT INTO ai_eval_events
			(tenant_id, feature, occurred_at, latency_ms, success, hit_round_limit,
			 rounds, tool_call_count, tool_call_errors, prompt_tokens, completion_tokens, error_message)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, tenantID, ev.Feature, time.Now().Unix(), ev.LatencyMS, boolToInt(ev.Success), boolToInt(ev.HitRoundLimit),
		ev.Rounds, ev.ToolCallCount, ev.ToolCallErrors, ev.PromptTokens, ev.CompletionTokens, nullableString(ev.ErrorMessage),
	)
	if err != nil {
		return fmt.Errorf("record ai eval event: %w", err)
	}
	return nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func nullableString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// AIEvalFeatureStats is one feature's real, aggregated quality signal --
// the actual answer to "is the AI working," not a marketing claim.
type AIEvalFeatureStats struct {
	Feature               string   `json:"feature"`
	TotalRequests         int      `json:"total_requests"`
	SuccessCount          int      `json:"success_count"`
	SuccessRate           float64  `json:"success_rate"`
	HitRoundLimitCount    int      `json:"hit_round_limit_count"`
	AvgLatencyMS          float64  `json:"avg_latency_ms"`
	TotalToolCalls        int      `json:"total_tool_calls"`
	TotalToolCallErrors   int      `json:"total_tool_call_errors"`
	ToolSuccessRate       *float64 `json:"tool_success_rate,omitempty"`
	TotalPromptTokens     int      `json:"total_prompt_tokens"`
	TotalCompletionTokens int      `json:"total_completion_tokens"`
}

// AIEvalStats returns one tenant's real, current AI-quality stats, broken
// down per feature -- the data behind the "AI evaluation" section, not
// an estimate or a sampled trace.
func (s *Store) AIEvalStats(tenantID int64) ([]AIEvalFeatureStats, error) {
	rows, err := s.db.Query(`
		SELECT feature, COUNT(*), SUM(success), SUM(hit_round_limit), AVG(latency_ms),
		       SUM(tool_call_count), SUM(tool_call_errors), SUM(prompt_tokens), SUM(completion_tokens)
		FROM ai_eval_events
		WHERE tenant_id = ?
		GROUP BY feature
		ORDER BY feature
	`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("query ai eval stats: %w", err)
	}
	defer rows.Close()

	out := make([]AIEvalFeatureStats, 0)
	for rows.Next() {
		var st AIEvalFeatureStats
		if err := rows.Scan(
			&st.Feature, &st.TotalRequests, &st.SuccessCount, &st.HitRoundLimitCount, &st.AvgLatencyMS,
			&st.TotalToolCalls, &st.TotalToolCallErrors, &st.TotalPromptTokens, &st.TotalCompletionTokens,
		); err != nil {
			return nil, fmt.Errorf("scan ai eval stats: %w", err)
		}
		if st.TotalRequests > 0 {
			st.SuccessRate = float64(st.SuccessCount) / float64(st.TotalRequests)
		}
		if st.TotalToolCalls > 0 {
			rate := 1 - float64(st.TotalToolCallErrors)/float64(st.TotalToolCalls)
			st.ToolSuccessRate = &rate
		}
		out = append(out, st)
	}
	return out, rows.Err()
}
