// SPDX-License-Identifier: Apache-2.0
package main

import (
	"encoding/json"
	"log"
	"net/http"
)

type createInviteRequest struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

type inviteResponse struct {
	Email     string `json:"email"`
	Role      string `json:"role"`
	CreatedAt string `json:"created_at"`
}

// handleInvites lets an admin manage their tenant's pending invites --
// GET lists them, POST creates/replaces one, DELETE revokes one. This is
// the real self-service replacement for "an admin needs shell access to
// add a teammate": no CLI, no operator involved.
func handleInvites(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID, ok := tenantIDFromContext(r.Context())
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		switch r.Method {
		case http.MethodGet:
			invites, err := store.ListInvites(tenantID)
			if err != nil {
				log.Printf("invites: list: %v", err)
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			resp := make([]inviteResponse, 0, len(invites))
			for _, inv := range invites {
				resp = append(resp, inviteResponse{Email: inv.Email, Role: inv.Role, CreatedAt: inv.CreatedAt.Format("2006-01-02T15:04:05Z07:00")})
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)

		case http.MethodPost:
			var req createInviteRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
				return
			}
			if req.Email == "" {
				http.Error(w, "email is required", http.StatusBadRequest)
				return
			}
			if err := store.CreateInvite(tenantID, req.Email, req.Role); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			w.WriteHeader(http.StatusNoContent)

		case http.MethodDelete:
			email := r.URL.Query().Get("email")
			if email == "" {
				http.Error(w, "email query param is required", http.StatusBadRequest)
				return
			}
			if err := store.DeleteInvite(tenantID, email); err != nil {
				log.Printf("invites: delete: %v", err)
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusNoContent)

		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}
