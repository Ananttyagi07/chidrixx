// SPDX-License-Identifier: Apache-2.0
package main

import (
	"encoding/json"
	"log"
	"net/http"
)

type budgetResponse struct {
	BudgetINR float64 `json:"budget_inr"`
	IsSet     bool    `json:"is_set"`
}

type setBudgetRequest struct {
	BudgetINR float64 `json:"budget_inr"`
}

// handleBudget serves the one real, user-set budget figure this control
// plane supports (see budgetSettingKey in store.go): GET reads it, POST
// sets it. There is no fabricated "AI-recommended budget" here — only
// what the user actually typed in.
func handleBudget(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID, ok := tenantIDFromContext(r.Context())
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		switch r.Method {
		case http.MethodGet:
			amount, isSet, err := store.GetBudget(tenantID)
			if err != nil {
				log.Printf("budget: get: %v", err)
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(budgetResponse{BudgetINR: amount, IsSet: isSet})

		case http.MethodPost:
			// Setting the budget is the one write a logged-in user can make
			// (beyond ingest, which agents do with a separate API token) --
			// gated to admin by requireAdmin at the routing layer in main.go,
			// not re-checked here.
			var req setBudgetRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
				return
			}

			if req.BudgetINR < 0 {
				http.Error(w, "budget_inr must not be negative", http.StatusBadRequest)
				return
			}

			if err := store.SetBudget(tenantID, req.BudgetINR); err != nil {
				log.Printf("budget: set: %v", err)
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(budgetResponse{BudgetINR: req.BudgetINR, IsSet: true})

		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}
