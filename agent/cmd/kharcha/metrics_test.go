// SPDX-License-Identifier: Apache-2.0
package main

import (
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestServeMetricsExposesCustomMetrics(t *testing.T) {
	// Bind an ephemeral port ourselves so the test doesn't depend on a
	// fixed port being free.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	ln.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	serveMetrics(ctx, addr)

	recordFlowBytes("chidrixx-test/server", "8.8.8.8", PathInternetEgress, 100, 0)

	// kharcha_cost_inr is a GaugeVec: with zero WithLabelValues() calls
	// recorded, client_golang correctly emits no samples for it at all —
	// that's not a bug, so drive it through recordCostMetrics with a real
	// finding instead of asserting on an empty vector.
	agg := NewAggregate(testPriceBook(), func(string) string { return "" }, nil, true, "node-a")
	agg.Add(WorkloadIdentity{CgroupID: 1, CgroupPath: "some/pod"}, net.ParseIP("8.8.8.8"), nil, nil, nil, nil, 1_000_000_000, 0)
	recordCostMetrics(agg)

	var (
		resp *http.Response
	)
	for i := 0; i < 50; i++ {
		resp, err = http.Get("http://" + addr + "/metrics")
		if err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("GET /metrics: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}

	out := string(body)
	for _, want := range []string{
		"kharcha_flow_bytes_total",
		"kharcha_cost_inr",
		"kharcha_map_entries",
		"kharcha_scrape_lag_seconds",
		`local_workload="chidrixx-test/server"`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("metrics output missing %q:\n%s", want, out)
		}
	}
}
