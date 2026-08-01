// SPDX-License-Identifier: Apache-2.0
package main

import (
	"encoding/json"
	"log"
	"net/http"
)

// topWorkloadGrowthLimit caps the "which workload grew the most" ranking
// to a reviewable list, matching the existing topFixes cap in
// summary_api.go for the same reason -- a real ranking, just not an
// unbounded one.
const topWorkloadGrowthLimit = 10

// handleWorkloadGrowth serves the real "which workload's cost grew the
// most, and did a deploy event happen in that window" ranking (gap #3:
// historical trend-change intelligence). Kept as its own endpoint rather
// than folded into dashboard-summary since it scans the full retained
// history, not just the latest snapshot per cluster.
func handleWorkloadGrowth(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID, ok := tenantIDFromContext(r.Context())
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		growth, err := store.WorkloadCostGrowth(tenantID, topWorkloadGrowthLimit)
		if err != nil {
			log.Printf("workload-growth: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(growth)
	}
}
