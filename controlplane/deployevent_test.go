// SPDX-License-Identifier: Apache-2.0
package main

import (
	"testing"
	"time"
)

func TestIngestAndQueryDeployEvents(t *testing.T) {
	s := testStore(t)
	tenantID := testTenant(t, s)

	now := time.Now().Truncate(time.Second)
	events := []DeployEvent{
		{Namespace: "checkout", Name: "checkout", Reason: "ReplicaCountChanged", Message: "replicas increased from 2 to 8", OccurredAt: now.Add(-10 * time.Minute)},
		{Namespace: "ai-gateway", Name: "ai-gateway", Reason: "ReplicaCountChanged", Message: "replicas increased from 1 to 5", OccurredAt: now.Add(-2 * time.Hour)},
	}

	if err := s.IngestDeployEvents(tenantID, "cluster-a", events); err != nil {
		t.Fatalf("IngestDeployEvents: %v", err)
	}

	// A 30-minute window should catch the checkout event but not the
	// 2-hour-old ai-gateway one.
	got, err := s.RecentDeployEvents(tenantID, "cluster-a", now.Add(-30*time.Minute), now)
	if err != nil {
		t.Fatalf("RecentDeployEvents: %v", err)
	}
	if len(got) != 1 || got[0].Namespace != "checkout" {
		t.Fatalf("expected only the checkout event within the 30min window, got: %+v", got)
	}

	// A wider window catches both, most recent first.
	got, err = s.RecentDeployEvents(tenantID, "cluster-a", now.Add(-3*time.Hour), now)
	if err != nil {
		t.Fatalf("RecentDeployEvents (wide window): %v", err)
	}
	if len(got) != 2 || got[0].Namespace != "checkout" || got[1].Namespace != "ai-gateway" {
		t.Fatalf("expected both events, most recent first, got: %+v", got)
	}
}

func TestRecentDeployEventsIsolatesByTenantAndCluster(t *testing.T) {
	s := testStore(t)
	tenantA := testTenant(t, s)
	tenantB := testTenant(t, s)

	now := time.Now()
	if err := s.IngestDeployEvents(tenantA, "shared-cluster-name", []DeployEvent{
		{Namespace: "checkout", Name: "checkout", Reason: "ReplicaCountChanged", OccurredAt: now},
	}); err != nil {
		t.Fatalf("ingest tenant a: %v", err)
	}
	if err := s.IngestDeployEvents(tenantB, "shared-cluster-name", []DeployEvent{
		{Namespace: "billing", Name: "billing", Reason: "ReplicaCountChanged", OccurredAt: now},
	}); err != nil {
		t.Fatalf("ingest tenant b: %v", err)
	}

	gotA, err := s.RecentDeployEvents(tenantA, "shared-cluster-name", now.Add(-time.Hour), now.Add(time.Hour))
	if err != nil {
		t.Fatalf("RecentDeployEvents(a): %v", err)
	}
	if len(gotA) != 1 || gotA[0].Namespace != "checkout" {
		t.Fatalf("tenant a's events leaked tenant b's, got: %+v", gotA)
	}
}
