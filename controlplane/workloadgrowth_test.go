// SPDX-License-Identifier: Apache-2.0
package main

import (
	"testing"
	"time"
)

func TestWorkloadCostGrowthRanksByRealDelta(t *testing.T) {
	s := testStore(t)
	tenantID := testTenant(t, s)

	// ai-gateway: 10 -> 200, a real +190 delta.
	if err := s.Ingest(tenantID, "cluster-a", []Finding{
		{Source: "ai-gateway/ai-gw-1", CostHighINR: 10},
	}); err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	time.Sleep(1100 * time.Millisecond)
	if err := s.Ingest(tenantID, "cluster-a", []Finding{
		{Source: "ai-gateway/ai-gw-1", CostHighINR: 200},
	}); err != nil {
		t.Fatalf("Ingest: %v", err)
	}

	// checkout: 50 -> 60, a smaller +10 delta.
	if err := s.Ingest(tenantID, "cluster-a", []Finding{
		{Source: "checkout/checkout-1", CostHighINR: 50},
	}); err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	time.Sleep(1100 * time.Millisecond)
	if err := s.Ingest(tenantID, "cluster-a", []Finding{
		{Source: "checkout/checkout-1", CostHighINR: 60},
	}); err != nil {
		t.Fatalf("Ingest: %v", err)
	}

	growth, err := s.WorkloadCostGrowth(tenantID, 10)
	if err != nil {
		t.Fatalf("WorkloadCostGrowth: %v", err)
	}

	if len(growth) != 2 {
		t.Fatalf("expected 2 workloads with real growth, got %d: %+v", len(growth), growth)
	}
	if growth[0].Workload != "ai-gateway/ai-gw-1" || growth[0].DeltaINR != 190 {
		t.Errorf("expected ai-gateway ranked first with delta 190, got %+v", growth[0])
	}
	if growth[1].Workload != "checkout/checkout-1" || growth[1].DeltaINR != 10 {
		t.Errorf("expected checkout ranked second with delta 10, got %+v", growth[1])
	}
}

func TestWorkloadCostGrowthExcludesSingleSnapshotWorkloads(t *testing.T) {
	s := testStore(t)
	tenantID := testTenant(t, s)

	if err := s.Ingest(tenantID, "cluster-a", []Finding{
		{Source: "checkout/checkout-1", CostHighINR: 50},
	}); err != nil {
		t.Fatalf("Ingest: %v", err)
	}

	growth, err := s.WorkloadCostGrowth(tenantID, 10)
	if err != nil {
		t.Fatalf("WorkloadCostGrowth: %v", err)
	}
	if len(growth) != 0 {
		t.Fatalf("expected no entries for a workload seen only once, got: %+v", growth)
	}
}

func TestWorkloadCostGrowthRespectsTopN(t *testing.T) {
	s := testStore(t)
	tenantID := testTenant(t, s)

	for i, name := range []string{"a/a", "b/b", "c/c"} {
		if err := s.Ingest(tenantID, "cluster-a", []Finding{{Source: name, CostHighINR: float64(i + 1)}}); err != nil {
			t.Fatalf("Ingest: %v", err)
		}
		time.Sleep(1100 * time.Millisecond)
		if err := s.Ingest(tenantID, "cluster-a", []Finding{{Source: name, CostHighINR: float64((i + 1) * 10)}}); err != nil {
			t.Fatalf("Ingest: %v", err)
		}
	}

	growth, err := s.WorkloadCostGrowth(tenantID, 2)
	if err != nil {
		t.Fatalf("WorkloadCostGrowth: %v", err)
	}
	if len(growth) != 2 {
		t.Fatalf("expected topN=2 to cap results at 2, got %d", len(growth))
	}
}

func TestWorkloadCostGrowthIsolatesByTenant(t *testing.T) {
	s := testStore(t)
	tenantA := testTenant(t, s)
	tenantB := testTenant(t, s)

	if err := s.Ingest(tenantA, "cluster-a", []Finding{{Source: "checkout/checkout-1", CostHighINR: 10}}); err != nil {
		t.Fatalf("ingest a: %v", err)
	}
	time.Sleep(1100 * time.Millisecond)
	if err := s.Ingest(tenantA, "cluster-a", []Finding{{Source: "checkout/checkout-1", CostHighINR: 100}}); err != nil {
		t.Fatalf("ingest a: %v", err)
	}

	growthB, err := s.WorkloadCostGrowth(tenantB, 10)
	if err != nil {
		t.Fatalf("WorkloadCostGrowth(b): %v", err)
	}
	if len(growthB) != 0 {
		t.Fatalf("tenant b's growth query leaked tenant a's workload: %+v", growthB)
	}
}

func TestWorkloadCostGrowthCorrelatesRealDeployEventsInNamespace(t *testing.T) {
	s := testStore(t)
	tenantID := testTenant(t, s)

	if err := s.Ingest(tenantID, "cluster-a", []Finding{{Source: "ai-gateway/ai-gw-1", CostHighINR: 10}}); err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	time.Sleep(1100 * time.Millisecond)
	if err := s.Ingest(tenantID, "cluster-a", []Finding{{Source: "ai-gateway/ai-gw-1", CostHighINR: 200}}); err != nil {
		t.Fatalf("Ingest: %v", err)
	}

	// A real deploy event squarely inside the trend window.
	if err := s.IngestDeployEvents(tenantID, "cluster-a", []DeployEvent{
		{Namespace: "ai-gateway", Name: "ai-gateway", Reason: "ReplicaCountChanged",
			Message: "replicas increased from 2 to 20", OccurredAt: time.Now().Add(-500 * time.Millisecond)},
	}); err != nil {
		t.Fatalf("IngestDeployEvents: %v", err)
	}

	growth, err := s.WorkloadCostGrowth(tenantID, 10)
	if err != nil {
		t.Fatalf("WorkloadCostGrowth: %v", err)
	}
	if len(growth) != 1 {
		t.Fatalf("expected 1 workload, got %d", len(growth))
	}
	if len(growth[0].RelatedEvents) != 1 || growth[0].RelatedEvents[0].Namespace != "ai-gateway" {
		t.Fatalf("expected the real deploy event correlated by namespace, got: %+v", growth[0].RelatedEvents)
	}
}
