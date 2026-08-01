// SPDX-License-Identifier: Apache-2.0
package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleBudgetUnsetByDefault(t *testing.T) {
	store := testStore(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/budget", nil)
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

	body, _ := json.Marshal(setBudgetRequest{BudgetINR: 500})
	postReq := httptest.NewRequest(http.MethodPost, "/api/v1/budget", bytes.NewReader(body))
	postRec := httptest.NewRecorder()
	handleBudget(store)(postRec, postReq)

	if postRec.Code != http.StatusOK {
		t.Fatalf("POST status = %d, want 200; body: %s", postRec.Code, postRec.Body.String())
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/budget", nil)
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

	body, _ := json.Marshal(setBudgetRequest{BudgetINR: -10})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/budget", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	handleBudget(store)(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for a negative budget", rec.Code)
	}
}

func TestHandleBudgetRejectsWrongMethod(t *testing.T) {
	store := testStore(t)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/budget", nil)
	rec := httptest.NewRecorder()
	handleBudget(store)(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
}
