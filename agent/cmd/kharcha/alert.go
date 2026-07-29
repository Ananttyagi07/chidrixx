// SPDX-License-Identifier: Apache-2.0
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"
)

// Alerter watches cumulative findings and posts to a Slack-compatible
// incoming webhook when a finding's estimated cost crosses a threshold, or
// has grown substantially since the last alert for that same flow (build
// manual FR-R3/§5.3 — "rose 3x this week"). It never re-alerts on every
// tick once a flow settles at a given cost; only genuine threshold
// crossings and further growth trigger a new message.
type Alerter struct {
	webhookURL   string
	thresholdINR float64
	growthRatio  float64 // e.g. 2.0 = alert again once cost has doubled since the last alert

	httpClient *http.Client

	mu          sync.Mutex
	lastAlerted map[string]float64 // finding key -> CostHighINR at last alert
}

func NewAlerter(webhookURL string, thresholdINR, growthRatio float64) *Alerter {
	return &Alerter{
		webhookURL:   webhookURL,
		thresholdINR: thresholdINR,
		growthRatio:  growthRatio,
		httpClient:   &http.Client{Timeout: 5 * time.Second},
		lastAlerted:  make(map[string]float64),
	}
}

// Check evaluates every current finding and fires alerts for the ones that
// just crossed the threshold, or have grown by growthRatio since the last
// alert for that same flow. A failed webhook post is logged, not fatal —
// and deliberately doesn't record lastAlerted, so the same finding is
// retried on the next tick instead of being silently dropped.
func (a *Alerter) Check(findings []*Finding) {
	if a.webhookURL == "" {
		return
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	for _, f := range findings {
		if f.CostHighINR < a.thresholdINR {
			continue
		}

		key := findingKey(f)
		last := a.lastAlerted[key]

		if last > 0 && f.CostHighINR < last*a.growthRatio {
			continue // already alerted at this scale; hasn't grown enough to alert again
		}

		if err := a.post(f); err != nil {
			log.Printf("slack alert: %v", err)
			continue
		}

		a.lastAlerted[key] = f.CostHighINR
	}
}

func findingKey(f *Finding) string {
	return fmt.Sprintf("%d|%s|%s", f.CgroupID, f.Class, f.DestIP)
}

type slackMessage struct {
	Text string `json:"text"`
}

func alertText(f *Finding) string {
	text := fmt.Sprintf(
		"*Kharcha alert*: %s -> %s (%s) estimated at ₹%.2f-%.2f so far.",
		f.Source, f.Destination, f.Class, f.CostLowINR, f.CostHighINR,
	)

	if f.FixHint != "" {
		text += " Fix: " + f.FixHint
	}

	return text
}

func (a *Alerter) post(f *Finding) error {
	body, err := json.Marshal(slackMessage{Text: alertText(f)})
	if err != nil {
		return fmt.Errorf("marshal slack payload: %w", err)
	}

	resp, err := a.httpClient.Post(a.webhookURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("post to webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}

	return nil
}
