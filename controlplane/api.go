// SPDX-License-Identifier: Apache-2.0
package main

import (
	"encoding/json"
	"log"
	"net/http"
)

// handleIngest is the frozen ingest contract from the build manual (§15,
// §Step 15): receives aggregates only, never raw flows. Idempotent in the
// sense that re-posting the same snapshot just adds another point-in-time
// row — safe to retry.
func handleIngest(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req IngestRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}

		if req.ClusterID == "" {
			http.Error(w, "cluster_id is required", http.StatusBadRequest)
			return
		}

		tenantID, ok := tenantIDFromContext(r.Context())
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		if err := store.Ingest(tenantID, req.ClusterID, req.Findings); err != nil {
			log.Printf("ingest %s: %v", req.ClusterID, err)
			http.Error(w, "ingest failed", http.StatusInternalServerError)
			return
		}

		// Deploy events are a best-effort correlation signal, not part of
		// the frozen findings contract -- a storage failure here logs but
		// doesn't fail the whole ingest (the real cost data above already
		// committed).
		if len(req.Events) > 0 {
			if err := store.IngestDeployEvents(tenantID, req.ClusterID, req.Events); err != nil {
				log.Printf("ingest deploy events %s: %v", req.ClusterID, err)
			}
		}

		w.WriteHeader(http.StatusAccepted)
	}
}

// handleFindingsAPI serves the current cross-cluster state as JSON, for
// anything other than the built-in HTML dashboard to consume (FR-I2).
func handleFindingsAPI(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID, ok := tenantIDFromContext(r.Context())
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		findings, err := store.LatestFindings(tenantID, 500)
		if err != nil {
			log.Printf("query findings: %v", err)
			http.Error(w, "query failed", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(findings)
	}
}
