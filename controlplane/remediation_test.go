// SPDX-License-Identifier: Apache-2.0
package main

import (
	"errors"
	"testing"
)

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

func realManifestDecision() RemediationDecision {
	return RemediationDecision{
		ClusterID: "cluster-a", Source: "checkout/checkout-1", Destination: "8.8.8.8",
		PathClass: "INTERNET_EGRESS", Confidence: "high",
		FixHint: "deny this destination", FixManifest: realGeneratedManifest,
		CostHighINR: 40, SavingsHighINR: 35,
		WouldAutoApply: true,
		Reasons:        []string{"has a real generated manifest, high confidence, and positive predicted savings"},
	}
}

func TestSimulateTrafficReplayLeavesSafeDecisionQualifying(t *testing.T) {
	decisions := []RemediationDecision{realManifestDecision()}

	fetch := func(clusterID, destinationIP string) ([]HistoricalFlowCost, error) {
		return []HistoricalFlowCost{{SrcWorkload: "checkout/checkout-1", CostHighINR: 40}}, nil
	}

	out, err := SimulateTrafficReplay(decisions, DefaultRemediationPolicy(), fetch)
	if err != nil {
		t.Fatalf("SimulateTrafficReplay: %v", err)
	}
	if !out[0].WouldAutoApply {
		t.Fatalf("expected a decision with no real collateral traffic to stay qualifying, got: %+v", out[0])
	}
	if out[0].TrafficReplay == nil || !out[0].TrafficReplay.Safe {
		t.Fatalf("expected a real safe TrafficReplay result attached, got: %+v", out[0].TrafficReplay)
	}
	found := false
	for _, r := range out[0].Reasons {
		if r == "traffic replay confirmed no other real workload in this namespace depends on the blocked destination" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected the confirming replay reason to be present, got: %v", out[0].Reasons)
	}
}

func TestSimulateTrafficReplayAttachesResultAndDisqualifiesUnsafeDecision(t *testing.T) {
	decisions := []RemediationDecision{realManifestDecision()}

	fetch := func(clusterID, destinationIP string) ([]HistoricalFlowCost, error) {
		return []HistoricalFlowCost{
			{SrcWorkload: "checkout/checkout-1", CostHighINR: 40},
			{SrcWorkload: "checkout/checkout-2", CostHighINR: 10}, // real collateral, same namespace
		}, nil
	}

	out, err := SimulateTrafficReplay(decisions, DefaultRemediationPolicy(), fetch)
	if err != nil {
		t.Fatalf("SimulateTrafficReplay: %v", err)
	}
	if out[0].WouldAutoApply {
		t.Fatalf("expected a decision with real collateral traffic to be disqualified, got: %+v", out[0])
	}
	if out[0].TrafficReplay == nil || out[0].TrafficReplay.Safe {
		t.Fatalf("expected an unsafe TrafficReplay result attached, got: %+v", out[0].TrafficReplay)
	}
	if len(out[0].Reasons) != 1 {
		t.Fatalf("expected the stale qualifying reason to be replaced, not kept alongside the new one, got: %v", out[0].Reasons)
	}
}

func TestSimulateTrafficReplaySkipsDecisionsWithNoManifest(t *testing.T) {
	decisions := []RemediationDecision{
		{ClusterID: "cluster-a", Source: "redis/redis-1", FixManifest: "", WouldAutoApply: false},
	}

	fetch := func(clusterID, destinationIP string) ([]HistoricalFlowCost, error) {
		t.Fatal("fetchHistorical must not be called for a decision with no manifest")
		return nil, nil
	}

	out, err := SimulateTrafficReplay(decisions, DefaultRemediationPolicy(), fetch)
	if err != nil {
		t.Fatalf("SimulateTrafficReplay: %v", err)
	}
	if out[0].TrafficReplay != nil {
		t.Fatalf("expected TrafficReplay to stay nil with no manifest to test, got: %+v", out[0].TrafficReplay)
	}
}

func TestSimulateTrafficReplaySkipsDecisionsAlreadyDisqualified(t *testing.T) {
	// The replay lookup is genuinely expensive at real scale, and its
	// result can't change the outcome for a fix that already failed
	// another policy bar -- so it must not be run at all there.
	d := realManifestDecision()
	d.Confidence = "low"
	d.WouldAutoApply = false
	d.Reasons = []string{"confidence is \"low\", not \"high\""}
	decisions := []RemediationDecision{d}

	fetch := func(clusterID, destinationIP string) ([]HistoricalFlowCost, error) {
		t.Fatal("fetchHistorical must not be called for an already-disqualified decision")
		return nil, nil
	}

	out, err := SimulateTrafficReplay(decisions, DefaultRemediationPolicy(), fetch)
	if err != nil {
		t.Fatalf("SimulateTrafficReplay: %v", err)
	}
	if out[0].TrafficReplay != nil {
		t.Fatalf("expected no replay result for an already-disqualified decision, got: %+v", out[0].TrafficReplay)
	}
	if len(out[0].Reasons) != 1 {
		t.Fatalf("expected the original disqualifying reason left intact, got: %v", out[0].Reasons)
	}
}

func TestSimulateTrafficReplayTreatsUnparseableManifestConservatively(t *testing.T) {
	d := realManifestDecision()
	d.FixManifest = "not-a-real-manifest"
	decisions := []RemediationDecision{d}

	fetch := func(clusterID, destinationIP string) ([]HistoricalFlowCost, error) {
		t.Fatal("fetchHistorical must not be called when the manifest itself can't be parsed")
		return nil, nil
	}

	out, err := SimulateTrafficReplay(decisions, DefaultRemediationPolicy(), fetch)
	if err != nil {
		t.Fatalf("SimulateTrafficReplay: %v", err)
	}
	if out[0].WouldAutoApply {
		t.Fatal("expected an unparseable manifest to be treated conservatively as not safe, not silently left qualifying")
	}
	if out[0].TrafficReplay == nil || out[0].TrafficReplay.Safe {
		t.Fatalf("expected an unsafe TrafficReplay result with an honest note, got: %+v", out[0].TrafficReplay)
	}
}

func TestSimulateTrafficReplayPropagatesRealFetchErrors(t *testing.T) {
	decisions := []RemediationDecision{realManifestDecision()}

	wantErr := errors.New("real database failure")
	fetch := func(clusterID, destinationIP string) ([]HistoricalFlowCost, error) {
		return nil, wantErr
	}

	_, err := SimulateTrafficReplay(decisions, DefaultRemediationPolicy(), fetch)
	if err == nil {
		t.Fatal("expected a real fetch failure to be surfaced as a real error, not swallowed")
	}
}

func TestSimulateTrafficReplayCanBeDisabledByPolicy(t *testing.T) {
	decisions := []RemediationDecision{realManifestDecision()}

	fetch := func(clusterID, destinationIP string) ([]HistoricalFlowCost, error) {
		return []HistoricalFlowCost{
			{SrcWorkload: "checkout/checkout-1", CostHighINR: 40},
			{SrcWorkload: "checkout/checkout-2", CostHighINR: 10},
		}, nil
	}

	policy := DefaultRemediationPolicy()
	policy.RequireSafeTrafficReplay = false

	out, err := SimulateTrafficReplay(decisions, policy, fetch)
	if err != nil {
		t.Fatalf("SimulateTrafficReplay: %v", err)
	}
	if out[0].TrafficReplay == nil || out[0].TrafficReplay.Safe {
		t.Fatalf("expected the real replay result still computed and attached even when not enforced, got: %+v", out[0].TrafficReplay)
	}
	if !out[0].WouldAutoApply {
		t.Fatal("expected WouldAutoApply to stay untouched when RequireSafeTrafficReplay is off")
	}
}
