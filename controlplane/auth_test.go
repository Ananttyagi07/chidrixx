// SPDX-License-Identifier: Apache-2.0
package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequireTokenDisabledWhenEmpty(t *testing.T) {
	called := false
	handler := requireToken("", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !called {
		t.Fatal("expected the wrapped handler to run when no token is configured")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestRequireTokenRejectsMissingOrWrongToken(t *testing.T) {
	handler := requireToken("secret-token", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler must not run without a valid token")
	}))

	cases := []struct {
		name     string
		setBasic bool
		user     string
		pass     string
	}{
		{"no credentials at all", false, "", ""},
		{"wrong password", true, "anyone", "wrong-token"},
		{"empty password", true, "anyone", ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if c.setBasic {
				req.SetBasicAuth(c.user, c.pass)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", rec.Code)
			}
			if got := rec.Header().Get("WWW-Authenticate"); got == "" {
				t.Fatal("expected a WWW-Authenticate header so browsers prompt for credentials")
			}
		})
	}
}

func TestRequireTokenAllowsCorrectTokenRegardlessOfUsername(t *testing.T) {
	handler := requireToken("secret-token", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Username is deliberately ignored — the shared token is the only
	// real credential.
	for _, user := range []string{"anyone", "admin", ""} {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.SetBasicAuth(user, "secret-token")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("username %q: status = %d, want 200", user, rec.Code)
		}
	}
}
