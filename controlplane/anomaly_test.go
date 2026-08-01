// SPDX-License-Identifier: Apache-2.0
package main

import (
	"testing"
	"time"
)

func TestDetectAnomaliesFlagsGrowthAboveThreshold(t *testing.T) {
	s := testStore(t)

	// cluster-a: 1 -> 5, a 5x jump, should be flagged.
	if err := s.Ingest("cluster-a", []Finding{{Source: "ns/a", CostHighINR: 1}}); err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	time.Sleep(1100 * time.Millisecond)
	if err := s.Ingest("cluster-a", []Finding{{Source: "ns/a", CostHighINR: 5}}); err != nil {
		t.Fatalf("Ingest: %v", err)
	}

	// cluster-b: 10 -> 12, only 1.2x, should not be flagged.
	if err := s.Ingest("cluster-b", []Finding{{Source: "ns/b", CostHighINR: 10}}); err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	time.Sleep(1100 * time.Millisecond)
	if err := s.Ingest("cluster-b", []Finding{{Source: "ns/b", CostHighINR: 12}}); err != nil {
		t.Fatalf("Ingest: %v", err)
	}

	anomalies, err := detectAnomalies(s)
	if err != nil {
		t.Fatalf("detectAnomalies: %v", err)
	}

	if len(anomalies) != 1 || anomalies[0].ClusterID != "cluster-a" {
		t.Fatalf("expected exactly one anomaly for cluster-a, got: %+v", anomalies)
	}

	if anomalies[0].PreviousCostINR != 1 || anomalies[0].CurrentCostINR != 5 || anomalies[0].GrowthRatio != 5 {
		t.Errorf("unexpected anomaly values: %+v", anomalies[0])
	}
}

func TestDetectAnomaliesSkipsClustersWithOnlyOneSnapshot(t *testing.T) {
	s := testStore(t)

	if err := s.Ingest("cluster-a", []Finding{{Source: "ns/a", CostHighINR: 100}}); err != nil {
		t.Fatalf("Ingest: %v", err)
	}

	anomalies, err := detectAnomalies(s)
	if err != nil {
		t.Fatalf("detectAnomalies: %v", err)
	}

	if len(anomalies) != 0 {
		t.Errorf("expected no anomalies with only one snapshot ever ingested, got: %+v", anomalies)
	}
}

func TestDetectAnomaliesSkipsZeroBaseline(t *testing.T) {
	s := testStore(t)

	if err := s.Ingest("cluster-a", []Finding{{Source: "ns/a", CostHighINR: 0}}); err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	time.Sleep(1100 * time.Millisecond)
	if err := s.Ingest("cluster-a", []Finding{{Source: "ns/a", CostHighINR: 50}}); err != nil {
		t.Fatalf("Ingest: %v", err)
	}

	anomalies, err := detectAnomalies(s)
	if err != nil {
		t.Fatalf("detectAnomalies: %v", err)
	}

	if len(anomalies) != 0 {
		t.Errorf("expected no anomalies from a zero-cost baseline (undefined ratio), got: %+v", anomalies)
	}
}
