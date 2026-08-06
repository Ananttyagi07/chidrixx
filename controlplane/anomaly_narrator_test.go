// SPDX-License-Identifier: Apache-2.0
package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNarrateAnomalyMentionsRealNumbersAndLikelyCause(t *testing.T) {
	var capturedPrompt string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req chatCompletionRequest
		json.NewDecoder(r.Body).Decode(&req)
		for _, m := range req.Messages {
			if m.Role == "user" {
				capturedPrompt = m.Content
			}
		}
		json.NewEncoder(w).Encode(contentResponse("Cost jumped from real numbers, likely due to the deploy event."))
	}))
	t.Cleanup(srv.Close)
	client := newGroqClient("test-key", "", srv.URL)

	anomaly := Anomaly{
		ClusterID:       "cluster-a",
		PreviousCostINR: 10,
		CurrentCostINR:  50,
		GrowthRatio:     5,
		LikelyCause: &DeployEvent{
			Namespace: "checkout",
			Name:      "checkout",
			Reason:    "ScalingReplicaSet",
			Message:   "scaled up to 10 replicas",
		},
	}

	narrative, _, err := narrateAnomaly(t.Context(), client, anomaly, NewSanitizer(AIModeRaw))
	if err != nil {
		t.Fatalf("narrateAnomaly: %v", err)
	}
	if narrative == "" {
		t.Fatal("expected a non-empty narrative")
	}
	// The real numbers must actually reach the model -- this is what
	// keeps the narration grounded instead of generic.
	if !bytes.Contains([]byte(capturedPrompt), []byte("cluster-a")) {
		t.Fatalf("expected the real cluster ID in the prompt, got: %s", capturedPrompt)
	}
	if !bytes.Contains([]byte(capturedPrompt), []byte("checkout")) {
		t.Fatalf("expected the real deploy event in the prompt, got: %s", capturedPrompt)
	}
}

func TestNarrateAnomalyWithNoLikelyCauseStillPrompts(t *testing.T) {
	var capturedPrompt string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req chatCompletionRequest
		json.NewDecoder(r.Body).Decode(&req)
		for _, m := range req.Messages {
			if m.Role == "user" {
				capturedPrompt = m.Content
			}
		}
		json.NewEncoder(w).Encode(contentResponse("No correlated deploy event was found."))
	}))
	t.Cleanup(srv.Close)
	client := newGroqClient("test-key", "", srv.URL)

	anomaly := Anomaly{ClusterID: "cluster-a", PreviousCostINR: 10, CurrentCostINR: 50, GrowthRatio: 5}
	if _, _, err := narrateAnomaly(t.Context(), client, anomaly, NewSanitizer(AIModeRaw)); err != nil {
		t.Fatalf("narrateAnomaly: %v", err)
	}
	if !bytes.Contains([]byte(capturedPrompt), []byte("none found")) {
		t.Fatalf("expected the prompt to honestly say no cause was found, got: %s", capturedPrompt)
	}
}

func TestHandleNarrateAnomalyReturns503WhenNotConfigured(t *testing.T) {
	store := testStore(t)
	tenantID := testTenant(t, store)

	body, _ := json.Marshal(narrateAnomalyRequest{ClusterID: "cluster-a"})
	req := withTenant(httptest.NewRequest(http.MethodPost, "/api/v1/anomalies/narrate", bytes.NewReader(body)), tenantID)
	rec := httptest.NewRecorder()
	handleNarrateAnomaly(store, nil)(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503 when no GROQ_API_KEY is configured", rec.Code)
	}
}

func TestHandleNarrateAnomalyReturns404WhenNoCurrentAnomaly(t *testing.T) {
	store := testStore(t)
	tenantID := testTenant(t, store)
	srv := fakeGroq(t, "test-key", nil)
	client := newGroqClient("test-key", "", srv.URL)

	body, _ := json.Marshal(narrateAnomalyRequest{ClusterID: "cluster-a"})
	req := withTenant(httptest.NewRequest(http.MethodPost, "/api/v1/anomalies/narrate", bytes.NewReader(body)), tenantID)
	rec := httptest.NewRecorder()
	handleNarrateAnomaly(store, client)(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 for a cluster with no current anomaly", rec.Code)
	}
}

func TestHandleNarrateAnomalyEndToEndWithRealAnomaly(t *testing.T) {
	store := testStore(t)
	tenantID := testTenant(t, store)

	if err := store.Ingest(tenantID, "cluster-a", []Finding{{Source: "checkout/checkout-1", CostHighINR: 10}}); err != nil {
		t.Fatalf("Ingest (1): %v", err)
	}
	time.Sleep(1100 * time.Millisecond)
	if err := store.Ingest(tenantID, "cluster-a", []Finding{{Source: "checkout/checkout-1", CostHighINR: 50}}); err != nil {
		t.Fatalf("Ingest (2): %v", err)
	}

	srv := fakeGroq(t, "test-key", []chatCompletionResponse{contentResponse("Cost roughly 5x'd for cluster-a.")})
	client := newGroqClient("test-key", "", srv.URL)

	body, _ := json.Marshal(narrateAnomalyRequest{ClusterID: "cluster-a"})
	req := withTenant(httptest.NewRequest(http.MethodPost, "/api/v1/anomalies/narrate", bytes.NewReader(body)), tenantID)
	rec := httptest.NewRecorder()
	handleNarrateAnomaly(store, client)(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	var got narrateAnomalyResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Narrative != "Cost roughly 5x'd for cluster-a." {
		t.Fatalf("unexpected narrative: %q", got.Narrative)
	}

	stats, err := store.AIEvalStats(tenantID)
	if err != nil {
		t.Fatalf("AIEvalStats: %v", err)
	}
	if len(stats) != 1 || stats[0].Feature != "anomaly_narrator" || stats[0].SuccessCount != 1 {
		t.Fatalf("expected handleNarrateAnomaly to record a real successful ai eval event, got %+v", stats)
	}
}

func TestHandleNarrateAnomalyIsIsolatedByTenant(t *testing.T) {
	store := testStore(t)
	tenantA := testTenant(t, store)
	tenantB := testTenant(t, store)

	if err := store.Ingest(tenantA, "cluster-a", []Finding{{Source: "checkout/checkout-1", CostHighINR: 10}}); err != nil {
		t.Fatalf("Ingest (1): %v", err)
	}
	time.Sleep(1100 * time.Millisecond)
	if err := store.Ingest(tenantA, "cluster-a", []Finding{{Source: "checkout/checkout-1", CostHighINR: 50}}); err != nil {
		t.Fatalf("Ingest (2): %v", err)
	}

	srv := fakeGroq(t, "test-key", nil)
	client := newGroqClient("test-key", "", srv.URL)

	body, _ := json.Marshal(narrateAnomalyRequest{ClusterID: "cluster-a"})
	req := withTenant(httptest.NewRequest(http.MethodPost, "/api/v1/anomalies/narrate", bytes.NewReader(body)), tenantB)
	rec := httptest.NewRecorder()
	handleNarrateAnomaly(store, client)(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("tenant b saw tenant a's anomaly: status = %d", rec.Code)
	}
}
