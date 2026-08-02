// SPDX-License-Identifier: Apache-2.0
package main

import (
	"context"
	"fmt"
)

const anomalyNarratorSystemPrompt = `You turn one real detected cost anomaly into a short, plain-English explanation for a Kubernetes operator. You are given the real numbers already computed -- you do not have tools, and must not invent any fact not present in the data given to you.

Rules:
- 2-3 sentences, no more.
- State the real cost jump (previous -> current, and the ratio) in plain language.
- If a likely-cause deploy event is given, mention it and explicitly call it a correlation worth checking, never proven causation -- the data was already labeled this way, don't upgrade its certainty.
- If no likely-cause event is given, say plainly that no correlated deploy event was found, and that it may be organic growth or an unobserved change (e.g. outside Kubernetes, or before this control plane started watching deploy events).
- Never invent a dollar/rupee figure, workload name, or event not present in the input.`

// narrateAnomaly turns one real, already-computed Anomaly into a short
// natural-language explanation. Deliberately no tool-calling here -- all
// the real data (the two costs, the ratio, the optional real deploy
// event) is already known, so this is a single completion, not an
// agentic loop, and cheaper/faster because of it.
func narrateAnomaly(ctx context.Context, groq *GroqClient, a Anomaly) (string, error) {
	prompt := fmt.Sprintf(
		"Cluster: %s\nPrevious cost: INR %.2f\nCurrent cost: INR %.2f\nGrowth ratio: %.2fx\n",
		a.ClusterID, a.PreviousCostINR, a.CurrentCostINR, a.GrowthRatio,
	)
	if a.LikelyCause != nil {
		prompt += fmt.Sprintf(
			"Likely-cause deploy event: namespace=%s name=%s reason=%s message=%q\n",
			a.LikelyCause.Namespace, a.LikelyCause.Name, a.LikelyCause.Reason, a.LikelyCause.Message,
		)
	} else {
		prompt += "Likely-cause deploy event: none found\n"
	}

	msg, err := groq.Complete(ctx, []ChatMessage{
		{Role: "system", Content: anomalyNarratorSystemPrompt},
		{Role: "user", Content: prompt},
	}, nil)
	if err != nil {
		return "", err
	}
	return msg.Content, nil
}
