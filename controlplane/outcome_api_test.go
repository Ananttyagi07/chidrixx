// SPDX-License-Identifier: Apache-2.0
package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleOutcomesGetReturnsWhatWasShown(t *testing.T) {
	store := testStore(t)
	tenantID := testTenant(t, store)

	fix := testFindingRow("cluster-a", "checkout/checkout-1", "redis/redis-master", "cross_az", "co-locate zones", 40, 30, 40)
	if err := store.RecordRecommendationsShown(tenantID, []FindingRow{fix}); err != nil {
		t.Fatalf("RecordRecommendationsShown: %v", err)
	}

	req := withTenant(httptest.NewRequest(http.MethodGet, "/api/v1/outcomes", nil), tenantID)
	rec := httptest.NewRecorder()
	handleOutcomes(store)(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	var got []outcomeResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 || got[0].Source != "checkout/checkout-1" || got[0].CostBeforeINR != 40 {
		t.Fatalf("unexpected outcomes: %+v", got)
	}
	if got[0].AppliedAt != nil {
		t.Fatalf("expected an unapplied outcome, got: %+v", got[0])
	}
}

func TestHandleMarkOutcomeAppliedThenReflectedInGet(t *testing.T) {
	store := testStore(t)
	tenantID := testTenant(t, store)

	fix := testFindingRow("cluster-a", "checkout/checkout-1", "redis/redis-master", "cross_az", "co-locate zones", 40, 30, 40)
	if err := store.RecordRecommendationsShown(tenantID, []FindingRow{fix}); err != nil {
		t.Fatalf("RecordRecommendationsShown: %v", err)
	}

	body, _ := json.Marshal(markAppliedRequest{ClusterID: "cluster-a", Source: "checkout/checkout-1", Destination: "redis/redis-master", PathClass: "cross_az"})
	postReq := withTenant(httptest.NewRequest(http.MethodPost, "/api/v1/outcomes/apply", bytes.NewReader(body)), tenantID)
	postRec := httptest.NewRecorder()
	handleMarkOutcomeApplied(store)(postRec, postReq)

	if postRec.Code != http.StatusNoContent {
		t.Fatalf("POST status = %d, want 204; body: %s", postRec.Code, postRec.Body.String())
	}

	getReq := withTenant(httptest.NewRequest(http.MethodGet, "/api/v1/outcomes", nil), tenantID)
	getRec := httptest.NewRecorder()
	handleOutcomes(store)(getRec, getReq)

	var got []outcomeResponse
	if err := json.Unmarshal(getRec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 || got[0].AppliedAt == nil {
		t.Fatalf("expected the outcome to now show applied_at set: %+v", got)
	}
}

func TestHandleMarkOutcomeAppliedRejectsIncompleteIdentity(t *testing.T) {
	store := testStore(t)
	tenantID := testTenant(t, store)

	body, _ := json.Marshal(markAppliedRequest{ClusterID: "cluster-a", Source: "checkout/checkout-1"})
	req := withTenant(httptest.NewRequest(http.MethodPost, "/api/v1/outcomes/apply", bytes.NewReader(body)), tenantID)
	rec := httptest.NewRecorder()
	handleMarkOutcomeApplied(store)(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for a missing destination/path_class", rec.Code)
	}
}

func TestHandleMarkOutcomeAppliedReturns404ForUnknownIdentity(t *testing.T) {
	store := testStore(t)
	tenantID := testTenant(t, store)

	body, _ := json.Marshal(markAppliedRequest{ClusterID: "cluster-a", Source: "nobody/here", Destination: "nowhere/there", PathClass: "cross_az"})
	req := withTenant(httptest.NewRequest(http.MethodPost, "/api/v1/outcomes/apply", bytes.NewReader(body)), tenantID)
	rec := httptest.NewRecorder()
	handleMarkOutcomeApplied(store)(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 for a recommendation that was never shown", rec.Code)
	}
}

func TestOutcomesRouteIsIsolatedByTenant(t *testing.T) {
	store := testStore(t)
	tenantA := testTenant(t, store)
	tenantB := testTenant(t, store)

	fix := testFindingRow("cluster-a", "checkout/checkout-1", "redis/redis-master", "cross_az", "co-locate zones", 40, 30, 40)
	if err := store.RecordRecommendationsShown(tenantA, []FindingRow{fix}); err != nil {
		t.Fatalf("RecordRecommendationsShown(a): %v", err)
	}

	getReq := withTenant(httptest.NewRequest(http.MethodGet, "/api/v1/outcomes", nil), tenantB)
	getRec := httptest.NewRecorder()
	handleOutcomes(store)(getRec, getReq)

	var got []outcomeResponse
	json.Unmarshal(getRec.Body.Bytes(), &got)
	if len(got) != 0 {
		t.Fatalf("tenant b's outcomes list leaked tenant a's recommendation: %+v", got)
	}
}
