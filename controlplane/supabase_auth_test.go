// SPDX-License-Identifier: Apache-2.0
package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// fakeSupabase stands in for the real Supabase Auth API in tests --
// asserting against the real service on every test run isn't practical
// (network dependency, rate limits, a real account needed), so this
// implements the one contract this code actually depends on: a GET to
// /auth/v1/user with a Bearer token returns either a real user body or a
// non-200. The live end-to-end pass against the real project is done
// separately, by hand, not as part of `go test`.
func fakeSupabase(t *testing.T, validToken string, user SupabaseUser) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/auth/v1/user" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		auth := r.Header.Get("Authorization")
		if auth != "Bearer "+validToken {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.Header.Get("apikey") == "" {
			t.Error("expected an apikey header on every request, matching the real Supabase contract")
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(user)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestSupabaseAuthenticatorVerifyToken(t *testing.T) {
	srv := fakeSupabase(t, "real-token", SupabaseUser{ID: "supa-user-1", Email: "founder@example.com"})
	auth := NewSupabaseAuthenticator(srv.URL, "test-publishable-key")

	user, err := auth.VerifyToken("real-token")
	if err != nil {
		t.Fatalf("VerifyToken: %v", err)
	}
	if user.ID != "supa-user-1" || user.Email != "founder@example.com" {
		t.Fatalf("unexpected user: %+v", user)
	}

	if _, err := auth.VerifyToken("wrong-token"); err != ErrInvalidSupabaseToken {
		t.Fatalf("expected ErrInvalidSupabaseToken for a wrong token, got: %v", err)
	}
}

func TestRequireSessionAcceptsRealSupabaseBearerToken(t *testing.T) {
	store := testStore(t)
	srv := fakeSupabase(t, "real-token", SupabaseUser{ID: "supa-user-1", Email: "founder@example.com"})
	auth := NewSupabaseAuthenticator(srv.URL, "test-publishable-key")

	var gotTenant int64
	var gotRole, gotUsername string
	handler := requireSession(store, auth, func(w http.ResponseWriter, r *http.Request) {
		gotTenant, _ = tenantIDFromContext(r.Context())
		gotRole = roleFromContext(r.Context())
		gotUsername = usernameFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard-summary", nil)
	req.Header.Set("Authorization", "Bearer real-token")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if gotTenant == 0 {
		t.Fatal("expected a real auto-provisioned tenant ID, got 0")
	}
	if gotRole != RoleAdmin {
		t.Fatalf("expected the first-ever Supabase user on a tenant to be admin, got %q", gotRole)
	}
	if gotUsername != "founder@example.com" {
		t.Fatalf("expected username = the real Supabase email, got %q", gotUsername)
	}
}

func TestRequireSessionProvisionsExactlyOnceForTheSameSupabaseUser(t *testing.T) {
	store := testStore(t)
	srv := fakeSupabase(t, "real-token", SupabaseUser{ID: "supa-user-1", Email: "founder@example.com"})
	auth := NewSupabaseAuthenticator(srv.URL, "test-publishable-key")

	var tenants []int64
	handler := requireSession(store, auth, func(w http.ResponseWriter, r *http.Request) {
		t, _ := tenantIDFromContext(r.Context())
		tenants = append(tenants, t)
		w.WriteHeader(http.StatusOK)
	})

	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard-summary", nil)
		req.Header.Set("Authorization", "Bearer real-token")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d: status = %d, want 200", i, rec.Code)
		}
	}

	if len(tenants) != 3 || tenants[0] != tenants[1] || tenants[1] != tenants[2] {
		t.Fatalf("expected the same tenant resolved on every request for the same Supabase user, got: %v", tenants)
	}

	count, err := store.TenantCount()
	if err != nil {
		t.Fatalf("TenantCount: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 tenant provisioned despite 3 requests, got %d", count)
	}
}

func TestRequireSessionRejectsInvalidSupabaseBearerToken(t *testing.T) {
	store := testStore(t)
	srv := fakeSupabase(t, "real-token", SupabaseUser{ID: "supa-user-1", Email: "founder@example.com"})
	auth := NewSupabaseAuthenticator(srv.URL, "test-publishable-key")

	handler := requireSession(store, auth, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler must not run for an invalid bearer token")
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard-summary", nil)
	req.Header.Set("Authorization", "Bearer not-the-real-token")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestRequireSessionFallsBackToCookieWhenNoBearerHeader(t *testing.T) {
	store := testStore(t)
	tenantID, _, err := store.CreateTenant("acme", "admin", "hunter2hunter2")
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	user, err := store.AuthenticateUser("admin", "hunter2hunter2")
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	sessionID, err := store.CreateSession(user)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	// A real Supabase authenticator is configured, but this request has no
	// Authorization header at all -- must fall back to the legacy cookie
	// path, not be rejected outright.
	srv := fakeSupabase(t, "real-token", SupabaseUser{ID: "unused", Email: "unused@example.com"})
	auth := NewSupabaseAuthenticator(srv.URL, "test-publishable-key")

	var gotTenant int64
	handler := requireSession(store, auth, func(w http.ResponseWriter, r *http.Request) {
		gotTenant, _ = tenantIDFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard-summary", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sessionID})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if gotTenant != tenantID {
		t.Fatalf("resolved tenant = %d, want %d", gotTenant, tenantID)
	}
}
