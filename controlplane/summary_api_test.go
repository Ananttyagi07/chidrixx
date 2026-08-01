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
	tenantID := testTenant(t, store)

	if err := store.Ingest(tenantID, "cluster-a", []Finding{
		{Source: "ns/app", Destination: "8.8.8.8", PathClass: "INTERNET_EGRESS", Confidence: "high",
			BytesTx: 100, CostLowINR: 1, CostHighINR: 2, FixHint: "confirm this needs to leave"},
		{Source: "ns/db", Destination: "ns/app", PathClass: "SAME_NODE", Confidence: "high"},
	}); err != nil {
		t.Fatalf("Ingest: %v", err)
	}

	req := withTenant(httptest.NewRequest(http.MethodGet, "/api/v1/dashboard-summary", nil), tenantID)
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
	if len(got.SpendByCloud) != 1 || got.SpendByCloud[0].Cloud != "unknown" {
		t.Errorf("expected a single unknown cloud bucket (no Cloud set in this test's findings), got %+v", got.SpendByCloud)
	}
	if len(got.Clusters) != 1 || got.Clusters[0].ClusterID != "cluster-a" {
		t.Errorf("unexpected clusters: %+v", got.Clusters)
	}
	if len(got.TopFixes) != 1 || got.TopFixes[0].Source != "ns/app" {
		t.Errorf("expected the one finding with a fix hint, got: %+v", got.TopFixes)
	}
	if len(got.SpendByTeam) != 1 || got.SpendByTeam[0].Team != unassignedTeam || got.SpendByTeam[0].CostHighINR != 2 {
		t.Errorf("expected everything Unassigned (no team ownership configured in this test), got: %+v", got.SpendByTeam)
	}
}

// TestHandleDashboardSummaryEmptyStateHasNoNullArrays guards a real bug: Go's
// nil-slice zero value marshals to JSON `null`, not `[]`. The frontend calls
// .map() directly on these fields (e.g. the trend series backing the total
// spend sparkline) without a null-guard, so a brand-new install with zero
// ingests ever received would crash the dashboard on first load.
func TestHandleDashboardSummaryEmptyStateHasNoNullArrays(t *testing.T) {
	store := testStore(t)
	tenantID := testTenant(t, store)

	req := withTenant(httptest.NewRequest(http.MethodGet, "/api/v1/dashboard-summary", nil), tenantID)
	rec := httptest.NewRecorder()
	handleDashboardSummary(store)(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	for _, field := range []string{`"spend_by_class":null`, `"spend_by_cloud":null`, `"spend_by_team":null`, `"trend":null`, `"clusters":null`, `"top_fixes":null`, `"anomalies":null`} {
		if bytes.Contains(rec.Body.Bytes(), []byte(field)) {
			t.Errorf("empty-state response contains %s — must be [] for the frontend's .map() calls, got: %s", field, rec.Body.String())
		}
	}
}
