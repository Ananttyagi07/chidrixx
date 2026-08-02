// SPDX-License-Identifier: Apache-2.0
package main

import (
	"testing"
	"time"
)

func TestOutcomeDatasetStatsWithNoDataIsHonestlyAllZero(t *testing.T) {
	s := testStore(t)
	tenantID := testTenant(t, s)

	stats, err := s.OutcomeDatasetStats(tenantID)
	if err != nil {
		t.Fatalf("OutcomeDatasetStats: %v", err)
	}
	if stats.TotalShown != 0 || stats.TotalApplied != 0 || stats.TotalMeasured != 0 {
		t.Fatalf("expected all-zero stats for a tenant with no outcomes, got %+v", stats)
	}
	if stats.MeanAbsPredictionErrorINR != nil {
		t.Fatalf("expected a nil mean prediction error with nothing measured, got %v", *stats.MeanAbsPredictionErrorINR)
	}
}

func TestOutcomeDatasetStatsCountsShownWithoutApplyingOrMeasuring(t *testing.T) {
	s := testStore(t)
	tenantID := testTenant(t, s)

	if err := s.Ingest(tenantID, "cluster-a", []Finding{
		{Source: "checkout/checkout-1", Destination: "redis/redis-master", PathClass: "cross_az", CostHighINR: 40, FixHint: "co-locate zones", SavingsHighINR: 40},
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

	stats, err := s.OutcomeDatasetStats(tenantID)
	if err != nil {
		t.Fatalf("OutcomeDatasetStats: %v", err)
	}
	if stats.TotalShown != 1 || stats.TotalApplied != 0 || stats.TotalMeasured != 0 {
		t.Fatalf("expected shown=1, applied=0, measured=0 (never marked applied), got %+v", stats)
	}
	if stats.MeanAbsPredictionErrorINR != nil {
		t.Fatalf("expected nil mean prediction error with nothing measured yet, got %v", *stats.MeanAbsPredictionErrorINR)
	}
}

func TestOutcomeDatasetStatsComputesRealMeanPredictionError(t *testing.T) {
	s := testStore(t)
	tenantID := testTenant(t, s)

	// Predicted savings 40, but the real observed drop is only 38 (40 -> 2)
	// -- a real, small prediction error of 2, not a perfect match.
	if err := s.Ingest(tenantID, "cluster-a", []Finding{
		{Source: "checkout/checkout-1", Destination: "redis/redis-master", PathClass: "cross_az", CostHighINR: 40, FixHint: "co-locate zones", SavingsHighINR: 40},
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
	if err := s.Ingest(tenantID, "cluster-a", []Finding{
		{Source: "checkout/checkout-1", Destination: "redis/redis-master", PathClass: "same_az", CostHighINR: 2},
	}); err != nil {
		t.Fatalf("Ingest (2): %v", err)
	}

	stats, err := s.OutcomeDatasetStats(tenantID)
	if err != nil {
		t.Fatalf("OutcomeDatasetStats: %v", err)
	}
	if stats.TotalShown != 1 || stats.TotalApplied != 1 || stats.TotalMeasured != 1 {
		t.Fatalf("expected shown=1, applied=1, measured=1, got %+v", stats)
	}
	if stats.MeanAbsPredictionErrorINR == nil {
		t.Fatal("expected a real mean prediction error now that one outcome is measured")
	}
	if *stats.MeanAbsPredictionErrorINR != 2 {
		t.Fatalf("expected mean prediction error 2 (predicted 40, real drop 38), got %v", *stats.MeanAbsPredictionErrorINR)
	}
}

func TestOutcomeDatasetStatsIsolatesByTenant(t *testing.T) {
	s := testStore(t)
	tenantA := testTenant(t, s)
	tenantB := testTenant(t, s)

	if err := s.Ingest(tenantA, "cluster-a", []Finding{
		{Source: "checkout/checkout-1", Destination: "redis/redis-master", PathClass: "cross_az", CostHighINR: 40, FixHint: "co-locate zones"},
	}); err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	fixes, err := s.LatestFindings(tenantA, 10)
	if err != nil {
		t.Fatalf("LatestFindings: %v", err)
	}
	if err := s.RecordRecommendationsShown(tenantA, fixes); err != nil {
		t.Fatalf("RecordRecommendationsShown: %v", err)
	}

	statsB, err := s.OutcomeDatasetStats(tenantB)
	if err != nil {
		t.Fatalf("OutcomeDatasetStats(tenantB): %v", err)
	}
	if statsB.TotalShown != 0 {
		t.Fatalf("tenant b's outcome stats leaked tenant a's data: %+v", statsB)
	}
}
