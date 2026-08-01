// SPDX-License-Identifier: Apache-2.0
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// withTenant simulates what requireSession would have already done by the
// time a handler runs -- these tests exercise handleBudget directly, below
// the auth middleware, so they set up its context precondition by hand.
func withTenant(r *http.Request, tenantID int64) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), ctxTenantID, tenantID))
}

func TestHandleBudgetUnsetByDefault(t *testing.T) {
	store := testStore(t)
	tenantID := testTenant(t, store)

	req := withTenant(httptest.NewRequest(http.MethodGet, "/api/v1/budget", nil), tenantID)
	rec := httptest.NewRecorder()
	handleBudget(store)(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var got budgetResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if got.IsSet {
		t.Errorf("expected IsSet=false before any budget is configured, got %+v", got)
	}
}

func TestHandleBudgetSetThenGet(t *testing.T) {
	store := testStore(t)
	tenantID := testTenant(t, store)

	body, _ := json.Marshal(setBudgetRequest{BudgetINR: 500})
	postReq := withTenant(httptest.NewRequest(http.MethodPost, "/api/v1/budget", bytes.NewReader(body)), tenantID)
	postRec := httptest.NewRecorder()
	handleBudget(store)(postRec, postReq)

	if postRec.Code != http.StatusOK {
		t.Fatalf("POST status = %d, want 200; body: %s", postRec.Code, postRec.Body.String())
	}

	getReq := withTenant(httptest.NewRequest(http.MethodGet, "/api/v1/budget", nil), tenantID)
	getRec := httptest.NewRecorder()
	handleBudget(store)(getRec, getReq)

	var got budgetResponse
	if err := json.Unmarshal(getRec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if !got.IsSet || got.BudgetINR != 500 {
		t.Errorf("expected {500, true}, got %+v", got)
	}
}

func TestHandleBudgetRejectsNegative(t *testing.T) {
	store := testStore(t)
	tenantID := testTenant(t, store)

	body, _ := json.Marshal(setBudgetRequest{BudgetINR: -10})
	req := withTenant(httptest.NewRequest(http.MethodPost, "/api/v1/budget", bytes.NewReader(body)), tenantID)
	rec := httptest.NewRecorder()
	handleBudget(store)(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for a negative budget", rec.Code)
	}
}

func TestHandleBudgetRejectsWrongMethod(t *testing.T) {
	store := testStore(t)
	tenantID := testTenant(t, store)

	req := withTenant(httptest.NewRequest(http.MethodDelete, "/api/v1/budget", nil), tenantID)
	rec := httptest.NewRecorder()
	handleBudget(store)(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
}

func TestHandleBudgetRejectsMissingTenantContext(t *testing.T) {
	store := testStore(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/budget", nil)
	rec := httptest.NewRecorder()
	handleBudget(store)(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 when no tenant is resolved", rec.Code)
	}
}

func TestBudgetRouteRequiresAdminForPost(t *testing.T) {
	store := testStore(t)
	tenantID := testTenant(t, store)
	route := budgetRoute(store)

	body, _ := json.Marshal(setBudgetRequest{BudgetINR: 500})
	req := withTenant(httptest.NewRequest(http.MethodPost, "/api/v1/budget", bytes.NewReader(body)), tenantID)
	req = req.WithContext(contextWithRole(req.Context(), RoleViewer))
	rec := httptest.NewRecorder()
	route(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 for a viewer POSTing a budget", rec.Code)
	}

	req2 := withTenant(httptest.NewRequest(http.MethodGet, "/api/v1/budget", nil), tenantID)
	req2 = req2.WithContext(contextWithRole(req2.Context(), RoleViewer))
	rec2 := httptest.NewRecorder()
	route(rec2, req2)

	if rec2.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want 200 for a viewer reading the budget", rec2.Code)
	}
}
