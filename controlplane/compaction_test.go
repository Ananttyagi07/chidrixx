// SPDX-License-Identifier: Apache-2.0
package main

import (
	"testing"
	"time"
)

// insertRawFindingAt writes one flow_aggregate row with an explicit
// reported_at, bypassing Ingest (which always stamps time.Now()) -- the
// compaction tests need real, controllable ages, not "wait a second and a
// half" like the growth tests do.
func insertRawFindingAt(t *testing.T, s *Store, tenantID int64, clusterID string, reportedAt time.Time, f Finding) {
	t.Helper()
	_, err := s.db.Exec(`
		INSERT INTO flow_aggregate
			(tenant_id, cluster_id, reported_at, src_workload, dst_workload_or_endpoint,
			 path_class, confidence, bytes_tx, bytes_rx, cost_low_inr, cost_high_inr, fix_hint, fix_manifest,
			 cloud, region, savings_low_inr, savings_high_inr)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, tenantID, clusterID, reportedAt.Unix(), f.Source, f.Destination,
		f.PathClass, f.Confidence, f.BytesTx, f.BytesRx,
		f.CostLowINR, f.CostHighINR, f.FixHint, f.FixManifest,
		f.Cloud, f.Region, f.SavingsLowINR, f.SavingsHighINR,
	)
	if err != nil {
		t.Fatalf("insertRawFindingAt: %v", err)
	}
}

func countRows(t *testing.T, s *Store, table string) int {
	t.Helper()
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

func TestCompactFindingsOlderThanFoldsOldRowsIntoOneDailyRollup(t *testing.T) {
	s := testStore(t)
	tenantID := testTenant(t, s)

	day := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	// Two real raw rows for the same pair, same real day, different
	// times of day -- both must fold into a single summed rollup row.
	insertRawFindingAt(t, s, tenantID, "cluster-a", day.Add(2*time.Hour), Finding{
		Source: "checkout/checkout-1", Destination: "redis/redis-1", PathClass: "CROSS_AZ",
		BytesTx: 100, BytesRx: 50, CostLowINR: 10, CostHighINR: 20, SavingsHighINR: 5,
	})
	insertRawFindingAt(t, s, tenantID, "cluster-a", day.Add(10*time.Hour), Finding{
		Source: "checkout/checkout-1", Destination: "redis/redis-1", PathClass: "CROSS_AZ",
		BytesTx: 200, BytesRx: 150, CostLowINR: 15, CostHighINR: 25, SavingsHighINR: 7,
	})

	groups, deleted, err := s.CompactFindingsOlderThan(time.Now())
	if err != nil {
		t.Fatalf("CompactFindingsOlderThan: %v", err)
	}
	if groups != 1 {
		t.Fatalf("expected 1 real rollup group, got %d", groups)
	}
	if deleted != 2 {
		t.Fatalf("expected 2 raw rows deleted, got %d", deleted)
	}

	if n := countRows(t, s, "flow_aggregate"); n != 0 {
		t.Fatalf("expected raw table empty after compaction, got %d rows", n)
	}

	var bytesTx, bytesRx, sampleCount int64
	var costLow, costHigh, savingsHigh float64
	err = s.db.QueryRow(`
		SELECT bytes_tx, bytes_rx, cost_low_inr, cost_high_inr, savings_high_inr, sample_count
		FROM flow_aggregate_daily WHERE tenant_id = ? AND cluster_id = ?
	`, tenantID, "cluster-a").Scan(&bytesTx, &bytesRx, &costLow, &costHigh, &savingsHigh, &sampleCount)
	if err != nil {
		t.Fatalf("query rollup row: %v", err)
	}
	if bytesTx != 300 || bytesRx != 200 || costLow != 25 || costHigh != 45 || savingsHigh != 12 || sampleCount != 2 {
		t.Fatalf("unexpected rollup sums: bytesTx=%d bytesRx=%d costLow=%v costHigh=%v savingsHigh=%v sampleCount=%d",
			bytesTx, bytesRx, costLow, costHigh, savingsHigh, sampleCount)
	}
}

func TestCompactFindingsOlderThanLeavesRecentRowsAlone(t *testing.T) {
	s := testStore(t)
	tenantID := testTenant(t, s)

	old := time.Now().Add(-40 * 24 * time.Hour)
	insertRawFindingAt(t, s, tenantID, "cluster-a", old, Finding{
		Source: "old/old-1", Destination: "8.8.8.8", PathClass: "INTERNET_EGRESS", CostHighINR: 10,
	})
	if err := s.Ingest(tenantID, "cluster-a", []Finding{
		{Source: "fresh/fresh-1", Destination: "8.8.8.8", PathClass: "INTERNET_EGRESS", CostHighINR: 99},
	}); err != nil {
		t.Fatalf("Ingest: %v", err)
	}

	cutoff := time.Now().Add(-DefaultRawRetention)
	groups, deleted, err := s.CompactFindingsOlderThan(cutoff)
	if err != nil {
		t.Fatalf("CompactFindingsOlderThan: %v", err)
	}
	if groups != 1 || deleted != 1 {
		t.Fatalf("expected exactly the old row compacted, got groups=%d deleted=%d", groups, deleted)
	}

	remaining, err := s.LatestFindings(tenantID, 10)
	if err != nil {
		t.Fatalf("LatestFindings: %v", err)
	}
	if len(remaining) != 1 || remaining[0].Source != "fresh/fresh-1" {
		t.Fatalf("expected only the fresh row to remain in flow_aggregate, got %+v", remaining)
	}
}

func TestCompactFindingsOlderThanIsIdempotentOnRerun(t *testing.T) {
	s := testStore(t)
	tenantID := testTenant(t, s)

	old := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	insertRawFindingAt(t, s, tenantID, "cluster-a", old, Finding{
		Source: "a/a", Destination: "b/b", PathClass: "CROSS_AZ", CostHighINR: 10,
	})

	if _, _, err := s.CompactFindingsOlderThan(time.Now()); err != nil {
		t.Fatalf("first compaction: %v", err)
	}
	groups2, deleted2, err := s.CompactFindingsOlderThan(time.Now())
	if err != nil {
		t.Fatalf("second compaction: %v", err)
	}
	if groups2 != 0 || deleted2 != 0 {
		t.Fatalf("expected a re-run over already-compacted data to be a real no-op, got groups=%d deleted=%d", groups2, deleted2)
	}
	if n := countRows(t, s, "flow_aggregate_daily"); n != 1 {
		t.Fatalf("expected exactly 1 rollup row (not duplicated by the re-run), got %d", n)
	}
}

func TestCompactFindingsOlderThanKeepsSeparateDaysSeparate(t *testing.T) {
	s := testStore(t)
	tenantID := testTenant(t, s)

	day1 := time.Date(2026, 1, 1, 6, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 1, 2, 6, 0, 0, 0, time.UTC)
	insertRawFindingAt(t, s, tenantID, "cluster-a", day1, Finding{Source: "a/a", Destination: "b/b", PathClass: "CROSS_AZ", CostHighINR: 10})
	insertRawFindingAt(t, s, tenantID, "cluster-a", day2, Finding{Source: "a/a", Destination: "b/b", PathClass: "CROSS_AZ", CostHighINR: 20})

	groups, deleted, err := s.CompactFindingsOlderThan(time.Now())
	if err != nil {
		t.Fatalf("CompactFindingsOlderThan: %v", err)
	}
	if groups != 2 || deleted != 2 {
		t.Fatalf("expected 2 separate daily rollups (different real days), got groups=%d deleted=%d", groups, deleted)
	}
}

func TestCompactFindingsOlderThanIsolatesByTenant(t *testing.T) {
	s := testStore(t)
	tenantA := testTenant(t, s)
	tenantB := testTenant(t, s)

	old := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	insertRawFindingAt(t, s, tenantA, "cluster-a", old, Finding{Source: "a/a", Destination: "b/b", PathClass: "CROSS_AZ", CostHighINR: 10})
	insertRawFindingAt(t, s, tenantB, "cluster-a", old, Finding{Source: "a/a", Destination: "b/b", PathClass: "CROSS_AZ", CostHighINR: 999})

	if _, _, err := s.CompactFindingsOlderThan(time.Now()); err != nil {
		t.Fatalf("CompactFindingsOlderThan: %v", err)
	}

	var costA, costB float64
	if err := s.db.QueryRow(`SELECT cost_high_inr FROM flow_aggregate_daily WHERE tenant_id = ?`, tenantA).Scan(&costA); err != nil {
		t.Fatalf("query tenant a rollup: %v", err)
	}
	if err := s.db.QueryRow(`SELECT cost_high_inr FROM flow_aggregate_daily WHERE tenant_id = ?`, tenantB).Scan(&costB); err != nil {
		t.Fatalf("query tenant b rollup: %v", err)
	}
	if costA != 10 || costB != 999 {
		t.Fatalf("expected tenant-isolated rollups (10 and 999), got costA=%v costB=%v", costA, costB)
	}
}

// TestWorkloadCostGrowthSurvivesCompactionOfItsFirstSnapshot is the real
// integration test for why WorkloadCostGrowth reads flow_aggregate_daily
// too: without it, compacting a workload's oldest raw snapshot would
// silently make its "first appearance" drift forward to whatever raw data
// happens to remain, quietly changing the real delta being reported.
func TestWorkloadCostGrowthSurvivesCompactionOfItsFirstSnapshot(t *testing.T) {
	s := testStore(t)
	tenantID := testTenant(t, s)

	old := time.Now().Add(-40 * 24 * time.Hour)
	insertRawFindingAt(t, s, tenantID, "cluster-a", old, Finding{Source: "checkout/checkout-1", CostHighINR: 10})

	if err := s.Ingest(tenantID, "cluster-a", []Finding{
		{Source: "checkout/checkout-1", CostHighINR: 200},
	}); err != nil {
		t.Fatalf("Ingest: %v", err)
	}

	before, err := s.WorkloadCostGrowth(tenantID, 10)
	if err != nil {
		t.Fatalf("WorkloadCostGrowth before compaction: %v", err)
	}
	if len(before) != 1 || before[0].DeltaINR != 190 {
		t.Fatalf("expected a real 190 delta before compaction, got %+v", before)
	}

	cutoff := time.Now().Add(-DefaultRawRetention)
	if _, deleted, err := s.CompactFindingsOlderThan(cutoff); err != nil || deleted != 1 {
		t.Fatalf("CompactFindingsOlderThan: deleted=%d err=%v", deleted, err)
	}

	after, err := s.WorkloadCostGrowth(tenantID, 10)
	if err != nil {
		t.Fatalf("WorkloadCostGrowth after compaction: %v", err)
	}
	if len(after) != 1 {
		t.Fatalf("expected the workload to still show real growth after its first snapshot was compacted, got %+v", after)
	}
	if after[0].DeltaINR != 190 {
		t.Fatalf("expected the real delta to stay 190 after compaction (read from the daily rollup), got %v", after[0].DeltaINR)
	}
}

func TestCompactFindingsOlderThanIsANoOpWithNoOldData(t *testing.T) {
	s := testStore(t)
	tenantID := testTenant(t, s)

	if err := s.Ingest(tenantID, "cluster-a", []Finding{{Source: "a/a", CostHighINR: 10}}); err != nil {
		t.Fatalf("Ingest: %v", err)
	}

	groups, deleted, err := s.CompactFindingsOlderThan(time.Now().Add(-DefaultRawRetention))
	if err != nil {
		t.Fatalf("CompactFindingsOlderThan: %v", err)
	}
	if groups != 0 || deleted != 0 {
		t.Fatalf("expected no-op against fresh data within the retention window, got groups=%d deleted=%d", groups, deleted)
	}
	if n := countRows(t, s, "flow_aggregate"); n != 1 {
		t.Fatalf("expected the fresh row untouched, got %d rows", n)
	}
}
