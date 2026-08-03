// SPDX-License-Identifier: Apache-2.0
package main

import (
	"testing"
	"time"
)

func ingestGrowthAnomaly(t *testing.T, s *Store, tenantID int64, clusterID string, before, after float64) {
	t.Helper()
	if err := s.Ingest(tenantID, clusterID, []Finding{{Source: "ns/a", CostHighINR: before}}); err != nil {
		t.Fatalf("Ingest (before): %v", err)
	}
	time.Sleep(1100 * time.Millisecond)
	if err := s.Ingest(tenantID, clusterID, []Finding{{Source: "ns/a", CostHighINR: after}}); err != nil {
		t.Fatalf("Ingest (after): %v", err)
	}
}

func TestWatchTenantForNewAnomaliesRecordsARealNewAlert(t *testing.T) {
	s := testStore(t)
	tenantID := testTenant(t, s)
	ingestGrowthAnomaly(t, s, tenantID, "cluster-a", 1, 5)

	n, err := s.WatchTenantForNewAnomalies(tenantID)
	if err != nil {
		t.Fatalf("WatchTenantForNewAnomalies: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 real new alert, got %d", n)
	}

	alerts, err := s.UnacknowledgedAnomalyAlerts(tenantID)
	if err != nil {
		t.Fatalf("UnacknowledgedAnomalyAlerts: %v", err)
	}
	if len(alerts) != 1 || alerts[0].ClusterID != "cluster-a" {
		t.Fatalf("expected 1 real unacknowledged alert for cluster-a, got %+v", alerts)
	}
	if alerts[0].PreviousCostINR != 1 || alerts[0].CurrentCostINR != 5 || alerts[0].GrowthRatio != 5 {
		t.Fatalf("unexpected alert values: %+v", alerts[0])
	}
}

func TestWatchTenantForNewAnomaliesIsIdempotentOnRepeatedTicks(t *testing.T) {
	s := testStore(t)
	tenantID := testTenant(t, s)
	ingestGrowthAnomaly(t, s, tenantID, "cluster-a", 1, 5)

	if _, err := s.WatchTenantForNewAnomalies(tenantID); err != nil {
		t.Fatalf("first WatchTenantForNewAnomalies: %v", err)
	}
	// A second tick against the exact same unchanged snapshot (no new
	// ingest happened) must not re-alert -- this is the real point of
	// keying on snapshot_reported_at, not wall-clock time.
	n, err := s.WatchTenantForNewAnomalies(tenantID)
	if err != nil {
		t.Fatalf("second WatchTenantForNewAnomalies: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 new alerts on a repeated tick with no new data, got %d", n)
	}

	alerts, err := s.UnacknowledgedAnomalyAlerts(tenantID)
	if err != nil {
		t.Fatalf("UnacknowledgedAnomalyAlerts: %v", err)
	}
	if len(alerts) != 1 {
		t.Fatalf("expected exactly 1 real alert (not duplicated), got %d: %+v", len(alerts), alerts)
	}
}

func TestWatchTenantForNewAnomaliesDetectsARealSecondAlertAfterAFreshSnapshot(t *testing.T) {
	s := testStore(t)
	tenantID := testTenant(t, s)
	ingestGrowthAnomaly(t, s, tenantID, "cluster-a", 1, 5)

	if _, err := s.WatchTenantForNewAnomalies(tenantID); err != nil {
		t.Fatalf("first WatchTenantForNewAnomalies: %v", err)
	}

	// A genuinely new snapshot with a fresh, still-anomalous jump --
	// this must count as a real new alert, not be silently absorbed by
	// the first one's identity key.
	time.Sleep(1100 * time.Millisecond)
	if err := s.Ingest(tenantID, "cluster-a", []Finding{{Source: "ns/a", CostHighINR: 25}}); err != nil {
		t.Fatalf("Ingest: %v", err)
	}

	n, err := s.WatchTenantForNewAnomalies(tenantID)
	if err != nil {
		t.Fatalf("second WatchTenantForNewAnomalies: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 real new alert from the fresh snapshot, got %d", n)
	}

	alerts, err := s.UnacknowledgedAnomalyAlerts(tenantID)
	if err != nil {
		t.Fatalf("UnacknowledgedAnomalyAlerts: %v", err)
	}
	if len(alerts) != 2 {
		t.Fatalf("expected 2 real distinct alerts now, got %d: %+v", len(alerts), alerts)
	}
}

func TestWatchTenantForNewAnomaliesFindsNothingWithNoRealAnomaly(t *testing.T) {
	s := testStore(t)
	tenantID := testTenant(t, s)
	ingestGrowthAnomaly(t, s, tenantID, "cluster-a", 10, 12) // 1.2x, below threshold

	n, err := s.WatchTenantForNewAnomalies(tenantID)
	if err != nil {
		t.Fatalf("WatchTenantForNewAnomalies: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 alerts with no real anomaly present, got %d", n)
	}
}

func TestAcknowledgeAnomalyAlertRemovesItFromUnacknowledged(t *testing.T) {
	s := testStore(t)
	tenantID := testTenant(t, s)
	ingestGrowthAnomaly(t, s, tenantID, "cluster-a", 1, 5)
	if _, err := s.WatchTenantForNewAnomalies(tenantID); err != nil {
		t.Fatalf("WatchTenantForNewAnomalies: %v", err)
	}

	alerts, err := s.UnacknowledgedAnomalyAlerts(tenantID)
	if err != nil || len(alerts) != 1 {
		t.Fatalf("setup: expected 1 real alert, got %+v, err=%v", alerts, err)
	}

	if err := s.AcknowledgeAnomalyAlert(tenantID, alerts[0].ID); err != nil {
		t.Fatalf("AcknowledgeAnomalyAlert: %v", err)
	}

	after, err := s.UnacknowledgedAnomalyAlerts(tenantID)
	if err != nil {
		t.Fatalf("UnacknowledgedAnomalyAlerts: %v", err)
	}
	if len(after) != 0 {
		t.Fatalf("expected 0 unacknowledged alerts after acknowledging the only one, got %+v", after)
	}
}

func TestAcknowledgeAnomalyAlertIsIdempotent(t *testing.T) {
	s := testStore(t)
	tenantID := testTenant(t, s)
	ingestGrowthAnomaly(t, s, tenantID, "cluster-a", 1, 5)
	if _, err := s.WatchTenantForNewAnomalies(tenantID); err != nil {
		t.Fatalf("WatchTenantForNewAnomalies: %v", err)
	}
	alerts, _ := s.UnacknowledgedAnomalyAlerts(tenantID)

	if err := s.AcknowledgeAnomalyAlert(tenantID, alerts[0].ID); err != nil {
		t.Fatalf("first acknowledge: %v", err)
	}
	if err := s.AcknowledgeAnomalyAlert(tenantID, alerts[0].ID); err != nil {
		t.Fatalf("second acknowledge (already acknowledged) should be a real no-op success, got: %v", err)
	}
}

func TestAcknowledgeAnomalyAlertReturnsErrNotFoundForUnknownID(t *testing.T) {
	s := testStore(t)
	tenantID := testTenant(t, s)

	err := s.AcknowledgeAnomalyAlert(tenantID, 999999)
	if err != ErrAnomalyAlertNotFound {
		t.Fatalf("expected ErrAnomalyAlertNotFound, got %v", err)
	}
}

func TestWatchTenantForNewAnomaliesIsolatesByTenant(t *testing.T) {
	s := testStore(t)
	tenantA := testTenant(t, s)
	tenantB := testTenant(t, s)
	ingestGrowthAnomaly(t, s, tenantA, "cluster-a", 1, 5)

	if _, err := s.WatchTenantForNewAnomalies(tenantA); err != nil {
		t.Fatalf("WatchTenantForNewAnomalies(tenantA): %v", err)
	}

	alertsB, err := s.UnacknowledgedAnomalyAlerts(tenantB)
	if err != nil {
		t.Fatalf("UnacknowledgedAnomalyAlerts(tenantB): %v", err)
	}
	if len(alertsB) != 0 {
		t.Fatalf("tenant b saw tenant a's real anomaly alert: %+v", alertsB)
	}
}

func TestAllTenantIDsListsRealTenants(t *testing.T) {
	s := testStore(t)
	tenantA := testTenant(t, s)
	tenantB := testTenant(t, s)

	ids, err := s.AllTenantIDs()
	if err != nil {
		t.Fatalf("AllTenantIDs: %v", err)
	}

	found := map[int64]bool{}
	for _, id := range ids {
		found[id] = true
	}
	if !found[tenantA] || !found[tenantB] {
		t.Fatalf("expected both real tenants in AllTenantIDs, got %v", ids)
	}
}
