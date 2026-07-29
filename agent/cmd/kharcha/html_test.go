// SPDX-License-Identifier: Apache-2.0
package main

import (
	"bytes"
	"net"
	"strings"
	"testing"
)

func testPriceBook() *PriceBook {
	pb := &PriceBook{
		Cloud:        "aws",
		Region:       "ap-south-1",
		LastVerified: "2026-07-29",
		Rates:        map[string]float64{"internet_egress": 0.10},
	}
	pb.FX.UsdToInr = 84.0
	return pb
}

func TestWriteHTMLEmpty(t *testing.T) {
	agg := NewAggregate(testPriceBook(), func(string) string { return "" }, nil, true, "node-a")

	var buf bytes.Buffer
	if err := agg.WriteHTML(&buf, 100); err != nil {
		t.Fatalf("WriteHTML: %v", err)
	}

	if !strings.Contains(buf.String(), "No traffic observed yet.") {
		t.Fatal("expected the empty-state message in the rendered HTML")
	}
}

func TestWriteHTMLWithFindings(t *testing.T) {
	agg := NewAggregate(testPriceBook(), func(string) string { return "" }, nil, true, "node-a")

	agg.Add(
		WorkloadIdentity{CgroupID: 1, CgroupPath: "some/pod"},
		net.ParseIP("8.8.8.8"),
		nil, nil, nil, nil,
		1_000_000_000, 0,
	)

	var buf bytes.Buffer
	if err := agg.WriteHTML(&buf, 100); err != nil {
		t.Fatalf("WriteHTML: %v", err)
	}

	out := buf.String()

	for _, want := range []string{"INTERNET_EGRESS", "some/pod", "8.8.8.8"} {
		if !strings.Contains(out, want) {
			t.Fatalf("rendered HTML missing %q:\n%s", want, out)
		}
	}
}
