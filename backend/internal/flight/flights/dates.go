// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package flights

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/qorvenai/qorven/internal/flight/batchexec"
	"github.com/qorvenai/qorven/internal/flight/models"
)

// maxConcurrentDateSearches limits the number of parallel flight searches
// when scanning a date range. This prevents overwhelming Google's API while
// still providing ~3x speedup over sequential execution.
const maxConcurrentDateSearches = 3

// DateSearchOptions configures a date-range price search.
type DateSearchOptions struct {
	FromDate  string // Start of date range (YYYY-MM-DD); default: tomorrow
	ToDate    string // End of date range (YYYY-MM-DD); default: FromDate + 30 days
	Duration  int    // Trip duration in days (for round-trip); 0 = one-way
	RoundTrip bool   // Search round-trip prices
	Adults    int    // Number of adults (default: 1)
}

// defaults fills in zero-value fields.
func (o *DateSearchOptions) defaults() {
	if o.Adults <= 0 {
		o.Adults = 1
	}
	if o.FromDate == "" {
		o.FromDate = time.Now().AddDate(0, 0, 1).Format("2006-01-02")
	}
	if o.ToDate == "" {
		from, err := time.Parse("2006-01-02", o.FromDate)
		if err == nil {
			o.ToDate = from.AddDate(0, 0, 30).Format("2006-01-02")
		}
	}
	if o.RoundTrip && o.Duration <= 0 {
		o.Duration = 7
	}
}

// SearchDates searches for the cheapest flight prices across a date range.
//
// It performs individual flight searches for each date in the range and
// collects the minimum price found. This mirrors the approach used by fli's
// date search functionality.
//
// For round-trip searches, each departure date is paired with a return date
// that is Duration days later.
func SearchDates(ctx context.Context, origin, destination string, opts DateSearchOptions) (*models.DateSearchResult, error) {
	opts.defaults()

	if origin == "" || destination == "" {
		return &models.DateSearchResult{
			Error: "origin and destination are required",
		}, fmt.Errorf("origin and destination are required")
	}

	fromDate, err := time.Parse("2006-01-02", opts.FromDate)
	if err != nil {
		return nil, fmt.Errorf("invalid from_date %q: %w", opts.FromDate, err)
	}

	toDate, err := time.Parse("2006-01-02", opts.ToDate)
	if err != nil {
		return nil, fmt.Errorf("invalid to_date %q: %w", opts.ToDate, err)
	}

	if toDate.Before(fromDate) {
		return nil, fmt.Errorf("to_date %s is before from_date %s", opts.ToDate, opts.FromDate)
	}

	client := batchexec.NewClient()

	// Collect all dates to search.
	var searchDates []time.Time
	for d := fromDate; !d.After(toDate); d = d.AddDate(0, 0, 1) {
		searchDates = append(searchDates, d)
	}

	// Search dates concurrently with bounded parallelism.
	type dateResult struct {
		dp  models.DatePriceResult
		ok  bool
		idx int // original index for stable ordering
	}

	results := make([]dateResult, len(searchDates))
	sem := make(chan struct{}, maxConcurrentDateSearches)
	var wg sync.WaitGroup

	for i, d := range searchDates {
		wg.Add(1)
		go func(idx int, date time.Time) {
			defer wg.Done()

			// Acquire semaphore slot.
			sem <- struct{}{}
			defer func() { <-sem }()

			// Check if context is cancelled before starting.
			if ctx.Err() != nil {
				return
			}

			dateStr := date.Format("2006-01-02")

			searchOpts := SearchOptions{
				Adults: opts.Adults,
			}
			if opts.RoundTrip && opts.Duration > 0 {
				returnDate := date.AddDate(0, 0, opts.Duration)
				searchOpts.ReturnDate = returnDate.Format("2006-01-02")
			}

			result, err := SearchFlightsWithClient(ctx, client, origin, destination, dateStr, searchOpts)
			if err != nil {
				return
			}

			if !result.Success || len(result.Flights) == 0 {
				return
			}

			// Find the cheapest flight with a positive price for this date.
			var cheapest *models.FlightResult
			for i := range result.Flights {
				if result.Flights[i].Price > 0 {
					if cheapest == nil || result.Flights[i].Price < cheapest.Price {
						cheapest = &result.Flights[i]
					}
				}
			}

			if cheapest == nil {
				return // no priced flights for this date
			}

			dp := models.DatePriceResult{
				Date:     dateStr,
				Price:    cheapest.Price,
				Currency: cheapest.Currency,
			}
			if searchOpts.ReturnDate != "" {
				dp.ReturnDate = searchOpts.ReturnDate
			}

			results[idx] = dateResult{dp: dp, ok: true, idx: idx}
		}(i, d)
	}

	wg.Wait()

	// Collect successful results in date order.
	var dates []models.DatePriceResult
	for _, r := range results {
		if r.ok {
			dates = append(dates, r.dp)
		}
	}

	// Sort by date (already in order from results array, but be explicit).
	sort.Slice(dates, func(i, j int) bool {
		return dates[i].Date < dates[j].Date
	})

	tripType := "one_way"
	if opts.RoundTrip {
		tripType = "round_trip"
	}

	return &models.DateSearchResult{
		Success:   true,
		Count:     len(dates),
		TripType:  tripType,
		DateRange: fmt.Sprintf("%s to %s", opts.FromDate, opts.ToDate),
		Dates:     dates,
	}, nil
}
