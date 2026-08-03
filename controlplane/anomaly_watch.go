// SPDX-License-Identifier: Apache-2.0
package main

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// AnomalyAlert is one real, persisted "this was proactively detected"
// record -- the actual notice-and-surface half of "notice, investigate,
// recommend, notify." Anomaly detection itself (detectAnomalies) already
// existed; what was missing was anything that ran it without a human
// first opening a page or asking the chat assistant.
type AnomalyAlert struct {
	ID              int64
	ClusterID       string
	DetectedAt      time.Time
	PreviousCostINR float64
	CurrentCostINR  float64
	GrowthRatio     float64
	AcknowledgedAt  *time.Time
}

var ErrAnomalyAlertNotFound = errors.New("anomaly alert not found")

// WatchTenantForNewAnomalies runs the real, existing detectAnomalies
// against every real cluster this tenant currently has, and records a
// real new alert for any anomaly not already captured for that exact
// snapshot (see anomaly_alerts' unique index in store.go). Returns how
// many genuinely new alerts were recorded -- 0 on a normal tick where
// nothing changed is the common case, not an error.
func (s *Store) WatchTenantForNewAnomalies(tenantID int64) (newAlerts int, err error) {
	clusters, err := s.Clusters(tenantID)
	if err != nil {
		return 0, fmt.Errorf("clusters for tenant %d: %w", tenantID, err)
	}
	if len(clusters) == 0 {
		return 0, nil
	}
	clusterIDs := make([]string, len(clusters))
	for i, c := range clusters {
		clusterIDs[i] = c.ClusterID
	}

	anomalies, err := detectAnomalies(s, tenantID, clusterIDs)
	if err != nil {
		return 0, fmt.Errorf("detect anomalies for tenant %d: %w", tenantID, err)
	}

	now := time.Now().Unix()
	for _, a := range anomalies {
		res, err := s.db.Exec(`
			INSERT INTO anomaly_alerts
				(tenant_id, cluster_id, detected_at, snapshot_reported_at, previous_cost_inr, current_cost_inr, growth_ratio)
			VALUES (?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(tenant_id, cluster_id, snapshot_reported_at) DO NOTHING
		`, tenantID, a.ClusterID, now, a.SnapshotReportedAt.Unix(), a.PreviousCostINR, a.CurrentCostINR, a.GrowthRatio)
		if err != nil {
			return newAlerts, fmt.Errorf("record anomaly alert for %s: %w", a.ClusterID, err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return newAlerts, fmt.Errorf("rows affected for %s: %w", a.ClusterID, err)
		}
		if n > 0 {
			newAlerts++
		}
	}
	return newAlerts, nil
}

// StartAnomalyWatchLoop runs WatchTenantForNewAnomalies for every real
// tenant on a recurring tick, logged, non-fatal on a per-tenant error (a
// transient failure for one tenant must not stop every other tenant's
// real check that tick). Same background-loop shape as
// StartCompactionLoop in compaction.go -- this control plane has no
// separate cron/worker deployment to hand periodic jobs to instead.
func (s *Store) StartAnomalyWatchLoop(interval time.Duration, stop <-chan struct{}, logf func(format string, args ...any)) {
	runOnce := func() {
		tenantIDs, err := s.AllTenantIDs()
		if err != nil {
			logf("anomaly-watch: list tenants: %v", err)
			return
		}
		for _, tenantID := range tenantIDs {
			n, err := s.WatchTenantForNewAnomalies(tenantID)
			if err != nil {
				logf("anomaly-watch: tenant %d: %v", tenantID, err)
				continue
			}
			if n > 0 {
				logf("anomaly-watch: tenant %d: %d new real anomaly alert(s) detected", tenantID, n)
			}
		}
	}

	runOnce()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			runOnce()
		case <-stop:
			return
		}
	}
}

// UnacknowledgedAnomalyAlerts returns every real alert this tenant hasn't
// dismissed yet, most recently detected first -- the "new since you last
// looked" surface a dashboard shows without the operator having to
// actively open the Anomalies page.
func (s *Store) UnacknowledgedAnomalyAlerts(tenantID int64) ([]AnomalyAlert, error) {
	rows, err := s.db.Query(`
		SELECT id, cluster_id, detected_at, previous_cost_inr, current_cost_inr, growth_ratio
		FROM anomaly_alerts
		WHERE tenant_id = ? AND acknowledged_at IS NULL
		ORDER BY detected_at DESC
	`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("query unacknowledged anomaly alerts: %w", err)
	}
	defer rows.Close()

	out := make([]AnomalyAlert, 0)
	for rows.Next() {
		var a AnomalyAlert
		var detectedAt int64
		if err := rows.Scan(&a.ID, &a.ClusterID, &detectedAt, &a.PreviousCostINR, &a.CurrentCostINR, &a.GrowthRatio); err != nil {
			return nil, fmt.Errorf("scan anomaly alert: %w", err)
		}
		a.DetectedAt = time.Unix(detectedAt, 0)
		out = append(out, a)
	}
	return out, rows.Err()
}

// AcknowledgeAnomalyAlert marks one real alert as seen by an operator --
// dismissing the notification, not claiming the underlying anomaly is
// resolved (that's still only ever observed by a fresh, lower snapshot).
// A no-op on an already-acknowledged or unknown ID returns
// ErrAnomalyAlertNotFound so the caller can tell those apart from a real
// tenant-scoped success.
func (s *Store) AcknowledgeAnomalyAlert(tenantID, alertID int64) error {
	res, err := s.db.Exec(`
		UPDATE anomaly_alerts SET acknowledged_at = ?
		WHERE id = ? AND tenant_id = ? AND acknowledged_at IS NULL
	`, time.Now().Unix(), alertID, tenantID)
	if err != nil {
		return fmt.Errorf("acknowledge anomaly alert: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("acknowledge anomaly alert: rows affected: %w", err)
	}
	if n == 0 {
		var exists bool
		if err := s.db.QueryRow(`SELECT 1 FROM anomaly_alerts WHERE id = ? AND tenant_id = ?`, alertID, tenantID).Scan(&exists); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrAnomalyAlertNotFound
			}
			return fmt.Errorf("acknowledge anomaly alert: check existing: %w", err)
		}
		// Already acknowledged -- treat as success, matching
		// MarkRecommendationApplied's idempotency in outcome.go.
	}
	return nil
}
