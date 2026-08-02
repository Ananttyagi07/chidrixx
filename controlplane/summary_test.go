// SPDX-License-Identifier: Apache-2.0
package main

import "testing"

// These mirror TestSummaryAggregatesLatestSnapshotsOnly/
// TestSpendByClassGroupsAndRanksByCost/TestSpendByCloudGroupsByCloudAndRegion
// in store_test.go, which test the original SQL-based Store methods --
// same real fixtures and expected values, proving the new pure-Go
// versions (which dashboard-summary actually calls now) compute
// identical results, not just "some" result.

func TestComputeSummaryAggregatesGivenFindings(t *testing.T) {
	findings := []FindingRow{
		{Finding: Finding{Source: "ns/app", CostLowINR: 1, CostHighINR: 2, BytesTx: 100}, ClusterID: "cluster-a"},
		{Finding: Finding{Source: "ns/db", CostLowINR: 0, CostHighINR: 0, BytesTx: 10}, ClusterID: "cluster-a"},
		{Finding: Finding{Source: "ns/other", CostLowINR: 2, CostHighINR: 3, BytesTx: 20}, ClusterID: "cluster-b"},
	}

	sum := computeSummary(findings, 2)

	if sum.ClusterCount != 2 {
		t.Errorf("ClusterCount = %d, want 2", sum.ClusterCount)
	}
	if sum.WorkloadCount != 3 {
		t.Errorf("WorkloadCount = %d, want 3", sum.WorkloadCount)
	}
	if sum.FindingCount != 3 {
		t.Errorf("FindingCount = %d, want 3", sum.FindingCount)
	}
	if sum.TotalBytesTx != 130 {
		t.Errorf("TotalBytesTx = %d, want 130", sum.TotalBytesTx)
	}
	if sum.TotalCostHighINR != 5 {
		t.Errorf("TotalCostHighINR = %v, want 5", sum.TotalCostHighINR)
	}
}

func TestComputeSpendByClassGroupsAndRanksByCost(t *testing.T) {
	findings := []FindingRow{
		{Finding: Finding{PathClass: "INTERNET_EGRESS", CostHighINR: 10}},
		{Finding: Finding{PathClass: "INTERNET_EGRESS", CostHighINR: 5}},
		{Finding: Finding{PathClass: "CROSS_AZ", CostHighINR: 20}},
	}

	classes := computeSpendByClass(findings)

	if len(classes) != 2 {
		t.Fatalf("expected 2 classes, got %d: %+v", len(classes), classes)
	}
	if classes[0].PathClass != "CROSS_AZ" || classes[0].CostHighINR != 20 {
		t.Errorf("expected CROSS_AZ (20) ranked first, got %+v", classes[0])
	}
	if classes[1].PathClass != "INTERNET_EGRESS" || classes[1].CostHighINR != 15 || classes[1].FindingCount != 2 {
		t.Errorf("expected INTERNET_EGRESS (15, 2 findings) ranked second, got %+v", classes[1])
	}
}

func TestComputeSpendByCloudGroupsByCloudAndRegion(t *testing.T) {
	findings := []FindingRow{
		{Finding: Finding{PathClass: "INTERNET_EGRESS", CostHighINR: 10, Cloud: "aws", Region: "ap-south-1"}},
		{Finding: Finding{PathClass: "INTERNET_EGRESS", CostHighINR: 5, Cloud: "aws", Region: "ap-south-1"}},
		{Finding: Finding{PathClass: "INTERNET_EGRESS", CostHighINR: 20, Cloud: "gcp", Region: "asia-south1"}},
	}

	clouds := computeSpendByCloud(findings)

	if len(clouds) != 2 {
		t.Fatalf("expected 2 (cloud, region) groups, got %d: %+v", len(clouds), clouds)
	}
	if clouds[0].Cloud != "gcp" || clouds[0].Region != "asia-south1" || clouds[0].CostHighINR != 20 {
		t.Errorf("expected gcp/asia-south1 (20) ranked first, got %+v", clouds[0])
	}
	if clouds[1].Cloud != "aws" || clouds[1].Region != "ap-south-1" || clouds[1].CostHighINR != 15 || clouds[1].FindingCount != 2 {
		t.Errorf("expected aws/ap-south-1 (15, 2 findings) ranked second, got %+v", clouds[1])
	}
}

func TestComputeSpendByCloudFoldsMissingCloudIntoUnknown(t *testing.T) {
	findings := []FindingRow{
		{Finding: Finding{PathClass: "INTERNET_EGRESS", CostHighINR: 10}},
	}

	clouds := computeSpendByCloud(findings)

	if len(clouds) != 1 || clouds[0].Cloud != "unknown" || clouds[0].Region != "unknown" {
		t.Fatalf("expected a single 'unknown' bucket for findings shipped without cloud/region, got: %+v", clouds)
	}
}

func TestComputeSummaryWithNoFindingsIsAllZero(t *testing.T) {
	sum := computeSummary(nil, 0)
	if sum.ClusterCount != 0 || sum.WorkloadCount != 0 || sum.FindingCount != 0 {
		t.Fatalf("expected an all-zero summary for no findings, got: %+v", sum)
	}
}
