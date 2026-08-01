// SPDX-License-Identifier: Apache-2.0
package main

import (
	"encoding/json"
	"log"
	"net/http"
)

type setTeamRequest struct {
	Namespace string `json:"namespace"`
	Team      string `json:"team"`
}

// handleTeams serves and updates the tenant's real namespace->team
// mapping: GET lists it (any logged-in role), POST/DELETE change it
// (admin only, gated by requireAdmin at the routing layer in main.go).
func handleTeams(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID, ok := tenantIDFromContext(r.Context())
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		switch r.Method {
		case http.MethodGet:
			ownership, err := store.ListTeamOwnership(tenantID)
			if err != nil {
				log.Printf("teams: list: %v", err)
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(ownership)

		case http.MethodPost:
			var req setTeamRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
				return
			}
			if req.Namespace == "" || req.Team == "" {
				http.Error(w, "namespace and team are both required", http.StatusBadRequest)
				return
			}
			if err := store.SetTeamOwnership(tenantID, req.Namespace, req.Team); err != nil {
				log.Printf("teams: set: %v", err)
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusNoContent)

		case http.MethodDelete:
			namespace := r.URL.Query().Get("namespace")
			if namespace == "" {
				http.Error(w, "namespace query param is required", http.StatusBadRequest)
				return
			}
			if err := store.DeleteTeamOwnership(tenantID, namespace); err != nil {
				log.Printf("teams: delete: %v", err)
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusNoContent)

		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}
