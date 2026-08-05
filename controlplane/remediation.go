// SPDX-License-Identifier: Apache-2.0
package main

import "fmt"

// This is the safe, dry-run first increment of closed-loop remediation:
// a real, deterministic decision engine that evaluates the tenant's
// current real top fixes against a real policy and reports which ones
// *would* be auto-applied if that were ever enabled -- and, just as
// importantly, which ones wouldn't and why. It never touches a real
// cluster: the control plane has no write access to any cluster
// (AutomationsPage.tsx's own disclaimer already says so, and nothing
// here changes that). This is deliberately the evidence-gathering phase
// before ever asking for that write access -- see PROJECT_STATUS.md's
// future-vision section for why real auto-apply is a separate, much
// bigger, explicitly-opt-in decision.

// RemediationPolicy is the real, tunable bar a fix must clear to be
// flagged as "would auto-apply." Deliberately conservative defaults:
// only fixes with a real, mechanically-generated manifest (not a vague
// sentence), only high-confidence classifications, and only savings
// above a real floor -- never everything just because it's flagged.
type RemediationPolicy struct {
	RequireManifest       bool
	RequireHighConfidence bool
	MinSavingsHighINR     float64
	// RequireSafeTrafficReplay gates WouldAutoApply on traffic_replay.go's
	// real replay check: does this tenant's own historical traffic show
	// any OTHER real workload in the same namespace also depending on the
	// destination this manifest would block. On by default -- a manifest
	// clearing every other bar but never checked against real traffic
	// isn't actually safe, just untested.
	RequireSafeTrafficReplay bool
}

// DefaultRemediationPolicy is the real, currently-shipped policy.
func DefaultRemediationPolicy() RemediationPolicy {
	return RemediationPolicy{
		RequireManifest:          true,
		RequireHighConfidence:    true,
		MinSavingsHighINR:        0.01, // any real positive saving, excluding float noise at exactly 0
		RequireSafeTrafficReplay: true,
	}
}

// RemediationDecision is one real finding's evaluation against the
// policy -- Reasons always explains the decision, whether it qualifies
// or not, so this is transparent by construction, not just a filtered
// list with no explanation.
type RemediationDecision struct {
	ClusterID      string   `json:"cluster_id"`
	Source         string   `json:"source"`
	Destination    string   `json:"destination"`
	PathClass      string   `json:"path_class"`
	Confidence     string   `json:"confidence"`
	FixHint        string   `json:"fix_hint"`
	FixManifest    string   `json:"fix_manifest,omitempty"`
	CostHighINR    float64  `json:"cost_high_inr"`
	SavingsHighINR float64  `json:"savings_high_inr"`
	WouldAutoApply bool     `json:"would_auto_apply"`
	Reasons        []string `json:"reasons"`
	// TrafficReplay is nil until SimulateTrafficReplay runs (traffic_replay.go)
	// -- e.g. a decision with no manifest at all is never replayed, since
	// there's no generated policy to test.
	TrafficReplay *TrafficReplayResult `json:"traffic_replay,omitempty"`
}

// EvaluateRemediation runs the real policy over the tenant's current
// findings. Only findings with a fix_hint are considered at all --
// matching TopFixes' own definition of "a real flagged opportunity."
func EvaluateRemediation(findings []FindingRow, policy RemediationPolicy) []RemediationDecision {
	out := make([]RemediationDecision, 0)

	for _, f := range findings {
		if f.FixHint == "" {
			continue
		}

		d := RemediationDecision{
			ClusterID:      f.ClusterID,
			Source:         f.Source,
			Destination:    f.Destination,
			PathClass:      f.PathClass,
			Confidence:     f.Confidence,
			FixHint:        f.FixHint,
			FixManifest:    f.FixManifest,
			CostHighINR:    f.CostHighINR,
			SavingsHighINR: f.SavingsHighINR,
		}

		var reasons []string
		qualifies := true

		if policy.RequireManifest && f.FixManifest == "" {
			qualifies = false
			reasons = append(reasons, "no mechanically-generated manifest exists for this path class -- only a plain-text hint, which needs a human to interpret")
		}
		if policy.RequireHighConfidence && f.Confidence != "high" {
			qualifies = false
			reasons = append(reasons, "confidence is \""+f.Confidence+"\", not \"high\"")
		}
		if f.SavingsHighINR < policy.MinSavingsHighINR {
			qualifies = false
			reasons = append(reasons, "predicted savings too small to justify an automatic change")
		}

		if qualifies {
			reasons = []string{"has a real generated manifest, high confidence, and positive predicted savings"}
		}

		d.WouldAutoApply = qualifies
		d.Reasons = reasons
		out = append(out, d)
	}

	return out
}

// SimulateTrafficReplay runs traffic_replay.go's real safety check against
// every decision that has a real generated manifest, using historical
// flow data fetched through fetchHistorical -- a real store lookup at the
// API layer, a fake closure in tests, same pattern chat_tools.go already
// uses for tenant-scoped, injectable data access. Never mutates the input
// slice; returns a new one, same convention as EvaluateRemediation.
//
// A decision with no manifest at all is left untouched (TrafficReplay
// stays nil) -- there's no generated policy to test. A decision whose
// manifest can't be parsed is treated conservatively as unsafe, not
// silently skipped: "has a manifest" and "we understood the manifest well
// enough to verify it" are different claims, and this must not report
// confidence it never earned.
//
// Decisions already disqualified by an earlier policy bar are also left
// untouched, deliberately: this is a gate on auto-applying, so a fix
// that already isn't a candidate doesn't need the check, and the lookup
// is genuinely expensive at real scale -- measured live at ~4s per
// destination against a real 4.1M-row flow_aggregate (an index to make
// it faster was tried and reverted for costing far more on the ingest
// path than it saved here; see store.go). Running it on every
// manifest-bearing finding regardless made the real live endpoint ~7.5s
// slower for results that couldn't change any outcome.
func SimulateTrafficReplay(decisions []RemediationDecision, policy RemediationPolicy, fetchHistorical func(clusterID, destinationIP string) ([]HistoricalFlowCost, error)) ([]RemediationDecision, error) {
	out := make([]RemediationDecision, len(decisions))
	copy(out, decisions)

	for i := range out {
		d := &out[i]
		if d.FixManifest == "" || !d.WouldAutoApply {
			continue
		}

		namespace, blockedIPs, parseErr := parseGeneratedNetworkPolicy(d.FixManifest)
		if parseErr != nil || namespace == "" || len(blockedIPs) == 0 {
			replay := TrafficReplayResult{
				Safe: false,
				Note: "couldn't parse the generated manifest to run a real traffic-replay safety check -- treating conservatively as not confirmed safe",
			}
			applyTrafficReplayResult(d, replay, policy)
			continue
		}

		var historical []HistoricalFlowCost
		for _, ip := range blockedIPs {
			rows, err := fetchHistorical(d.ClusterID, ip)
			if err != nil {
				return nil, fmt.Errorf("fetch historical flows to %s: %w", ip, err)
			}
			historical = append(historical, rows...)
		}

		replay := EvaluateTrafficReplay(namespace, d.Source, blockedIPs, historical)
		applyTrafficReplayResult(d, replay, policy)
	}

	return out, nil
}

// applyTrafficReplayResult attaches a computed replay result to a
// decision and, when the policy requires it, lets a real collateral risk
// override an otherwise-qualifying decision -- replacing the stale
// "qualifies" reason rather than leaving it alongside a contradicting new
// one, so Reasons never reads as self-contradictory.
func applyTrafficReplayResult(d *RemediationDecision, replay TrafficReplayResult, policy RemediationPolicy) {
	d.TrafficReplay = &replay

	if !policy.RequireSafeTrafficReplay {
		return
	}

	if replay.Safe {
		d.Reasons = append(d.Reasons, "traffic replay confirmed no other real workload in this namespace depends on the blocked destination")
		return
	}

	if d.WouldAutoApply {
		d.Reasons = nil
	}
	d.WouldAutoApply = false

	if replay.Note != "" {
		d.Reasons = append(d.Reasons, replay.Note)
		return
	}
	d.Reasons = append(d.Reasons, fmt.Sprintf(
		"traffic replay found %d other real workload(s) in this namespace also depend on the destination this policy would block (safety score %.2f) -- applying it would silently affect them too",
		len(replay.AffectedWorkloads), replay.SafetyScore))
}
