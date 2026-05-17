// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package models

// GridCell represents a single cell in a price grid: a departure date,
// return date, and the cheapest price for that combination.
type GridCell struct {
	DepartureDate string  `json:"departure_date"`
	ReturnDate    string  `json:"return_date"`
	Price         float64 `json:"price"`
	Currency      string  `json:"currency,omitempty"`
}

// PriceGrid is the top-level response for a calendar grid search.
// It contains a 2D matrix of departure date x return date -> price.
type PriceGrid struct {
	Success        bool       `json:"success"`
	Count          int        `json:"count"`
	DepartureDates []string   `json:"departure_dates"`
	ReturnDates    []string   `json:"return_dates"`
	Cells          []GridCell `json:"cells"`
	Error          string     `json:"error,omitempty"`
}
