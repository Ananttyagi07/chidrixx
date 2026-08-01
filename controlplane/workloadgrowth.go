// SPDX-License-Identifier: Apache-2.0
package main

import (
	"fmt"
	"sort"
	"time"
)

// WorkloadCostPoint is one snapshot's total cost for a single workload
// (summed across every destination it talked to in that snapshot).
type WorkloadCostPoint struct {
	ReportedAt time.Time `json:"reported_at"`
	CostHigh   float64   `json:"cost_high"`
}

// WorkloadGrowth is one workload's real cost history across every
// snapshot this control plane has ever retained for it -- "over the
// retained history," not a fabricated fixed window like "last 6 months"
// when the database might only have a day of data. DeltaINR is the real
// difference between its first and most recent appearance, the ranking
// key for "which workload grew the most."
type WorkloadGrowth struct {
	ClusterID string              `json:"cluster_id"`
	Workload  string              `json:"workload"`
	Trend     []WorkloadCostPoint `json:"trend"`
	DeltaINR  float64             `json:"delta_inr"`
	// RelatedEvents are real deploy events in this workload's own
	// namespace, anywhere within its trend window -- the "why" behind a
	// cost increase, when there is a real one to show. Correlation, not
	// proven causation, same honesty as Anomaly.LikelyCause.
	RelatedEvents []DeployEvent `json:"related_events,omitempty"`
}

// WorkloadCostGrowth ranks every workload with at least two real
// snapshots by how much its cost changed between its first and most
// recent appearance, highest increase first, capped at topN. A workload
// seen only once has nothing to compare against and is excluded, same
// reasoning as detectAnomalies skipping single-snapshot clusters.
func (s *Store) WorkloadCostGrowth(tenantID int64, topN int) ([]WorkloadGrowth, error) {
	rows, err := s.db.Query(`
		SELECT cluster_id, src_workload, reported_at, SUM(cost_high_inr)
		FROM flow_aggregate
		WHERE tenant_id = ?
		GROUP BY cluster_id, src_workload, reported_at
		ORDER BY cluster_id, src_workload, reported_at
	`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("query workload cost history: %w", err)
	}
	defer rows.Close()

	type key struct{ clusterID, workload string }
	byWorkload := make(map[key][]WorkloadCostPoint)
	order := make([]key, 0)

	for rows.Next() {
		var k key
		var reportedAt int64
		var cost float64
		if err := rows.Scan(&k.clusterID, &k.workload, &reportedAt, &cost); err != nil {
			return nil, fmt.Errorf("scan workload cost point: %w", err)
		}
		if _, seen := byWorkload[k]; !seen {
			order = append(order, k)
		}
		byWorkload[k] = append(byWorkload[k], WorkloadCostPoint{
			ReportedAt: time.Unix(reportedAt, 0),
			CostHigh:   cost,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]WorkloadGrowth, 0)
	for _, k := range order {
		trend := byWorkload[k]
		if len(trend) < 2 {
			continue
		}
		out = append(out, WorkloadGrowth{
			ClusterID: k.clusterID,
			Workload:  k.workload,
			Trend:     trend,
			DeltaINR:  trend[len(trend)-1].CostHigh - trend[0].CostHigh,
		})
	}

	sort.Slice(out, func(i, j int) bool { return out[i].DeltaINR > out[j].DeltaINR })

	if len(out) > topN {
		out = out[:topN]
	}

	// Real deploy-event correlation only makes sense for the workloads
	// actually being shown -- computed after truncation to topN, not for
	// every workload the query ever saw.
	for i := range out {
		ns, ok := extractNamespace(out[i].Workload)
		if !ok {
			continue
		}
		events, err := s.RecentDeployEvents(
			tenantID, out[i].ClusterID,
			out[i].Trend[0].ReportedAt, out[i].Trend[len(out[i].Trend)-1].ReportedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("related deploy events for %s: %w", out[i].Workload, err)
		}
		related := events[:0:0]
		for _, e := range events {
			if e.Namespace == ns {
				related = append(related, e)
			}
		}
		if len(related) > 0 {
			out[i].RelatedEvents = related
		}
	}

	return out, nil
}
