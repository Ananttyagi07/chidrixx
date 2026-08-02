// SPDX-License-Identifier: Apache-2.0
package main

import (
	"fmt"
	"testing"
	"time"
)

func crossAZFinding(source, dest string, cost float64) FindingRow {
	return FindingRow{
		Finding: Finding{
			Source: source, Destination: dest,
			PathClass: "CROSS_AZ", CostHighINR: cost,
		},
	}
}

func TestOptimizePlacementSeparatesTwoDisconnectedPairs(t *testing.T) {
	// A-B (cost 10) and C-D (cost 10) never talk to each other -- a real
	// optimal 2-way split can eliminate ALL cross-group cost by grouping
	// each pair together (the algorithm is free to use fewer than
	// numGroups if that's optimal; there's no balance constraint, stated
	// honestly in placement.go's own comment).
	findings := []FindingRow{
		crossAZFinding("a", "b", 10),
		crossAZFinding("c", "d", 10),
	}

	result := OptimizePlacement(findings, 2)

	if result.ObservedCrossZoneINR != 20 {
		t.Fatalf("ObservedCrossZoneINR = %v, want 20", result.ObservedCrossZoneINR)
	}
	if result.OptimizedCrossZoneINR != 0 {
		t.Fatalf("expected a real optimal split to eliminate all cross-group cost for two disconnected pairs, got OptimizedCrossZoneINR = %v", result.OptimizedCrossZoneINR)
	}
	if result.PotentialSavingsINR != 20 {
		t.Fatalf("PotentialSavingsINR = %v, want 20", result.PotentialSavingsINR)
	}
	// A and B must end up in the same real group, and so must C and D --
	// not just "some 0 cost," the actual grouping must make graph sense.
	if result.Assignment["a"] != result.Assignment["b"] {
		t.Errorf("expected a and b in the same group, got %+v", result.Assignment)
	}
	if result.Assignment["c"] != result.Assignment["d"] {
		t.Errorf("expected c and d in the same group, got %+v", result.Assignment)
	}
}

func TestOptimizePlacementOnATriangleCannotFullySeparate(t *testing.T) {
	// A real triangle (every pair connected) can't be 2-colored without
	// at least one edge crossing -- pigeonhole: 3 nodes into 2 groups
	// means one group has 2, the lone third node's 2 edges must both
	// cross. This proves the algorithm respects real graph structure
	// rather than always reporting "fully optimized."
	findings := []FindingRow{
		crossAZFinding("a", "b", 10),
		crossAZFinding("b", "c", 10),
		crossAZFinding("a", "c", 10),
	}

	result := OptimizePlacement(findings, 2)

	if result.ObservedCrossZoneINR != 30 {
		t.Fatalf("ObservedCrossZoneINR = %v, want 30", result.ObservedCrossZoneINR)
	}
	if result.OptimizedCrossZoneINR != 20 {
		t.Fatalf("expected the real best-possible 2-way split of a triangle to leave exactly 2 edges (20) crossing, got %v", result.OptimizedCrossZoneINR)
	}
	if result.PotentialSavingsINR != 10 {
		t.Fatalf("PotentialSavingsINR = %v, want 10", result.PotentialSavingsINR)
	}
}

func TestOptimizePlacementWithOneGroupIsTriviallyZeroCost(t *testing.T) {
	findings := []FindingRow{crossAZFinding("a", "b", 10)}

	result := OptimizePlacement(findings, 1)

	if result.OptimizedCrossZoneINR != 0 {
		t.Fatalf("a single group can never have cross-group cost, got %v", result.OptimizedCrossZoneINR)
	}
	if result.PotentialSavingsINR != 10 {
		t.Fatalf("PotentialSavingsINR = %v, want 10", result.PotentialSavingsINR)
	}
}

func TestOptimizePlacementIgnoresNonCrossAZFindings(t *testing.T) {
	findings := []FindingRow{
		{Finding: Finding{Source: "a", Destination: "b", PathClass: "INTERNET_EGRESS", CostHighINR: 100}},
		{Finding: Finding{Source: "c", Destination: "d", PathClass: "SAME_NODE", CostHighINR: 50}},
	}

	result := OptimizePlacement(findings, 2)

	if result.Workloads != 0 || result.Edges != 0 {
		t.Fatalf("expected non-CROSS_AZ findings to contribute nothing to the placement graph, got %+v", result)
	}
	if result.ObservedCrossZoneINR != 0 {
		t.Fatalf("ObservedCrossZoneINR = %v, want 0", result.ObservedCrossZoneINR)
	}
}

func TestOptimizePlacementHandlesNoFindingsAtAll(t *testing.T) {
	result := OptimizePlacement(nil, 3)

	if result.Workloads != 0 || result.ObservedCrossZoneINR != 0 || result.PotentialSavingsINR != 0 {
		t.Fatalf("expected an honest all-zero result with no findings, got %+v", result)
	}
}

func TestOptimizePlacementExcludesSelfLoops(t *testing.T) {
	findings := []FindingRow{
		crossAZFinding("a", "a", 999), // same source and destination -- not a real edge
	}

	result := OptimizePlacement(findings, 2)

	if result.Workloads != 0 || result.Edges != 0 {
		t.Fatalf("expected a self-referencing finding to be excluded, got %+v", result)
	}
}

func TestOptimizePlacementAggregatesMultipleFindingsBetweenTheSamePair(t *testing.T) {
	findings := []FindingRow{
		crossAZFinding("a", "b", 10),
		crossAZFinding("b", "a", 5), // same real pair, reversed order -- must still aggregate into one edge
	}

	result := OptimizePlacement(findings, 2)

	if result.Edges != 1 {
		t.Fatalf("expected the two findings between a and b to aggregate into 1 real edge, got %d", result.Edges)
	}
	if result.ObservedCrossZoneINR != 15 {
		t.Fatalf("ObservedCrossZoneINR = %v, want 15 (10+5 aggregated)", result.ObservedCrossZoneINR)
	}
}

// TestOptimizePlacementWithFewerWorkloadsThanGroupsForcesEachAlone documents
// real, correct (not buggy) behavior found while validating the algorithm:
// numGroups means "how many real zones you need genuine redundant
// presence across" (e.g. for HA), not an arbitrary bucket count. With
// only 2 real workloads and a 3-zone redundancy requirement, each
// workload is forced into its own zone (every zone must stay used), so
// the one real edge between them is unavoidably cross-zone -- 0 real
// savings is the honest answer here, not a bug. Caught while manually
// tracing this exact scenario before shipping; kept as a permanent
// regression test so it can't silently start reporting a fabricated
// non-zero saving instead.
func TestOptimizePlacementWithFewerWorkloadsThanGroupsForcesEachAlone(t *testing.T) {
	findings := []FindingRow{crossAZFinding("checkout", "redis", 40)}

	result := OptimizePlacement(findings, 3)

	if result.PotentialSavingsINR != 0 {
		t.Fatalf("expected 0 real savings when 2 workloads must maintain presence across 3 real zones (each forced alone), got %+v", result)
	}
	if result.Assignment["checkout"] == result.Assignment["redis"] {
		t.Fatalf("expected checkout and redis in different zones under a 3-zone requirement, got: %+v", result.Assignment)
	}
}

// TestOptimizePlacementCoLocatesTheCostlierPairWhenNotAllCanBeSeparated
// is a real regression test for a genuine quality bug caught by hand
// while screenshotting the live feature, and for the wrong fix that was
// initially tried for it. With 4 real workloads (2 disconnected real
// pairs, costing ₹25 and ₹35) and a real 3-zone requirement, the true
// mathematical optimum is NOT 0: with only 4 workloads that must spread
// across 3 non-empty real zones, group sizes can only ever be {2,1,1},
// so at most one of the two real pairs can ever be co-located -- the
// other is structurally forced to keep crossing zones. The genuinely
// best real answer co-locates the ₹35 pair (the bigger one) and accepts
// the ₹25 pair's cost as unavoidable, i.e. ₹35 saved, ₹25 remaining --
// not ₹60 saved (impossible under the real constraint) and not "stuck at
// only ₹25 saved" either (a real quality bug this project's first
// attempt at a fix actually had: one single run's result depended on
// which pair the arbitrary initial split happened to favor, sometimes
// landing on saving only the smaller ₹25 pair instead of the bigger
// ₹35 one). Multiple deterministic restarts from varied initial splits
// -- keeping whichever converges to the lowest real cross-zone cost --
// is what reliably finds the genuinely best of the two valid outcomes.
func TestOptimizePlacementCoLocatesTheCostlierPairWhenNotAllCanBeSeparated(t *testing.T) {
	findings := []FindingRow{
		crossAZFinding("checkout-1", "payments-1", 25),
		crossAZFinding("analytics-1", "warehouse-1", 35),
	}

	result := OptimizePlacement(findings, 3)

	if result.ObservedCrossZoneINR != 60 {
		t.Fatalf("ObservedCrossZoneINR = %v, want 60", result.ObservedCrossZoneINR)
	}
	if result.OptimizedCrossZoneINR != 25 {
		t.Fatalf("expected the real best achievable (25, the smaller pair's cost, since only one pair can be co-located under a 3-zone requirement), got %v", result.OptimizedCrossZoneINR)
	}
	if result.PotentialSavingsINR != 35 {
		t.Fatalf("expected 35 real savings (the costlier pair co-located), not 60 (impossible) or less than 35 (a worse local optimum), got %v", result.PotentialSavingsINR)
	}
	if result.Assignment["analytics-1"] != result.Assignment["warehouse-1"] {
		t.Fatalf("expected the costlier real pair (analytics-1, warehouse-1) co-located, got: %+v", result.Assignment)
	}
}

// TestOptimizePlacementStaysFastAtRealisticProductionScale is a real
// performance regression test, following the hard lesson from §3.15's
// live performance investigation (a different feature shipped without
// one and measured ~1.6 real CPU-seconds per request against actual
// production data before being caught and fixed): 12 restarts x up to
// 500 iterations x O(workloads x groups) per iteration must still finish
// well within a real request's budget at a realistic real workload
// count, not just on toy 2-4 node fixtures.
func TestOptimizePlacementStaysFastAtRealisticProductionScale(t *testing.T) {
	findings := make([]FindingRow, 0, 100)
	for i := 0; i < 100; i++ {
		findings = append(findings, crossAZFinding(
			fmt.Sprintf("workload-%d", i), fmt.Sprintf("workload-%d", (i+1)%100), float64(i%10+1),
		))
	}

	start := time.Now()
	result := OptimizePlacement(findings, 3)
	elapsed := time.Since(start)

	if result.Workloads != 100 {
		t.Fatalf("expected 100 real workloads, got %d", result.Workloads)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("OptimizePlacement took %v against 100 real-scale workloads, want < 2s", elapsed)
	}
}

func TestOptimizePlacementDefaultsGroupCountToAtLeastOne(t *testing.T) {
	findings := []FindingRow{crossAZFinding("a", "b", 10)}

	result := OptimizePlacement(findings, 0)

	if result.Groups != 1 {
		t.Fatalf("expected an invalid group count to be floored at 1, got %d", result.Groups)
	}
}
