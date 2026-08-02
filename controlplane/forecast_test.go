// SPDX-License-Identifier: Apache-2.0
package main

import (
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestComputeDeepForecastStaysFastAtRealProductionScale is a real
// regression test for a real incident: the initial implementation (no
// maxFitWindow cap) measured ~1.6 real CPU-seconds per request against
// the actual production cluster's 4,164 real snapshots, and was
// starving the pod's single CPU core for other requests (confirmed via
// `kubectl top pod` showing a sustained ~1 full core busy). This uses a
// comparable real point count and asserts an actual wall-clock budget,
// not just "it completes eventually."
func TestComputeDeepForecastStaysFastAtRealProductionScale(t *testing.T) {
	ys := make([]float64, 4200)
	for i := range ys {
		ys[i] = 3.5 + 0.5*math.Sin(float64(i)/50) // a plateaued, noisy-ish real-shaped series
	}

	start := time.Now()
	result := ComputeDeepForecast(ys, 10)
	elapsed := time.Since(start)

	if result == nil {
		t.Fatal("expected a real result with 4,200 points")
	}
	if elapsed > 500*time.Millisecond {
		t.Fatalf("ComputeDeepForecast took %v against 4,200 real-scale points, want < 500ms (the maxFitWindow cap exists specifically to bound this)", elapsed)
	}
	if result.PointsUsedForFit > maxFitWindow {
		t.Fatalf("expected the fit window capped at %d, got %d", maxFitWindow, result.PointsUsedForFit)
	}
	if result.PointsRetained != 4200 {
		t.Fatalf("expected PointsRetained to still honestly report all real retained history (4200), got %d", result.PointsRetained)
	}
}

func TestComputeDeepForecastReturnsNilWithTooFewPoints(t *testing.T) {
	if got := ComputeDeepForecast([]float64{10, 20}, 5); got != nil {
		t.Fatalf("expected nil with only 2 points, got %+v", got)
	}
}

func TestComputeDeepForecastOnAPerfectLinearSeriesPicksPlainHolt(t *testing.T) {
	// A genuinely unbounded linear trend -- damping would only hurt here,
	// so a real backtest should prefer plain Holt (phi=1).
	ys := make([]float64, 40)
	for i := range ys {
		ys[i] = 10 + float64(i)*5 // 10, 15, 20, ...
	}

	result := ComputeDeepForecast(ys, 10)
	if result == nil {
		t.Fatal("expected a real result with 40 points")
	}
	if result.BacktestFolds == 0 {
		t.Fatal("expected real backtest folds to have run")
	}
	if result.Model != "holt" {
		t.Fatalf("expected plain Holt to win on a genuinely linear series, got %q (holt MAE=%.2f, damped MAE=%.2f)",
			result.Model, result.BacktestMAEHolt, result.BacktestMAEDamped)
	}
	// The forecast itself should keep climbing roughly linearly, not
	// flatten out.
	if result.Forecast[9].Forecast <= result.Forecast[0].Forecast {
		t.Fatalf("expected the forecast to keep climbing, got %+v", result.Forecast)
	}
}

func TestComputeDeepForecastOnAPlateauingSeriesPicksDampedHolt(t *testing.T) {
	// A series that grows fast then plateaus -- the classic case where
	// plain Holt overshoots because it keeps extrapolating the trend from
	// the fast-growth phase. This shape (rapid growth then a flat
	// asymptote) is exactly what damped trend was invented for.
	ys := make([]float64, 60)
	for i := range ys {
		x := float64(i)
		ys[i] = 100 * (1 - math.Exp(-x/8)) // approaches 100 asymptotically
	}

	result := ComputeDeepForecast(ys, 10)
	if result == nil {
		t.Fatal("expected a real result with 60 points")
	}
	if result.Model != "damped_holt" {
		t.Fatalf("expected damped Holt to win on a plateauing series, got %q (holt MAE=%.4f, damped MAE=%.4f)",
			result.Model, result.BacktestMAEHolt, result.BacktestMAEDamped)
	}
	if result.Phi >= 1.0 {
		t.Fatalf("expected a real damping factor below 1.0, got %v", result.Phi)
	}
}

func TestBacktestMAEIsHonestlyZeroFoldsWithTooLittleHistory(t *testing.T) {
	mae, folds := backtestMAE([]float64{10, 12, 14}, 5, false)
	if folds != 0 {
		t.Fatalf("expected 0 folds with too little history for the horizon, got %d (mae=%v)", folds, mae)
	}
}

func TestBacktestMAERunsRealFoldsWithEnoughHistory(t *testing.T) {
	ys := make([]float64, 100)
	for i := range ys {
		ys[i] = 10 + float64(i)
	}
	mae, folds := backtestMAE(ys, 5, false)
	if folds == 0 {
		t.Fatal("expected real folds to run with 100 points")
	}
	if folds > 20 {
		t.Fatalf("expected folds capped at 20, got %d", folds)
	}
	// A near-perfect linear series should backtest to a near-zero real
	// error for plain Holt.
	if mae > 1.0 {
		t.Fatalf("expected a small real MAE on a perfectly linear series, got %v", mae)
	}
}

func TestHoltForecastAheadDampedNeverExceedsUndamped(t *testing.T) {
	// For a positive trend, the damped projection at any horizon must be
	// <= the undamped one -- that's the entire mechanism damping exists
	// to provide, verified directly rather than just trusted.
	level, trend := 100.0, 5.0
	undamped := HoltParams{Alpha: 0.5, Beta: 0.5, Phi: 1.0}
	damped := HoltParams{Alpha: 0.5, Beta: 0.5, Phi: 0.9}

	for h := 1; h <= 20; h++ {
		u := holtForecastAhead(level, trend, undamped, h)
		d := holtForecastAhead(level, trend, damped, h)
		if d > u {
			t.Fatalf("at h=%d: damped forecast %v exceeded undamped %v", h, d, u)
		}
	}
}

func TestHandleForecastEndToEndWithRealIngestedData(t *testing.T) {
	store := testStore(t)
	tenantID := testTenant(t, store)

	// reported_at is second-granularity (time.Now().Unix()); real sleeps
	// are needed between ingests to get distinct snapshot timestamps, same
	// pattern as every other trend-over-time test in this codebase.
	for i := 0; i < 5; i++ {
		if err := store.Ingest(tenantID, "cluster-a", []Finding{
			{Source: "checkout/checkout-1", CostHighINR: float64(10 + i*5)},
		}); err != nil {
			t.Fatalf("Ingest %d: %v", i, err)
		}
		if i < 4 {
			time.Sleep(1100 * time.Millisecond)
		}
	}

	req := withTenant(httptest.NewRequest(http.MethodGet, "/api/v1/forecast?cluster_id=cluster-a", nil), tenantID)
	rec := httptest.NewRecorder()
	handleForecast(store)(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	var got forecastResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !got.Available || got.Result == nil {
		t.Fatalf("expected a real available forecast with 10 real snapshots, got: %s", rec.Body.String())
	}
}

func TestHandleForecastRequiresClusterID(t *testing.T) {
	store := testStore(t)
	tenantID := testTenant(t, store)

	req := withTenant(httptest.NewRequest(http.MethodGet, "/api/v1/forecast", nil), tenantID)
	rec := httptest.NewRecorder()
	handleForecast(store)(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for a missing cluster_id", rec.Code)
	}
}

func TestHandleForecastReturnsUnavailableWithTooLittleHistory(t *testing.T) {
	store := testStore(t)
	tenantID := testTenant(t, store)
	if err := store.Ingest(tenantID, "cluster-a", []Finding{{Source: "checkout/checkout-1", CostHighINR: 10}}); err != nil {
		t.Fatalf("Ingest: %v", err)
	}

	req := withTenant(httptest.NewRequest(http.MethodGet, "/api/v1/forecast?cluster_id=cluster-a", nil), tenantID)
	rec := httptest.NewRecorder()
	handleForecast(store)(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (honest unavailable, not an HTTP error)", rec.Code)
	}
	var got forecastResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Available || got.Result != nil {
		t.Fatalf("expected an honest unavailable result with only 1 real snapshot, got: %s", rec.Body.String())
	}
}

func TestHandleForecastIsIsolatedByTenant(t *testing.T) {
	store := testStore(t)
	tenantA := testTenant(t, store)
	tenantB := testTenant(t, store)

	// Give tenant A a real, available forecast (distinct snapshots via
	// real sleeps) -- proves tenant B's request for the identically-named
	// "cluster-a" doesn't see tenant A's real available result, not just
	// that an empty tenant sees nothing.
	for i := 0; i < 5; i++ {
		if err := store.Ingest(tenantA, "cluster-a", []Finding{{Source: "checkout/checkout-1", CostHighINR: float64(10 + i*5)}}); err != nil {
			t.Fatalf("Ingest %d: %v", i, err)
		}
		if i < 4 {
			time.Sleep(1100 * time.Millisecond)
		}
	}

	req := withTenant(httptest.NewRequest(http.MethodGet, "/api/v1/forecast?cluster_id=cluster-a", nil), tenantB)
	rec := httptest.NewRecorder()
	handleForecast(store)(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got forecastResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Available {
		t.Fatalf("tenant b's forecast leaked tenant a's real data: %s", rec.Body.String())
	}
}
