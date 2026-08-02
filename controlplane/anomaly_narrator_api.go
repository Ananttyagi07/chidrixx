// SPDX-License-Identifier: Apache-2.0
package main

import (
	"encoding/json"
	"log"
	"net/http"
)

type narrateAnomalyRequest struct {
	ClusterID string `json:"cluster_id"`
}

type narrateAnomalyResponse struct {
	Narrative string `json:"narrative"`
}

// handleNarrateAnomaly turns one real anomaly into a plain-English
// explanation on demand -- not run automatically for every anomaly on
// every dashboard load (that would mean an LLM call per anomaly per page
// view), only when an operator actually asks for it. Re-derives the
// anomaly fresh from real data rather than trusting client-supplied
// numbers, so the narration is always grounded in what's actually true
// right now, not whatever the browser happened to have cached.
func handleNarrateAnomaly(store *Store, groq *GroqClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if groq == nil {
			http.Error(w, "chat assistant is not configured (no GROQ_API_KEY set)", http.StatusServiceUnavailable)
			return
		}
		tenantID, ok := tenantIDFromContext(r.Context())
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		var req narrateAnomalyRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}
		if req.ClusterID == "" {
			http.Error(w, "cluster_id is required", http.StatusBadRequest)
			return
		}

		anomalies, err := detectAnomalies(store, tenantID)
		if err != nil {
			log.Printf("narrate-anomaly: detect: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		var match *Anomaly
		for i := range anomalies {
			if anomalies[i].ClusterID == req.ClusterID {
				match = &anomalies[i]
				break
			}
		}
		if match == nil {
			http.Error(w, "no current anomaly for that cluster_id", http.StatusNotFound)
			return
		}

		narrative, err := narrateAnomaly(r.Context(), groq, *match)
		if err != nil {
			log.Printf("narrate-anomaly: %v", err)
			http.Error(w, "the assistant hit an error -- try again", http.StatusBadGateway)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(narrateAnomalyResponse{Narrative: narrative})
	}
}
