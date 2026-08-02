// SPDX-License-Identifier: Apache-2.0
package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
)

// handlePlacementPreview serves the real offline placement simulation
// (placement.go): given the tenant's current real CROSS_AZ findings,
// how much of that real cost could a real graph partitioner eliminate
// by grouping frequently-communicating workloads together. Read-only,
// computed on demand -- no new persistence, no live cluster access.
func handlePlacementPreview(store *Store) http.HandlerFunc {
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

		numGroups := 3 // matches a common real region's AZ count (e.g. ap-south-1); overridable
		if v := r.URL.Query().Get("groups"); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil || n < 1 {
				http.Error(w, "groups must be a positive integer", http.StatusBadRequest)
				return
			}
			numGroups = n
		}

		findings, err := store.LatestFindings(tenantID, 5000)
		if err != nil {
			log.Printf("placement-preview: findings: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		result := OptimizePlacement(findings, numGroups)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	}
}
