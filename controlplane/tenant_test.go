// SPDX-License-Identifier: Apache-2.0
package main

import (
	"database/sql"
	"testing"
)

func TestGetUserBySupabaseIDReturnsErrNoRowsWhenUnprovisioned(t *testing.T) {
	s := testStore(t)

	_, err := s.GetUserBySupabaseID("nonexistent-supabase-id")
	if err != sql.ErrNoRows {
		t.Fatalf("expected sql.ErrNoRows for an unprovisioned Supabase user, got: %v", err)
	}
}

func TestProvisionTenantForSupabaseUserCreatesRealTenantAdminAndToken(t *testing.T) {
	s := testStore(t)

	user, apiToken, err := s.ProvisionTenantForSupabaseUser("supa-abc", "founder@example.com")
	if err != nil {
		t.Fatalf("ProvisionTenantForSupabaseUser: %v", err)
	}
	if user.TenantID == 0 {
		t.Fatal("expected a real non-zero tenant ID")
	}
	if user.Role != RoleAdmin {
		t.Fatalf("expected the first user on a new tenant to be admin, got %q", user.Role)
	}
	if user.Username != "founder@example.com" {
		t.Fatalf("expected username = email, got %q", user.Username)
	}
	if apiToken == "" {
		t.Fatal("expected a real non-empty ingest token")
	}

	// The real ingest token must actually work.
	resolvedTenant, err := s.AuthenticateAPIToken(apiToken)
	if err != nil {
		t.Fatalf("AuthenticateAPIToken: %v", err)
	}
	if resolvedTenant != user.TenantID {
		t.Fatalf("token resolved to tenant %d, want %d", resolvedTenant, user.TenantID)
	}

	// And the user must be findable again by their Supabase ID.
	found, err := s.GetUserBySupabaseID("supa-abc")
	if err != nil {
		t.Fatalf("GetUserBySupabaseID: %v", err)
	}
	if found.TenantID != user.TenantID {
		t.Fatalf("re-lookup resolved a different tenant: %d vs %d", found.TenantID, user.TenantID)
	}
}

func TestProvisionTenantForSupabaseUserIsIsolatedFromOtherTenants(t *testing.T) {
	s := testStore(t)

	userA, _, err := s.ProvisionTenantForSupabaseUser("supa-a", "a@example.com")
	if err != nil {
		t.Fatalf("provision a: %v", err)
	}
	userB, _, err := s.ProvisionTenantForSupabaseUser("supa-b", "b@example.com")
	if err != nil {
		t.Fatalf("provision b: %v", err)
	}

	if userA.TenantID == userB.TenantID {
		t.Fatal("two different Supabase users must never be provisioned onto the same tenant")
	}

	// Real isolation, not just distinct IDs: give tenant A a real finding
	// and confirm tenant B's queries don't see it.
	if err := s.Ingest(userA.TenantID, "cluster-a", []Finding{{Source: "ns/a", CostHighINR: 100}}); err != nil {
		t.Fatalf("ingest: %v", err)
	}
	clustersB, err := s.Clusters(userB.TenantID)
	if err != nil {
		t.Fatalf("Clusters(b): %v", err)
	}
	if len(clustersB) != 0 {
		t.Fatalf("tenant B's query leaked tenant A's data: %+v", clustersB)
	}
}
