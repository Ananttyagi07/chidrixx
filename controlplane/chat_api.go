// SPDX-License-Identifier: Apache-2.0
package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
)

const chatSystemPrompt = `You are the chidrixx cost assistant, embedded in a real Kubernetes network-cost dashboard. You answer questions about ONE real tenant's real infrastructure cost data using the tools available to you.

Rules:
- Always call a tool before answering any question about cost, findings, anomalies, workloads, teams, or fix outcomes. Never guess or invent a number.
- If a tool returns no data, say so honestly ("no anomalies detected in the current data" / "no fixes have been applied yet") rather than making something up.
- Quote real figures from tool results (INR amounts, workload names, cluster IDs) exactly as returned.
- Keep answers concise and actionable -- this is an operator glancing at a dashboard, not reading a report.
- If asked something with no relevant tool (e.g. general chit-chat, unrelated topics), say this assistant only answers questions about this tenant's real cost data.`

const maxChatToolRounds = 5

type chatHistoryItem struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Message string            `json:"message"`
	History []chatHistoryItem `json:"history"`
}

type chatResponse struct {
	Reply string `json:"reply"`
}

// handleChat is the real grounded chat assistant: every question is
// answered by having an LLM call real, tenant-scoped tools (see
// chat_tools.go) against this tenant's actual data, never by inventing
// numbers. Read-only -- same access level as the GET endpoints these
// tools wrap, so any authenticated tenant member can use it, not just
// admins.
func handleChat(store *Store, groq *GroqClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if groq == nil {
			http.Error(w, "chat assistant is not configured (no GROQ_API_KEY set)", http.StatusServiceUnavailable)
			return
		}
		tenantID, ok := tenantIDFromContext(r.Context())
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		var req chatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}
		if req.Message == "" {
			http.Error(w, "message is required", http.StatusBadRequest)
			return
		}

		tools := buildChatTools(store, tenantID)
		defs := toolDefs(tools)

		messages := []ChatMessage{{Role: "system", Content: chatSystemPrompt}}
		for _, h := range req.History {
			messages = append(messages, ChatMessage{Role: h.Role, Content: h.Content})
		}
		messages = append(messages, ChatMessage{Role: "user", Content: req.Message})

		reply, err := runChatLoop(r.Context(), groq, tools, defs, messages)
		if err != nil {
			log.Printf("chat: %v", err)
			http.Error(w, "the chat assistant hit an error -- try again", http.StatusBadGateway)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(chatResponse{Reply: reply})
	}
}

// runChatLoop drives the real tool-calling round trip: ask the model,
// and if it asks for a tool instead of answering, execute the tool
// against real tenant-scoped data and feed the result back, up to
// maxChatToolRounds times before giving up honestly.
func runChatLoop(ctx context.Context, groq *GroqClient, tools []chatTool, defs []ToolDef, messages []ChatMessage) (string, error) {
	for i := 0; i < maxChatToolRounds; i++ {
		msg, err := completeWithRetry(ctx, groq, messages, defs)
		if err != nil {
			return "", err
		}
		if len(msg.ToolCalls) == 0 {
			return msg.Content, nil
		}

		messages = append(messages, msg)
		for _, tc := range msg.ToolCalls {
			var result string
			tool, found := findTool(tools, tc.Function.Name)
			if !found {
				result = `{"error":"unknown tool"}`
			} else {
				out, err := tool.call([]byte(tc.Function.Arguments))
				if err != nil {
					result = mustJSON(map[string]string{"error": err.Error()})
				} else {
					result = out
				}
			}
			messages = append(messages, ChatMessage{
				Role:       "tool",
				ToolCallID: tc.ID,
				Content:    result,
			})
		}
	}
	return "I wasn't able to finish answering that within a reasonable number of steps -- try asking something more specific.", nil
}

// completeWithRetry retries a single completion once. Groq validates a
// model's own tool-call arguments against the declared JSON schema
// server-side and rejects the whole response with a 400 if the model
// generated a malformed argument (e.g. a string where an integer was
// declared) -- a real, observed failure mode of smaller/faster models,
// not something our request caused. Resampling the same request often
// produces a conforming tool call on the second try.
func completeWithRetry(ctx context.Context, groq *GroqClient, messages []ChatMessage, defs []ToolDef) (ChatMessage, error) {
	msg, err := groq.Complete(ctx, messages, defs)
	if err == nil {
		return msg, nil
	}
	return groq.Complete(ctx, messages, defs)
}
