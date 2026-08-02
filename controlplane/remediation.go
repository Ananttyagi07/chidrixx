// SPDX-License-Identifier: Apache-2.0
package main

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
}

// DefaultRemediationPolicy is the real, currently-shipped policy.
func DefaultRemediationPolicy() RemediationPolicy {
	return RemediationPolicy{
		RequireManifest:       true,
		RequireHighConfidence: true,
		MinSavingsHighINR:     0.01, // any real positive saving, excluding float noise at exactly 0
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
