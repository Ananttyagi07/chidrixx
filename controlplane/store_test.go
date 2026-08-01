// SPDX-License-Identifier: Apache-2.0
package main

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func testStore(t *testing.T) *Store {
	t.Helper()

	s, err := OpenStore(":memory:")
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	return s
}

func TestIngestAndClusters(t *testing.T) {
	s := testStore(t)

	err := s.Ingest("cluster-a", []Finding{
		{Source: "ns/app", Destination: "8.8.8.8", PathClass: "INTERNET_EGRESS", Confidence: "high", BytesTx: 100, CostLowINR: 1, CostHighINR: 2},
		{Source: "ns/db", Destination: "ns/app", PathClass: "SAME_NODE", Confidence: "high", BytesTx: 50},
	})
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}

	clusters, err := s.Clusters()
	if err != nil {
		t.Fatalf("Clusters: %v", err)
	}
	if len(clusters) != 1 {
		t.Fatalf("expected 1 cluster, got %d", len(clusters))
	}
	if clusters[0].ClusterID != "cluster-a" || clusters[0].FindingCount != 2 {
		t.Fatalf("unexpected cluster summary: %+v", clusters[0])
	}
	if clusters[0].CostHighINR != 2 {
		t.Fatalf("expected total high cost 2, got %v", clusters[0].CostHighINR)
	}
}

func TestFixManifestRoundTrips(t *testing.T) {
	s := testStore(t)

	manifest := "apiVersion: networking.k8s.io/v1\nkind: NetworkPolicy\n"

	if err := s.Ingest("cluster-a", []Finding{
		{Source: "ns/app", Destination: "8.8.8.8", PathClass: "INTERNET_EGRESS", Confidence: "high",
			FixHint: "confirm this needs to leave", FixManifest: manifest},
	}); err != nil {
		t.Fatalf("Ingest: %v", err)
	}

	findings, err := s.LatestFindings(10)
	if err != nil {
		t.Fatalf("LatestFindings: %v", err)
	}

	if len(findings) != 1 || findings[0].FixManifest != manifest {
		t.Fatalf("expected FixManifest to round-trip through ingest+read, got: %+v", findings)
	}
}

// TestOpenStoreMigratesExistingDatabaseWithoutFixManifestColumn guards the
// real gap that would otherwise bite anyone with a control plane already
// deployed before this column existed: CREATE TABLE IF NOT EXISTS is a
// no-op against a database that already has the table, so a fresh
// in-memory test can't catch a broken migration -- it needs a database
// file that was genuinely created with the old schema.
func TestOpenStoreMigratesExistingDatabaseWithoutFixManifestColumn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")

	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open pre-migration db: %v", err)
	}

	const oldSchema = `
CREATE TABLE flow_aggregate (
	id                       INTEGER PRIMARY KEY AUTOINCREMENT,
	cluster_id               TEXT NOT NULL,
	reported_at              INTEGER NOT NULL,
	src_workload             TEXT NOT NULL,
	dst_workload_or_endpoint TEXT NOT NULL,
	path_class               TEXT NOT NULL,
	confidence               TEXT NOT NULL,
	bytes_tx                 INTEGER NOT NULL,
	bytes_rx                 INTEGER NOT NULL,
	cost_low_inr             REAL NOT NULL,
	cost_high_inr            REAL NOT NULL,
	fix_hint                 TEXT
);`
	if _, err := db.Exec(oldSchema); err != nil {
		t.Fatalf("create old schema: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close pre-migration db: %v", err)
	}

	s, err := OpenStore(path)
	if err != nil {
		t.Fatalf("OpenStore against pre-existing old-schema database: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	if err := s.Ingest("cluster-a", []Finding{
		{Source: "ns/app", Destination: "8.8.8.8", PathClass: "INTERNET_EGRESS", Confidence: "high", FixManifest: "some-manifest"},
	}); err != nil {
		t.Fatalf("Ingest against migrated database: %v", err)
	}

	findings, err := s.LatestFindings(10)
	if err != nil {
		t.Fatalf("LatestFindings against migrated database: %v", err)
	}

	if len(findings) != 1 || findings[0].FixManifest != "some-manifest" {
		t.Fatalf("expected fix_manifest column to work after migration, got: %+v", findings)
	}
}

func TestLatestFindingsOnlyUsesMostRecentSnapshotPerCluster(t *testing.T) {
	s := testStore(t)

	// First (stale) snapshot for cluster-a.
	if err := s.Ingest("cluster-a", []Finding{
		{Source: "ns/old", Destination: "1.1.1.1", PathClass: "INTERNET_EGRESS", Confidence: "high", CostHighINR: 100},
	}); err != nil {
		t.Fatalf("Ingest (stale): %v", err)
	}

	time.Sleep(1100 * time.Millisecond) // reported_at has second granularity

	// Second (current) snapshot for cluster-a — should replace the stale one
	// in "current state" queries, not add to it.
	if err := s.Ingest("cluster-a", []Finding{
		{Source: "ns/current", Destination: "2.2.2.2", PathClass: "INTERNET_EGRESS", Confidence: "high", CostHighINR: 5},
	}); err != nil {
		t.Fatalf("Ingest (current): %v", err)
	}

	// A second cluster's snapshot should also appear.
	if err := s.Ingest("cluster-b", []Finding{
		{Source: "ns/other", Destination: "3.3.3.3", PathClass: "CROSS_AZ", Confidence: "med", CostHighINR: 3},
	}); err != nil {
		t.Fatalf("Ingest (cluster-b): %v", err)
	}

	findings, err := s.LatestFindings(100)
	if err != nil {
		t.Fatalf("LatestFindings: %v", err)
	}

	if len(findings) != 2 {
		t.Fatalf("expected 2 findings (latest per cluster), got %d: %+v", len(findings), findings)
	}

	for _, f := range findings {
		if f.Source == "ns/old" {
			t.Fatalf("stale snapshot leaked into LatestFindings: %+v", f)
		}
	}

	// Ranked by cost descending: ns/current (5) before cluster-b's finding (3).
	if findings[0].Source != "ns/current" || findings[1].Source != "ns/other" {
		t.Fatalf("unexpected ranking: %+v", findings)
	}
}

func TestSummaryAggregatesLatestSnapshotsOnly(t *testing.T) {
	s := testStore(t)

	// Stale snapshot for cluster-a — must not be double-counted once a
	// newer one exists.
	if err := s.Ingest("cluster-a", []Finding{
		{Source: "ns/old", Destination: "1.1.1.1", PathClass: "INTERNET_EGRESS", BytesTx: 999, CostHighINR: 999},
	}); err != nil {
		t.Fatalf("Ingest (stale): %v", err)
	}
	time.Sleep(1100 * time.Millisecond)

	if err := s.Ingest("cluster-a", []Finding{
		{Source: "ns/app", Destination: "8.8.8.8", PathClass: "INTERNET_EGRESS", BytesTx: 100, BytesRx: 50, CostLowINR: 1, CostHighINR: 2},
		{Source: "ns/db", Destination: "ns/app", PathClass: "SAME_NODE", BytesTx: 10, BytesRx: 10, CostLowINR: 0, CostHighINR: 0},
	}); err != nil {
		t.Fatalf("Ingest (cluster-a current): %v", err)
	}

	if err := s.Ingest("cluster-b", []Finding{
		{Source: "ns/other", Destination: "3.3.3.3", PathClass: "CROSS_AZ", BytesTx: 20, BytesRx: 5, CostLowINR: 2, CostHighINR: 3},
	}); err != nil {
		t.Fatalf("Ingest (cluster-b): %v", err)
	}

	sum, err := s.Summary()
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}

	if sum.ClusterCount != 2 {
		t.Errorf("ClusterCount = %d, want 2", sum.ClusterCount)
	}
	if sum.WorkloadCount != 3 {
		t.Errorf("WorkloadCount = %d, want 3 (ns/app, ns/db, ns/other)", sum.WorkloadCount)
	}
	if sum.FindingCount != 3 {
		t.Errorf("FindingCount = %d, want 3", sum.FindingCount)
	}
	if sum.TotalBytesTx != 130 {
		t.Errorf("TotalBytesTx = %d, want 130 (100+10+20, excluding the stale 999)", sum.TotalBytesTx)
	}
	if sum.TotalCostHighINR != 5 {
		t.Errorf("TotalCostHighINR = %v, want 5 (2+0+3, excluding the stale 999)", sum.TotalCostHighINR)
	}
}

func TestSpendByClassGroupsAndRanksByCost(t *testing.T) {
	s := testStore(t)

	if err := s.Ingest("cluster-a", []Finding{
		{Source: "ns/a", Destination: "1.1.1.1", PathClass: "INTERNET_EGRESS", CostHighINR: 10},
		{Source: "ns/b", Destination: "2.2.2.2", PathClass: "INTERNET_EGRESS", CostHighINR: 5},
		{Source: "ns/c", Destination: "ns/d", PathClass: "CROSS_AZ", CostHighINR: 20},
	}); err != nil {
		t.Fatalf("Ingest: %v", err)
	}

	classes, err := s.SpendByClass()
	if err != nil {
		t.Fatalf("SpendByClass: %v", err)
	}

	if len(classes) != 2 {
		t.Fatalf("expected 2 classes, got %d: %+v", len(classes), classes)
	}
	if classes[0].PathClass != "CROSS_AZ" || classes[0].CostHighINR != 20 {
		t.Errorf("expected CROSS_AZ (20) ranked first, got %+v", classes[0])
	}
	if classes[1].PathClass != "INTERNET_EGRESS" || classes[1].CostHighINR != 15 || classes[1].FindingCount != 2 {
		t.Errorf("expected INTERNET_EGRESS (15, 2 findings) ranked second, got %+v", classes[1])
	}
}

func TestGlobalTrendSumsAcrossClustersOldestFirst(t *testing.T) {
	s := testStore(t)

	if err := s.Ingest("cluster-a", []Finding{{Source: "ns/a", CostHighINR: 1}}); err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if err := s.Ingest("cluster-b", []Finding{{Source: "ns/b", CostHighINR: 2}}); err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	time.Sleep(1100 * time.Millisecond)
	if err := s.Ingest("cluster-a", []Finding{{Source: "ns/a", CostHighINR: 3}}); err != nil {
		t.Fatalf("Ingest: %v", err)
	}

	trend, err := s.GlobalTrend(10)
	if err != nil {
		t.Fatalf("GlobalTrend: %v", err)
	}

	if len(trend) != 2 {
		t.Fatalf("expected 2 distinct reported_at points, got %d: %+v", len(trend), trend)
	}
	if !trend[0].ReportedAt.Before(trend[1].ReportedAt) {
		t.Fatalf("expected oldest-first ordering, got %+v", trend)
	}
	if trend[0].CostHigh != 3 { // cluster-a(1) + cluster-b(2) at the first timestamp
		t.Errorf("first point CostHigh = %v, want 3", trend[0].CostHigh)
	}
	if trend[1].CostHigh != 3 { // cluster-a(3) alone at the second timestamp
		t.Errorf("second point CostHigh = %v, want 3", trend[1].CostHigh)
	}
}

func TestCostTrendOrdersOldestFirst(t *testing.T) {
	s := testStore(t)

	for i := 0; i < 3; i++ {
		if err := s.Ingest("cluster-a", []Finding{
			{Source: "ns/app", Destination: "8.8.8.8", PathClass: "INTERNET_EGRESS", CostHighINR: float64(i + 1)},
		}); err != nil {
			t.Fatalf("Ingest #%d: %v", i, err)
		}
		time.Sleep(1100 * time.Millisecond)
	}

	trend, err := s.CostTrend("cluster-a", 10)
	if err != nil {
		t.Fatalf("CostTrend: %v", err)
	}

	if len(trend) != 3 {
		t.Fatalf("expected 3 trend points, got %d", len(trend))
	}

	for i := 0; i < len(trend)-1; i++ {
		if !trend[i].ReportedAt.Before(trend[i+1].ReportedAt) {
			t.Fatalf("trend not oldest-first: %+v", trend)
		}
	}

	if trend[0].CostHigh != 1 || trend[2].CostHigh != 3 {
		t.Fatalf("unexpected trend values: %+v", trend)
	}
}

func TestBudgetUnsetThenSetAndOverwrite(t *testing.T) {
	s := testStore(t)

	_, isSet, err := s.GetBudget()
	if err != nil {
		t.Fatalf("GetBudget (unset): %v", err)
	}
	if isSet {
		t.Fatalf("expected no budget set initially")
	}

	if err := s.SetBudget(1000); err != nil {
		t.Fatalf("SetBudget: %v", err)
	}

	amount, isSet, err := s.GetBudget()
	if err != nil {
		t.Fatalf("GetBudget: %v", err)
	}
	if !isSet || amount != 1000 {
		t.Fatalf("expected (1000, true), got (%v, %v)", amount, isSet)
	}

	// Setting again overwrites rather than erroring or duplicating.
	if err := s.SetBudget(2500); err != nil {
		t.Fatalf("SetBudget (overwrite): %v", err)
	}

	amount, isSet, err = s.GetBudget()
	if err != nil {
		t.Fatalf("GetBudget (after overwrite): %v", err)
	}
	if !isSet || amount != 2500 {
		t.Fatalf("expected (2500, true) after overwrite, got (%v, %v)", amount, isSet)
	}
}
