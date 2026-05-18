// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package models

import (
	"fmt"
	"strings"
)

// AirportInfo identifies an airport by IATA code and name.
type AirportInfo struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

// FlightLeg represents a single segment of a flight itinerary.
type FlightLeg struct {
	DepartureAirport AirportInfo `json:"departure_airport"`
	ArrivalAirport   AirportInfo `json:"arrival_airport"`
	DepartureTime    string      `json:"departure_time"`
	ArrivalTime      string      `json:"arrival_time"`
	Duration         int         `json:"duration"`        // minutes
	Airline          string      `json:"airline"`
	AirlineCode      string      `json:"airline_code"`
	FlightNumber     string      `json:"flight_number"`
	Aircraft         string      `json:"aircraft,omitempty"`       // e.g. "Airbus A350"
	LayoverMinutes   int         `json:"layover_minutes,omitempty"` // time between arrival of previous leg and this departure (0 for first leg)
}

// FlightResult represents a single flight option with price and routing.
type FlightResult struct {
	Price               float64     `json:"price"`
	Currency            string      `json:"currency"`
	Duration            int         `json:"duration"` // total minutes
	Stops               int         `json:"stops"`
	Legs                []FlightLeg `json:"legs"`
	BookingURL          string      `json:"booking_url,omitempty"`
	CarryOnIncluded     *bool       `json:"carry_on_included,omitempty"`     // true if carry-on bag is included in price
	CheckedBagsIncluded *int        `json:"checked_bags_included,omitempty"` // 0=not included, 1=one bag, 2=two bags
	Emissions           int         `json:"emissions,omitempty"`             // estimated CO2 in grams; 0 if unavailable
}

// FlightSearchResult is the top-level response for a flight search.
type FlightSearchResult struct {
	Success  bool           `json:"success"`
	Count    int            `json:"count"`
	TripType string         `json:"trip_type"`
	Flights  []FlightResult `json:"flights"`
	Error    string         `json:"error,omitempty"`
}

// DatePriceResult represents the cheapest price for a single departure date.
type DatePriceResult struct {
	Date       string  `json:"date"`
	Price      float64 `json:"price"`
	Currency   string  `json:"currency"`
	ReturnDate string  `json:"return_date,omitempty"`
}

// DateSearchResult is the top-level response for a date range price search.
type DateSearchResult struct {
	Success   bool              `json:"success"`
	Count     int               `json:"count"`
	TripType  string            `json:"trip_type"`
	DateRange string            `json:"date_range"`
	Dates     []DatePriceResult `json:"dates"`
	Error     string            `json:"error,omitempty"`
}

// CabinClass represents the cabin/service class for a flight.
type CabinClass int

const (
	Economy        CabinClass = 1
	PremiumEconomy CabinClass = 2
	Business       CabinClass = 3
	First          CabinClass = 4
)

// String returns the human-readable name of the cabin class.
func (c CabinClass) String() string {
	switch c {
	case Economy:
		return "economy"
	case PremiumEconomy:
		return "premium_economy"
	case Business:
		return "business"
	case First:
		return "first"
	default:
		return "economy"
	}
}

// ParseCabinClass converts a string to a CabinClass. Case-insensitive.
func ParseCabinClass(s string) (CabinClass, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "economy", "e", "1":
		return Economy, nil
	case "premium_economy", "premium-economy", "premiumeconomy", "pe", "2":
		return PremiumEconomy, nil
	case "business", "b", "3":
		return Business, nil
	case "first", "f", "4":
		return First, nil
	default:
		return Economy, fmt.Errorf("unknown cabin class: %q", s)
	}
}

// MaxStops constrains the number of stops in a flight search.
type MaxStops int

const (
	AnyStops     MaxStops = 0
	NonStop      MaxStops = 1
	OneStop      MaxStops = 2
	TwoPlusStops MaxStops = 3
)

// String returns the human-readable name of the stop filter.
func (m MaxStops) String() string {
	switch m {
	case AnyStops:
		return "any"
	case NonStop:
		return "nonstop"
	case OneStop:
		return "one_stop"
	case TwoPlusStops:
		return "two_plus"
	default:
		return "any"
	}
}

// ParseMaxStops converts a string to a MaxStops value. Case-insensitive.
func ParseMaxStops(s string) (MaxStops, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "any", "0", "":
		return AnyStops, nil
	case "nonstop", "non_stop", "non-stop", "1":
		return NonStop, nil
	case "one_stop", "one-stop", "onestop", "2":
		return OneStop, nil
	case "two_plus", "two-plus", "twoplus", "3":
		return TwoPlusStops, nil
	default:
		return AnyStops, fmt.Errorf("unknown max stops: %q", s)
	}
}

// SortBy controls the ordering of flight search results.
type SortBy int

const (
	SortCheapest      SortBy = 0
	SortDuration      SortBy = 1
	SortDepartureTime SortBy = 2
	SortArrivalTime   SortBy = 3
)

// String returns the human-readable name of the sort order.
func (s SortBy) String() string {
	switch s {
	case SortCheapest:
		return "cheapest"
	case SortDuration:
		return "duration"
	case SortDepartureTime:
		return "departure"
	case SortArrivalTime:
		return "arrival"
	default:
		return "cheapest"
	}
}

// ParseSortBy converts a string to a SortBy value. Case-insensitive.
func ParseSortBy(s string) (SortBy, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "cheapest", "price", "0", "":
		return SortCheapest, nil
	case "duration", "time", "1":
		return SortDuration, nil
	case "departure", "departure_time", "depart", "2":
		return SortDepartureTime, nil
	case "arrival", "arrival_time", "arrive", "3":
		return SortArrivalTime, nil
	default:
		return SortCheapest, fmt.Errorf("unknown sort order: %q", s)
	}
}
