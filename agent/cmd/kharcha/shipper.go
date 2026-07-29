// SPDX-License-Identifier: Apache-2.0
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// shipperFinding is the wire format the control plane's ingest API expects
// (controlplane/model.go's Finding) — a small translation layer so the
// agent's own Finding struct (report.go) doesn't have to match another
// module's JSON tags field-for-field.
type shipperFinding struct {
	Source      string  `json:"source"`
	Destination string  `json:"destination"`
	PathClass   string  `json:"path_class"`
	Confidence  string  `json:"confidence"`
	BytesTx     uint64  `json:"bytes_tx"`
	BytesRx     uint64  `json:"bytes_rx"`
	CostLowINR  float64 `json:"cost_low_inr"`
	CostHighINR float64 `json:"cost_high_inr"`
	FixHint     string  `json:"fix_hint"`
}

type shipperRequest struct {
	ClusterID string           `json:"cluster_id"`
	Findings  []shipperFinding `json:"findings"`
}

// Shipper posts the agent's current cumulative findings to a control
// plane, matching the frozen ingest contract (build manual §15): the
// control plane receives aggregates only, never raw flows, and each POST
// is a full snapshot — the control plane treats it as a point-in-time
// reading, not a delta.
type Shipper struct {
	url        string
	clusterID  string
	authToken  string
	httpClient *http.Client
}

func NewShipper(url, clusterID, authToken string) *Shipper {
	return &Shipper{
		url:        url,
		clusterID:  clusterID,
		authToken:  authToken,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// Ship posts the current findings. Disabled (no-op) when url is empty. A
// failure is returned, not fatal — the caller logs and keeps going, since
// a control plane being briefly unreachable shouldn't stop local
// reporting (CLI/HTML/metrics/alerts all keep working independently).
func (s *Shipper) Ship(ctx context.Context, findings []*Finding) error {
	if s.url == "" {
		return nil
	}

	wire := make([]shipperFinding, 0, len(findings))
	for _, f := range findings {
		wire = append(wire, shipperFinding{
			Source:      f.Source,
			Destination: f.Destination,
			PathClass:   string(f.Class),
			Confidence:  f.Confidence,
			BytesTx:     f.BytesTx,
			BytesRx:     f.BytesRx,
			CostLowINR:  f.CostLowINR,
			CostHighINR: f.CostHighINR,
			FixHint:     f.FixHint,
		})
	}

	body, err := json.Marshal(shipperRequest{ClusterID: s.clusterID, Findings: wire})
	if err != nil {
		return fmt.Errorf("marshal ship request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build ship request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if s.authToken != "" {
		// Basic Auth, not Bearer: matches the control plane's
		// requireToken, which checks the Basic Auth password field so a
		// human viewing the dashboard in a plain browser and this agent
		// posting JSON can share one auth mechanism. Username is ignored
		// on the receiving end.
		req.SetBasicAuth("agent", s.authToken)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("post to control plane: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("control plane returned status %d", resp.StatusCode)
	}

	return nil
}
