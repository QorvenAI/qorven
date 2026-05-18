// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package flights

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/qorvenai/qorven/internal/flight/models"
)

// SearchMultiAirport searches flights across multiple origin and destination airports.
// Runs all origin×destination combinations in parallel (max 5 concurrent) and merges
// results sorted by price. Each flight already contains departure/arrival airport codes.
func SearchMultiAirport(ctx context.Context, origins, destinations []string, date string, opts SearchOptions) (*models.FlightSearchResult, error) {
	client := DefaultClient()
	opts.defaults()

	if len(origins) == 0 || len(destinations) == 0 || date == "" {
		return nil, fmt.Errorf("origins, destinations, and date are required")
	}

	sem := make(chan struct{}, 5) // max 5 concurrent searches
	var mu sync.Mutex
	var allFlights []models.FlightResult
	var wg sync.WaitGroup

	for _, orig := range origins {
		for _, dest := range destinations {
			if orig == dest {
				continue
			}
			wg.Add(1)
			go func(o, d string) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				result, err := SearchFlightsWithClient(ctx, client, o, d, date, opts)
				if err != nil || !result.Success {
					return // skip failed combos silently
				}

				mu.Lock()
				allFlights = append(allFlights, result.Flights...)
				mu.Unlock()
			}(orig, dest)
		}
	}

	wg.Wait()

	// Sort by price.
	sort.Slice(allFlights, func(i, j int) bool {
		return allFlights[i].Price < allFlights[j].Price
	})

	tripType := "one_way"
	if opts.ReturnDate != "" {
		tripType = "round_trip"
	}

	return &models.FlightSearchResult{
		Success:  len(allFlights) > 0,
		Count:    len(allFlights),
		TripType: tripType,
		Flights:  allFlights,
	}, nil
}

// ParseAirports splits a comma-separated airport string into a slice.
// Trims whitespace and uppercases each code.
func ParseAirports(s string) []string {
	parts := strings.Split(s, ",")
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(strings.ToUpper(p))
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}
