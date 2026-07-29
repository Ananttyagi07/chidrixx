// SPDX-License-Identifier: Apache-2.0
package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleIngestAndFindingsAPI(t *testing.T) {
	store := testStore(t)

	body, _ := json.Marshal(IngestRequest{
		ClusterID: "cluster-a",
		Findings: []Finding{
			{Source: "ns/app", Destination: "8.8.8.8", PathClass: "INTERNET_EGRESS", Confidence: "high", CostHighINR: 4.2},
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/ingest", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	handleIngest(store)(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("ingest status = %d, want %d; body: %s", rec.Code, http.StatusAccepted, rec.Body.String())
	}

	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/findings", nil)
	rec2 := httptest.NewRecorder()
	handleFindingsAPI(store)(rec2, req2)

	if rec2.Code != http.StatusOK {
		t.Fatalf("findings API status = %d, want 200", rec2.Code)
	}

	var got []FindingRow
	if err := json.Unmarshal(rec2.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode findings response: %v", err)
	}

	if len(got) != 1 || got[0].Source != "ns/app" || got[0].ClusterID != "cluster-a" {
		t.Fatalf("unexpected findings response: %+v", got)
	}
}

func TestHandleIngestRejectsMissingClusterID(t *testing.T) {
	store := testStore(t)

	body, _ := json.Marshal(IngestRequest{Findings: []Finding{{Source: "ns/app"}}})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ingest", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	handleIngest(store)(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for missing cluster_id", rec.Code)
	}
}

func TestHandleIngestRejectsWrongMethod(t *testing.T) {
	store := testStore(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ingest", nil)
	rec := httptest.NewRecorder()
	handleIngest(store)(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
}
