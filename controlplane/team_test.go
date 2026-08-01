// SPDX-License-Identifier: Apache-2.0
package main

import "testing"

func TestExtractNamespaceFromRealSources(t *testing.T) {
	cases := []struct {
		source    string
		wantNS    string
		wantFound bool
	}{
		{"checkout/checkout-abc123", "checkout", true},
		{"ai-gateway/ai-gateway-7f9c8d-x2z1q", "ai-gateway", true},
		{"kube-system/coredns-5d78c9869d-abcde", "kube-system", true},
		// Real cgroup-path fallback for non-k8s-resolved sources -- must
		// NOT be misparsed as a namespace named "user-1000.slice".
		{"user-1000.slice/user@1000.service/app.slice/app-org.chromium.Chromium-6727.scope", "", false},
		{"system.slice/apt-daily.service", "", false},
		{"", "", false},
		{"no-slash-at-all", "", false},
	}

	for _, c := range cases {
		ns, ok := extractNamespace(c.source)
		if ok != c.wantFound || ns != c.wantNS {
			t.Errorf("extractNamespace(%q) = (%q, %v), want (%q, %v)", c.source, ns, ok, c.wantNS, c.wantFound)
		}
	}
}

func TestComputeSpendByTeamGroupsAndFallsBackToUnassigned(t *testing.T) {
	findings := []FindingRow{
		{Finding: Finding{Source: "checkout/pod-1", CostHighINR: 100}},
		{Finding: Finding{Source: "checkout/pod-2", CostHighINR: 50}},
		{Finding: Finding{Source: "ai-gateway/pod-1", CostHighINR: 200}},
		{Finding: Finding{Source: "user-1000.slice/user@1000.service/app.slice/x.scope", CostHighINR: 30}},
	}
	ownership := []TeamOwnership{
		{Namespace: "checkout", Team: "Payments"},
		{Namespace: "ai-gateway", Team: "AI"},
	}

	spend := computeSpendByTeam(findings, ownership)

	if len(spend) != 3 {
		t.Fatalf("expected 3 team buckets (AI, Payments, Unassigned), got %d: %+v", len(spend), spend)
	}
	if spend[0].Team != "AI" || spend[0].CostHighINR != 200 {
		t.Errorf("expected AI (200) ranked first, got %+v", spend[0])
	}
	if spend[1].Team != "Payments" || spend[1].CostHighINR != 150 || spend[1].FindingCount != 2 {
		t.Errorf("expected Payments (150, 2 findings) ranked second, got %+v", spend[1])
	}
	if spend[2].Team != unassignedTeam || spend[2].CostHighINR != 30 {
		t.Errorf("expected Unassigned (30) for the cgroup-path source, got %+v", spend[2])
	}
}

func TestComputeSpendByTeamAllUnassignedWithNoOwnershipConfigured(t *testing.T) {
	findings := []FindingRow{
		{Finding: Finding{Source: "checkout/pod-1", CostHighINR: 100}},
	}

	spend := computeSpendByTeam(findings, nil)

	if len(spend) != 1 || spend[0].Team != unassignedTeam || spend[0].CostHighINR != 100 {
		t.Fatalf("expected everything Unassigned with no ownership configured, got: %+v", spend)
	}
}

func TestTeamOwnershipStoreCRUDAndTenantIsolation(t *testing.T) {
	s := testStore(t)
	tenantA := testTenant(t, s)
	tenantB := testTenant(t, s)

	if err := s.SetTeamOwnership(tenantA, "checkout", "Payments"); err != nil {
		t.Fatalf("SetTeamOwnership(a): %v", err)
	}
	if err := s.SetTeamOwnership(tenantB, "checkout", "SomeOtherTeam"); err != nil {
		t.Fatalf("SetTeamOwnership(b): %v", err)
	}

	listA, err := s.ListTeamOwnership(tenantA)
	if err != nil {
		t.Fatalf("ListTeamOwnership(a): %v", err)
	}
	if len(listA) != 1 || listA[0].Team != "Payments" {
		t.Fatalf("tenant a's mapping leaked or missed tenant b's, got: %+v", listA)
	}

	// Overwrite, not duplicate.
	if err := s.SetTeamOwnership(tenantA, "checkout", "Payments-Renamed"); err != nil {
		t.Fatalf("SetTeamOwnership overwrite: %v", err)
	}
	listA, err = s.ListTeamOwnership(tenantA)
	if err != nil {
		t.Fatalf("ListTeamOwnership(a) after overwrite: %v", err)
	}
	if len(listA) != 1 || listA[0].Team != "Payments-Renamed" {
		t.Fatalf("expected overwrite not duplicate, got: %+v", listA)
	}

	if err := s.DeleteTeamOwnership(tenantA, "checkout"); err != nil {
		t.Fatalf("DeleteTeamOwnership: %v", err)
	}
	listA, err = s.ListTeamOwnership(tenantA)
	if err != nil {
		t.Fatalf("ListTeamOwnership(a) after delete: %v", err)
	}
	if len(listA) != 0 {
		t.Fatalf("expected empty mapping after delete, got: %+v", listA)
	}

	// tenant b's own mapping must be untouched by tenant a's delete.
	listB, err := s.ListTeamOwnership(tenantB)
	if err != nil {
		t.Fatalf("ListTeamOwnership(b): %v", err)
	}
	if len(listB) != 1 || listB[0].Team != "SomeOtherTeam" {
		t.Fatalf("tenant b's mapping was affected by tenant a's delete: %+v", listB)
	}
}
