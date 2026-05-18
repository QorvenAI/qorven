// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

// Package models defines shared data types for flight and hotel search results.
package models

// Price represents a monetary amount with currency.
type Price struct {
	Amount   float64 `json:"amount"`
	Currency string  `json:"currency"`
}

// DateRange represents a check-in/check-out or departure/return date pair.
// Dates are formatted as YYYY-MM-DD strings.
type DateRange struct {
	CheckIn  string `json:"check_in"`
	CheckOut string `json:"check_out"`
}

// Location represents a named geographic point.
type Location struct {
	Name      string  `json:"name"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}
