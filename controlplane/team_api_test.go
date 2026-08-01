// SPDX-License-Identifier: Apache-2.0
package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleTeamsGetListsConfiguredMapping(t *testing.T) {
	store := testStore(t)
	tenantID := testTenant(t, store)

	if err := store.SetTeamOwnership(tenantID, "checkout", "Payments"); err != nil {
		t.Fatalf("SetTeamOwnership: %v", err)
	}

	req := withTenant(httptest.NewRequest(http.MethodGet, "/api/v1/teams", nil), tenantID)
	rec := httptest.NewRecorder()
	handleTeams(store)(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var got []TeamOwnership
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 || got[0].Namespace != "checkout" || got[0].Team != "Payments" {
		t.Fatalf("unexpected mapping: %+v", got)
	}
}

func TestHandleTeamsPostThenGet(t *testing.T) {
	store := testStore(t)
	tenantID := testTenant(t, store)

	body, _ := json.Marshal(setTeamRequest{Namespace: "ai-gateway", Team: "AI"})
	postReq := withTenant(httptest.NewRequest(http.MethodPost, "/api/v1/teams", bytes.NewReader(body)), tenantID)
	postRec := httptest.NewRecorder()
	handleTeams(store)(postRec, postReq)

	if postRec.Code != http.StatusNoContent {
		t.Fatalf("POST status = %d, want 204; body: %s", postRec.Code, postRec.Body.String())
	}

	getReq := withTenant(httptest.NewRequest(http.MethodGet, "/api/v1/teams", nil), tenantID)
	getRec := httptest.NewRecorder()
	handleTeams(store)(getRec, getReq)

	var got []TeamOwnership
	json.Unmarshal(getRec.Body.Bytes(), &got)
	if len(got) != 1 || got[0].Team != "AI" {
		t.Fatalf("expected the posted mapping to round-trip, got: %+v", got)
	}
}

func TestHandleTeamsPostRejectsMissingFields(t *testing.T) {
	store := testStore(t)
	tenantID := testTenant(t, store)

	body, _ := json.Marshal(setTeamRequest{Namespace: "checkout"})
	req := withTenant(httptest.NewRequest(http.MethodPost, "/api/v1/teams", bytes.NewReader(body)), tenantID)
	rec := httptest.NewRecorder()
	handleTeams(store)(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for a missing team", rec.Code)
	}
}

func TestHandleTeamsDelete(t *testing.T) {
	store := testStore(t)
	tenantID := testTenant(t, store)

	if err := store.SetTeamOwnership(tenantID, "checkout", "Payments"); err != nil {
		t.Fatalf("SetTeamOwnership: %v", err)
	}

	req := withTenant(httptest.NewRequest(http.MethodDelete, "/api/v1/teams?namespace=checkout", nil), tenantID)
	rec := httptest.NewRecorder()
	handleTeams(store)(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE status = %d, want 204", rec.Code)
	}

	list, err := store.ListTeamOwnership(tenantID)
	if err != nil {
		t.Fatalf("ListTeamOwnership: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected mapping removed, got: %+v", list)
	}
}

func TestTeamsRouteRequiresAdminForPostAndDelete(t *testing.T) {
	store := testStore(t)
	tenantID := testTenant(t, store)
	route := teamsRoute(store)

	body, _ := json.Marshal(setTeamRequest{Namespace: "checkout", Team: "Payments"})
	postReq := withTenant(httptest.NewRequest(http.MethodPost, "/api/v1/teams", bytes.NewReader(body)), tenantID)
	postReq = postReq.WithContext(contextWithRole(postReq.Context(), RoleViewer))
	postRec := httptest.NewRecorder()
	route(postRec, postReq)
	if postRec.Code != http.StatusForbidden {
		t.Fatalf("POST as viewer: status = %d, want 403", postRec.Code)
	}

	delReq := withTenant(httptest.NewRequest(http.MethodDelete, "/api/v1/teams?namespace=checkout", nil), tenantID)
	delReq = delReq.WithContext(contextWithRole(delReq.Context(), RoleViewer))
	delRec := httptest.NewRecorder()
	route(delRec, delReq)
	if delRec.Code != http.StatusForbidden {
		t.Fatalf("DELETE as viewer: status = %d, want 403", delRec.Code)
	}

	getReq := withTenant(httptest.NewRequest(http.MethodGet, "/api/v1/teams", nil), tenantID)
	getReq = getReq.WithContext(contextWithRole(getReq.Context(), RoleViewer))
	getRec := httptest.NewRecorder()
	route(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GET as viewer: status = %d, want 200", getRec.Code)
	}
}
