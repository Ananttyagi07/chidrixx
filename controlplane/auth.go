// SPDX-License-Identifier: Apache-2.0
package main

import (
	"crypto/subtle"
	"net/http"
)

// requireToken wraps every route with HTTP Basic Auth when a token is
// configured, username ignored — the token is the password. Basic Auth
// (not Bearer) deliberately: this service has two different clients, an
// agent posting JSON (any HTTP client sets Basic Auth trivially) and a
// human viewing the dashboard in a plain browser (which has no way to
// attach a custom header to plain navigation, but natively prompts for
// and caches Basic Auth credentials). One shared secret, not per-user
// accounts or RBAC — that's real infra work, P3 in the build manual —
// but it closes the gap that mattered most before this: with no token
// set, anyone who can reach this service has full read/write access to
// every cluster's cost data. An empty token disables the check entirely,
// for local dev — same opt-in pattern the rest of this codebase uses for
// alerting/shipping.
func requireToken(token string, next http.Handler) http.Handler {
	if token == "" {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, pass, ok := r.BasicAuth()

		// constant-time compare: a token check that short-circuits on the
		// first mismatched byte leaks how much of the guess was correct
		// via response timing.
		if !ok || len(pass) != len(token) || subtle.ConstantTimeCompare([]byte(pass), []byte(token)) != 1 {
			w.Header().Set("WWW-Authenticate", `Basic realm="chidrixx control plane"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}
