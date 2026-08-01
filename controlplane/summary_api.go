// SPDX-License-Identifier: Apache-2.0
package main

import (
	"encoding/json"
	"log"
	"net/http"
	"sort"
)

// clusterSummaryView is one cluster's headline plus its own cost trend,
// for a per-cluster sparkline.
type clusterSummaryView struct {
	ClusterSummary
	Trend []CostTrendPoint `json:"trend"`
}

// dashboardSummaryResponse is everything the dashboard SPA needs in one
// call — every field here is a real aggregate/count over ingested data,
// nothing modeled beyond the price-book cost bounds the agent already
// computes.
type dashboardSummaryResponse struct {
	Summary      Summary              `json:"summary"`
	SpendByClass []ClassSpend         `json:"spend_by_class"`
	SpendByCloud []CloudSpend         `json:"spend_by_cloud"`
	Trend        []CostTrendPoint     `json:"trend"`
	Clusters     []clusterSummaryView `json:"clusters"`
	TopFixes     []FindingRow         `json:"top_fixes"`
	Anomalies    []Anomaly            `json:"anomalies"`
}

// handleDashboardSummary serves the aggregate JSON the React dashboard
// renders. Kept as one call (rather than one request per widget) since
// the SPA needs all of it to paint its first frame.
func handleDashboardSummary(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		summary, err := store.Summary()
		if err != nil {
			log.Printf("dashboard-summary: summary: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		spendByClass, err := store.SpendByClass()
		if err != nil {
			log.Printf("dashboard-summary: spend by class: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		trend, err := store.GlobalTrend(30)
		if err != nil {
			log.Printf("dashboard-summary: global trend: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		clusters, err := store.Clusters()
		if err != nil {
			log.Printf("dashboard-summary: clusters: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		clusterViews := make([]clusterSummaryView, 0, len(clusters))
		for _, c := range clusters {
			clusterTrend, err := store.CostTrend(c.ClusterID, 20)
			if err != nil {
				log.Printf("dashboard-summary: cost trend for %s: %v", c.ClusterID, err)
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}

			clusterViews = append(clusterViews, clusterSummaryView{
				ClusterSummary: c,
				Trend:          clusterTrend,
			})
		}

		findings, err := store.LatestFindings(500)
		if err != nil {
			log.Printf("dashboard-summary: findings: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		topFixes := make([]FindingRow, 0, len(findings))
		for _, f := range findings {
			if f.FixHint != "" {
				topFixes = append(topFixes, f)
			}
		}
		sort.Slice(topFixes, func(i, j int) bool {
			return topFixes[i].CostHighINR > topFixes[j].CostHighINR
		})
		if len(topFixes) > 10 {
			topFixes = topFixes[:10]
		}

		anomalies, err := detectAnomalies(store)
		if err != nil {
			log.Printf("dashboard-summary: anomalies: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		spendByCloud, err := store.SpendByCloud()
		if err != nil {
			log.Printf("dashboard-summary: spend by cloud: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		resp := dashboardSummaryResponse{
			Summary:      summary,
			SpendByClass: spendByClass,
			SpendByCloud: spendByCloud,
			Trend:        trend,
			Clusters:     clusterViews,
			TopFixes:     topFixes,
			Anomalies:    anomalies,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}
