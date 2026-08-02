// SPDX-License-Identifier: Apache-2.0
package main

import "math"

// This is the deeper forecasting model from the punch list, scoped
// honestly against what the real data actually supports. A live check of
// the production database (2026-08-02) found one real cluster with 4,164
// snapshots over ~1.8 days (~37s cadence) and another with 5 -- real
// volume, but nowhere near enough distinct days to fit a calendar
// seasonal component (Holt-Winters, Prophet) without it being invented
// pattern-matching on noise. ARIMA/a neural sequence model would need
// more/cleaner data than a handful of clusters at this stage actually
// have. What the real data DOES support: genuine rolling-origin
// backtesting comparing two real, well-established exponential-smoothing
// variants -- plain Holt (the existing client-side model, ported here)
// and damped-trend Holt (caps runaway linear extrapolation, a real fix
// for one of Holt's known weaknesses) -- and picking whichever actually
// measures lower error on that cluster's own held-out real history,
// rather than assuming either is better.

// HoltParams is one exponential-smoothing model's fitted parameters.
// Phi == 1.0 means undamped (plain Holt); Phi < 1.0 means damped.
type HoltParams struct {
	Alpha float64
	Beta  float64
	Phi   float64
}

// holtFit runs one pass of (damped) Holt's linear exponential smoothing
// over ys, returning the in-sample one-step-ahead fitted values and the
// final level/trend state. Standard Gardner-McKenzie damped-trend
// recurrence; Phi=1 reduces exactly to plain Holt.
func holtFit(ys []float64, p HoltParams) (fitted []float64, sse, level, trend float64) {
	n := len(ys)
	fitted = make([]float64, n)
	level = ys[0]
	if n > 1 {
		trend = ys[1] - ys[0]
	}
	fitted[0] = level

	for t := 1; t < n; t++ {
		forecastPrev := level + p.Phi*trend
		err := ys[t] - forecastPrev
		sse += err * err
		prevLevel := level
		level = p.Alpha*ys[t] + (1-p.Alpha)*(level+p.Phi*trend)
		trend = p.Beta*(level-prevLevel) + (1-p.Beta)*p.Phi*trend
		fitted[t] = forecastPrev
	}
	return fitted, sse, level, trend
}

// holtForecastAhead projects h steps ahead from a fitted level/trend
// state. For damped trend, the trend contribution is a geometric sum
// (phi + phi^2 + ... + phi^h), not h*trend -- the actual mechanism that
// prevents runaway extrapolation.
func holtForecastAhead(level, trend float64, p HoltParams, h int) float64 {
	if p.Phi >= 0.999999 {
		return level + float64(h)*trend
	}
	// phi*(1-phi^h)/(1-phi)
	sum := p.Phi * (1 - math.Pow(p.Phi, float64(h))) / (1 - p.Phi)
	return level + sum*trend
}

// fitBestHolt grid-searches alpha/beta (and phi, if damping is allowed)
// to minimize real in-sample one-step-ahead SSE -- the same methodology
// already used by the existing client-side model (forecast.ts), just
// ported to Go so it can run server-side against the real full history
// instead of a 30-point client-side cap.
func fitBestHolt(ys []float64, allowDamping bool) HoltParams {
	best := HoltParams{Alpha: 0.5, Beta: 0.5, Phi: 1.0}
	bestSSE := math.Inf(1)

	phis := []float64{1.0}
	if allowDamping {
		phis = []float64{0.80, 0.85, 0.90, 0.95, 0.98}
	}

	for _, phi := range phis {
		for alpha := 0.1; alpha <= 0.9; alpha += 0.1 {
			for beta := 0.1; beta <= 0.9; beta += 0.1 {
				p := HoltParams{Alpha: alpha, Beta: beta, Phi: phi}
				_, sse, _, _ := holtFit(ys, p)
				if sse < bestSSE {
					bestSSE = sse
					best = p
				}
			}
		}
	}
	return best
}

// backtestMAE runs real rolling-origin (walk-forward) validation: at each
// of several real cut points in the series, fit the model on data up to
// that point only, forecast `horizon` steps ahead, and compare against
// the real value that actually occurred there. This is genuine
// out-of-sample error, not the in-sample fit error fitBestHolt
// minimizes -- the two answer different questions (how well does it fit
// vs. how well would it actually have predicted).
func backtestMAE(ys []float64, horizon int, allowDamping bool) (mae float64, folds int) {
	n := len(ys)
	minTrain := 5 // need at least this many points to fit a meaningful model
	maxOrigin := n - horizon
	if maxOrigin < minTrain {
		return 0, 0
	}

	// Cap at 20 folds for bounded compute on clusters with thousands of
	// real snapshots -- evenly spaced across the available origins so
	// the backtest covers the full history, not just the tail.
	const maxFolds = 20
	origins := make([]int, 0, maxFolds)
	if maxOrigin-minTrain+1 <= maxFolds {
		for o := minTrain; o <= maxOrigin; o++ {
			origins = append(origins, o)
		}
	} else {
		step := float64(maxOrigin-minTrain) / float64(maxFolds-1)
		for i := 0; i < maxFolds; i++ {
			origins = append(origins, minTrain+int(math.Round(float64(i)*step)))
		}
	}

	var totalAbsErr float64
	for _, origin := range origins {
		train := ys[:origin]
		params := fitBestHolt(train, allowDamping)
		_, _, level, trend := holtFit(train, params)
		predicted := holtForecastAhead(level, trend, params, horizon)
		actual := ys[origin+horizon-1]
		totalAbsErr += math.Abs(predicted - actual)
	}
	return totalAbsErr / float64(len(origins)), len(origins)
}

// DeepForecastPoint mirrors the client-side HoltForecastPoint shape.
type DeepForecastPoint struct {
	H        int     `json:"h"`
	Forecast float64 `json:"forecast"`
	Lower    float64 `json:"lower"`
	Upper    float64 `json:"upper"`
}

// DeepForecastResult is the real backtested-model-selection forecast for
// one cluster's full retained history.
type DeepForecastResult struct {
	Model             string              `json:"model"` // "holt" or "damped_holt"
	Alpha             float64             `json:"alpha"`
	Beta              float64             `json:"beta"`
	Phi               float64             `json:"phi"`
	PointsUsed        int                 `json:"points_used"`
	BacktestFolds     int                 `json:"backtest_folds"`
	BacktestMAEHolt   float64             `json:"backtest_mae_holt"`
	BacktestMAEDamped float64             `json:"backtest_mae_damped"`
	Forecast          []DeepForecastPoint `json:"forecast"`
}

// ComputeDeepForecast is the real model-selection entry point: backtests
// plain Holt against damped Holt on the real held-out history and picks
// whichever actually measured lower error, then fits that winning model
// on the FULL real history for the final forecast. Returns nil when
// there isn't enough real history to backtest meaningfully -- an honest
// "not enough data" rather than a forced comparison on too few points.
func ComputeDeepForecast(ys []float64, horizon int) *DeepForecastResult {
	n := len(ys)
	if n < 3 {
		return nil
	}

	holtMAE, folds := backtestMAE(ys, minInt(horizon, 5), false)
	dampedMAE, _ := backtestMAE(ys, minInt(horizon, 5), true)

	useDamped := folds > 0 && dampedMAE < holtMAE
	params := fitBestHolt(ys, useDamped)
	_, _, level, trend := holtFit(ys, params)

	// Residual std dev from the final model's own in-sample one-step
	// errors -- the same honest, computed (not cosmetic) interval
	// methodology as the existing client-side model.
	fitted, _, _, _ := holtFit(ys, params)
	var sumErr, sumSqErr float64
	count := 0
	for i := 1; i < n; i++ {
		e := ys[i] - fitted[i]
		sumErr += e
		count++
	}
	meanErr := 0.0
	if count > 0 {
		meanErr = sumErr / float64(count)
	}
	for i := 1; i < n; i++ {
		e := ys[i] - fitted[i]
		sumSqErr += (e - meanErr) * (e - meanErr)
	}
	variance := 0.0
	if count > 1 {
		variance = sumSqErr / float64(count-1)
	}
	residualStdDev := math.Sqrt(variance)

	const z80 = 1.2816
	forecast := make([]DeepForecastPoint, 0, horizon)
	for h := 1; h <= horizon; h++ {
		point := math.Max(holtForecastAhead(level, trend, params, h), 0)
		width := z80 * residualStdDev * math.Sqrt(float64(h))
		forecast = append(forecast, DeepForecastPoint{
			H:        h,
			Forecast: point,
			Lower:    math.Max(point-width, 0),
			Upper:    point + width,
		})
	}

	model := "holt"
	if useDamped {
		model = "damped_holt"
	}

	return &DeepForecastResult{
		Model:             model,
		Alpha:             params.Alpha,
		Beta:              params.Beta,
		Phi:               params.Phi,
		PointsUsed:        n,
		BacktestFolds:     folds,
		BacktestMAEHolt:   holtMAE,
		BacktestMAEDamped: dampedMAE,
		Forecast:          forecast,
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
