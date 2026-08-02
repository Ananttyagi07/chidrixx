// SPDX-License-Identifier: Apache-2.0
package main

import "sort"

// computeSummary, computeSpendByClass, and computeSpendByCloud replace
// what were three separate SQL queries (Summary/SpendByClass/SpendByCloud
// in store.go), each independently re-deriving "the latest snapshot per
// cluster" via its own full index scan of flow_aggregate. Found necessary
// against real production data: with flow_aggregate never pruned (by
// design) and grown to ~2M real rows, each of those scans measured
// ~1.5-2 real seconds, and dashboard-summary ran effectively the same
// scan 4+ times (Summary, SpendByClass, SpendByCloud, Clusters), enough
// to be the dominant cost in a 13+ real-second response. Since
// handleDashboardSummary already fetches exactly this data once via
// LatestFindings, computing these three purely in Go over that one
// result (the same pattern computeSpendByTeam already used) eliminates
// the redundant queries entirely rather than trying to out-clever
// SQLite's query planner.

func computeSummary(findings []FindingRow, clusterCount int) Summary {
	out := Summary{ClusterCount: clusterCount, FindingCount: len(findings)}
	workloads := make(map[string]struct{})
	for _, f := range findings {
		workloads[f.Source] = struct{}{}
		out.TotalBytesTx += f.BytesTx
		out.TotalBytesRx += f.BytesRx
		out.TotalCostLowINR += f.CostLowINR
		out.TotalCostHighINR += f.CostHighINR
	}
	out.WorkloadCount = len(workloads)
	return out
}

func computeSpendByClass(findings []FindingRow) []ClassSpend {
	byClass := make(map[string]*ClassSpend)
	order := make([]string, 0)
	for _, f := range findings {
		c, exists := byClass[f.PathClass]
		if !exists {
			c = &ClassSpend{PathClass: f.PathClass}
			byClass[f.PathClass] = c
			order = append(order, f.PathClass)
		}
		c.CostHighINR += f.CostHighINR
		c.FindingCount++
	}
	out := make([]ClassSpend, 0, len(order))
	for _, k := range order {
		out = append(out, *byClass[k])
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CostHighINR > out[j].CostHighINR })
	return out
}

func computeSpendByCloud(findings []FindingRow) []CloudSpend {
	type key struct{ cloud, region string }
	byCloud := make(map[key]*CloudSpend)
	order := make([]key, 0)
	for _, f := range findings {
		cloud, region := f.Cloud, f.Region
		if cloud == "" {
			cloud = "unknown"
		}
		if region == "" {
			region = "unknown"
		}
		k := key{cloud, region}
		c, exists := byCloud[k]
		if !exists {
			c = &CloudSpend{Cloud: cloud, Region: region}
			byCloud[k] = c
			order = append(order, k)
		}
		c.CostHighINR += f.CostHighINR
		c.FindingCount++
	}
	out := make([]CloudSpend, 0, len(order))
	for _, k := range order {
		out = append(out, *byCloud[k])
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CostHighINR > out[j].CostHighINR })
	return out
}
