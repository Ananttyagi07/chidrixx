package main

import (
	"net"
	"testing"
)

func TestClassify(t *testing.T) {
	zones := map[string]string{
		"node-a-1": "ap-south-1a",
		"node-a-2": "ap-south-1a",
		"node-b-1": "ap-south-1b",
		"node-c-1": "us-east-1a",
		"node-x":   "", // present but unlabeled, like the single-node lab
	}
	zoneOf := func(n string) string { return zones[n] }

	cases := []struct {
		name       string
		in         ClassifyInput
		wantClass  PathClass
		wantConfid string
	}{
		{
			name:       "loopback",
			in:         ClassifyInput{Remote: net.ParseIP("127.0.0.1")},
			wantClass:  PathSameNode,
			wantConfid: ConfHigh,
		},
		{
			name:       "same node",
			in:         ClassifyInput{Remote: net.ParseIP("10.42.0.2"), SourceNode: "node-a-1", DestNode: "node-a-1"},
			wantClass:  PathSameNode,
			wantConfid: ConfHigh,
		},
		{
			name:       "same zone, different node",
			in:         ClassifyInput{Remote: net.ParseIP("10.42.0.2"), SourceNode: "node-a-1", DestNode: "node-a-2"},
			wantClass:  PathSameAZ,
			wantConfid: ConfHigh,
		},
		{
			name:       "cross AZ, same region",
			in:         ClassifyInput{Remote: net.ParseIP("10.42.0.2"), SourceNode: "node-a-1", DestNode: "node-b-1"},
			wantClass:  PathCrossAZ,
			wantConfid: ConfHigh,
		},
		{
			name:       "cross region",
			in:         ClassifyInput{Remote: net.ParseIP("10.42.0.2"), SourceNode: "node-a-1", DestNode: "node-c-1"},
			wantClass:  PathCrossRegion,
			wantConfid: ConfHigh,
		},
		{
			name:       "in-cluster but zone unknown degrades, doesn't fabricate",
			in:         ClassifyInput{Remote: net.ParseIP("10.42.0.2"), SourceNode: "node-x", DestNode: "node-a-1"},
			wantClass:  PathSameAZ,
			wantConfid: ConfLow,
		},
		{
			name:       "managed service",
			in:         ClassifyInput{Remote: net.ParseIP("172.31.5.5"), Managed: true},
			wantClass:  PathManagedService,
			wantConfid: ConfHigh,
		},
		{
			name:       "private off-cluster",
			in:         ClassifyInput{Remote: net.ParseIP("10.99.0.1")},
			wantClass:  PathPrivateOffCluster,
			wantConfid: ConfHigh,
		},
		{
			name:       "internet egress, node has public IP",
			in:         ClassifyInput{Remote: net.ParseIP("8.8.8.8"), NodeHasPublicIP: true},
			wantClass:  PathInternetEgress,
			wantConfid: ConfHigh,
		},
		{
			name:       "NAT egress heuristic, never high confidence",
			in:         ClassifyInput{Remote: net.ParseIP("8.8.8.8"), NodeHasPublicIP: false},
			wantClass:  PathNATEgress,
			wantConfid: ConfMed,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			class, confidence := Classify(c.in, zoneOf)
			if class != c.wantClass || confidence != c.wantConfid {
				t.Fatalf("Classify() = %s/%s, want %s/%s", class, confidence, c.wantClass, c.wantConfid)
			}
		})
	}
}

func TestPriceBookCostINRWidensBandWithLowerConfidence(t *testing.T) {
	pb := &PriceBook{
		Rates: map[string]float64{"internet_egress": 0.10},
	}
	pb.FX.UsdToInr = 80.0

	const oneGB = 1_000_000_000

	lowHigh, highHigh := pb.CostINR(PathInternetEgress, ConfHigh, oneGB)
	lowLow, highLow := pb.CostINR(PathInternetEgress, ConfLow, oneGB)

	mid := (lowHigh + highHigh) / 2
	if mid <= 0 {
		t.Fatalf("expected a positive cost estimate, got mid=%v", mid)
	}

	if (highHigh - lowHigh) >= (highLow - lowLow) {
		t.Fatalf("expected the low-confidence band to be wider: high-confidence [%.2f,%.2f], low-confidence [%.2f,%.2f]",
			lowHigh, highHigh, lowLow, highLow)
	}

	// Free classes must stay exactly zero regardless of confidence.
	if low, high := pb.CostINR(PathSameNode, ConfLow, oneGB); low != 0 || high != 0 {
		t.Fatalf("SAME_NODE must cost 0, got [%.4f, %.4f]", low, high)
	}
}
