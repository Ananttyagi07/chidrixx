// SPDX-License-Identifier: Apache-2.0
package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleAnomalyAlertsReturnsRealUnacknowledgedAlerts(t *testing.T) {
	store := testStore(t)
	tenantID := testTenant(t, store)
	ingestGrowthAnomaly(t, store, tenantID, "cluster-a", 1, 5)
	if _, err := store.WatchTenantForNewAnomalies(tenantID); err != nil {
		t.Fatalf("WatchTenantForNewAnomalies: %v", err)
	}

	req := withTenant(httptest.NewRequest(http.MethodGet, "/api/v1/anomalies/alerts", nil), tenantID)
	rec := httptest.NewRecorder()
	handleAnomalyAlerts(store)(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	var got []anomalyAlertResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 || got[0].ClusterID != "cluster-a" || got[0].CurrentCostINR != 5 {
		t.Fatalf("unexpected alerts: %+v", got)
	}
}

func TestHandleAnomalyAlertsIsIsolatedByTenant(t *testing.T) {
	store := testStore(t)
	tenantA := testTenant(t, store)
	tenantB := testTenant(t, store)
	ingestGrowthAnomaly(t, store, tenantA, "cluster-a", 1, 5)
	if _, err := store.WatchTenantForNewAnomalies(tenantA); err != nil {
		t.Fatalf("WatchTenantForNewAnomalies: %v", err)
	}

	req := withTenant(httptest.NewRequest(http.MethodGet, "/api/v1/anomalies/alerts", nil), tenantB)
	rec := httptest.NewRecorder()
	handleAnomalyAlerts(store)(rec, req)

	var got []anomalyAlertResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("tenant b's alerts route leaked tenant a's data: %+v", got)
	}
}

func TestHandleAcknowledgeAnomalyAlertThenGoneFromList(t *testing.T) {
	store := testStore(t)
	tenantID := testTenant(t, store)
	ingestGrowthAnomaly(t, store, tenantID, "cluster-a", 1, 5)
	if _, err := store.WatchTenantForNewAnomalies(tenantID); err != nil {
		t.Fatalf("WatchTenantForNewAnomalies: %v", err)
	}
	alerts, _ := store.UnacknowledgedAnomalyAlerts(tenantID)

	body, _ := json.Marshal(acknowledgeAnomalyAlertRequest{ID: alerts[0].ID})
	req := withTenant(httptest.NewRequest(http.MethodPost, "/api/v1/anomalies/alerts/acknowledge", bytes.NewReader(body)), tenantID)
	rec := httptest.NewRecorder()
	handleAcknowledgeAnomalyAlert(store)(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204; body: %s", rec.Code, rec.Body.String())
	}

	getReq := withTenant(httptest.NewRequest(http.MethodGet, "/api/v1/anomalies/alerts", nil), tenantID)
	getRec := httptest.NewRecorder()
	handleAnomalyAlerts(store)(getRec, getReq)
	var got []anomalyAlertResponse
	json.Unmarshal(getRec.Body.Bytes(), &got)
	if len(got) != 0 {
		t.Fatalf("expected the acknowledged alert to be gone from the list, got %+v", got)
	}
}

func TestHandleAcknowledgeAnomalyAlertReturns404ForUnknownID(t *testing.T) {
	store := testStore(t)
	tenantID := testTenant(t, store)

	body, _ := json.Marshal(acknowledgeAnomalyAlertRequest{ID: 999999})
	req := withTenant(httptest.NewRequest(http.MethodPost, "/api/v1/anomalies/alerts/acknowledge", bytes.NewReader(body)), tenantID)
	rec := httptest.NewRecorder()
	handleAcknowledgeAnomalyAlert(store)(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestHandleAcknowledgeAnomalyAlertRejectsMissingID(t *testing.T) {
	store := testStore(t)
	tenantID := testTenant(t, store)

	body, _ := json.Marshal(acknowledgeAnomalyAlertRequest{})
	req := withTenant(httptest.NewRequest(http.MethodPost, "/api/v1/anomalies/alerts/acknowledge", bytes.NewReader(body)), tenantID)
	rec := httptest.NewRecorder()
	handleAcknowledgeAnomalyAlert(store)(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}
