// SPDX-License-Identifier: Apache-2.0
package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
)

type anomalyAlertResponse struct {
	ID              int64   `json:"id"`
	ClusterID       string  `json:"cluster_id"`
	DetectedAt      string  `json:"detected_at"`
	PreviousCostINR float64 `json:"previous_cost_inr"`
	CurrentCostINR  float64 `json:"current_cost_inr"`
	GrowthRatio     float64 `json:"growth_ratio"`
}

// handleAnomalyAlerts lists every real anomaly the background watch loop
// (anomaly_watch.go) has proactively detected and this tenant hasn't
// dismissed yet -- the real "notice" surface, computed on a recurring
// tick rather than only when an operator asks.
func handleAnomalyAlerts(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		tenantID, ok := tenantIDFromContext(r.Context())
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		alerts, err := store.UnacknowledgedAnomalyAlerts(tenantID)
		if err != nil {
			log.Printf("anomaly-alerts: list: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		resp := make([]anomalyAlertResponse, 0, len(alerts))
		for _, a := range alerts {
			resp = append(resp, anomalyAlertResponse{
				ID:              a.ID,
				ClusterID:       a.ClusterID,
				DetectedAt:      a.DetectedAt.Format(timeFormat),
				PreviousCostINR: a.PreviousCostINR,
				CurrentCostINR:  a.CurrentCostINR,
				GrowthRatio:     a.GrowthRatio,
			})
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}

type acknowledgeAnomalyAlertRequest struct {
	ID int64 `json:"id"`
}

// handleAcknowledgeAnomalyAlert dismisses one real proactively-detected
// alert -- an operator saying "I've seen this," not a claim that the
// underlying anomaly has actually been resolved.
func handleAcknowledgeAnomalyAlert(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		tenantID, ok := tenantIDFromContext(r.Context())
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		var req acknowledgeAnomalyAlertRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}
		if req.ID == 0 {
			http.Error(w, "id is required", http.StatusBadRequest)
			return
		}

		err := store.AcknowledgeAnomalyAlert(tenantID, req.ID)
		if errors.Is(err, ErrAnomalyAlertNotFound) {
			http.Error(w, "no alert with that id for this tenant", http.StatusNotFound)
			return
		}
		if err != nil {
			log.Printf("anomaly-alerts: acknowledge: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
