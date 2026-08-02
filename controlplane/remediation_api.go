// SPDX-License-Identifier: Apache-2.0
package main

import (
	"encoding/json"
	"log"
	"net/http"
)

type remediationPreviewResponse struct {
	Decisions []RemediationDecision `json:"decisions"`
}

// handleRemediationPreview serves the real, computed-on-demand
// simulation described in remediation.go: what the current real top
// fixes would qualify for under the real auto-remediation policy, and
// why. Read-only, same access level as findings/dashboard-summary --
// this never mutates anything, real or simulated.
func handleRemediationPreview(store *Store) http.HandlerFunc {
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

		findings, err := store.LatestFindings(tenantID, 500)
		if err != nil {
			log.Printf("remediation-preview: findings: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		decisions := EvaluateRemediation(findings, DefaultRemediationPolicy())

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(remediationPreviewResponse{Decisions: decisions})
	}
}
