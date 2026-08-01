// SPDX-License-Identifier: Apache-2.0
package main

import "fmt"

// anomalyGrowthRatioThreshold matches the agent's own alert.go default
// (AlertGrowthRatio) for the same reasoning: a cluster's total cost more
// than doubling snapshot-to-snapshot is worth flagging, not a fixed rupee
// amount that would mean different things at different scales.
const anomalyGrowthRatioThreshold = 2.0

// Anomaly is a real cross-snapshot cost comparison for one cluster --
// never a fabricated "285% spike" narrative, just the two real numbers
// and the ratio between them.
type Anomaly struct {
	ClusterID       string  `json:"cluster_id"`
	PreviousCostINR float64 `json:"previous_cost_inr"`
	CurrentCostINR  float64 `json:"current_cost_inr"`
	GrowthRatio     float64 `json:"growth_ratio"`
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
			anomalies = append(anomalies, Anomaly{
				ClusterID:       c.ClusterID,
				PreviousCostINR: previous,
				CurrentCostINR:  current,
				GrowthRatio:     ratio,
			})
		}
	}

	return anomalies, nil
}
