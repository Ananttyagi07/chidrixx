// SPDX-License-Identifier: Apache-2.0
package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"sort"
	"strings"
)

// This is the boundary between real tenant infrastructure data and a
// third-party LLM provider. Before this existed, every tool result went to
// Groq verbatim -- real namespace names, real pod names, real destination
// IPs, and (via fix_manifest) entire generated NetworkPolicy YAML
// documents. For a product whose agent already runs at kernel level in
// someone else's cluster, "what leaves my environment" is the first
// question a security review asks, and the honest answer needed to be
// better than "your whole topology".
//
// The approach is deliberately PSEUDONYMISATION, not summarisation. A
// summary would lose real numeric fidelity and make the assistant worse.
// Aliasing identifiers keeps every byte count, cost, ratio, path class and
// confidence value exactly as measured -- the model reasons on identical
// numbers -- while the identity of the workloads stays inside the cluster.
//
// Relational structure is preserved on purpose: two workloads that really
// share a namespace still share one aliased namespace, so the model can
// still reason about "these are in the same namespace" (which is exactly
// what the traffic-replay collateral logic is about). Only the names are
// opaque.

// AIMode controls what reaches the LLM provider.
type AIMode string

const (
	// AIModeSanitized is the default: alias identifiers, drop manifests.
	AIModeSanitized AIMode = "sanitized"
	// AIModeRaw sends real identifiers. Opt-in, for operators who would
	// rather have the model see real names and accept the exposure.
	AIModeRaw AIMode = "raw"
)

// resolveAIMode reads CHIDRIXX_AI_MODE. Anything unset or unrecognised
// resolves to sanitized -- the safe direction. A typo must never silently
// downgrade to sending real infrastructure names.
func resolveAIMode() AIMode {
	if strings.EqualFold(strings.TrimSpace(os.Getenv("CHIDRIXX_AI_MODE")), string(AIModeRaw)) {
		return AIModeRaw
	}
	return AIModeSanitized
}

// normalizeKey folds a JSON key to a canonical form so both naming
// conventions this codebase really emits are matched by one entry. Go
// structs marshal untagged fields by their Go name, so FindingRow embeds
// tagged fields as `source`/`destination` but emits `ClusterID` for its
// own untagged field -- a real leak found by testing against the actual
// marshalled payload rather than an idealised one.
func normalizeKey(k string) string {
	return strings.ToLower(strings.ReplaceAll(k, "_", ""))
}

// sanitizedKeys are the JSON fields carrying real infrastructure identity,
// keyed by normalized name. Anything not listed here (byte counts, costs,
// path classes, confidence, timestamps, ratios) passes through untouched
// and at full fidelity.
var sanitizedKeys = map[string]string{
	"source":                "workload",
	"destination":           "endpoint",
	"srcworkload":           "workload",
	"dstworkloadorendpoint": "endpoint",
	"workload":              "workload",
	"clusterid":             "cluster",
	"namespace":             "namespace",
	"name":                  "resource",
	"team":                  "team",
}

// droppedKeys are removed outright: they carry real infrastructure detail
// and contribute nothing to the model's reasoning about cost. fix_manifest
// is a complete NetworkPolicy YAML (real namespace, real destination IP)
// that a human applies with kubectl -- the model never needs to read it,
// and sending it was pure exposure with a token cost attached.
var droppedKeys = map[string]bool{
	"fixmanifest": true,
}

// Sanitizer holds one request's stable alias mapping. Stable within a
// request means the same real workload always maps to the same alias, so
// the model can correlate across several tool calls; scoped to a request
// means aliases are not a long-lived pseudonymous identifier.
type Sanitizer struct {
	mode     AIMode
	forward  map[string]string
	reverse  map[string]string
	counters map[string]int
}

func NewSanitizer(mode AIMode) *Sanitizer {
	return &Sanitizer{
		mode:     mode,
		forward:  make(map[string]string),
		reverse:  make(map[string]string),
		counters: make(map[string]int),
	}
}

// Enabled reports whether this sanitizer actually rewrites anything.
func (s *Sanitizer) Enabled() bool { return s != nil && s.mode == AIModeSanitized }

// alias returns a stable alias for one real value under a given kind.
func (s *Sanitizer) alias(kind, real string) string {
	if real == "" {
		return real
	}
	key := kind + "\x00" + real
	if a, ok := s.forward[key]; ok {
		return a
	}
	s.counters[kind]++
	a := fmt.Sprintf("%s_%d", kind, s.counters[kind])
	s.forward[key] = a
	// Reverse is keyed by alias alone: Restore rewrites model output text
	// where the kind isn't recoverable from context.
	s.reverse[a] = real
	return a
}

// Identifier aliases one real identifier of a given kind, preserving the
// real "namespace/pod" shape so same-namespace relationships survive. A
// bare IP or non-namespaced value becomes a single opaque alias.
func (s *Sanitizer) Identifier(kind, real string) string {
	if !s.Enabled() || real == "" {
		return real
	}
	// "namespace/pod" -- alias each half separately so two workloads that
	// genuinely share a namespace still visibly share one.
	if ns, pod, found := strings.Cut(real, "/"); found && ns != "" && pod != "" {
		if net.ParseIP(real) == nil {
			return s.alias("namespace", ns) + "/" + s.alias("workload", pod)
		}
	}
	return s.alias(kind, real)
}

// SanitizeJSON walks a tool result and rewrites identity-bearing fields,
// leaving every numeric and categorical value untouched. It walks the
// decoded structure rather than regexing the text, so it cannot corrupt
// JSON or partially match a number.
//
// On a decode failure the raw input is NOT passed through -- an
// unparseable payload is replaced with an explicit error object, because
// "we couldn't sanitize it" must never silently become "we sent it".
func (s *Sanitizer) SanitizeJSON(raw string) string {
	if !s.Enabled() {
		return raw
	}
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return `{"error":"tool result could not be sanitized and was withheld"}`
	}
	out, err := json.Marshal(s.walk(v))
	if err != nil {
		return `{"error":"tool result could not be sanitized and was withheld"}`
	}
	return string(out)
}

func (s *Sanitizer) walk(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			nk := normalizeKey(k)
			if droppedKeys[nk] {
				continue
			}
			if kind, ok := sanitizedKeys[nk]; ok {
				if str, isStr := val.(string); isStr {
					out[k] = s.Identifier(kind, str)
					continue
				}
			}
			out[k] = s.walk(val)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, val := range t {
			out[i] = s.walk(val)
		}
		return out
	default:
		return v
	}
}

// Restore maps aliases in the model's own output back to real names, so
// the operator reads their actual workload names even though the model
// only ever saw aliases. Longest aliases first, so "workload_1" can never
// be partially rewritten by a match on "workload_10".
func (s *Sanitizer) Restore(text string) string {
	if !s.Enabled() || text == "" {
		return text
	}
	aliases := make([]string, 0, len(s.reverse))
	for a := range s.reverse {
		aliases = append(aliases, a)
	}
	sort.Slice(aliases, func(i, j int) bool { return len(aliases[i]) > len(aliases[j]) })

	for _, a := range aliases {
		text = strings.ReplaceAll(text, a, s.reverse[a])
	}
	return text
}

// AliasCount reports how many distinct real identifiers were aliased --
// recorded as real audit evidence that sanitization actually ran on a
// given request, rather than being assumed.
func (s *Sanitizer) AliasCount() int {
	if s == nil {
		return 0
	}
	return len(s.forward)
}
