// SPDX-License-Identifier: Apache-2.0
package main

// OutcomeDatasetStats is a real, honest snapshot of how mature the
// recommendation_outcomes dataset actually is right now -- not a
// fabricated progress metric. Turning this dataset into something
// "worthy of fine-tuning a custom model" depends entirely on real
// operators actually applying real recommendations over real time; that
// can't be coded into existence, only made visible, so real progress
// toward it is honestly trackable instead of invisible.
type OutcomeDatasetStats struct {
	TotalShown    int `json:"total_shown"`
	TotalApplied  int `json:"total_applied"`
	TotalMeasured int `json:"total_measured"`
	// MeanAbsPredictionErrorINR is the real average gap, in INR, between
	// what was predicted (SavingsHighINR at the time a fix was shown) and
	// what actually happened (CostBeforeINR - CostAfterINR) -- the actual
	// "is this dataset good enough to learn from" signal, not just a row
	// count. A pointer, not a bare 0: with zero measured outcomes this
	// number doesn't exist yet, and reporting 0 would dishonestly read as
	// "predictions are perfect" instead of "nothing to measure yet."
	MeanAbsPredictionErrorINR *float64 `json:"mean_abs_prediction_error_inr,omitempty"`
}

// OutcomeDatasetStats aggregates one tenant's current real outcome-tracking
// state. Reuses ListRecommendationOutcomes (which already measures any
// newly-measurable pending outcomes before returning) and aggregates in
// pure Go over the same slice, rather than a separate query -- same
// pattern as computeSummary/computeSpendByClass in summary.go.
func (s *Store) OutcomeDatasetStats(tenantID int64) (OutcomeDatasetStats, error) {
	outcomes, err := s.ListRecommendationOutcomes(tenantID)
	if err != nil {
		return OutcomeDatasetStats{}, err
	}

	var out OutcomeDatasetStats
	out.TotalShown = len(outcomes)

	var errSum float64
	var errCount int
	for _, o := range outcomes {
		if o.AppliedAt != nil {
			out.TotalApplied++
		}
		if o.CostAfterINR != nil {
			out.TotalMeasured++
			actualSavings := o.CostBeforeINR - *o.CostAfterINR
			diff := o.PredictedSavingsHighINR - actualSavings
			if diff < 0 {
				diff = -diff
			}
			errSum += diff
			errCount++
		}
	}
	if errCount > 0 {
		mean := errSum / float64(errCount)
		out.MeanAbsPredictionErrorINR = &mean
	}

	return out, nil
}
