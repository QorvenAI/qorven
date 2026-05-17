// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package tools

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/qorvenai/qorven/internal/flight/batchexec"
	"github.com/qorvenai/qorven/internal/flight/flights"
)

type FlightSearchTool struct{}

func NewFlightSearchTool() *FlightSearchTool { return &FlightSearchTool{} }

func (t *FlightSearchTool) Name() string { return "flight_search" }
func (t *FlightSearchTool) Description() string {
	return "Search flights with real prices from Google Flights. Returns airlines, prices, duration, stops. Use airport codes (DXB, BOM, DEL, LHR)."
}
func (t *FlightSearchTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"from": map[string]any{"type": "string", "description": "Departure airport code (DXB, BOM, DEL)"},
			"to":   map[string]any{"type": "string", "description": "Arrival airport code"},
			"date": map[string]any{"type": "string", "description": "Date YYYY-MM-DD"},
		},
		"required": []string{"from", "to", "date"},
	}
}

func (t *FlightSearchTool) Execute(ctx context.Context, args map[string]any) *Result {
	from, _ := args["from"].(string)
	if from == "" { from, _ = args["from1_"].(string) }
	to, _ := args["to"].(string)
	if to == "" { to, _ = args["destination"].(string) }
	date, _ := args["date"].(string)

	if from == "" || to == "" || date == "" {
		return ErrorResult("from, to, date required (airport codes like DXB, BOM)")
	}

	start := time.Now()
	client := batchexec.NewClient()
	result, err := flights.SearchFlightsWithClient(ctx, client, strings.ToUpper(from), strings.ToUpper(to), date, flights.SearchOptions{})
	elapsed := time.Since(start).Milliseconds()

	if err != nil {
		return ErrorResult(fmt.Sprintf("search failed: %v", err))
	}
	if result.Count == 0 {
		return TextResult(fmt.Sprintf("No flights %s → %s on %s", from, to, date))
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("✈️ %s → %s on %s (%d flights, %dms)\n\n", from, to, date, result.Count, elapsed))

	best := float64(0)
	// widgetOptions mirrors the frontend FlightOption shape (see
	// web/components/chat/cards/travel-cards.tsx). We keep the
	// markdown output for the LLM — it narrates the result — while
	// the widget gives the chat UI a rich card alongside.
	widgetOptions := make([]map[string]any, 0, len(result.Flights))
	for i, f := range result.Flights {
		if i >= 10 { break }
		if best == 0 || (f.Price > 0 && f.Price < best) { best = f.Price }
		stops := "nonstop"
		if f.Stops > 0 { stops = fmt.Sprintf("%d stop", f.Stops) }
		leg := ""
		if len(f.Legs) > 0 {
			l := f.Legs[0]
			leg = fmt.Sprintf("%s %s→%s %s", l.Airline, l.DepartureTime, l.ArrivalTime, l.FlightNumber)
		}
		sb.WriteString(fmt.Sprintf("%s%.0f | %s | %s | %s\n", f.Currency, f.Price, leg, fmt.Sprintf("%dmin", f.Duration), stops))

		// Build one option per result: flatten FlightLegs into the
		// card's segment shape. Duration is minutes → "Xh YYm".
		segs := make([]map[string]any, 0, len(f.Legs))
		for _, l := range f.Legs {
			segs = append(segs, map[string]any{
				"airline":        l.Airline,
				"airline_code":   l.AirlineCode,
				"flight_number":  l.FlightNumber,
				"from_airport":   l.DepartureAirport.Code,
				"from_city":      l.DepartureAirport.Name,
				"to_airport":     l.ArrivalAirport.Code,
				"to_city":        l.ArrivalAirport.Name,
				"depart_time":    l.DepartureTime,
				"arrive_time":    l.ArrivalTime,
				"duration":       formatMinutes(l.Duration),
				"stops":          0, // per-leg; the option-level stops matters for display
			})
		}
		if len(segs) > 0 {
			// Tag the first segment with the option-level stop count
			// so the card's "direct / N stops" chip is accurate.
			segs[0]["stops"] = f.Stops
			segs[0]["duration"] = formatMinutes(f.Duration)
		}
		widgetOptions = append(widgetOptions, map[string]any{
			"segments":    segs,
			"price":       f.Price,
			"currency":    f.Currency,
			"booking_url": f.BookingURL,
			"provider":    "Google Flights",
		})
	}
	if best > 0 { sb.WriteString(fmt.Sprintf("\n💰 Best: %.0f\n", best)) }

	slog.Info("flight_search.done", "from", from, "to", to, "flights", result.Count, "best", best, "ms", elapsed)

	return &Result{
		ForLLM:  sb.String(),
		ForUser: sb.String(),
		Widget: &Widget{
			Type: "flights",
			Data: map[string]any{
				"query":   fmt.Sprintf("%s → %s, %s", strings.ToUpper(from), strings.ToUpper(to), date),
				"options": widgetOptions,
			},
		},
	}
}

// formatMinutes turns 470 into "7h 50m". The card renders this
// verbatim, so keep it short and consistent.
func formatMinutes(m int) string {
	if m <= 0 { return "" }
	h := m / 60
	min := m % 60
	if h == 0 { return fmt.Sprintf("%dm", min) }
	if min == 0 { return fmt.Sprintf("%dh", h) }
	return fmt.Sprintf("%dh %dm", h, min)
}
