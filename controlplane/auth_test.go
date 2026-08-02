// SPDX-License-Identifier: Apache-2.0
package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequireAPITokenRejectsMissingOrWrongToken(t *testing.T) {
	store := testStore(t)
	_, realToken, err := store.CreateTenant("acme", "admin", "hunter2hunter2")
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}

	handler := requireAPIToken(store, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler must not run without a valid token")
	})

	cases := []struct {
		name     string
		setBasic bool
		pass     string
	}{
		{"no credentials at all", false, ""},
		{"wrong token", true, "not-the-real-token"},
		{"empty token", true, ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/ingest", nil)
			if c.setBasic {
				req.SetBasicAuth("agent", c.pass)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", rec.Code)
			}
		})
	}

	_ = realToken // used in the positive-path test below
}

func TestCreateAPITokenMintsARealWorkingSecondToken(t *testing.T) {
	store := testStore(t)
	tenantID, originalToken, err := store.CreateTenant("acme", "admin", "hunter2hunter2")
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}

	newToken, err := store.CreateAPIToken(tenantID, "rotated")
	if err != nil {
		t.Fatalf("CreateAPIToken: %v", err)
	}
	if newToken == originalToken {
		t.Fatal("expected a genuinely new token, got the same value")
	}

	for _, tok := range []string{originalToken, newToken} {
		got, err := store.AuthenticateAPIToken(tok)
		if err != nil {
			t.Fatalf("AuthenticateAPIToken(%q): %v", tok, err)
		}
		if got != tenantID {
			t.Fatalf("token resolved to tenant %d, want %d", got, tenantID)
		}
	}
}

func TestRequireAPITokenResolvesRealTenant(t *testing.T) {
	store := testStore(t)
	tenantID, token, err := store.CreateTenant("acme", "admin", "hunter2hunter2")
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}

	var gotTenant int64
	handler := requireAPIToken(store, func(w http.ResponseWriter, r *http.Request) {
		gotTenant, _ = tenantIDFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/ingest", nil)
	req.SetBasicAuth("agent", token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if gotTenant != tenantID {
		t.Fatalf("resolved tenant = %d, want %d", gotTenant, tenantID)
	}
}

func TestRequireAPITokenNeverResolvesAnotherTenantsToken(t *testing.T) {
	store := testStore(t)
	tenantA, tokenA, err := store.CreateTenant("acme", "admin-a", "hunter2hunter2")
	if err != nil {
		t.Fatalf("create tenant a: %v", err)
	}
	tenantB, tokenB, err := store.CreateTenant("globex", "admin-b", "hunter3hunter3")
	if err != nil {
		t.Fatalf("create tenant b: %v", err)
	}

	handler := requireAPIToken(store, func(w http.ResponseWriter, r *http.Request) {
		got, _ := tenantIDFromContext(r.Context())
		if got != tenantA && got != tenantB {
			t.Fatalf("resolved an unexpected tenant %d", got)
		}
		w.Header().Set("X-Tenant", "ok")
		w.WriteHeader(http.StatusOK)
	})

	for tok, want := range map[string]int64{tokenA: tenantA, tokenB: tenantB} {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/ingest", nil)
		req.SetBasicAuth("agent", tok)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("token for tenant %d: status = %d, want 200", want, rec.Code)
		}
	}
}

func TestRequireSessionRejectsMissingOrInvalidCookie(t *testing.T) {
	store := testStore(t)
	handler := requireSession(store, nil, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler must not run without a valid session")
	})

	t.Run("no cookie", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard-summary", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", rec.Code)
		}
	})

	t.Run("garbage cookie", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard-summary", nil)
		req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "not-a-real-session"})
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", rec.Code)
		}
	})
}

func TestRequireSessionResolvesRealTenantAndRole(t *testing.T) {
	store := testStore(t)
	tenantID, _, err := store.CreateTenant("acme", "admin", "hunter2hunter2")
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}

	user, err := store.AuthenticateUser("admin", "hunter2hunter2")
	if err != nil {
		t.Fatalf("authenticate user: %v", err)
	}
	sessionID, err := store.CreateSession(user)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	var gotTenant int64
	var gotRole string
	handler := requireSession(store, nil, func(w http.ResponseWriter, r *http.Request) {
		gotTenant, _ = tenantIDFromContext(r.Context())
		gotRole = roleFromContext(r.Context())
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
	if gotRole != RoleAdmin {
		t.Fatalf("resolved role = %q, want %q", gotRole, RoleAdmin)
	}
}

func TestRequireAdminRejectsViewerRole(t *testing.T) {
	handler := requireAdmin(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler must not run for a non-admin role")
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/budget", nil)
	ctx := req.Context()
	req = req.WithContext(contextWithRole(ctx, RoleViewer))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestRequireAdminAllowsAdminRole(t *testing.T) {
	handler := requireAdmin(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/budget", nil)
	req = req.WithContext(contextWithRole(req.Context(), RoleAdmin))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}
