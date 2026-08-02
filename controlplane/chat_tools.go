// SPDX-License-Identifier: Apache-2.0
package main

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// parseLenientInt reads an integer-ish tool argument that the model may
// have generated as either a JSON number or a JSON string -- Groq's
// Llama models observably stringify numeric tool-call arguments often
// enough in practice that declaring the schema type as "integer" causes
// Groq's own server-side schema validation to reject the model's tool
// call outright before this code ever sees it. Declaring the schema type
// as "string" instead and parsing leniently here avoids that failure
// class entirely.
func parseLenientInt(raw json.RawMessage, fallback int) int {
	if len(raw) == 0 {
		return fallback
	}
	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		if n, err := strconv.Atoi(asString); err == nil {
			return n
		}
		return fallback
	}
	var asNumber float64
	if err := json.Unmarshal(raw, &asNumber); err == nil {
		return int(asNumber)
	}
	return fallback
}

// chatTool pairs an OpenAI-compatible tool declaration with the real,
// tenant-scoped function that answers it. Every tool call the model makes
// is executed against this exact tenant's real data -- there is no way
// for the model to see another tenant's numbers, because tenantID is
// bound into the closure at construction time, not read from the model's
// output.
type chatTool struct {
	def  ToolDef
	call func(args json.RawMessage) (string, error)
}

func rawSchema(properties string) json.RawMessage {
	return json.RawMessage(fmt.Sprintf(`{"type":"object","properties":{%s}}`, properties))
}

// buildChatTools returns the real tool surface the assistant can call for
// one tenant. Every function here wraps an already-existing, already-
// tested store method or computation -- no new business logic, just a
// grounding layer so the model answers from real numbers instead of
// guessing.
func buildChatTools(store *Store, tenantID int64) []chatTool {
	return []chatTool{
		{
			def: ToolDef{
				Type: "function",
				Function: ToolFunction{
					Name:        "get_top_fixes",
					Description: "Get the current real flagged cost-saving opportunities (top fixes) across this tenant's clusters, ranked by cost. Use this to answer 'what should I fix first' or questions about specific waste.",
					Parameters:  rawSchema(`"limit":{"type":"string","description":"max rows to return, as a numeric string, default 10"}`),
				},
			},
			call: func(args json.RawMessage) (string, error) {
				var req map[string]json.RawMessage
				_ = json.Unmarshal(args, &req)
				limit := parseLenientInt(req["limit"], 10)
				if limit <= 0 {
					limit = 10
				}
				findings, err := store.LatestFindings(tenantID, 500)
				if err != nil {
					return "", err
				}
				top := make([]FindingRow, 0, limit)
				for _, f := range findings {
					if f.FixHint != "" {
						top = append(top, f)
						if len(top) >= limit {
							break
						}
					}
				}
				return mustJSON(top), nil
			},
		},
		{
			def: ToolDef{
				Type: "function",
				Function: ToolFunction{
					Name:        "get_anomalies",
					Description: "Get real detected cost anomalies (clusters whose cost roughly doubled or more between the two most recent snapshots), each with a likely root cause if a real Kubernetes deploy event correlates. Use this to answer 'why did my bill spike'.",
					Parameters:  rawSchema(``),
				},
			},
			call: func(_ json.RawMessage) (string, error) {
				anomalies, err := detectAnomalies(store, tenantID)
				if err != nil {
					return "", err
				}
				return mustJSON(anomalies), nil
			},
		},
		{
			def: ToolDef{
				Type: "function",
				Function: ToolFunction{
					Name:        "get_workload_growth",
					Description: "Get the real workloads whose cost has grown the most over their full retained history, each with related deploy events in its own namespace if any correlate. Use this for 'what's driving my cost trend up' or 'which workload is growing fastest'.",
					Parameters:  rawSchema(`"top_n":{"type":"string","description":"how many workloads to return, as a numeric string, default 10"}`),
				},
			},
			call: func(args json.RawMessage) (string, error) {
				var req map[string]json.RawMessage
				_ = json.Unmarshal(args, &req)
				topN := parseLenientInt(req["top_n"], 10)
				if topN <= 0 {
					topN = 10
				}
				growth, err := store.WorkloadCostGrowth(tenantID, topN)
				if err != nil {
					return "", err
				}
				return mustJSON(growth), nil
			},
		},
		{
			def: ToolDef{
				Type: "function",
				Function: ToolFunction{
					Name:        "get_spend_by_team",
					Description: "Get real cost broken down by team, using this tenant's configured namespace-to-team ownership mappings. Namespaces with no configured owner show up as 'Unassigned'. Use this for 'which team is spending the most'.",
					Parameters:  rawSchema(``),
				},
			},
			call: func(_ json.RawMessage) (string, error) {
				findings, err := store.LatestFindings(tenantID, 5000)
				if err != nil {
					return "", err
				}
				ownership, err := store.ListTeamOwnership(tenantID)
				if err != nil {
					return "", err
				}
				return mustJSON(computeSpendByTeam(findings, ownership)), nil
			},
		},
		{
			def: ToolDef{
				Type: "function",
				Function: ToolFunction{
					Name:        "get_recommendation_outcomes",
					Description: "Get the real history of fix recommendations shown to this tenant, whether an operator marked them applied, and the observed before/after cost where measured. Use this for 'did the fixes I applied actually work' or 'what's our track record'.",
					Parameters:  rawSchema(``),
				},
			},
			call: func(_ json.RawMessage) (string, error) {
				outcomes, err := store.ListRecommendationOutcomes(tenantID)
				if err != nil {
					return "", err
				}
				return mustJSON(outcomes), nil
			},
		},
	}
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(b)
}

func toolDefs(tools []chatTool) []ToolDef {
	defs := make([]ToolDef, len(tools))
	for i, t := range tools {
		defs[i] = t.def
	}
	return defs
}

func findTool(tools []chatTool, name string) (chatTool, bool) {
	for _, t := range tools {
		if t.def.Function.Name == name {
			return t, true
		}
	}
	return chatTool{}, false
}
