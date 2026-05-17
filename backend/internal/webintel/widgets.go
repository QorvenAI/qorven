// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package webintel

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Widget is a rich card rendered in chat (weather, calculation, stock).
type Widget struct {
	Type   string         `json:"type"` // weather, calculation
	Params map[string]any `json:"params"`
}

// DetectWidget checks if the query should trigger a widget.
func DetectWidget(query string) *Widget {
	q := strings.ToLower(query)
	if isWeatherQuery(q) {
		return &Widget{Type: "weather", Params: map[string]any{"query": query}}
	}
	if isCalcQuery(q) {
		return &Widget{Type: "calculation", Params: map[string]any{"expression": query}}
	}
	return nil
}

func isWeatherQuery(q string) bool {
	triggers := []string{"weather", "temperature", "forecast", "rain", "snow", "humid", "wind speed", "climate", "hot today", "cold today", "sunny", "cloudy"}
	for _, t := range triggers {
		if strings.Contains(q, t) { return true }
	}
	// "what about X" after weather context — detect city-only queries
	// If query is just a city name (short, no question words about non-weather topics)
	if strings.HasPrefix(q, "what about ") || strings.HasPrefix(q, "how about ") || strings.HasPrefix(q, "and in ") || strings.HasPrefix(q, "and ") {
		// Likely a follow-up — treat as weather if it's just a location
		rest := q
		for _, p := range []string{"what about ", "how about ", "and in ", "and "} {
			rest = strings.TrimPrefix(rest, p)
		}
		rest = strings.TrimRight(rest, "? ")
		if len(rest) > 1 && len(rest) < 50 && !strings.ContainsAny(rest, "?!.") {
			return true // likely a city name follow-up
		}
	}
	return false
}

func isCalcQuery(q string) bool {
	// Simple: contains math operators and mostly numbers
	hasOp := strings.ContainsAny(q, "+-*/^%")
	digits := 0
	for _, c := range q { if c >= '0' && c <= '9' { digits++ } }
	return hasOp && digits > 0 && float64(digits)/float64(len(q)) > 0.3
}

// FetchWeather gets weather data from Open-Meteo (free, no key).
func FetchWeather(ctx context.Context, location string) (map[string]any, error) {
	client := &http.Client{Timeout: 10 * time.Second}

	// 1. Geocode location via Nominatim (OpenStreetMap, free)
	geoURL := fmt.Sprintf("https://nominatim.openstreetmap.org/search?q=%s&format=json&limit=1&accept-language=en", url.QueryEscape(location))
	req, _ := http.NewRequestWithContext(ctx, "GET", geoURL, nil)
	req.Header.Set("User-Agent", "Qorven/2.0")
	req.Header.Set("Accept-Language", "en")
	resp, err := client.Do(req)
	if err != nil { return nil, err }
	defer resp.Body.Close()

	var geo []struct {
		Lat     string `json:"lat"`
		Lon     string `json:"lon"`
		Display string `json:"display_name"`
	}
	json.NewDecoder(resp.Body).Decode(&geo)
	if len(geo) == 0 { return nil, fmt.Errorf("location not found: %s", location) }

	lat, _ := strconv.ParseFloat(geo[0].Lat, 64)
	lon, _ := strconv.ParseFloat(geo[0].Lon, 64)

	// 2. Fetch weather from Open-Meteo (free, no key)
	wxURL := fmt.Sprintf(
		"https://api.open-meteo.com/v1/forecast?latitude=%.4f&longitude=%.4f"+
			"&current=temperature_2m,relative_humidity_2m,apparent_temperature,is_day,weather_code,wind_speed_10m,wind_direction_10m"+
			"&daily=weather_code,temperature_2m_max,temperature_2m_min,precipitation_probability_max"+
			"&timezone=auto&forecast_days=5", lat, lon)

	resp2, err := client.Get(wxURL)
	if err != nil { return nil, err }
	defer resp2.Body.Close()

	var wx map[string]any
	json.NewDecoder(resp2.Body).Decode(&wx)

	current, _ := wx["current"].(map[string]any)
	daily, _ := wx["daily"].(map[string]any)

	code := int(toFloat(current["weather_code"]))
	isDay := toFloat(current["is_day"]) == 1

	return map[string]any{
		"location":    simplifyLocation(geo[0].Display, location),
		"temperature": current["temperature_2m"],
		"feels_like":  current["apparent_temperature"],
		"humidity":    current["relative_humidity_2m"],
		"wind_speed":  current["wind_speed_10m"],
		"condition":   weatherCondition(code),
		"icon":        weatherIcon(code, isDay),
		"is_day":      isDay,
		"daily":       daily,
		"unit":        "°C",
	}, nil
}

// EvalCalc evaluates a simple math expression.
func EvalCalc(expr string) (float64, error) {
	// Clean expression
	expr = strings.ReplaceAll(expr, " ", "")
	expr = strings.ReplaceAll(expr, "×", "*")
	expr = strings.ReplaceAll(expr, "÷", "/")
	expr = strings.ReplaceAll(expr, "^", "**")

	// Simple recursive descent for +, -, *, /
	return evalExpr(expr)
}

func evalExpr(s string) (float64, error) {
	// Find last + or - (not inside parens)
	depth := 0
	lastOp := -1
	for i := len(s) - 1; i >= 0; i-- {
		switch s[i] {
		case ')': depth++
		case '(': depth--
		case '+', '-':
			if depth == 0 && i > 0 { lastOp = i }
		}
	}
	if lastOp > 0 {
		left, err := evalExpr(s[:lastOp])
		if err != nil { return 0, err }
		right, err := evalExpr(s[lastOp+1:])
		if err != nil { return 0, err }
		if s[lastOp] == '+' { return left + right, nil }
		return left - right, nil
	}
	// Find last * or /
	for i := len(s) - 1; i >= 0; i-- {
		switch s[i] {
		case ')': depth++
		case '(': depth--
		case '*', '/':
			if depth == 0 { lastOp = i }
		}
	}
	if lastOp > 0 {
		left, err := evalExpr(s[:lastOp])
		if err != nil { return 0, err }
		right, err := evalExpr(s[lastOp+1:])
		if err != nil { return 0, err }
		if s[lastOp] == '*' { return left * right, nil }
		if right == 0 { return 0, fmt.Errorf("division by zero") }
		return left / right, nil
	}
	// Parentheses
	if len(s) > 2 && s[0] == '(' && s[len(s)-1] == ')' {
		return evalExpr(s[1 : len(s)-1])
	}
	// Number
	return strconv.ParseFloat(s, 64)
}

func toFloat(v any) float64 {
	switch n := v.(type) {
	case float64: return n
	case int: return float64(n)
	case json.Number: f, _ := n.Float64(); return f
	}
	return 0
}

func weatherCondition(code int) string {
	switch {
	case code == 0: return "Clear"
	case code <= 3: return "Partly Cloudy"
	case code <= 48: return "Fog"
	case code <= 57: return "Drizzle"
	case code <= 67: return "Rain"
	case code <= 77: return "Snow"
	case code <= 82: return "Rain Showers"
	case code <= 86: return "Snow Showers"
	case code == 95: return "Thunderstorm"
	case code >= 96: return "Thunderstorm with Hail"
	default: return "Clear"
	}
}

func weatherIcon(code int, isDay bool) string {
	_ = isDay
	switch {
	case code == 0: return "☀️"
	case code <= 3: return "⛅"
	case code <= 48: return "🌫️"
	case code <= 67: return "🌧️"
	case code <= 77: return "❄️"
	case code <= 86: return "🌨️"
	default: return "⛈️"
	}
}

func round1(f float64) float64 { return math.Round(f*10) / 10 }

// simplifyLocation extracts city + country from a full Nominatim display_name.
// "Mumbai, Mumbai Suburban District, Maharashtra, 400051, India" → "Mumbai, India"
func simplifyLocation(display, query string) string {
	parts := strings.Split(display, ", ")
	if len(parts) <= 2 { return display }
	city := parts[0]
	country := parts[len(parts)-1]
	// If the query itself is a known city name, use it directly
	q := strings.TrimSpace(query)
	if len(q) > 0 {
		// Capitalize first letter
		city = strings.ToUpper(q[:1]) + strings.ToLower(q[1:])
	}
	return city + ", " + country
}
