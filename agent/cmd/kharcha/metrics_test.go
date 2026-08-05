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

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
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

	// A real utilization sample, so the assertions below check a real
	// exposed value rather than only the metric name existing.
	recordMapUtilization(4096, 16384)

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

	// Re-scrape after recording utilization, so these assert on a real
	// served payload rather than the one captured before the call above.
	resp2, err := http.Get("http://" + addr + "/metrics")
	if err != nil {
		t.Fatalf("second GET /metrics: %v", err)
	}
	defer resp2.Body.Close()
	body2, err := io.ReadAll(resp2.Body)
	if err != nil {
		t.Fatalf("read second body: %v", err)
	}
	out2 := string(body2)

	for _, want := range []string{
		"kharcha_map_max_entries 16384",
		"kharcha_map_utilization_ratio 0.25",
		"kharcha_map_entries 4096",
	} {
		if !strings.Contains(out2, want) {
			t.Fatalf("metrics output missing %q:\n%s", want, out2)
		}
	}
}

func TestRecordMapUtilizationComputesRealRatio(t *testing.T) {
	recordMapUtilization(4096, 16384)

	if got := readGauge(t, mapEntries); got != 4096 {
		t.Fatalf("kharcha_map_entries = %v, want 4096", got)
	}
	if got := readGauge(t, mapMaxEntries); got != 16384 {
		t.Fatalf("kharcha_map_max_entries = %v, want 16384", got)
	}
	if got := readGauge(t, mapUtilizationRatio); got != 0.25 {
		t.Fatalf("kharcha_map_utilization_ratio = %v, want 0.25 at 4096/16384", got)
	}
}

// TestMapUtilizationRatioCrossesEightyFivePercentAtTheRealBoundary pins
// the exact integer where the shipped alert rule (deploy/prometheus/
// kharcha-alerts.yaml, > 0.85) actually starts firing at the current
// 16384 map size. Worth an explicit test because the obvious arithmetic
// is off by one: 0.85 * 16384 = 13926.4, so 13926 entries is really
// 84.998% -- still below the threshold -- and 13927 is the first count
// that genuinely crosses it. A hand-written "kharcha_map_entries > 13926"
// fallback alert is therefore correct only because it is strictly
// greater-than; ">= 13926" would fire early.
func TestMapUtilizationRatioCrossesEightyFivePercentAtTheRealBoundary(t *testing.T) {
	recordMapUtilization(13926, 16384)
	if got := readGauge(t, mapUtilizationRatio); got > 0.85 {
		t.Fatalf("13926/16384 = %v; expected it to still be <= 0.85 (it is 84.998%%)", got)
	}

	recordMapUtilization(13927, 16384)
	if got := readGauge(t, mapUtilizationRatio); got <= 0.85 {
		t.Fatalf("13927/16384 = %v; expected the first real crossing above 0.85", got)
	}
}

// TestRecordMapUtilizationLeavesRatioAloneWithUnknownCapacity guards the
// deliberate choice in recordMapUtilization: reporting a fabricated 0
// ratio when capacity is unreadable would read as "plenty of headroom,"
// the exact opposite of what this metric is for.
func TestRecordMapUtilizationLeavesRatioAloneWithUnknownCapacity(t *testing.T) {
	recordMapUtilization(9000, 16384)
	before := readGauge(t, mapUtilizationRatio)

	recordMapUtilization(9500, 0)

	if got := readGauge(t, mapUtilizationRatio); got != before {
		t.Fatalf("ratio changed to %v on unknown capacity; want it left at %v, never a fabricated 0", got, before)
	}
	// The real entry count is still honest and still published.
	if got := readGauge(t, mapEntries); got != 9500 {
		t.Fatalf("kharcha_map_entries = %v, want the real 9500 even when capacity is unknown", got)
	}
}

func readGauge(t *testing.T, g prometheus.Gauge) float64 {
	t.Helper()

	var m dto.Metric
	if err := g.Write(&m); err != nil {
		t.Fatalf("write gauge: %v", err)
	}
	return m.GetGauge().GetValue()
}
