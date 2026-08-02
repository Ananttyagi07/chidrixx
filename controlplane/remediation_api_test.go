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
			FixHint: "deny this destination", FixManifest: "real-manifest-text",
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
}

func TestHandleRemediationPreviewIsIsolatedByTenant(t *testing.T) {
	store := testStore(t)
	tenantA := testTenant(t, store)
	tenantB := testTenant(t, store)

	if err := store.Ingest(tenantA, "cluster-a", []Finding{
		{
			Source: "checkout/checkout-1", Destination: "8.8.8.8",
			PathClass: "INTERNET_EGRESS", Confidence: "high",
			FixHint: "deny this destination", FixManifest: "real-manifest-text",
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
