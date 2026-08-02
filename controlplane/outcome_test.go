// SPDX-License-Identifier: Apache-2.0
package main

import (
	"errors"
	"testing"
	"time"
)

func testFindingRow(clusterID, source, destination, pathClass, fixHint string, costHigh, savingsLow, savingsHigh float64) FindingRow {
	return FindingRow{
		Finding: Finding{
			Source:         source,
			Destination:    destination,
			PathClass:      pathClass,
			CostHighINR:    costHigh,
			FixHint:        fixHint,
			SavingsLowINR:  savingsLow,
			SavingsHighINR: savingsHigh,
		},
		ClusterID: clusterID,
	}
}

func TestRecordRecommendationsShownThenList(t *testing.T) {
	s := testStore(t)
	tenantID := testTenant(t, s)

	fix := testFindingRow("cluster-a", "checkout/checkout-1", "redis/redis-master", "cross_az", "co-locate zones", 40, 30, 40)
	if err := s.RecordRecommendationsShown(tenantID, []FindingRow{fix}); err != nil {
		t.Fatalf("RecordRecommendationsShown: %v", err)
	}

	outcomes, err := s.ListRecommendationOutcomes(tenantID)
	if err != nil {
		t.Fatalf("ListRecommendationOutcomes: %v", err)
	}
	if len(outcomes) != 1 {
		t.Fatalf("expected 1 outcome, got %d: %+v", len(outcomes), outcomes)
	}
	o := outcomes[0]
	if o.ClusterID != "cluster-a" || o.Source != "checkout/checkout-1" || o.Destination != "redis/redis-master" {
		t.Fatalf("unexpected identity: %+v", o)
	}
	if o.CostBeforeINR != 40 || o.PredictedSavingsLowINR != 30 || o.PredictedSavingsHighINR != 40 {
		t.Fatalf("unexpected predicted/before values: %+v", o)
	}
	if o.AppliedAt != nil || o.CostAfterINR != nil {
		t.Fatalf("expected an unapplied, unmeasured outcome, got: %+v", o)
	}
}

func TestRecordRecommendationsShownSkipsFindingsWithNoFixHint(t *testing.T) {
	s := testStore(t)
	tenantID := testTenant(t, s)

	noFix := testFindingRow("cluster-a", "checkout/checkout-1", "redis/redis-master", "cross_az", "", 40, 0, 0)
	if err := s.RecordRecommendationsShown(tenantID, []FindingRow{noFix}); err != nil {
		t.Fatalf("RecordRecommendationsShown: %v", err)
	}

	outcomes, err := s.ListRecommendationOutcomes(tenantID)
	if err != nil {
		t.Fatalf("ListRecommendationOutcomes: %v", err)
	}
	if len(outcomes) != 0 {
		t.Fatalf("expected no outcome logged for a finding with no fix, got: %+v", outcomes)
	}
}

func TestRecordRecommendationsShownFreezesBaselineOnceApplied(t *testing.T) {
	s := testStore(t)
	tenantID := testTenant(t, s)

	fix := testFindingRow("cluster-a", "checkout/checkout-1", "redis/redis-master", "cross_az", "co-locate zones", 40, 30, 40)
	if err := s.RecordRecommendationsShown(tenantID, []FindingRow{fix}); err != nil {
		t.Fatalf("RecordRecommendationsShown (1): %v", err)
	}
	if err := s.MarkRecommendationApplied(tenantID, "cluster-a", "checkout/checkout-1", "redis/redis-master", "cross_az"); err != nil {
		t.Fatalf("MarkRecommendationApplied: %v", err)
	}

	// The same finding shows up again on a later dashboard load with a
	// different (still-real, not-yet-fixed) cost -- the pre-fix baseline
	// must not move once the fix has been marked applied.
	stillShowing := testFindingRow("cluster-a", "checkout/checkout-1", "redis/redis-master", "cross_az", "co-locate zones", 55, 45, 55)
	if err := s.RecordRecommendationsShown(tenantID, []FindingRow{stillShowing}); err != nil {
		t.Fatalf("RecordRecommendationsShown (2): %v", err)
	}

	outcomes, err := s.ListRecommendationOutcomes(tenantID)
	if err != nil {
		t.Fatalf("ListRecommendationOutcomes: %v", err)
	}
	if len(outcomes) != 1 {
		t.Fatalf("expected 1 outcome, got %d", len(outcomes))
	}
	if outcomes[0].CostBeforeINR != 40 {
		t.Fatalf("expected cost_before_inr frozen at 40, got %v", outcomes[0].CostBeforeINR)
	}
	if outcomes[0].PredictedSavingsHighINR != 40 {
		t.Fatalf("expected predicted_savings_high_inr frozen at 40, got %v", outcomes[0].PredictedSavingsHighINR)
	}
}

func TestMarkRecommendationAppliedIsIdempotent(t *testing.T) {
	s := testStore(t)
	tenantID := testTenant(t, s)

	fix := testFindingRow("cluster-a", "checkout/checkout-1", "redis/redis-master", "cross_az", "co-locate zones", 40, 30, 40)
	if err := s.RecordRecommendationsShown(tenantID, []FindingRow{fix}); err != nil {
		t.Fatalf("RecordRecommendationsShown: %v", err)
	}
	if err := s.MarkRecommendationApplied(tenantID, "cluster-a", "checkout/checkout-1", "redis/redis-master", "cross_az"); err != nil {
		t.Fatalf("MarkRecommendationApplied (1): %v", err)
	}
	firstApplied := mustOutcome(t, s, tenantID).AppliedAt

	time.Sleep(1100 * time.Millisecond)
	if err := s.MarkRecommendationApplied(tenantID, "cluster-a", "checkout/checkout-1", "redis/redis-master", "cross_az"); err != nil {
		t.Fatalf("MarkRecommendationApplied (2, should be a no-op not an error): %v", err)
	}
	secondApplied := mustOutcome(t, s, tenantID).AppliedAt

	if firstApplied == nil || secondApplied == nil || !firstApplied.Equal(*secondApplied) {
		t.Fatalf("expected applied_at to stay fixed at the first call, got %v then %v", firstApplied, secondApplied)
	}
}

func TestMarkRecommendationAppliedReturnsErrOutcomeNotFoundForUnknownIdentity(t *testing.T) {
	s := testStore(t)
	tenantID := testTenant(t, s)

	err := s.MarkRecommendationApplied(tenantID, "cluster-a", "nobody/here", "nowhere/there", "cross_az")
	if !errors.Is(err, ErrOutcomeNotFound) {
		t.Fatalf("expected ErrOutcomeNotFound, got: %v", err)
	}
}

func TestListRecommendationOutcomesMeasuresRealCostAfterFromFreshSnapshot(t *testing.T) {
	s := testStore(t)
	tenantID := testTenant(t, s)

	if err := s.Ingest(tenantID, "cluster-a", []Finding{
		{Source: "checkout/checkout-1", Destination: "redis/redis-master", PathClass: "cross_az", CostHighINR: 40, FixHint: "co-locate zones", SavingsLowINR: 30, SavingsHighINR: 40},
	}); err != nil {
		t.Fatalf("Ingest (1): %v", err)
	}
	fixes, err := s.LatestFindings(tenantID, 10)
	if err != nil {
		t.Fatalf("LatestFindings: %v", err)
	}
	if err := s.RecordRecommendationsShown(tenantID, fixes); err != nil {
		t.Fatalf("RecordRecommendationsShown: %v", err)
	}
	if err := s.MarkRecommendationApplied(tenantID, "cluster-a", "checkout/checkout-1", "redis/redis-master", "cross_az"); err != nil {
		t.Fatalf("MarkRecommendationApplied: %v", err)
	}

	time.Sleep(1100 * time.Millisecond)
	// The operator's fix changed the path class (same_az now, not
	// cross_az) and lowered the real cost -- a working fix, observed.
	if err := s.Ingest(tenantID, "cluster-a", []Finding{
		{Source: "checkout/checkout-1", Destination: "redis/redis-master", PathClass: "same_az", CostHighINR: 2},
	}); err != nil {
		t.Fatalf("Ingest (2): %v", err)
	}

	o := mustOutcome(t, s, tenantID)
	if o.CostAfterINR == nil {
		t.Fatal("expected cost_after_inr to be measured now that fresher data exists")
	}
	if *o.CostAfterINR != 2 {
		t.Fatalf("expected the real post-fix cost (2), got %v", *o.CostAfterINR)
	}
	if o.MeasuredAt == nil {
		t.Fatal("expected measured_at to be set")
	}
}

func TestListRecommendationOutcomesMeasuresZeroWhenFlowIsGone(t *testing.T) {
	s := testStore(t)
	tenantID := testTenant(t, s)

	if err := s.Ingest(tenantID, "cluster-a", []Finding{
		{Source: "checkout/checkout-1", Destination: "redis/redis-master", PathClass: "cross_az", CostHighINR: 40, FixHint: "co-locate zones"},
	}); err != nil {
		t.Fatalf("Ingest (1): %v", err)
	}
	fixes, err := s.LatestFindings(tenantID, 10)
	if err != nil {
		t.Fatalf("LatestFindings: %v", err)
	}
	if err := s.RecordRecommendationsShown(tenantID, fixes); err != nil {
		t.Fatalf("RecordRecommendationsShown: %v", err)
	}
	if err := s.MarkRecommendationApplied(tenantID, "cluster-a", "checkout/checkout-1", "redis/redis-master", "cross_az"); err != nil {
		t.Fatalf("MarkRecommendationApplied: %v", err)
	}

	time.Sleep(1100 * time.Millisecond)
	// The fix eliminated the flow entirely -- a fresh snapshot with no
	// checkout->redis traffic at all. Real success, not missing data.
	if err := s.Ingest(tenantID, "cluster-a", []Finding{
		{Source: "unrelated/other", Destination: "other/dest", PathClass: "same_node", CostHighINR: 1},
	}); err != nil {
		t.Fatalf("Ingest (2): %v", err)
	}

	o := mustOutcome(t, s, tenantID)
	if o.CostAfterINR == nil {
		t.Fatal("expected cost_after_inr to be measured (as zero) now that fresher data exists")
	}
	if *o.CostAfterINR != 0 {
		t.Fatalf("expected 0 (flow gone = fixed), got %v", *o.CostAfterINR)
	}
}

func TestListRecommendationOutcomesLeavesUnmeasuredWithoutFresherData(t *testing.T) {
	s := testStore(t)
	tenantID := testTenant(t, s)

	if err := s.Ingest(tenantID, "cluster-a", []Finding{
		{Source: "checkout/checkout-1", Destination: "redis/redis-master", PathClass: "cross_az", CostHighINR: 40, FixHint: "co-locate zones"},
	}); err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	fixes, err := s.LatestFindings(tenantID, 10)
	if err != nil {
		t.Fatalf("LatestFindings: %v", err)
	}
	if err := s.RecordRecommendationsShown(tenantID, fixes); err != nil {
		t.Fatalf("RecordRecommendationsShown: %v", err)
	}
	if err := s.MarkRecommendationApplied(tenantID, "cluster-a", "checkout/checkout-1", "redis/redis-master", "cross_az"); err != nil {
		t.Fatalf("MarkRecommendationApplied: %v", err)
	}

	// No new ingest happened after applying -- there's no fresher data to
	// measure against yet, so this must stay honestly unmeasured rather
	// than guessing.
	o := mustOutcome(t, s, tenantID)
	if o.CostAfterINR != nil {
		t.Fatalf("expected cost_after_inr to stay nil with no fresher snapshot, got %v", *o.CostAfterINR)
	}
}

func TestRecommendationOutcomesAreIsolatedByTenant(t *testing.T) {
	s := testStore(t)
	tenantA := testTenant(t, s)
	tenantB := testTenant(t, s)

	fixA := testFindingRow("cluster-a", "checkout/checkout-1", "redis/redis-master", "cross_az", "co-locate zones", 40, 30, 40)
	if err := s.RecordRecommendationsShown(tenantA, []FindingRow{fixA}); err != nil {
		t.Fatalf("RecordRecommendationsShown(a): %v", err)
	}

	outcomesB, err := s.ListRecommendationOutcomes(tenantB)
	if err != nil {
		t.Fatalf("ListRecommendationOutcomes(b): %v", err)
	}
	if len(outcomesB) != 0 {
		t.Fatalf("tenant b saw tenant a's recommendation outcome: %+v", outcomesB)
	}

	// A deliberately identical (cluster/source/destination/path_class)
	// identity in tenant b must not collide with tenant a's row -- the
	// unique index is scoped by tenant_id, this proves it.
	if err := s.RecordRecommendationsShown(tenantB, []FindingRow{fixA}); err != nil {
		t.Fatalf("RecordRecommendationsShown(b): %v", err)
	}
	if err := s.MarkRecommendationApplied(tenantB, "cluster-a", "checkout/checkout-1", "redis/redis-master", "cross_az"); err != nil {
		t.Fatalf("MarkRecommendationApplied(b): %v", err)
	}

	outcomesA, err := s.ListRecommendationOutcomes(tenantA)
	if err != nil {
		t.Fatalf("ListRecommendationOutcomes(a): %v", err)
	}
	if len(outcomesA) != 1 || outcomesA[0].AppliedAt != nil {
		t.Fatalf("tenant a's outcome was affected by tenant b's MarkRecommendationApplied: %+v", outcomesA)
	}
}

func mustOutcome(t *testing.T, s *Store, tenantID int64) RecommendationOutcome {
	t.Helper()
	outcomes, err := s.ListRecommendationOutcomes(tenantID)
	if err != nil {
		t.Fatalf("ListRecommendationOutcomes: %v", err)
	}
	if len(outcomes) != 1 {
		t.Fatalf("expected exactly 1 outcome, got %d: %+v", len(outcomes), outcomes)
	}
	return outcomes[0]
}
