// SPDX-License-Identifier: Apache-2.0
package main

// Finding is the wire format an agent ships one row of its cumulative
// report as. It mirrors the build manual's frozen flow_aggregate schema
// (src_workload, dst_workload_or_endpoint, path_class, bytes, cost bounds,
// confidence) closely enough that the ingest API never has to reshape it.
type Finding struct {
	Source      string  `json:"source"`
	Destination string  `json:"destination"`
	PathClass   string  `json:"path_class"`
	Confidence  string  `json:"confidence"`
	BytesTx     uint64  `json:"bytes_tx"`
	BytesRx     uint64  `json:"bytes_rx"`
	CostLowINR  float64 `json:"cost_low_inr"`
	CostHighINR float64 `json:"cost_high_inr"`
	FixHint     string  `json:"fix_hint"`
	FixManifest string  `json:"fix_manifest"`
}

// IngestRequest is one snapshot of an agent's cumulative-since-start
// aggregate, tagged with which cluster it came from. Each ingest is a full
// snapshot (the agent's own model is cumulative, not fixed time windows),
// not a delta — the control plane stores it as a point-in-time row and
// always serves the latest snapshot per cluster for "current state," while
// keeping every snapshot for trend-over-time.
type IngestRequest struct {
	ClusterID string    `json:"cluster_id"`
	Findings  []Finding `json:"findings"`
}
