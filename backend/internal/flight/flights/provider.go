// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package flights

import (
	"context"

	"github.com/qorvenai/qorven/internal/flight/models"
)

// DefaultProvider wraps the package-level SearchFlights function, implementing
// models.FlightSearcher. It uses the shared default client for connection reuse.
type DefaultProvider struct{}

// SearchFlights delegates to the package-level SearchFlights, converting
// models.FlightSearchOptions to the package's SearchOptions.
func (p *DefaultProvider) SearchFlights(ctx context.Context, origin, dest, date string, opts models.FlightSearchOptions) (*models.FlightSearchResult, error) {
	return SearchFlights(ctx, origin, dest, date, SearchOptions{
		ReturnDate: opts.ReturnDate,
		CabinClass: opts.CabinClass,
		MaxStops:   opts.MaxStops,
		SortBy:     opts.SortBy,
		Airlines:   opts.Airlines,
		Adults:     opts.Adults,
	})
}
