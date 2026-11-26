package recommendation

// number stores a known value as a pointer; a nil pointer means the value is unknown.
func number(v float64) *float64 {
	return &v
}

// copyNumber makes an independent copy, preserving nil for missing evidence.
func copyNumber(v *float64) *float64 {
	if v == nil {
		return nil
	}
	x := *v
	return &x
}

// Copies a zone's totals and hourly evidence so callers cannot change saved data.
func cloneArea(a AreaEvidence) AreaEvidence {
	a.Baseline = copyNumber(a.Baseline)
	a.Estimated = copyNumber(a.Estimated)
	a.Additional = copyNumber(a.Additional)
	a.RelativeUplift = copyNumber(a.RelativeUplift)
	a.Reasons = append([]string(nil), a.Reasons...)
	a.Hourly = append([]HourEvidence(nil), a.Hourly...)
	for i := range a.Hourly {
		h := &a.Hourly[i]
		h.DryMean = copyNumber(h.DryMean)
		h.RainMean = copyNumber(h.RainMean)
		h.HistoricalAbsoluteUplift = copyNumber(h.HistoricalAbsoluteUplift)
		h.HistoricalPercentageUplift = copyNumber(h.HistoricalPercentageUplift)
		h.RainStdDev = copyNumber(h.RainStdDev)
		h.Baseline = copyNumber(h.Baseline)
		h.Estimated = copyNumber(h.Estimated)
		h.Additional = copyNumber(h.Additional)
		if h.Forecast != nil {
			f := *h.Forecast
			h.Forecast = &f
		}
	}
	return a
}

// Copies all values shared by the saved result, including slices and pointers.
func cloneResult(r Result) Result {
	r.Areas = append([]AreaEvidence(nil), r.Areas...)
	for i := range r.Areas {
		r.Areas[i] = cloneArea(r.Areas[i])
	}
	if r.Primary != nil {
		a := cloneArea(*r.Primary)
		r.Primary = &a
	}
	r.Warnings = append([]string(nil), r.Warnings...)
	r.HighestModeledAdditional = copyNumber(r.HighestModeledAdditional)
	if r.ForecastFetchedAt != nil {
		v := *r.ForecastFetchedAt
		r.ForecastFetchedAt = &v
	}
	return r
}
