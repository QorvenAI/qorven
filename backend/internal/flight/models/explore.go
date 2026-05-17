// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package models

// ExploreDestination represents a single destination found by the explore search.
type ExploreDestination struct {
	CityID      string  `json:"city_id"`
	CityName    string  `json:"city_name,omitempty"`
	Country     string  `json:"country,omitempty"`
	AirportCode string  `json:"airport_code"`
	Price       float64 `json:"price"`
	AirlineName string  `json:"airline_name,omitempty"`
	AirlineCode string  `json:"airline_code,omitempty"`
	Stops       int     `json:"stops"`
}

// ExploreResult is the top-level response for an explore destination search.
type ExploreResult struct {
	Success      bool                 `json:"success"`
	Count        int                  `json:"count"`
	Destinations []ExploreDestination `json:"destinations"`
	Error        string               `json:"error,omitempty"`
}
