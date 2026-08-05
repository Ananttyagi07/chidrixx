// SPDX-License-Identifier: Apache-2.0
package main

import (
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// This is the real safety check that sits between "a generated manifest
// exists" (remediation.go's existing policy gate) and ever calling
// something safe to auto-apply. The fix engine's generated NetworkPolicy
// (agent/cmd/kharcha/fixengine.go) always sets podSelector: {} -- it
// applies to every real pod in the namespace, not just the one workload
// that was flagged. That means the real risk isn't "does this manifest
// look reasonable," it's "does any OTHER real workload in that same
// namespace also depend on the exact destination this policy would
// block." That's answerable directly from this tenant's own already-
// ingested traffic history, without any kernel tracing or symbolic
// execution: replay the manifest's blocked destination against real
// flow_aggregate/flow_aggregate_daily rows and see who else was really
// talking to it.

// HistoricalFlowCost is one real source workload's total real cost
// talking to a specific destination, summed across whatever history this
// tenant has actually retained (see store.go's HistoricalFlowsToDestination).
type HistoricalFlowCost struct {
	SrcWorkload string
	CostHighINR float64
}

// TrafficReplayResult is the real, transparent outcome of replaying one
// generated manifest against real historical traffic. Safe is
// deliberately conservative: it's only true when real historical data
// positively confirms no other real workload in the same namespace ever
// talked to the blocked destination -- never assumed true by default, and
// never true just because no data happened to be found (see
// EvaluateTrafficReplay's zero-history case).
type TrafficReplayResult struct {
	Safe                bool     `json:"safe"`
	SafetyScore         float64  `json:"safety_score"`
	BlockedDestinations []string `json:"blocked_destinations"`
	IntendedCostINR     float64  `json:"intended_cost_inr"`
	CollateralCostINR   float64  `json:"collateral_cost_inr"`
	AffectedWorkloads   []string `json:"affected_workloads,omitempty"`
	Note                string   `json:"note,omitempty"`
}

// generatedNetworkPolicyYAML mirrors only the fields this control plane's
// own fix engine ever generates (fixengine.go's networkPolicyDenyDestination)
// -- deliberately not the full Kubernetes NetworkPolicy API surface, since
// this parser only ever has to understand manifests this same codebase
// produces, not arbitrary user-authored YAML.
type generatedNetworkPolicyYAML struct {
	Metadata struct {
		Namespace string `yaml:"namespace"`
	} `yaml:"metadata"`
	Spec struct {
		Egress []struct {
			To []struct {
				IPBlock struct {
					CIDR   string   `yaml:"cidr"`
					Except []string `yaml:"except"`
				} `yaml:"ipBlock"`
			} `yaml:"to"`
		} `yaml:"egress"`
	} `yaml:"spec"`
}

// parseGeneratedNetworkPolicy extracts the real namespace and the real
// blocked IP(s) (the "except" entries, stripped of their /32 or /128
// mask -- flow_aggregate stores destinations as bare IP strings, never
// with a CIDR suffix) from a manifest this control plane itself
// generated. Returns ("", nil, nil) -- not an error -- for a manifest
// that parses as valid YAML but doesn't contain a real blocked
// destination; only a genuine parse failure returns a non-nil error.
func parseGeneratedNetworkPolicy(manifest string) (namespace string, blockedIPs []string, err error) {
	var doc generatedNetworkPolicyYAML
	if err := yaml.Unmarshal([]byte(manifest), &doc); err != nil {
		return "", nil, fmt.Errorf("parse generated manifest: %w", err)
	}

	namespace = doc.Metadata.Namespace

	for _, eg := range doc.Spec.Egress {
		for _, to := range eg.To {
			for _, except := range to.IPBlock.Except {
				ip, _, _ := strings.Cut(except, "/")
				if ip != "" {
					blockedIPs = append(blockedIPs, ip)
				}
			}
		}
	}

	return namespace, blockedIPs, nil
}

// EvaluateTrafficReplay is a pure function: given the namespace and
// flagged source workload the manifest was generated for, the real
// blocked destination IPs, and real historical (source workload -> summed
// cost) data already fetched for those destinations (store.go's
// HistoricalFlowsToDestination), decides whether applying this manifest
// would silently affect any other real workload.
//
// podSelector: {} means the generated policy applies to every pod in the
// namespace, so any OTHER real source workload (not the one this fix was
// actually about) whose own namespace matches and who has real recorded
// traffic to the same blocked destination counts as collateral -- a
// namespace match against a *different* namespace's workload doesn't,
// since that namespace's own NetworkPolicy (or lack of one) is
// unaffected by this one.
func EvaluateTrafficReplay(namespace, flaggedSource string, blockedIPs []string, historical []HistoricalFlowCost) TrafficReplayResult {
	result := TrafficReplayResult{BlockedDestinations: append([]string(nil), blockedIPs...)}

	var intended, collateral float64
	affected := make(map[string]bool)

	for _, row := range historical {
		if row.SrcWorkload == flaggedSource {
			intended += row.CostHighINR
			continue
		}
		ns, ok := extractNamespace(row.SrcWorkload)
		if !ok || ns != namespace {
			// A different (or unresolved) namespace's workload isn't
			// touched by this specific policy -- its own namespace has
			// no NetworkPolicy change here.
			continue
		}
		collateral += row.CostHighINR
		affected[row.SrcWorkload] = true
	}

	result.IntendedCostINR = intended
	result.CollateralCostINR = collateral
	for w := range affected {
		result.AffectedWorkloads = append(result.AffectedWorkloads, w)
	}
	sort.Strings(result.AffectedWorkloads)

	total := intended + collateral
	switch {
	case total == 0:
		// No real historical data matched this destination at all -- not
		// even the flagged workload's own traffic, which is genuinely
		// unexpected. Don't default to "safe" just because there's
		// nothing to measure against; that would be reporting confidence
		// this check never actually earned.
		result.Safe = false
		result.SafetyScore = 0
		result.Note = "no historical traffic to this destination was found at all -- can't confirm safety, treating conservatively"
	case collateral == 0:
		result.Safe = true
		result.SafetyScore = 1
	default:
		result.Safe = false
		result.SafetyScore = intended / total
	}

	return result
}
