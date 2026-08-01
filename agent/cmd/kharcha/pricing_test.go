// SPDX-License-Identifier: Apache-2.0
package main

import "testing"

// The repo's real price books (pricebook/aws.yaml, pricebook/gcp.yaml) must
// stay loadable and produce sane, ordered cost bands -- this would have
// caught the "second real price book" work shipping a YAML file that
// LoadPriceBook can't actually parse.
func TestRealPriceBooksLoad(t *testing.T) {
	cases := []struct {
		path   string
		cloud  string
		region string
	}{
		{"../../../pricebook/aws.yaml", "aws", "ap-south-1"},
		{"../../../pricebook/gcp.yaml", "gcp", "asia-south1"},
	}

	for _, c := range cases {
		pb, err := LoadPriceBook(c.path)
		if err != nil {
			t.Fatalf("LoadPriceBook(%s): %v", c.path, err)
		}
		if pb.Cloud != c.cloud {
			t.Errorf("%s: Cloud = %q, want %q", c.path, pb.Cloud, c.cloud)
		}
		if pb.Region != c.region {
			t.Errorf("%s: Region = %q, want %q", c.path, pb.Region, c.region)
		}
		if pb.FX.UsdToInr <= 0 {
			t.Errorf("%s: FX.UsdToInr = %v, want > 0", c.path, pb.FX.UsdToInr)
		}

		low, high := pb.CostINR(PathCrossAZ, ConfHigh, 1e9)
		if low <= 0 || high <= low {
			t.Errorf("%s: CostINR(cross_az, high, 1GB) = [%v, %v], want 0 < low < high", c.path, low, high)
		}

		// Internet egress must always be more expensive per GB than
		// same-region traffic -- a real invariant of every cloud's
		// published pricing, not something specific to one price book.
		lowIE, _ := pb.CostINR(PathInternetEgress, ConfHigh, 1e9)
		lowCAZ, _ := pb.CostINR(PathCrossAZ, ConfHigh, 1e9)
		if lowIE <= lowCAZ {
			t.Errorf("%s: internet_egress (%v) should cost more per GB than cross_az (%v)", c.path, lowIE, lowCAZ)
		}
	}
}
