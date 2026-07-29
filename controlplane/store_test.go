package main

import (
	"testing"
	"time"
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
