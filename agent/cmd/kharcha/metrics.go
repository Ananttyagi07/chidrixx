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

	scrapeLagSeconds = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "kharcha_scrape_lag_seconds",
		Help: "Wall-clock time the most recent map scrape + enrichment pass took.",
	})
)

func init() {
	prometheus.MustRegister(flowBytesTotal, costINR, mapEntries, scrapeLagSeconds)
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
