// SPDX-License-Identifier: Apache-2.0
package main

import (
	"fmt"
	"time"
)

// IngestDeployEvents stores real observed Deployment replica-count
// changes an agent shipped alongside its findings -- the raw material for
// anomaly root-cause correlation (see anomaly.go).
func (s *Store) IngestDeployEvents(tenantID int64, clusterID string, events []DeployEvent) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}

	stmt, err := tx.Prepare(`
		INSERT INTO deploy_event (tenant_id, cluster_id, namespace, name, reason, message, occurred_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("prepare insert: %w", err)
	}
	defer stmt.Close()

	for _, e := range events {
		if _, err := stmt.Exec(tenantID, clusterID, e.Namespace, e.Name, e.Reason, e.Message, e.OccurredAt.Unix()); err != nil {
			tx.Rollback()
			return fmt.Errorf("insert deploy event: %w", err)
		}
	}

	return tx.Commit()
}

// RecentDeployEvents returns every deploy event for one tenant's cluster
// that occurred within [since, until], most recent first -- the real
// window an anomaly's root-cause correlation searches.
func (s *Store) RecentDeployEvents(tenantID int64, clusterID string, since, until time.Time) ([]DeployEvent, error) {
	rows, err := s.db.Query(`
		SELECT namespace, name, reason, message, occurred_at
		FROM deploy_event
		WHERE tenant_id = ? AND cluster_id = ? AND occurred_at >= ? AND occurred_at <= ?
		ORDER BY occurred_at DESC
	`, tenantID, clusterID, since.Unix(), until.Unix())
	if err != nil {
		return nil, fmt.Errorf("query deploy events: %w", err)
	}
	defer rows.Close()

	out := make([]DeployEvent, 0)
	for rows.Next() {
		var e DeployEvent
		var occurredAt int64
		if err := rows.Scan(&e.Namespace, &e.Name, &e.Reason, &e.Message, &occurredAt); err != nil {
			return nil, fmt.Errorf("scan deploy event: %w", err)
		}
		e.OccurredAt = time.Unix(occurredAt, 0)
		out = append(out, e)
	}
	return out, rows.Err()
}
