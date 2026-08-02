// SPDX-License-Identifier: Apache-2.0
package main

import (
	"encoding/json"
	"log"
	"net/http"
)

// forecastQueryWindow bounds how many of a cluster's real snapshots this
// endpoint pulls from the database. See its use below for why this isn't
// unbounded despite flow_aggregate itself never being pruned.
const forecastQueryWindow = 1500

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

		// forecastQueryWindow (see const below) bounds how many real
		// snapshots this query pulls -- large enough to give the 20
		// backtest folds a meaningful spread across real history, but
		// nowhere near unbounded: any single Holt fit only ever trains on
		// maxFitWindow (400) of whatever's returned anyway (see
		// forecast.go), so pulling thousands more than that just makes the
		// SQL query itself slower for no benefit to the actual model. A
		// real production cluster's flow_aggregate table is never pruned
		// and grows forever (by design), so this query's own cost scales
		// with total ingested history, independent of the Go-side
		// windowing fix -- found live: at write time a real cluster's
		// query alone took several real seconds once its table grew large
		// enough to exceed the old (5000-point) request.
		trend, err := store.CostTrend(tenantID, clusterID, forecastQueryWindow)
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
