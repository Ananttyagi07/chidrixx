// SPDX-License-Identifier: Apache-2.0
package main

import (
	"encoding/json"
	"log"
	"net/http"
)

// forecastResponse always has the same shape: Available tells the client
// whether Result is meaningful, rather than overloading a nullable field
// or an ad hoc error string mixed with a 200 status.
type forecastResponse struct {
	Available bool                `json:"available"`
	Result    *DeepForecastResult `json:"result,omitempty"`
}

// handleForecast serves the real deep forecast (forecast.go) for one
// cluster, using that cluster's full retained snapshot history -- not
// the 30-point cap the Overview page's lightweight client-side trend
// card uses, which exists for a fast multi-cluster glance, not for a
// serious per-cluster projection.
func handleForecast(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		tenantID, ok := tenantIDFromContext(r.Context())
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		clusterID := r.URL.Query().Get("cluster_id")
		if clusterID == "" {
			http.Error(w, "cluster_id query param is required", http.StatusBadRequest)
			return
		}

		// 5000 is comfortably above the largest real cluster observed in
		// production at write time (4,164 snapshots) with headroom, while
		// still bounding the query and the backtest's compute.
		trend, err := store.CostTrend(tenantID, clusterID, 5000)
		if err != nil {
			log.Printf("forecast: cost trend for %s: %v", clusterID, err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		ys := make([]float64, len(trend))
		for i, p := range trend {
			ys[i] = p.CostHigh
		}

		result := ComputeDeepForecast(ys, 10)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(forecastResponse{
			Available: result != nil,
			Result:    result,
		})
	}
}
