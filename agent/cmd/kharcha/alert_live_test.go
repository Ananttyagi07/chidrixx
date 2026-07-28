package main

import (
	"os"
	"testing"
)

// TestAlerterRealWebhookDelivery proves the Alerter's HTTP delivery path
// against a REAL external endpoint over the real internet, not just a
// local httptest.Server (see alert_test.go for that). It's opt-in: point
// KHARCHA_ALERT_WEBHOOK_URL at a real webhook — a Slack incoming webhook,
// or any HTTP endpoint that logs what it receives — and run:
//
//	KHARCHA_ALERT_WEBHOOK_URL=https://hooks.slack.com/services/... \
//	  go test ./cmd/kharcha/... -run TestAlerterRealWebhookDelivery -v
//
// Skipped by default so `go test ./...` and CI never depend on network
// access or a live webhook existing.
func TestAlerterRealWebhookDelivery(t *testing.T) {
	url := os.Getenv("KHARCHA_ALERT_WEBHOOK_URL")
	if url == "" {
		t.Skip("set KHARCHA_ALERT_WEBHOOK_URL to a real webhook URL to run this")
	}

	alerter := NewAlerter(url, 0, 2.0) // threshold 0: any finding alerts

	f := &Finding{
		CgroupID:    1,
		Source:      "chidrixx-test/client",
		Destination: "8.8.8.8",
		Class:       PathInternetEgress,
		DestIP:      "8.8.8.8",
		CostLowINR:  12.34,
		CostHighINR: 15.67,
		FixHint:     "real end-to-end delivery test, not synthetic",
	}

	alerter.Check([]*Finding{f})

	t.Logf("posted a real alert to %s — verify it arrived", url)
}
