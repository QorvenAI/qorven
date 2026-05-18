// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package models

// HotelResult represents a single hotel from a search.
type HotelResult struct {
	Name         string   `json:"name"`
	HotelID      string   `json:"hotel_id"`
	Rating       float64  `json:"rating"`
	ReviewCount  int      `json:"review_count"`
	Stars        int      `json:"stars"`
	Price        float64  `json:"price"`
	Currency     string   `json:"currency"`
	Address      string   `json:"address"`
	Lat          float64  `json:"lat"`
	Lon          float64  `json:"lon"`
	Amenities    []string `json:"amenities,omitempty"`
	BookingURL   string   `json:"booking_url,omitempty"`
	EcoCertified bool     `json:"eco_certified,omitempty"`
}

// HotelSearchResult is the top-level response for a hotel search.
type HotelSearchResult struct {
	Success        bool          `json:"success"`
	Count          int           `json:"count"`
	TotalAvailable int           `json:"total_available,omitempty"`
	Hotels         []HotelResult `json:"hotels"`
	Error          string        `json:"error,omitempty"`
}

// ProviderPrice represents a single booking provider's price for a hotel.
type ProviderPrice struct {
	Provider string  `json:"provider"`
	Price    float64 `json:"price"`
	Currency string  `json:"currency"`
}

// HotelPriceResult is the top-level response for a hotel price lookup.
type HotelPriceResult struct {
	Success   bool            `json:"success"`
	HotelID   string          `json:"hotel_id"`
	Name      string          `json:"name"`
	CheckIn   string          `json:"check_in"`
	CheckOut  string          `json:"check_out"`
	Providers []ProviderPrice `json:"providers"`
	Error     string          `json:"error,omitempty"`
}
