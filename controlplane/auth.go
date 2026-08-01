// SPDX-License-Identifier: Apache-2.0
package main

import (
	"context"
	"net/http"
)

type contextKey string

const (
	ctxTenantID contextKey = "tenant_id"
	ctxRole     contextKey = "role"
	ctxUsername contextKey = "username"
)

func tenantIDFromContext(ctx context.Context) (int64, bool) {
	id, ok := ctx.Value(ctxTenantID).(int64)
	return id, ok
}

func roleFromContext(ctx context.Context) string {
	role, _ := ctx.Value(ctxRole).(string)
	return role
}

func usernameFromContext(ctx context.Context) string {
	username, _ := ctx.Value(ctxUsername).(string)
	return username
}

// contextWithRole exists for requireAdmin's own unit tests -- production
// code only ever gets a role in context via requireSession, which resolves
// it from a real session row, never sets it directly.
func contextWithRole(ctx context.Context, role string) context.Context {
	return context.WithValue(ctx, ctxRole, role)
}

const sessionCookieName = "chidrixx_session"

// requireAPIToken authenticates an agent's ingest request against the
// api_tokens table and resolves it to the tenant that token belongs to.
// The wire shape is unchanged from before real multi-tenancy: Basic Auth,
// username ignored, the token is the password (agent/cmd/kharcha/shipper.go
// never needed to change) -- what changed is that the token now looks up a
// specific tenant instead of being compared against one shared secret.
func requireAPIToken(store *Store, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, pass, ok := r.BasicAuth()
		if !ok || pass == "" {
			w.Header().Set("WWW-Authenticate", `Basic realm="chidrixx control plane"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		tenantID, err := store.AuthenticateAPIToken(pass)
		if err != nil {
			w.Header().Set("WWW-Authenticate", `Basic realm="chidrixx control plane"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), ctxTenantID, tenantID)
		next(w, r.WithContext(ctx))
	}
}

// requireSession authenticates a browser request against the session
// cookie set by /api/v1/auth/login, resolving it to a tenant + role. This
// replaces the single shared Basic Auth token every dashboard read used to
// go through -- a real login, not a password prompt for a secret everyone
// shares.
func requireSession(store *Store, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(sessionCookieName)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		sess, err := store.GetSession(cookie.Value)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), ctxTenantID, sess.TenantID)
		ctx = context.WithValue(ctx, ctxRole, sess.Role)
		ctx = context.WithValue(ctx, ctxUsername, sess.Username)
		next(w, r.WithContext(ctx))
	}
}

// requireAdmin gates a handler behind the viewer/admin role resolved by
// requireSession -- the one write path a logged-in user has (setting the
// budget figure) needs admin, not just "logged in to this tenant."
func requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if roleFromContext(r.Context()) != RoleAdmin {
			http.Error(w, "forbidden: admin role required", http.StatusForbidden)
			return
		}
		next(w, r)
	}
}
