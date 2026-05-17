// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package flights

import (
	"strings"

	"github.com/qorvenai/qorven/internal/flight/models"
)

// FilterFlightsByTimePreference drops flights whose first-leg departure time
// falls outside the [earliest, latest] window. Both bounds are "HH:MM" strings
// in 24-hour format (e.g. "06:00", "23:00"). An empty string means no bound.
//
// The departure time is extracted from the first leg's DepartureTime field,
// which is formatted as "YYYY-MM-DDTHH:MM" or "HH:MM" (we parse the last 5
// characters in either case).
//
// The function never mutates the input slice and always returns a valid
// (possibly empty) slice.
func FilterFlightsByTimePreference(flights []models.FlightResult, earliest, latest string) []models.FlightResult {
	if earliest == "" && latest == "" {
		return flights
	}

	out := make([]models.FlightResult, 0, len(flights))
	for _, f := range flights {
		depTime := extractDepartureHHMM(f)
		if depTime == "" {
			// Can't determine time — keep the flight rather than wrongly exclude.
			out = append(out, f)
			continue
		}
		if earliest != "" && depTime < earliest {
			continue
		}
		if latest != "" && depTime > latest {
			continue
		}
		out = append(out, f)
	}
	return out
}

// FilterFlightsByBudget drops flights whose price exceeds maxPrice.
// When maxPrice <= 0, all flights are kept.
func FilterFlightsByBudget(flights []models.FlightResult, maxPrice float64) []models.FlightResult {
	if maxPrice <= 0 {
		return flights
	}
	out := make([]models.FlightResult, 0, len(flights))
	for _, f := range flights {
		if f.Price > 0 && f.Price > maxPrice {
			continue
		}
		out = append(out, f)
	}
	return out
}

// extractDepartureHHMM extracts the "HH:MM" departure time from the first leg.
// Returns "" if the flight has no legs or the time cannot be parsed.
func extractDepartureHHMM(f models.FlightResult) string {
	if len(f.Legs) == 0 {
		return ""
	}
	dt := f.Legs[0].DepartureTime
	if dt == "" {
		return ""
	}
	// Handle "2026-06-15T10:30" or "10:30" or "2026-06-15 10:30".
	if idx := strings.LastIndex(dt, "T"); idx >= 0 && idx+6 <= len(dt) {
		return dt[idx+1 : idx+6]
	}
	if idx := strings.LastIndex(dt, " "); idx >= 0 && idx+6 <= len(dt) {
		return dt[idx+1 : idx+6]
	}
	// Bare "HH:MM"
	if len(dt) == 5 && dt[2] == ':' {
		return dt
	}
	return ""
}
