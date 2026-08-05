// SPDX-License-Identifier: Apache-2.0
package main

import "testing"

// realGeneratedManifest mirrors the exact shape
// agent/cmd/kharcha/fixengine.go's networkPolicyDenyDestination produces
// -- not a hand-simplified stand-in -- so these tests prove the parser
// understands what the fix engine actually ships, not an idealized shape
// of it.
const realGeneratedManifest = `apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: deny-8-8-8-8
  namespace: checkout
spec:
  podSelector: {}   # narrow this to the specific workload's own labels
  policyTypes: [Egress]
  egress:
    - to:
        - ipBlock:
            cidr: 0.0.0.0/0
            except: ["8.8.8.8/32"]
`

const realGeneratedManifestIPv6 = `apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: deny-2001-db8--1
  namespace: payments
spec:
  podSelector: {}
  policyTypes: [Egress]
  egress:
    - to:
        - ipBlock:
            cidr: ::/0
            except: ["2001:db8::1/128"]
`

func TestParseGeneratedNetworkPolicyExtractsNamespaceAndBlockedIP(t *testing.T) {
	namespace, blockedIPs, err := parseGeneratedNetworkPolicy(realGeneratedManifest)
	if err != nil {
		t.Fatalf("parseGeneratedNetworkPolicy: %v", err)
	}
	if namespace != "checkout" {
		t.Fatalf("namespace = %q, want checkout", namespace)
	}
	if len(blockedIPs) != 1 || blockedIPs[0] != "8.8.8.8" {
		t.Fatalf("blockedIPs = %v, want [8.8.8.8] (the /32 mask must be stripped -- flow_aggregate stores bare IPs)", blockedIPs)
	}
}

func TestParseGeneratedNetworkPolicyHandlesIPv6(t *testing.T) {
	namespace, blockedIPs, err := parseGeneratedNetworkPolicy(realGeneratedManifestIPv6)
	if err != nil {
		t.Fatalf("parseGeneratedNetworkPolicy: %v", err)
	}
	if namespace != "payments" {
		t.Fatalf("namespace = %q, want payments", namespace)
	}
	if len(blockedIPs) != 1 || blockedIPs[0] != "2001:db8::1" {
		t.Fatalf("blockedIPs = %v, want [2001:db8::1] (the /128 mask must be stripped)", blockedIPs)
	}
}

func TestParseGeneratedNetworkPolicyReturnsNoBlockedIPsForAManifestMissingIPBlock(t *testing.T) {
	// A real, validly-structured YAML mapping (no parse error) that just
	// isn't shaped like this fix engine's own template -- e.g. a manifest
	// with no egress/ipBlock/except section at all. Callers must treat
	// this the same as a parse error (can't verify), not silently
	// proceed as if there were zero blocked destinations to worry about.
	const noIPBlock = `apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: some-other-policy
  namespace: checkout
spec:
  podSelector: {}
`
	namespace, blockedIPs, err := parseGeneratedNetworkPolicy(noIPBlock)
	if err != nil {
		t.Fatalf("expected no hard error for well-formed YAML missing our expected fields, got: %v", err)
	}
	if len(blockedIPs) != 0 {
		t.Fatalf("expected no blocked IPs from a manifest with no ipBlock/except section, got %v", blockedIPs)
	}
	if namespace != "checkout" {
		t.Fatalf("namespace = %q, want checkout (the namespace field itself is still real and present)", namespace)
	}
}

func TestParseGeneratedNetworkPolicyErrorsOnABareScalarString(t *testing.T) {
	// A placeholder/garbage string that isn't even a YAML mapping should
	// be a real, surfaced parse error -- not silently treated as "zero
	// blocked IPs found."
	_, _, err := parseGeneratedNetworkPolicy("not-a-real-manifest")
	if err == nil {
		t.Fatal("expected a real parse error for a bare scalar string, not a YAML mapping at all")
	}
}

func TestParseGeneratedNetworkPolicyReturnsErrorForInvalidYAML(t *testing.T) {
	_, _, err := parseGeneratedNetworkPolicy("metadata: [unclosed")
	if err == nil {
		t.Fatal("expected a real parse error for genuinely invalid YAML")
	}
}

func TestEvaluateTrafficReplaySafeWhenOnlyFlaggedSourceHasHistory(t *testing.T) {
	historical := []HistoricalFlowCost{
		{SrcWorkload: "checkout/checkout-1", CostHighINR: 40},
	}

	result := EvaluateTrafficReplay("checkout", "checkout/checkout-1", []string{"8.8.8.8"}, historical)

	if !result.Safe {
		t.Fatalf("expected safe when only the flagged workload has real history, got: %+v", result)
	}
	if result.SafetyScore != 1 {
		t.Fatalf("SafetyScore = %v, want 1", result.SafetyScore)
	}
	if len(result.AffectedWorkloads) != 0 {
		t.Fatalf("expected no affected workloads, got %v", result.AffectedWorkloads)
	}
}

func TestEvaluateTrafficReplayUnsafeWhenOtherWorkloadInSameNamespaceSharesDestination(t *testing.T) {
	historical := []HistoricalFlowCost{
		{SrcWorkload: "checkout/checkout-1", CostHighINR: 40}, // the flagged workload
		{SrcWorkload: "checkout/checkout-2", CostHighINR: 10}, // a real, different workload, same namespace
	}

	result := EvaluateTrafficReplay("checkout", "checkout/checkout-1", []string{"8.8.8.8"}, historical)

	if result.Safe {
		t.Fatalf("expected unsafe when another real workload in the same namespace shares the destination, got: %+v", result)
	}
	if len(result.AffectedWorkloads) != 1 || result.AffectedWorkloads[0] != "checkout/checkout-2" {
		t.Fatalf("AffectedWorkloads = %v, want [checkout/checkout-2]", result.AffectedWorkloads)
	}
	wantScore := 40.0 / 50.0
	if result.SafetyScore != wantScore {
		t.Fatalf("SafetyScore = %v, want %v", result.SafetyScore, wantScore)
	}
	if result.CollateralCostINR != 10 {
		t.Fatalf("CollateralCostINR = %v, want 10", result.CollateralCostINR)
	}
}

func TestEvaluateTrafficReplayIgnoresOtherNamespaces(t *testing.T) {
	historical := []HistoricalFlowCost{
		{SrcWorkload: "checkout/checkout-1", CostHighINR: 40},
		{SrcWorkload: "payments/payments-1", CostHighINR: 999}, // a different namespace -- this policy's podSelector:{} never touches it
	}

	result := EvaluateTrafficReplay("checkout", "checkout/checkout-1", []string{"8.8.8.8"}, historical)

	if !result.Safe {
		t.Fatalf("expected safe -- a different namespace's workload isn't affected by this policy, got: %+v", result)
	}
	if len(result.AffectedWorkloads) != 0 {
		t.Fatalf("expected no affected workloads from a different namespace, got %v", result.AffectedWorkloads)
	}
}

func TestEvaluateTrafficReplayUnsafeWhenNoHistoricalDataMatches(t *testing.T) {
	result := EvaluateTrafficReplay("checkout", "checkout/checkout-1", []string{"8.8.8.8"}, nil)

	if result.Safe {
		t.Fatalf("expected unsafe (not confirmed) when there's no historical data to check against at all, got: %+v", result)
	}
	if result.Note == "" {
		t.Fatal("expected an honest note explaining why this couldn't be confirmed safe")
	}
}

func TestEvaluateTrafficReplaySortsAffectedWorkloadsDeterministically(t *testing.T) {
	historical := []HistoricalFlowCost{
		{SrcWorkload: "checkout/checkout-1", CostHighINR: 40},
		{SrcWorkload: "checkout/zeta", CostHighINR: 5},
		{SrcWorkload: "checkout/alpha", CostHighINR: 5},
	}

	result := EvaluateTrafficReplay("checkout", "checkout/checkout-1", []string{"8.8.8.8"}, historical)

	if len(result.AffectedWorkloads) != 2 || result.AffectedWorkloads[0] != "checkout/alpha" || result.AffectedWorkloads[1] != "checkout/zeta" {
		t.Fatalf("expected deterministically sorted affected workloads, got %v", result.AffectedWorkloads)
	}
}
