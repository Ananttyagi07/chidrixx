// SPDX-License-Identifier: Apache-2.0
package main

import "testing"

func TestEvaluateRemediationQualifiesARealHighConfidenceManifestFix(t *testing.T) {
	findings := []FindingRow{
		{
			Finding: Finding{
				Source: "checkout/checkout-1", Destination: "8.8.8.8",
				PathClass: "INTERNET_EGRESS", Confidence: "high",
				FixHint: "deny this destination", FixManifest: "apiVersion: networking.k8s.io/v1\nkind: NetworkPolicy",
				CostHighINR: 40, SavingsHighINR: 35,
			},
			ClusterID: "cluster-a",
		},
	}

	decisions := EvaluateRemediation(findings, DefaultRemediationPolicy())

	if len(decisions) != 1 {
		t.Fatalf("expected 1 decision, got %d", len(decisions))
	}
	if !decisions[0].WouldAutoApply {
		t.Fatalf("expected this real high-confidence, manifest-bearing, positive-savings fix to qualify, got: %+v", decisions[0])
	}
	if len(decisions[0].Reasons) == 0 {
		t.Fatal("expected a real reason explaining the qualifying decision")
	}
}

func TestEvaluateRemediationDisqualifiesAFixWithNoManifest(t *testing.T) {
	findings := []FindingRow{
		{
			Finding: Finding{
				Source: "checkout/checkout-1", Destination: "redis/redis-master",
				PathClass: "CROSS_AZ", Confidence: "high",
				FixHint: "co-locate these workloads", FixManifest: "", // CROSS_AZ never gets a real manifest -- fixengine.go
				CostHighINR: 40, SavingsHighINR: 35,
			},
			ClusterID: "cluster-a",
		},
	}

	decisions := EvaluateRemediation(findings, DefaultRemediationPolicy())

	if len(decisions) != 1 {
		t.Fatalf("expected 1 decision, got %d", len(decisions))
	}
	if decisions[0].WouldAutoApply {
		t.Fatalf("expected a fix with no real manifest to be disqualified, got: %+v", decisions[0])
	}
	if len(decisions[0].Reasons) == 0 {
		t.Fatal("expected a real reason explaining the disqualifying decision, not just a bare false")
	}
}

func TestEvaluateRemediationDisqualifiesLowConfidence(t *testing.T) {
	findings := []FindingRow{
		{
			Finding: Finding{
				Source: "checkout/checkout-1", Destination: "8.8.8.8",
				PathClass: "INTERNET_EGRESS", Confidence: "low",
				FixHint: "deny this destination", FixManifest: "real-manifest-text",
				CostHighINR: 40, SavingsHighINR: 35,
			},
			ClusterID: "cluster-a",
		},
	}

	decisions := EvaluateRemediation(findings, DefaultRemediationPolicy())

	if decisions[0].WouldAutoApply {
		t.Fatalf("expected a low-confidence finding to be disqualified even with a real manifest, got: %+v", decisions[0])
	}
}

func TestEvaluateRemediationDisqualifiesZeroSavings(t *testing.T) {
	findings := []FindingRow{
		{
			Finding: Finding{
				Source: "checkout/checkout-1", Destination: "8.8.8.8",
				PathClass: "INTERNET_EGRESS", Confidence: "high",
				FixHint: "deny this destination", FixManifest: "real-manifest-text",
				CostHighINR: 40, SavingsHighINR: 0,
			},
			ClusterID: "cluster-a",
		},
	}

	decisions := EvaluateRemediation(findings, DefaultRemediationPolicy())

	if decisions[0].WouldAutoApply {
		t.Fatalf("expected zero predicted savings to be disqualified, got: %+v", decisions[0])
	}
}

func TestEvaluateRemediationSkipsFindingsWithNoFixHintEntirely(t *testing.T) {
	findings := []FindingRow{
		{Finding: Finding{Source: "a", Destination: "b", FixHint: ""}, ClusterID: "cluster-a"},
	}

	decisions := EvaluateRemediation(findings, DefaultRemediationPolicy())

	if len(decisions) != 0 {
		t.Fatalf("expected findings with no fix_hint at all to be excluded entirely, got: %+v", decisions)
	}
}

func TestEvaluateRemediationNeverMutatesAnything(t *testing.T) {
	// This is the entire point of the dry-run phase: no real or simulated
	// cluster state is ever touched. There's no client-go call, no exec,
	// nothing to mock -- this test exists to document that guarantee
	// explicitly, not just leave it implicit.
	findings := []FindingRow{
		{
			Finding: Finding{
				Source: "checkout/checkout-1", Destination: "8.8.8.8",
				PathClass: "INTERNET_EGRESS", Confidence: "high",
				FixHint: "deny this destination", FixManifest: "real-manifest-text",
				CostHighINR: 40, SavingsHighINR: 35,
			},
			ClusterID: "cluster-a",
		},
	}
	before := append([]FindingRow(nil), findings...)

	_ = EvaluateRemediation(findings, DefaultRemediationPolicy())

	if len(findings) != len(before) || findings[0] != before[0] {
		t.Fatal("EvaluateRemediation must not mutate its input")
	}
}
