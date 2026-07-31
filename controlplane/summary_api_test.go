// SPDX-License-Identifier: Apache-2.0
package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleDashboardSummary(t *testing.T) {
	store := testStore(t)

	if err := store.Ingest("cluster-a", []Finding{
		{Source: "ns/app", Destination: "8.8.8.8", PathClass: "INTERNET_EGRESS", Confidence: "high",
			BytesTx: 100, CostLowINR: 1, CostHighINR: 2, FixHint: "confirm this needs to leave"},
		{Source: "ns/db", Destination: "ns/app", PathClass: "SAME_NODE", Confidence: "high"},
	}); err != nil {
		t.Fatalf("Ingest: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard-summary", nil)
	rec := httptest.NewRecorder()
	handleDashboardSummary(store)(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	var got dashboardSummaryResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if got.Summary.ClusterCount != 1 || got.Summary.FindingCount != 2 {
		t.Errorf("unexpected summary: %+v", got.Summary)
	}
	if len(got.SpendByClass) != 2 {
		t.Errorf("expected 2 path classes, got %+v", got.SpendByClass)
	}
	if len(got.Clusters) != 1 || got.Clusters[0].ClusterID != "cluster-a" {
		t.Errorf("unexpected clusters: %+v", got.Clusters)
	}
	if len(got.TopFixes) != 1 || got.TopFixes[0].Source != "ns/app" {
		t.Errorf("expected the one finding with a fix hint, got: %+v", got.TopFixes)
	}
}

// TestHandleDashboardSummaryEmptyStateHasNoNullArrays guards a real bug: Go's
// nil-slice zero value marshals to JSON `null`, not `[]`. The frontend calls
// .map() directly on these fields (e.g. the trend series backing the total
// spend sparkline) without a null-guard, so a brand-new install with zero
// ingests ever received would crash the dashboard on first load.
func TestHandleDashboardSummaryEmptyStateHasNoNullArrays(t *testing.T) {
	store := testStore(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard-summary", nil)
	rec := httptest.NewRecorder()
	handleDashboardSummary(store)(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	for _, field := range []string{`"spend_by_class":null`, `"trend":null`, `"clusters":null`, `"top_fixes":null`} {
		if bytes.Contains(rec.Body.Bytes(), []byte(field)) {
			t.Errorf("empty-state response contains %s — must be [] for the frontend's .map() calls, got: %s", field, rec.Body.String())
		}
	}
}
