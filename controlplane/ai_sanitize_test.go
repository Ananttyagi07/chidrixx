// SPDX-License-Identifier: Apache-2.0
package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// realToolPayload mirrors the exact shape get_top_fixes really returns --
// a marshalled FindingRow, fix_manifest and all -- so these tests prove
// the sanitizer handles what the product actually sends, not a simplified
// stand-in.
func realToolPayload(t *testing.T) string {
	t.Helper()
	rows := []FindingRow{
		{
			Finding: Finding{
				Source: "checkout/checkout-abc", Destination: "203.0.113.5",
				PathClass: "INTERNET_EGRESS", Confidence: "high",
				BytesTx: 2_000_000_000, CostHighINR: 20, SavingsHighINR: 20,
				FixHint:     "deny this destination if it's not required",
				FixManifest: realGeneratedManifest,
				Cloud:       "aws", Region: "ap-south-1",
			},
			ClusterID: "prod-us-east",
		},
		{
			Finding: Finding{
				Source: "checkout/reporting-xyz", Destination: "203.0.113.5",
				PathClass: "INTERNET_EGRESS", Confidence: "high",
				BytesTx: 400_000_000, CostHighINR: 5,
			},
			ClusterID: "prod-us-east",
		},
	}
	return mustJSON(rows)
}

func TestSanitizeJSONDropsTheGeneratedManifestEntirely(t *testing.T) {
	s := NewSanitizer(AIModeSanitized)
	out := s.SanitizeJSON(realToolPayload(t))

	if strings.Contains(out, "NetworkPolicy") || strings.Contains(out, "fix_manifest") {
		t.Fatalf("the generated NetworkPolicy manifest must never reach the LLM, got:\n%s", out)
	}
	// The manifest embeds the real namespace and destination IP in YAML --
	// neither may survive anywhere in the payload.
	if strings.Contains(out, "203.0.113.5") {
		t.Fatalf("real destination IP leaked, got:\n%s", out)
	}
}

func TestSanitizeJSONRemovesRealIdentifiersButKeepsEveryNumber(t *testing.T) {
	s := NewSanitizer(AIModeSanitized)
	out := s.SanitizeJSON(realToolPayload(t))

	for _, leaked := range []string{"checkout", "reporting-xyz", "checkout-abc", "prod-us-east", "203.0.113.5"} {
		if strings.Contains(out, leaked) {
			t.Fatalf("real identifier %q leaked to the LLM payload:\n%s", leaked, out)
		}
	}

	// Numeric and categorical fidelity is the whole point of choosing
	// pseudonymisation over summarisation -- assert it survives exactly.
	var got []map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("sanitized payload is not valid JSON: %v\n%s", err, out)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 rows preserved, got %d", len(got))
	}
	if got[0]["cost_high_inr"] != float64(20) {
		t.Fatalf("cost_high_inr changed: %v", got[0]["cost_high_inr"])
	}
	if got[0]["bytes_tx"] != float64(2_000_000_000) {
		t.Fatalf("bytes_tx changed: %v", got[0]["bytes_tx"])
	}
	if got[0]["path_class"] != "INTERNET_EGRESS" || got[0]["confidence"] != "high" {
		t.Fatalf("path class/confidence must pass through untouched: %+v", got[0])
	}
	if got[1]["cost_high_inr"] != float64(5) {
		t.Fatalf("second row cost changed: %v", got[1]["cost_high_inr"])
	}
}

// The traffic-replay feature's whole question is "does another workload in
// the SAME namespace depend on this destination" -- so the sanitizer must
// keep that relationship visible to the model even while hiding the names.
func TestSanitizeJSONPreservesSameNamespaceRelationships(t *testing.T) {
	s := NewSanitizer(AIModeSanitized)
	out := s.SanitizeJSON(realToolPayload(t))

	var got []map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	ns0, _, _ := strings.Cut(got[0]["source"].(string), "/")
	ns1, _, _ := strings.Cut(got[1]["source"].(string), "/")
	if ns0 != ns1 {
		t.Fatalf("both workloads are really in one namespace; aliases must share it, got %q and %q", ns0, ns1)
	}
	if got[0]["source"] == got[1]["source"] {
		t.Fatal("two genuinely different workloads must not collapse to the same alias")
	}
	// Both rows really point at the same destination -- that must stay true.
	if got[0]["destination"] != got[1]["destination"] {
		t.Fatalf("same real destination must map to the same alias, got %v and %v",
			got[0]["destination"], got[1]["destination"])
	}
}

func TestSanitizerRestoreMapsModelOutputBackToRealNames(t *testing.T) {
	s := NewSanitizer(AIModeSanitized)
	out := s.SanitizeJSON(realToolPayload(t))

	var got []map[string]any
	_ = json.Unmarshal([]byte(out), &got)
	alias := got[0]["source"].(string)

	// Simulate the model answering using only the aliases it was given.
	reply := "The workload " + alias + " is your largest cost at INR 20."
	restored := s.Restore(reply)

	if !strings.Contains(restored, "checkout/checkout-abc") {
		t.Fatalf("operator must see their real workload name, got: %s", restored)
	}
	if strings.Contains(restored, alias) {
		t.Fatalf("alias should have been replaced, got: %s", restored)
	}
}

// Guards a real off-by-one class of bug: replacing "workload_1" first
// would corrupt "workload_10" into "<real name>0".
func TestSanitizerRestoreHandlesOverlappingAliasNames(t *testing.T) {
	s := NewSanitizer(AIModeSanitized)
	var last string
	for i := 0; i < 12; i++ {
		last = s.Identifier("workload", "real-workload-"+string(rune('a'+i)))
	}
	if last != "workload_12" {
		t.Fatalf("expected workload_12 as the 12th alias, got %q", last)
	}

	restored := s.Restore("workload_1 and workload_12 differ")
	if !strings.Contains(restored, "real-workload-l") {
		t.Fatalf("workload_12 must map to its own real name, got: %s", restored)
	}
	if strings.Contains(restored, "workload_12") {
		t.Fatalf("workload_12 was not restored: %s", restored)
	}
}

func TestSanitizerIsStableAcrossRepeatedCallsWithinARequest(t *testing.T) {
	s := NewSanitizer(AIModeSanitized)
	a := s.Identifier("workload", "checkout/checkout-abc")
	b := s.Identifier("workload", "checkout/checkout-abc")
	if a != b {
		t.Fatalf("the same real workload must alias identically across tool calls, got %q then %q", a, b)
	}
}

func TestSanitizerRawModePassesEverythingThrough(t *testing.T) {
	s := NewSanitizer(AIModeRaw)
	payload := realToolPayload(t)
	if out := s.SanitizeJSON(payload); out != payload {
		t.Fatal("raw mode must be a genuine opt-out, not a partial sanitization")
	}
	if got := s.Identifier("workload", "checkout/checkout-abc"); got != "checkout/checkout-abc" {
		t.Fatalf("raw mode must not alias, got %q", got)
	}
}

// "We couldn't sanitize it" must never silently become "we sent it".
func TestSanitizeJSONWithholdsPayloadItCannotParse(t *testing.T) {
	s := NewSanitizer(AIModeSanitized)
	out := s.SanitizeJSON("{not valid json")

	if strings.Contains(out, "not valid json") {
		t.Fatalf("unparseable payload must be withheld, not forwarded: %s", out)
	}
	if !strings.Contains(out, "withheld") {
		t.Fatalf("expected an explicit withheld marker, got: %s", out)
	}
}

func TestResolveAIModeDefaultsToSanitizedOnAnythingUnrecognised(t *testing.T) {
	for _, v := range []string{"", "  ", "off", "nonsense", "SANITIZED"} {
		t.Setenv("CHIDRIXX_AI_MODE", v)
		if got := resolveAIMode(); got != AIModeSanitized {
			t.Fatalf("CHIDRIXX_AI_MODE=%q resolved to %q; a typo must never downgrade to raw", v, got)
		}
	}
	for _, v := range []string{"raw", "RAW", " raw "} {
		t.Setenv("CHIDRIXX_AI_MODE", v)
		if got := resolveAIMode(); got != AIModeRaw {
			t.Fatalf("CHIDRIXX_AI_MODE=%q should resolve to raw, got %q", v, got)
		}
	}
}

func TestSanitizerAliasCountIsRealAuditEvidence(t *testing.T) {
	s := NewSanitizer(AIModeSanitized)
	if s.AliasCount() != 0 {
		t.Fatal("a fresh sanitizer has aliased nothing")
	}
	s.SanitizeJSON(realToolPayload(t))
	if s.AliasCount() == 0 {
		t.Fatal("expected a real, non-zero count of aliased identifiers after sanitizing real data")
	}
}
