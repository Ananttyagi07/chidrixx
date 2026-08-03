// SPDX-License-Identifier: Apache-2.0
package main

import (
	"encoding/json"
	"log"
	"net/http"
)

type aiEvalStatsResponse struct {
	Features []AIEvalFeatureStats `json:"features"`
}

// handleAIEvalStats exposes real, aggregate AI-quality telemetry --
// success rate, tool-call success rate, latency, token cost -- broken
// down per real AI feature (chat, anomaly_narrator). Read-only, same
// access level as the features it's observing.
func handleAIEvalStats(store *Store) http.HandlerFunc {
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

		stats, err := store.AIEvalStats(tenantID)
		if err != nil {
			log.Printf("ai-eval: stats: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(aiEvalStatsResponse{Features: stats})
	}
}
