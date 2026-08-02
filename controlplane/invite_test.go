// SPDX-License-Identifier: Apache-2.0
package main

import (
	"database/sql"
	"testing"
)

func TestCreateInviteThenList(t *testing.T) {
	s := testStore(t)
	tenantID := testTenant(t, s)

	if err := s.CreateInvite(tenantID, "teammate@example.com", RoleViewer); err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}

	invites, err := s.ListInvites(tenantID)
	if err != nil {
		t.Fatalf("ListInvites: %v", err)
	}
	if len(invites) != 1 || invites[0].Email != "teammate@example.com" || invites[0].Role != RoleViewer {
		t.Fatalf("unexpected invites: %+v", invites)
	}
}

func TestCreateInviteRejectsInvalidRole(t *testing.T) {
	s := testStore(t)
	tenantID := testTenant(t, s)

	if err := s.CreateInvite(tenantID, "teammate@example.com", "superuser"); err == nil {
		t.Fatal("expected an error for an invalid role")
	}
}

func TestReInvitingSameEmailReplacesThePendingInvite(t *testing.T) {
	s := testStore(t)
	tenantA := testTenant(t, s)
	tenantB := testTenant(t, s)

	if err := s.CreateInvite(tenantA, "teammate@example.com", RoleViewer); err != nil {
		t.Fatalf("CreateInvite(a): %v", err)
	}
	if err := s.CreateInvite(tenantB, "teammate@example.com", RoleAdmin); err != nil {
		t.Fatalf("CreateInvite(b): %v", err)
	}

	invitesA, err := s.ListInvites(tenantA)
	if err != nil {
		t.Fatalf("ListInvites(a): %v", err)
	}
	if len(invitesA) != 0 {
		t.Fatalf("expected tenant a's invite replaced, got: %+v", invitesA)
	}

	invitesB, err := s.ListInvites(tenantB)
	if err != nil {
		t.Fatalf("ListInvites(b): %v", err)
	}
	if len(invitesB) != 1 || invitesB[0].Role != RoleAdmin {
		t.Fatalf("expected the latest invite (tenant b, admin) to win, got: %+v", invitesB)
	}
}

func TestDeleteInviteRevokesIt(t *testing.T) {
	s := testStore(t)
	tenantID := testTenant(t, s)

	if err := s.CreateInvite(tenantID, "teammate@example.com", RoleViewer); err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}
	if err := s.DeleteInvite(tenantID, "teammate@example.com"); err != nil {
		t.Fatalf("DeleteInvite: %v", err)
	}

	invites, err := s.ListInvites(tenantID)
	if err != nil {
		t.Fatalf("ListInvites: %v", err)
	}
	if len(invites) != 0 {
		t.Fatalf("expected the invite revoked, got: %+v", invites)
	}
}

func TestResolveOrProvisionSupabaseUserJoinsInvitedTenant(t *testing.T) {
	s := testStore(t)
	adminTenant := testTenant(t, s)

	if err := s.CreateInvite(adminTenant, "teammate@example.com", RoleViewer); err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}

	user, err := s.ResolveOrProvisionSupabaseUser("supa-teammate", "teammate@example.com")
	if err != nil {
		t.Fatalf("ResolveOrProvisionSupabaseUser: %v", err)
	}

	if user.TenantID != adminTenant {
		t.Fatalf("expected the invited user to join tenant %d, got %d", adminTenant, user.TenantID)
	}
	if user.Role != RoleViewer {
		t.Fatalf("expected the invited role (viewer), got %q", user.Role)
	}

	// The invite must be consumed -- gone after acceptance.
	invites, err := s.ListInvites(adminTenant)
	if err != nil {
		t.Fatalf("ListInvites: %v", err)
	}
	if len(invites) != 0 {
		t.Fatalf("expected the accepted invite consumed, got: %+v", invites)
	}

	// And the same Supabase identity resolves back to that same tenant on
	// a second call, not a fresh one.
	again, err := s.GetUserBySupabaseID("supa-teammate")
	if err != nil {
		t.Fatalf("GetUserBySupabaseID: %v", err)
	}
	if again.TenantID != adminTenant {
		t.Fatalf("expected re-resolution to the same tenant, got %d", again.TenantID)
	}
}

func TestResolveOrProvisionSupabaseUserCreatesNewTenantWithoutInvite(t *testing.T) {
	s := testStore(t)

	user, err := s.ResolveOrProvisionSupabaseUser("supa-founder", "founder@example.com")
	if err != nil {
		t.Fatalf("ResolveOrProvisionSupabaseUser: %v", err)
	}

	if user.Role != RoleAdmin {
		t.Fatalf("expected a founder with no invite to become admin of a new tenant, got role %q", user.Role)
	}
	if user.TenantID == 0 {
		t.Fatal("expected a real non-zero tenant ID")
	}
}

func TestInvitedUserDoesNotLeakIntoOtherTenants(t *testing.T) {
	s := testStore(t)
	tenantA := testTenant(t, s)
	tenantB := testTenant(t, s)

	if err := s.CreateInvite(tenantA, "teammate@example.com", RoleViewer); err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}
	invitedUser, err := s.ResolveOrProvisionSupabaseUser("supa-teammate", "teammate@example.com")
	if err != nil {
		t.Fatalf("ResolveOrProvisionSupabaseUser: %v", err)
	}

	if err := s.Ingest(invitedUser.TenantID, "cluster-a", []Finding{{Source: "ns/a", CostHighINR: 50}}); err != nil {
		t.Fatalf("ingest: %v", err)
	}

	clustersB, err := s.Clusters(tenantB)
	if err != nil {
		t.Fatalf("Clusters(b): %v", err)
	}
	if len(clustersB) != 0 {
		t.Fatalf("tenant b's query leaked the invited user's tenant data: %+v", clustersB)
	}
}

func TestGetPendingInviteReturnsErrNoRowsWhenNoneExists(t *testing.T) {
	s := testStore(t)

	if _, err := s.getPendingInvite("nobody@example.com"); err != sql.ErrNoRows {
		t.Fatalf("expected sql.ErrNoRows, got: %v", err)
	}
}
