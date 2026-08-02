// SPDX-License-Identifier: Apache-2.0
package main

import (
	"encoding/json"
	"log"
	"net/http"
	"sort"
)

// dashboardSummaryFindingsLimit bounds how many of the tenant's latest-
// snapshot rows get fetched once and reused for every aggregate this
// endpoint computes (Summary/SpendByClass/SpendByCloud/SpendByTeam/
// TopFixes) -- comfortably above the largest real combined snapshot
// fan-out observed in production (671 rows across 2 clusters at write
// time) with headroom, while still bounding the query.
const dashboardSummaryFindingsLimit = 5000

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
	SpendByTeam  []TeamSpend          `json:"spend_by_team"`
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
		tenantID, ok := tenantIDFromContext(r.Context())
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		// Fetched once, reused for every aggregate below (Summary,
		// SpendByClass, SpendByCloud, SpendByTeam, TopFixes) instead of
		// each independently re-querying "the latest snapshot per
		// cluster" -- see summary.go's comment for why this mattered
		// against real production data.
		findings, err := store.LatestFindings(tenantID, dashboardSummaryFindingsLimit)
		if err != nil {
			log.Printf("dashboard-summary: findings: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		trend, err := store.GlobalTrend(tenantID, 30)
		if err != nil {
			log.Printf("dashboard-summary: global trend: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		clusters, err := store.Clusters(tenantID)
		if err != nil {
			log.Printf("dashboard-summary: clusters: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		clusterIDs := make([]string, len(clusters))
		for i, c := range clusters {
			clusterIDs[i] = c.ClusterID
		}

		summary := computeSummary(findings, len(clusters))
		spendByClass := computeSpendByClass(findings)
		spendByCloud := computeSpendByCloud(findings)

		clusterViews := make([]clusterSummaryView, 0, len(clusters))
		for _, c := range clusters {
			clusterTrend, err := store.CostTrend(tenantID, c.ClusterID, 20)
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

		teamOwnership, err := store.ListTeamOwnership(tenantID)
		if err != nil {
			log.Printf("dashboard-summary: team ownership: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		spendByTeam := computeSpendByTeam(findings, teamOwnership)

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

		// Best-effort, like deploy-event ingestion: logging what got shown
		// is bookkeeping for the outcome-tracking dataset, not something a
		// dashboard load should ever fail over.
		if err := store.RecordRecommendationsShown(tenantID, topFixes); err != nil {
			log.Printf("dashboard-summary: record recommendations shown: %v", err)
		}

		anomalies, err := detectAnomalies(store, tenantID, clusterIDs)
		if err != nil {
			log.Printf("dashboard-summary: anomalies: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		resp := dashboardSummaryResponse{
			Summary:      summary,
			SpendByClass: spendByClass,
			SpendByCloud: spendByCloud,
			SpendByTeam:  spendByTeam,
			Trend:        trend,
			Clusters:     clusterViews,
			TopFixes:     topFixes,
			Anomalies:    anomalies,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}
