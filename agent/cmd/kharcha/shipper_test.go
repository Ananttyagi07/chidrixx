// SPDX-License-Identifier: Apache-2.0
package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
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

	shipper := NewShipper(srv.URL, "chidrixx-lab", "")

	findings := []*Finding{
		{
			Source: "ns/app", Destination: "8.8.8.8",
			Class: PathCrossAZ, Confidence: ConfHigh,
			BytesTx: 100, BytesRx: 200,
			CostLowINR: 1.5, CostHighINR: 2.5,
			FixHint:        "co-locate these two workloads in the same zone",
			FixManifest:    "apiVersion: networking.k8s.io/v1\nkind: NetworkPolicy\n",
			Cloud:          "aws",
			Region:         "ap-south-1",
			SavingsLowINR:  1.0,
			SavingsHighINR: 2.0,
		},
	}

	events := []DeployEvent{
		{Namespace: "checkout", Name: "checkout", Reason: "ReplicaCountChanged", Message: "replicas increased from 2 to 5", OccurredAt: time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)},
	}

	if err := shipper.Ship(context.Background(), findings, events); err != nil {
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
		"path_class": "CROSS_AZ", "confidence": "high",
		"bytes_tx": float64(100), "bytes_rx": float64(200),
		"cost_low_inr": 1.5, "cost_high_inr": 2.5,
		"fix_hint":         "co-locate these two workloads in the same zone",
		"fix_manifest":     "apiVersion: networking.k8s.io/v1\nkind: NetworkPolicy\n",
		"cloud":            "aws",
		"region":           "ap-south-1",
		"savings_low_inr":  1.0,
		"savings_high_inr": 2.0,
	} {
		if f[key] != want {
			t.Errorf("field %s = %v, want %v", key, f[key], want)
		}
	}

	evs, ok := captured["events"].([]any)
	if !ok || len(evs) != 1 {
		t.Fatalf("expected 1 event in events array, got: %+v", captured["events"])
	}
	ev := evs[0].(map[string]any)
	for key, want := range map[string]any{
		"namespace": "checkout", "name": "checkout",
		"reason": "ReplicaCountChanged", "message": "replicas increased from 2 to 5",
	} {
		if ev[key] != want {
			t.Errorf("event field %s = %v, want %v", key, ev[key], want)
		}
	}
}

// TestShipperDisabledWithoutURL proves an empty control-plane URL is a
// clean no-op — the default, unconfigured state — not an error.
func TestShipperDisabledWithoutURL(t *testing.T) {
	shipper := NewShipper("", "chidrixx-lab", "")

	if err := shipper.Ship(context.Background(), []*Finding{{Source: "ns/app"}}, nil); err != nil {
		t.Fatalf("expected no-op with empty URL, got error: %v", err)
	}
}

// TestShipperSendsBasicAuthToken proves the agent actually sends
// credentials in the form the control plane's requireToken middleware
// checks (Basic Auth password field) — this is the other half of that
// contract; a mismatch here would silently lock every agent out of an
// authenticated control plane.
func TestShipperSendsBasicAuthToken(t *testing.T) {
	var gotPass string
	var gotOK bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, gotPass, gotOK = r.BasicAuth()
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	shipper := NewShipper(srv.URL, "chidrixx-lab", "secret-token")

	if err := shipper.Ship(context.Background(), []*Finding{{Source: "ns/app"}}, nil); err != nil {
		t.Fatalf("Ship: %v", err)
	}

	if !gotOK || gotPass != "secret-token" {
		t.Fatalf("Basic Auth password = %q (present=%v), want %q", gotPass, gotOK, "secret-token")
	}
}
