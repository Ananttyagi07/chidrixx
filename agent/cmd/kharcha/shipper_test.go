// SPDX-License-Identifier: Apache-2.0
package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestShipperWireFormatMatchesControlPlaneContract proves the exact JSON
// shape posted to a control plane, against a real HTTP server — this is
// the seam between two separate Go modules (agent and controlplane) that
// only agree by convention, so it's worth locking down explicitly rather
// than trusting both sides stay in sync by memory.
func TestShipperWireFormatMatchesControlPlaneContract(t *testing.T) {
	var captured map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Errorf("decode request body: %v", err)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", got)
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	shipper := NewShipper(srv.URL, "chidrixx-lab")

	findings := []*Finding{
		{
			Source: "ns/app", Destination: "8.8.8.8",
			Class: PathInternetEgress, Confidence: ConfHigh,
			BytesTx: 100, BytesRx: 200,
			CostLowINR: 1.5, CostHighINR: 2.5,
			FixHint: "cache or compress it",
		},
	}

	if err := shipper.Ship(context.Background(), findings); err != nil {
		t.Fatalf("Ship: %v", err)
	}

	if captured["cluster_id"] != "chidrixx-lab" {
		t.Fatalf("cluster_id = %v, want chidrixx-lab", captured["cluster_id"])
	}

	fs, ok := captured["findings"].([]any)
	if !ok || len(fs) != 1 {
		t.Fatalf("expected 1 finding in findings array, got: %+v", captured["findings"])
	}

	f := fs[0].(map[string]any)
	for key, want := range map[string]any{
		"source": "ns/app", "destination": "8.8.8.8",
		"path_class": "INTERNET_EGRESS", "confidence": "high",
		"bytes_tx": float64(100), "bytes_rx": float64(200),
		"cost_low_inr": 1.5, "cost_high_inr": 2.5,
		"fix_hint": "cache or compress it",
	} {
		if f[key] != want {
			t.Errorf("field %s = %v, want %v", key, f[key], want)
		}
	}
}

// TestShipperDisabledWithoutURL proves an empty control-plane URL is a
// clean no-op — the default, unconfigured state — not an error.
func TestShipperDisabledWithoutURL(t *testing.T) {
	shipper := NewShipper("", "chidrixx-lab")

	if err := shipper.Ship(context.Background(), []*Finding{{Source: "ns/app"}}); err != nil {
		t.Fatalf("expected no-op with empty URL, got error: %v", err)
	}
}
