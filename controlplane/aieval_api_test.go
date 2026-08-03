// SPDX-License-Identifier: Apache-2.0
package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleAIEvalStatsReturnsRealAggregates(t *testing.T) {
	store := testStore(t)
	tenantID := testTenant(t, store)

	if err := store.RecordAIEvalEvent(tenantID, AIEvalEvent{Feature: "chat", LatencyMS: 150, Success: true, Rounds: 1}); err != nil {
		t.Fatalf("RecordAIEvalEvent: %v", err)
	}

	req := withTenant(httptest.NewRequest(http.MethodGet, "/api/v1/ai-eval/stats", nil), tenantID)
	rec := httptest.NewRecorder()
	handleAIEvalStats(store)(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	var got aiEvalStatsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Features) != 1 || got.Features[0].Feature != "chat" || got.Features[0].TotalRequests != 1 {
		t.Fatalf("unexpected stats: %+v", got)
	}
}

func TestHandleAIEvalStatsIsIsolatedByTenant(t *testing.T) {
	store := testStore(t)
	tenantA := testTenant(t, store)
	tenantB := testTenant(t, store)

	if err := store.RecordAIEvalEvent(tenantA, AIEvalEvent{Feature: "chat", LatencyMS: 100, Success: true}); err != nil {
		t.Fatalf("RecordAIEvalEvent: %v", err)
	}

	req := withTenant(httptest.NewRequest(http.MethodGet, "/api/v1/ai-eval/stats", nil), tenantB)
	rec := httptest.NewRecorder()
	handleAIEvalStats(store)(rec, req)

	var got aiEvalStatsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Features) != 0 {
		t.Fatalf("tenant b's ai-eval stats route leaked tenant a's data: %+v", got)
	}
}

func TestHandleAIEvalStatsRejectsWrongMethod(t *testing.T) {
	store := testStore(t)
	tenantID := testTenant(t, store)

	req := withTenant(httptest.NewRequest(http.MethodPost, "/api/v1/ai-eval/stats", nil), tenantID)
	rec := httptest.NewRecorder()
	handleAIEvalStats(store)(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
}
