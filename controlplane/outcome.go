// SPDX-License-Identifier: Apache-2.0
package main

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// RecommendationOutcome tracks one real fix recommendation from the moment
// it's first shown through to whether an operator applied it and what
// actually happened to cost afterward. This is the data set every future
// AI feature (a grounded chat assistant, an outcome-calibrated ranking
// model) needs and none of it exists yet -- this is where it starts.
type RecommendationOutcome struct {
	ID                      int64
	ClusterID               string
	Source                  string
	Destination             string
	PathClass               string
	FixHint                 string
	PredictedSavingsLowINR  float64
	PredictedSavingsHighINR float64
	CostBeforeINR           float64
	FirstShownAt            time.Time
	LastShownAt             time.Time
	AppliedAt               *time.Time
	CostAfterINR            *float64
	MeasuredAt              *time.Time
}

// RecordRecommendationsShown upserts one outcome row per real finding
// currently surfaced as a top fix. Once a recommendation has been marked
// applied, later calls here must not clobber cost_before_inr (that's the
// real "before" baseline the whole comparison depends on) -- they still
// bump last_shown_at so "still being shown after being marked applied"
// stays visible, but shown/predicted/cost-before fields freeze.
func (s *Store) RecordRecommendationsShown(tenantID int64, fixes []FindingRow) error {
	now := time.Now().Unix()
	for _, f := range fixes {
		if f.FixHint == "" {
			continue
		}
		_, err := s.db.Exec(`
			INSERT INTO recommendation_outcomes (
				tenant_id, cluster_id, source, destination, path_class, fix_hint,
				predicted_savings_low_inr, predicted_savings_high_inr, cost_before_inr,
				first_shown_at, last_shown_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(tenant_id, cluster_id, source, destination, path_class) DO UPDATE SET
				last_shown_at = excluded.last_shown_at,
				fix_hint = CASE WHEN applied_at IS NULL THEN excluded.fix_hint ELSE fix_hint END,
				predicted_savings_low_inr = CASE WHEN applied_at IS NULL THEN excluded.predicted_savings_low_inr ELSE predicted_savings_low_inr END,
				predicted_savings_high_inr = CASE WHEN applied_at IS NULL THEN excluded.predicted_savings_high_inr ELSE predicted_savings_high_inr END,
				cost_before_inr = CASE WHEN applied_at IS NULL THEN excluded.cost_before_inr ELSE cost_before_inr END
		`, tenantID, f.ClusterID, f.Source, f.Destination, f.PathClass, f.FixHint,
			f.SavingsLowINR, f.SavingsHighINR, f.CostHighINR, now, now)
		if err != nil {
			return fmt.Errorf("record recommendation shown: %w", err)
		}
	}
	return nil
}

var ErrOutcomeNotFound = errors.New("recommendation outcome not found")

// MarkRecommendationApplied records that an operator says they applied a
// real fix. It only sets applied_at the first time -- re-applying is a
// no-op, not a timestamp reset, so cost_before_inr's baseline stays tied
// to the original application.
func (s *Store) MarkRecommendationApplied(tenantID int64, clusterID, source, destination, pathClass string) error {
	res, err := s.db.Exec(`
		UPDATE recommendation_outcomes
		SET applied_at = ?
		WHERE tenant_id = ? AND cluster_id = ? AND source = ? AND destination = ? AND path_class = ?
		  AND applied_at IS NULL
	`, time.Now().Unix(), tenantID, clusterID, source, destination, pathClass)
	if err != nil {
		return fmt.Errorf("mark recommendation applied: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("mark recommendation applied: %w", err)
	}
	if n == 0 {
		// Either it doesn't exist, or it's already applied -- tell those
		// apart so the caller can give an honest response either way.
		var exists bool
		if err := s.db.QueryRow(`
			SELECT 1 FROM recommendation_outcomes
			WHERE tenant_id = ? AND cluster_id = ? AND source = ? AND destination = ? AND path_class = ?
		`, tenantID, clusterID, source, destination, pathClass).Scan(&exists); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrOutcomeNotFound
			}
			return fmt.Errorf("mark recommendation applied: check existing: %w", err)
		}
		// Already applied -- treat as success, not an error.
	}
	return nil
}

// measurePendingOutcomes closes the loop for every applied-but-unmeasured
// outcome in this tenant where fresher data now exists: it looks for the
// real cost of the same source->destination pair (any path_class, since a
// working fix often changes the path class rather than keeping it) in the
// cluster's latest snapshot, as long as that snapshot is newer than
// applied_at. If no matching flow exists anymore, that's a real result
// too -- cost dropped to 0, not "no data."
func (s *Store) measurePendingOutcomes(tenantID int64) error {
	rows, err := s.db.Query(`
		SELECT id, cluster_id, source, destination, applied_at
		FROM recommendation_outcomes
		WHERE tenant_id = ? AND applied_at IS NOT NULL AND cost_after_inr IS NULL
	`, tenantID)
	if err != nil {
		return fmt.Errorf("query pending outcomes: %w", err)
	}
	type pending struct {
		id                             int64
		clusterID, source, destination string
		appliedAt                      int64
	}
	var pendings []pending
	for rows.Next() {
		var p pending
		if err := rows.Scan(&p.id, &p.clusterID, &p.source, &p.destination, &p.appliedAt); err != nil {
			rows.Close()
			return fmt.Errorf("scan pending outcome: %w", err)
		}
		pendings = append(pendings, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	for _, p := range pendings {
		var latestReportedAt int64
		if err := s.db.QueryRow(`
			SELECT COALESCE(MAX(reported_at), 0) FROM flow_aggregate
			WHERE tenant_id = ? AND cluster_id = ?
		`, tenantID, p.clusterID).Scan(&latestReportedAt); err != nil {
			return fmt.Errorf("latest snapshot time for %s: %w", p.clusterID, err)
		}
		if latestReportedAt <= p.appliedAt {
			continue // no fresher data yet -- stays pending
		}

		var costAfter float64
		err := s.db.QueryRow(`
			SELECT cost_high_inr FROM flow_aggregate
			WHERE tenant_id = ? AND cluster_id = ? AND reported_at = ?
			  AND src_workload = ? AND dst_workload_or_endpoint = ?
			ORDER BY cost_high_inr DESC LIMIT 1
		`, tenantID, p.clusterID, latestReportedAt, p.source, p.destination).Scan(&costAfter)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("measure outcome %d: %w", p.id, err)
		}
		if errors.Is(err, sql.ErrNoRows) {
			costAfter = 0 // the flow is gone from the latest snapshot -- fixed.
		}

		if _, err := s.db.Exec(`
			UPDATE recommendation_outcomes SET cost_after_inr = ?, measured_at = ? WHERE id = ?
		`, costAfter, time.Now().Unix(), p.id); err != nil {
			return fmt.Errorf("save measured outcome %d: %w", p.id, err)
		}
	}
	return nil
}

// ListRecommendationOutcomes measures any outcomes that can now be
// measured, then returns every outcome for the tenant, most recently
// shown first.
func (s *Store) ListRecommendationOutcomes(tenantID int64) ([]RecommendationOutcome, error) {
	if err := s.measurePendingOutcomes(tenantID); err != nil {
		return nil, err
	}

	rows, err := s.db.Query(`
		SELECT id, cluster_id, source, destination, path_class, fix_hint,
		       predicted_savings_low_inr, predicted_savings_high_inr, cost_before_inr,
		       first_shown_at, last_shown_at, applied_at, cost_after_inr, measured_at
		FROM recommendation_outcomes
		WHERE tenant_id = ?
		ORDER BY last_shown_at DESC
	`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list recommendation outcomes: %w", err)
	}
	defer rows.Close()

	out := make([]RecommendationOutcome, 0)
	for rows.Next() {
		var o RecommendationOutcome
		var firstShown, lastShown int64
		var appliedAt, measuredAt sql.NullInt64
		var costAfter sql.NullFloat64

		if err := rows.Scan(
			&o.ID, &o.ClusterID, &o.Source, &o.Destination, &o.PathClass, &o.FixHint,
			&o.PredictedSavingsLowINR, &o.PredictedSavingsHighINR, &o.CostBeforeINR,
			&firstShown, &lastShown, &appliedAt, &costAfter, &measuredAt,
		); err != nil {
			return nil, fmt.Errorf("scan recommendation outcome: %w", err)
		}

		o.FirstShownAt = time.Unix(firstShown, 0)
		o.LastShownAt = time.Unix(lastShown, 0)
		if appliedAt.Valid {
			t := time.Unix(appliedAt.Int64, 0)
			o.AppliedAt = &t
		}
		if costAfter.Valid {
			v := costAfter.Float64
			o.CostAfterINR = &v
		}
		if measuredAt.Valid {
			t := time.Unix(measuredAt.Int64, 0)
			o.MeasuredAt = &t
		}
		out = append(out, o)
	}
	return out, rows.Err()
}
