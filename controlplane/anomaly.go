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
}

// detectAnomalies compares each cluster's two most recent snapshots and
// flags any whose total cost grew by at least anomalyGrowthRatioThreshold.
// A cluster with fewer than two snapshots ever ingested has nothing to
// compare against and is silently skipped -- not flagged, not assumed
// normal, just not enough history yet to say either way.
func detectAnomalies(store *Store, tenantID int64) ([]Anomaly, error) {
	clusters, err := store.Clusters(tenantID)
	if err != nil {
		return nil, fmt.Errorf("list clusters: %w", err)
	}

	anomalies := make([]Anomaly, 0)

	for _, c := range clusters {
		trend, err := store.CostTrend(tenantID, c.ClusterID, 2)
		if err != nil {
			return nil, fmt.Errorf("cost trend for %s: %w", c.ClusterID, err)
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
			anomaly := Anomaly{
				ClusterID:       c.ClusterID,
				PreviousCostINR: previous,
				CurrentCostINR:  current,
				GrowthRatio:     ratio,
			}

			currentSnapshotTime := trend[len(trend)-1].ReportedAt
			events, err := store.RecentDeployEvents(
				tenantID, c.ClusterID,
				currentSnapshotTime.Add(-anomalyCauseLookback), currentSnapshotTime,
			)
			if err != nil {
				return nil, fmt.Errorf("recent deploy events for %s: %w", c.ClusterID, err)
			}
			if len(events) > 0 {
				anomaly.LikelyCause = &events[0] // RecentDeployEvents orders most-recent-first
			}

			anomalies = append(anomalies, anomaly)
		}
	}

	return anomalies, nil
}
