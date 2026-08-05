// SPDX-License-Identifier: Apache-2.0
package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleRemediationPreviewReturnsRealDecisions(t *testing.T) {
	store := testStore(t)
	tenantID := testTenant(t, store)

	if err := store.Ingest(tenantID, "cluster-a", []Finding{
		{
			Source: "checkout/checkout-1", Destination: "8.8.8.8",
			PathClass: "INTERNET_EGRESS", Confidence: "high",
			FixHint: "deny this destination", FixManifest: realGeneratedManifest,
			CostHighINR: 40, SavingsHighINR: 35,
		},
	}); err != nil {
		t.Fatalf("Ingest: %v", err)
	}

	req := withTenant(httptest.NewRequest(http.MethodGet, "/api/v1/remediation/preview", nil), tenantID)
	rec := httptest.NewRecorder()
	handleRemediationPreview(store)(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	var got remediationPreviewResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Decisions) != 1 || !got.Decisions[0].WouldAutoApply {
		t.Fatalf("expected 1 real qualifying decision, got: %+v", got.Decisions)
	}
	if got.Decisions[0].TrafficReplay == nil || !got.Decisions[0].TrafficReplay.Safe {
		t.Fatalf("expected a real traffic-replay result confirming safety end-to-end, got: %+v", got.Decisions[0].TrafficReplay)
	}
}

// TestHandleRemediationPreviewDisqualifiesAFixWithRealCollateralTraffic is
// the real end-to-end proof of the traffic-replay safety check: a fix that
// clears every other bar (real manifest, high confidence, real savings)
// but whose generated policy would also block a genuinely different
// workload in the same namespace, evidenced by this tenant's own real
// ingested traffic -- through the real store and the real handler, not a
// fake fetch closure.
func TestHandleRemediationPreviewDisqualifiesAFixWithRealCollateralTraffic(t *testing.T) {
	store := testStore(t)
	tenantID := testTenant(t, store)

	if err := store.Ingest(tenantID, "cluster-a", []Finding{
		{
			Source: "checkout/checkout-1", Destination: "8.8.8.8",
			PathClass: "INTERNET_EGRESS", Confidence: "high",
			FixHint: "deny this destination", FixManifest: realGeneratedManifest,
			CostHighINR: 40, SavingsHighINR: 35,
		},
		{
			// A real, different workload in the same real namespace,
			// talking to the same real destination -- podSelector: {}
			// means the generated policy would block this one too.
			Source: "checkout/checkout-2", Destination: "8.8.8.8",
			PathClass: "INTERNET_EGRESS", Confidence: "high",
			CostHighINR: 10,
		},
	}); err != nil {
		t.Fatalf("Ingest: %v", err)
	}

	req := withTenant(httptest.NewRequest(http.MethodGet, "/api/v1/remediation/preview", nil), tenantID)
	rec := httptest.NewRecorder()
	handleRemediationPreview(store)(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	var got remediationPreviewResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Decisions) != 1 {
		t.Fatalf("expected exactly 1 decision (only the fix-hint-bearing finding), got: %+v", got.Decisions)
	}
	d := got.Decisions[0]
	if d.WouldAutoApply {
		t.Fatalf("expected real collateral traffic to disqualify this fix end-to-end, got: %+v", d)
	}
	if d.TrafficReplay == nil {
		t.Fatal("expected a real traffic-replay result attached")
	}
	if len(d.TrafficReplay.AffectedWorkloads) != 1 || d.TrafficReplay.AffectedWorkloads[0] != "checkout/checkout-2" {
		t.Fatalf("expected checkout/checkout-2 identified as real collateral, got: %+v", d.TrafficReplay.AffectedWorkloads)
	}
	if d.TrafficReplay.CollateralCostINR != 10 {
		t.Fatalf("expected a real 10 INR collateral cost, got %v", d.TrafficReplay.CollateralCostINR)
	}
}

func TestHandleRemediationPreviewIsIsolatedByTenant(t *testing.T) {
	store := testStore(t)
	tenantA := testTenant(t, store)
	tenantB := testTenant(t, store)

	if err := store.Ingest(tenantA, "cluster-a", []Finding{
		{
			Source: "checkout/checkout-1", Destination: "8.8.8.8",
			PathClass: "INTERNET_EGRESS", Confidence: "high",
			FixHint: "deny this destination", FixManifest: realGeneratedManifest,
			CostHighINR: 40, SavingsHighINR: 35,
		},
	}); err != nil {
		t.Fatalf("Ingest: %v", err)
	}

	req := withTenant(httptest.NewRequest(http.MethodGet, "/api/v1/remediation/preview", nil), tenantB)
	rec := httptest.NewRecorder()
	handleRemediationPreview(store)(rec, req)

	var got remediationPreviewResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Decisions) != 0 {
		t.Fatalf("tenant b saw tenant a's remediation decisions: %+v", got.Decisions)
	}
}

func TestHandleRemediationPreviewRejectsWrongMethod(t *testing.T) {
	store := testStore(t)
	tenantID := testTenant(t, store)

	req := withTenant(httptest.NewRequest(http.MethodPost, "/api/v1/remediation/preview", nil), tenantID)
	rec := httptest.NewRecorder()
	handleRemediationPreview(store)(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
}
