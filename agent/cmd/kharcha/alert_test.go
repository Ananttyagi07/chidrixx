// SPDX-License-Identifier: Apache-2.0
package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

// TestAlerterThresholdAndDebounce proves the actual HTTP/JSON delivery
// mechanics and the debounce logic against a real HTTP server — the part
// that's genuinely verifiable without a live Slack workspace. Whether a
// message shows up readably in an actual Slack channel is a separate
// concern this test can't and doesn't claim to cover.
func TestAlerterThresholdAndDebounce(t *testing.T) {
	var (
		posts    int32
		lastText string
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var msg slackMessage
		if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
			t.Errorf("decode webhook payload: %v", err)
		}
		lastText = msg.Text
		atomic.AddInt32(&posts, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	alerter := NewAlerter(srv.URL, 100.0, 2.0)

	below := &Finding{CgroupID: 1, Class: PathInternetEgress, DestIP: "8.8.8.8", CostHighINR: 50}
	alerter.Check([]*Finding{below})
	if got := atomic.LoadInt32(&posts); got != 0 {
		t.Fatalf("expected no alert below threshold, got %d posts", got)
	}

	crossed := &Finding{CgroupID: 1, Class: PathInternetEgress, DestIP: "8.8.8.8", Source: "ns/app", Destination: "8.8.8.8", CostLowINR: 90, CostHighINR: 120}
	alerter.Check([]*Finding{crossed})
	if got := atomic.LoadInt32(&posts); got != 1 {
		t.Fatalf("expected exactly 1 alert on threshold crossing, got %d", got)
	}
	if lastText == "" {
		t.Fatal("expected a non-empty alert message")
	}

	// Same cost, same finding: must NOT re-alert (debounce).
	alerter.Check([]*Finding{crossed})
	if got := atomic.LoadInt32(&posts); got != 1 {
		t.Fatalf("expected debounce to suppress a repeat alert at the same cost, got %d posts", got)
	}

	// Grown past the growth ratio (2x): must alert again.
	grown := &Finding{CgroupID: 1, Class: PathInternetEgress, DestIP: "8.8.8.8", CostHighINR: 300}
	alerter.Check([]*Finding{grown})
	if got := atomic.LoadInt32(&posts); got != 2 {
		t.Fatalf("expected a second alert once cost grew past the growth ratio, got %d posts", got)
	}
}

// TestAlerterDisabledWithoutWebhook proves an empty webhook URL is a clean
// no-op, not a nil-pointer panic waiting to happen in production when
// alerting isn't configured.
func TestAlerterDisabledWithoutWebhook(t *testing.T) {
	alerter := NewAlerter("", 1.0, 2.0)
	alerter.Check([]*Finding{{CostHighINR: 1_000_000}})
}

// TestAlerterHandlesWebhookFailureWithoutPanicking proves a broken webhook
// URL is logged and skipped, not a crash — the agent's core job (pricing,
// reporting) shouldn't die because Slack is unreachable.
func TestAlerterHandlesWebhookFailureWithoutPanicking(t *testing.T) {
	alerter := NewAlerter("http://127.0.0.1:0", 1.0, 2.0) // guaranteed-unreachable address
	alerter.Check([]*Finding{{CgroupID: 1, Class: PathInternetEgress, DestIP: "1.1.1.1", CostHighINR: 100}})
}
