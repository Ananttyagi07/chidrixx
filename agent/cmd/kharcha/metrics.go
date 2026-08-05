// SPDX-License-Identifier: Apache-2.0
package main

import (
	"context"
	"log"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	flowBytesTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "kharcha_flow_bytes_total",
		Help: "Cumulative bytes observed per source workload, remote, classified path, and direction.",
	}, []string{"local_workload", "remote", "path_class", "direction"})

	costINR = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "kharcha_cost_inr",
		Help: "Cumulative estimated cost in INR since the agent started, per source workload and path class.",
	}, []string{"local_workload", "path_class", "bound"})

	mapEntries = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "kharcha_map_entries",
		Help: "Distinct flow keys in the eBPF map as of the most recent scrape.",
	})

	// The eBPF flow map is a BPF_MAP_TYPE_LRU_PERCPU_HASH: once it is
	// full the kernel silently evicts the least-recently-used flow to
	// make room, with no notification of any kind -- there is no
	// kernel-side "dropped" counter to read, so the only honest early
	// warning available is how close to capacity the map actually is.
	// This has already bitten this project once for real: the map was
	// originally sized at 4096 and silently undercounted ~9.7k real
	// concurrent connections as ~3.6k, caught only by a deliberate load
	// test (see PROJECT_STATUS.md §2). Silent undercounting is
	// especially bad for a cost-attribution tool, because it makes the
	// bill look better than reality.
	//
	// Capacity is published as its own gauge rather than only baked into
	// the ratio so an operator can see both numbers and verify the ratio
	// rather than trusting it, and so an alert can be written either way.
	mapMaxEntries = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "kharcha_map_max_entries",
		Help: "Capacity of the eBPF flow map, read from the loaded map itself (not a hardcoded constant).",
	})

	mapUtilizationRatio = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "kharcha_map_utilization_ratio",
		Help: "kharcha_map_entries / kharcha_map_max_entries (0-1). Alert well below 1: at 1 the LRU map is silently evicting real flows.",
	})

	scrapeLagSeconds = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "kharcha_scrape_lag_seconds",
		Help: "Wall-clock time the most recent map scrape + enrichment pass took.",
	})
)

func init() {
	prometheus.MustRegister(flowBytesTotal, costINR, mapEntries, mapMaxEntries, mapUtilizationRatio, scrapeLagSeconds)
}

// recordMapUtilization publishes the real occupancy of the eBPF flow map
// after a scrape. maxEntries comes from the loaded *ebpf.Map itself, so
// resizing the map in bpf/flow_cgroup.c automatically rebases the ratio
// and any alert written against it -- nothing here duplicates the 16384
// literal from the C source.
//
// A zero maxEntries would mean the map's capacity couldn't be read at
// all; the ratio is left untouched rather than reported as 0, since a
// fabricated 0% would read as "plenty of headroom" -- the exact opposite
// of the truth this metric exists to surface.
func recordMapUtilization(entries int, maxEntries uint32) {
	mapEntries.Set(float64(entries))

	if maxEntries == 0 {
		return
	}

	mapMaxEntries.Set(float64(maxEntries))
	mapUtilizationRatio.Set(float64(entries) / float64(maxEntries))
}

// recordFlowBytes is called once per flow per scrape, after
// classification, so label cardinality stays bounded to real (workload,
// remote, class) tuples instead of raw cgroup IDs.
func recordFlowBytes(localWorkload, remote string, class PathClass, tx, rx uint64) {
	if tx > 0 {
		flowBytesTotal.WithLabelValues(localWorkload, remote, string(class), "tx").Add(float64(tx))
	}

	if rx > 0 {
		flowBytesTotal.WithLabelValues(localWorkload, remote, string(class), "rx").Add(float64(rx))
	}
}

// recordCostMetrics re-derives the cost gauges from the aggregate's
// current cumulative findings. Gauges, not counters — Aggregate already
// holds cost cumulative-since-start, so each tick re-Sets rather than
// double-accumulating.
func recordCostMetrics(agg *Aggregate) {
	costINR.Reset()

	for _, f := range agg.Findings() {
		costINR.WithLabelValues(f.Source, string(f.Class), "low").Set(f.CostLowINR)
		costINR.WithLabelValues(f.Source, string(f.Class), "high").Set(f.CostHighINR)
	}
}

// serveMetrics starts the Prometheus /metrics endpoint (FR-I1) and shuts it
// down when ctx is cancelled.
func serveMetrics(ctx context.Context, addr string) {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())

	srv := &http.Server{Addr: addr, Handler: mux}

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("metrics server: %v", err)
		}
	}()

	go func() {
		<-ctx.Done()
		_ = srv.Close()
	}()
}
