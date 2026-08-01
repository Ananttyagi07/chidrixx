// SPDX-License-Identifier: Apache-2.0
package main

import (
	"encoding/json"
	"log"
	"net/http"
	"time"
)

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type meResponse struct {
	Username string `json:"username"`
	Role     string `json:"role"`
}

func setSessionCookie(w http.ResponseWriter, sessionID string) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    sessionID,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(sessionTTL),
	})
}

func clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

// handleLogin replaces the browser side of the old single-shared-token
// Basic Auth prompt with a real login: a username/password checked against
// a real bcrypt hash, resulting in a real server-tracked session -- not a
// credential cached by the browser forever with no way to revoke it short
// of changing the one shared secret for everyone.
func handleLogin(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req loginRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}

		user, err := store.AuthenticateUser(req.Username, req.Password)
		if err != nil {
			http.Error(w, "invalid username or password", http.StatusUnauthorized)
			return
		}

		sessionID, err := store.CreateSession(user)
		if err != nil {
			log.Printf("login: create session: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		setSessionCookie(w, sessionID)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(meResponse{Username: user.Username, Role: user.Role})
	}
}

// handleLogout deletes the real session row (not just the cookie) so the
// session is actually revoked, not just hidden from this one browser.
func handleLogout(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if cookie, err := r.Cookie(sessionCookieName); err == nil {
			_ = store.DeleteSession(cookie.Value)
		}
		clearSessionCookie(w)
		w.WriteHeader(http.StatusNoContent)
	}
}

// handleMe lets the frontend ask "am I already logged in, and as whom"
// on page load, without needing a dedicated client-side session store.
func handleMe(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(meResponse{
		Username: usernameFromContext(r.Context()),
		Role:     roleFromContext(r.Context()),
	})
}
