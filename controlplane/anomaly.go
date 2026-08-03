// SPDX-License-Identifier: Apache-2.0
package main

import (
	"fmt"
	"time"
)

// anomalyGrowthRatioThreshold matches the agent's own alert.go default
// (AlertGrowthRatio) for the same reasoning: a cluster's total cost more
// than doubling snapshot-to-snapshot is worth flagging, not a fixed rupee
// amount that would mean different things at different scales.
const anomalyGrowthRatioThreshold = 2.0

// anomalyCauseLookback widens the deploy-event search past the exact
// previous-snapshot timestamp -- a real deployment can take a few minutes
// to actually shift traffic (rollout, connection draining, DNS caching),
// so a rigid "only the exact window between two snapshots" search would
// miss the very correlations this feature exists to surface.
const anomalyCauseLookback = 30 * time.Minute

// Anomaly is a real cross-snapshot cost comparison for one cluster --
// never a fabricated "285% spike" narrative, just the two real numbers
// and the ratio between them. LikelyCause is an optional real deploy
// event (a Deployment's replica count actually changing) found in the
// same cluster shortly before the jump -- correlation the operator can
// verify, explicitly not asserted as proven causation.
type Anomaly struct {
	ClusterID       string       `json:"cluster_id"`
	PreviousCostINR float64      `json:"previous_cost_inr"`
	CurrentCostINR  float64      `json:"current_cost_inr"`
	GrowthRatio     float64      `json:"growth_ratio"`
	LikelyCause     *DeployEvent `json:"likely_cause,omitempty"`
	// SnapshotReportedAt is the real "current" snapshot's own timestamp
	// this anomaly was computed from -- the real, stable identity a
	// proactive watcher (anomaly_watch.go) uses to tell "the same
	// anomaly, still unchanged since the last snapshot" apart from "a
	// genuinely new one," rather than re-alerting on every tick.
	SnapshotReportedAt time.Time `json:"snapshot_reported_at"`
}

// detectAnomalies compares each of the given clusters' two most recent
// snapshots and flags any whose total cost grew by at least
// anomalyGrowthRatioThreshold. A cluster with fewer than two snapshots
// ever ingested has nothing to compare against and is silently skipped --
// not flagged, not assumed normal, just not enough history yet to say
// either way.
//
// Takes clusterIDs rather than calling store.Clusters itself: that query
// re-derives "the latest snapshot per cluster" via a real, measured-
// expensive full index scan (see summary.go's comment), and
// handleDashboardSummary -- the hottest caller -- already has that list
// from its own earlier store.Clusters call. Callers with no such list
// handy (the chat assistant's tool, the anomaly narrator's API) just
// fetch it themselves first.
func detectAnomalies(store *Store, tenantID int64, clusterIDs []string) ([]Anomaly, error) {
	anomalies := make([]Anomaly, 0)

	for _, clusterID := range clusterIDs {
		trend, err := store.CostTrend(tenantID, clusterID, 2)
		if err != nil {
			return nil, fmt.Errorf("cost trend for %s: %w", clusterID, err)
		}

		if len(trend) < 2 {
			continue
		}

		previous := trend[0].CostHigh
		current := trend[len(trend)-1].CostHigh

		if previous <= 0 {
			continue // avoid a meaningless infinite/undefined ratio from a zero baseline
		}

		ratio := current / previous
		if ratio >= anomalyGrowthRatioThreshold {
			currentSnapshotTime := trend[len(trend)-1].ReportedAt
			anomaly := Anomaly{
				ClusterID:          clusterID,
				PreviousCostINR:    previous,
				CurrentCostINR:     current,
				GrowthRatio:        ratio,
				SnapshotReportedAt: currentSnapshotTime,
			}

			events, err := store.RecentDeployEvents(
				tenantID, clusterID,
				currentSnapshotTime.Add(-anomalyCauseLookback), currentSnapshotTime,
			)
			if err != nil {
				return nil, fmt.Errorf("recent deploy events for %s: %w", clusterID, err)
			}
			if len(events) > 0 {
				anomaly.LikelyCause = &events[0] // RecentDeployEvents orders most-recent-first
			}

			anomalies = append(anomalies, anomaly)
		}
	}

	return anomalies, nil
}
