// SPDX-License-Identifier: Apache-2.0
package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleInvitesPostThenGet(t *testing.T) {
	store := testStore(t)
	tenantID := testTenant(t, store)

	body, _ := json.Marshal(createInviteRequest{Email: "teammate@example.com", Role: RoleViewer})
	postReq := withTenant(httptest.NewRequest(http.MethodPost, "/api/v1/invites", bytes.NewReader(body)), tenantID)
	postRec := httptest.NewRecorder()
	handleInvites(store)(postRec, postReq)

	if postRec.Code != http.StatusNoContent {
		t.Fatalf("POST status = %d, want 204; body: %s", postRec.Code, postRec.Body.String())
	}

	getReq := withTenant(httptest.NewRequest(http.MethodGet, "/api/v1/invites", nil), tenantID)
	getRec := httptest.NewRecorder()
	handleInvites(store)(getRec, getReq)

	var got []inviteResponse
	if err := json.Unmarshal(getRec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 || got[0].Email != "teammate@example.com" || got[0].Role != RoleViewer {
		t.Fatalf("unexpected invites: %+v", got)
	}
}

func TestHandleInvitesPostRejectsMissingEmail(t *testing.T) {
	store := testStore(t)
	tenantID := testTenant(t, store)

	body, _ := json.Marshal(createInviteRequest{Role: RoleViewer})
	req := withTenant(httptest.NewRequest(http.MethodPost, "/api/v1/invites", bytes.NewReader(body)), tenantID)
	rec := httptest.NewRecorder()
	handleInvites(store)(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for a missing email", rec.Code)
	}
}

func TestHandleInvitesPostRejectsInvalidRole(t *testing.T) {
	store := testStore(t)
	tenantID := testTenant(t, store)

	body, _ := json.Marshal(createInviteRequest{Email: "teammate@example.com", Role: "superuser"})
	req := withTenant(httptest.NewRequest(http.MethodPost, "/api/v1/invites", bytes.NewReader(body)), tenantID)
	rec := httptest.NewRecorder()
	handleInvites(store)(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for an invalid role", rec.Code)
	}
}

func TestHandleInvitesDelete(t *testing.T) {
	store := testStore(t)
	tenantID := testTenant(t, store)

	if err := store.CreateInvite(tenantID, "teammate@example.com", RoleViewer); err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}

	req := withTenant(httptest.NewRequest(http.MethodDelete, "/api/v1/invites?email=teammate@example.com", nil), tenantID)
	rec := httptest.NewRecorder()
	handleInvites(store)(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE status = %d, want 204", rec.Code)
	}

	invites, err := store.ListInvites(tenantID)
	if err != nil {
		t.Fatalf("ListInvites: %v", err)
	}
	if len(invites) != 0 {
		t.Fatalf("expected the invite revoked, got: %+v", invites)
	}
}

func TestInvitesRouteRequiresAdminEvenForGet(t *testing.T) {
	store := testStore(t)
	tenantID := testTenant(t, store)
	route := requireAdmin(handleInvites(store))

	getReq := withTenant(httptest.NewRequest(http.MethodGet, "/api/v1/invites", nil), tenantID)
	getReq = getReq.WithContext(contextWithRole(getReq.Context(), RoleViewer))
	getRec := httptest.NewRecorder()
	route(getRec, getReq)

	if getRec.Code != http.StatusForbidden {
		t.Fatalf("viewer GET status = %d, want 403 -- team invite management is admin-only, including visibility", getRec.Code)
	}
}

func TestInvitesRouteIsIsolatedByTenant(t *testing.T) {
	store := testStore(t)
	tenantA := testTenant(t, store)
	tenantB := testTenant(t, store)

	if err := store.CreateInvite(tenantA, "a-teammate@example.com", RoleViewer); err != nil {
		t.Fatalf("CreateInvite(a): %v", err)
	}

	getReq := withTenant(httptest.NewRequest(http.MethodGet, "/api/v1/invites", nil), tenantB)
	getRec := httptest.NewRecorder()
	handleInvites(store)(getRec, getReq)

	var got []inviteResponse
	json.Unmarshal(getRec.Body.Bytes(), &got)
	if len(got) != 0 {
		t.Fatalf("tenant b's invite list leaked tenant a's invite: %+v", got)
	}
}
