// SPDX-License-Identifier: Apache-2.0
package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandlePlacementPreviewReturnsRealComputedResult(t *testing.T) {
	store := testStore(t)
	tenantID := testTenant(t, store)

	if err := store.Ingest(tenantID, "cluster-a", []Finding{
		{Source: "a", Destination: "b", PathClass: "CROSS_AZ", CostHighINR: 10},
		{Source: "c", Destination: "d", PathClass: "CROSS_AZ", CostHighINR: 10},
	}); err != nil {
		t.Fatalf("Ingest: %v", err)
	}

	req := withTenant(httptest.NewRequest(http.MethodGet, "/api/v1/placement/preview?groups=2", nil), tenantID)
	rec := httptest.NewRecorder()
	handlePlacementPreview(store)(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	var got PlacementResult
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.ObservedCrossZoneINR != 20 || got.OptimizedCrossZoneINR != 0 {
		t.Fatalf("expected a real computed result matching OptimizePlacement directly, got: %+v", got)
	}
}

func TestHandlePlacementPreviewDefaultsToThreeGroups(t *testing.T) {
	store := testStore(t)
	tenantID := testTenant(t, store)

	req := withTenant(httptest.NewRequest(http.MethodGet, "/api/v1/placement/preview", nil), tenantID)
	rec := httptest.NewRecorder()
	handlePlacementPreview(store)(rec, req)

	var got PlacementResult
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Groups != 3 {
		t.Fatalf("expected the real default of 3 groups, got %d", got.Groups)
	}
}

func TestHandlePlacementPreviewRejectsInvalidGroupsParam(t *testing.T) {
	store := testStore(t)
	tenantID := testTenant(t, store)

	req := withTenant(httptest.NewRequest(http.MethodGet, "/api/v1/placement/preview?groups=notanumber", nil), tenantID)
	rec := httptest.NewRecorder()
	handlePlacementPreview(store)(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for a non-numeric groups param", rec.Code)
	}
}

func TestHandlePlacementPreviewIsIsolatedByTenant(t *testing.T) {
	store := testStore(t)
	tenantA := testTenant(t, store)
	tenantB := testTenant(t, store)

	if err := store.Ingest(tenantA, "cluster-a", []Finding{
		{Source: "a", Destination: "b", PathClass: "CROSS_AZ", CostHighINR: 100},
	}); err != nil {
		t.Fatalf("Ingest: %v", err)
	}

	req := withTenant(httptest.NewRequest(http.MethodGet, "/api/v1/placement/preview", nil), tenantB)
	rec := httptest.NewRecorder()
	handlePlacementPreview(store)(rec, req)

	var got PlacementResult
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Workloads != 0 || got.ObservedCrossZoneINR != 0 {
		t.Fatalf("tenant b saw tenant a's real placement data: %+v", got)
	}
}
