// SPDX-License-Identifier: Apache-2.0
package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
)

type outcomeResponse struct {
	ClusterID               string   `json:"cluster_id"`
	Source                  string   `json:"source"`
	Destination             string   `json:"destination"`
	PathClass               string   `json:"path_class"`
	FixHint                 string   `json:"fix_hint"`
	PredictedSavingsLowINR  float64  `json:"predicted_savings_low_inr"`
	PredictedSavingsHighINR float64  `json:"predicted_savings_high_inr"`
	CostBeforeINR           float64  `json:"cost_before_inr"`
	FirstShownAt            string   `json:"first_shown_at"`
	LastShownAt             string   `json:"last_shown_at"`
	AppliedAt               *string  `json:"applied_at"`
	CostAfterINR            *float64 `json:"cost_after_inr"`
	MeasuredAt              *string  `json:"measured_at"`
}

type markAppliedRequest struct {
	ClusterID   string `json:"cluster_id"`
	Source      string `json:"source"`
	Destination string `json:"destination"`
	PathClass   string `json:"path_class"`
}

const timeFormat = "2006-01-02T15:04:05Z07:00"

// handleOutcomes is the real closed-loop tracking surface: GET lists every
// recommendation ever shown plus whatever's been observed about whether it
// worked, POST /apply marks one as applied by a real operator action. No
// AI here -- this is the plumbing that makes any future AI feature
// trainable on real outcomes instead of guesses.
func handleOutcomes(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID, ok := tenantIDFromContext(r.Context())
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		switch r.Method {
		case http.MethodGet:
			outcomes, err := store.ListRecommendationOutcomes(tenantID)
			if err != nil {
				log.Printf("outcomes: list: %v", err)
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			resp := make([]outcomeResponse, 0, len(outcomes))
			for _, o := range outcomes {
				item := outcomeResponse{
					ClusterID:               o.ClusterID,
					Source:                  o.Source,
					Destination:             o.Destination,
					PathClass:               o.PathClass,
					FixHint:                 o.FixHint,
					PredictedSavingsLowINR:  o.PredictedSavingsLowINR,
					PredictedSavingsHighINR: o.PredictedSavingsHighINR,
					CostBeforeINR:           o.CostBeforeINR,
					FirstShownAt:            o.FirstShownAt.Format(timeFormat),
					LastShownAt:             o.LastShownAt.Format(timeFormat),
					CostAfterINR:            o.CostAfterINR,
				}
				if o.AppliedAt != nil {
					s := o.AppliedAt.Format(timeFormat)
					item.AppliedAt = &s
				}
				if o.MeasuredAt != nil {
					s := o.MeasuredAt.Format(timeFormat)
					item.MeasuredAt = &s
				}
				resp = append(resp, item)
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)

		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

// handleMarkOutcomeApplied is a separate route (rather than a method on
// handleOutcomes) so it can require the finding's full identity in the
// body, not a query param, matching the identity the outcomes table is
// actually keyed on.
func handleMarkOutcomeApplied(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		tenantID, ok := tenantIDFromContext(r.Context())
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		var req markAppliedRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}
		if req.ClusterID == "" || req.Source == "" || req.Destination == "" || req.PathClass == "" {
			http.Error(w, "cluster_id, source, destination, and path_class are all required", http.StatusBadRequest)
			return
		}

		err := store.MarkRecommendationApplied(tenantID, req.ClusterID, req.Source, req.Destination, req.PathClass)
		if errors.Is(err, ErrOutcomeNotFound) {
			http.Error(w, "no recommendation matching that identity has been shown for this tenant", http.StatusNotFound)
			return
		}
		if err != nil {
			log.Printf("outcomes: mark applied: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
