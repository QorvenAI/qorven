// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package webintel

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// DetectAllWidgets returns all widgets that should be shown for a query.
func DetectAllWidgets(ctx context.Context, query string) []Widget {
	widgets := []Widget{}
	q := strings.ToLower(query)

	// Weather (already handled in agent loop, skip here)

	// Timezone
	if tz := detectTimezone(q); tz != nil {
		widgets = append(widgets, *tz)
	}

	// Currency conversion
	if curr := detectCurrency(q); curr != nil {
		widgets = append(widgets, *curr)
	}

	// Unit conversion
	if unit := detectUnit(q); unit != nil {
		widgets = append(widgets, *unit)
	}

	// YouTube URL
	if yt := detectYouTube(query); yt != nil {
		widgets = append(widgets, *yt)
	}

	// Stock/Crypto
	if stock := DetectStock(q); stock != nil {
		widgets = append(widgets, *stock)
	}

	// Definition
	if def := DetectDefinition(ctx, q); def != nil {
		widgets = append(widgets, *def)
	}

	// Link previews
	widgets = append(widgets, DetectLinks(query)...)

	return widgets
}

// ─── Timezone ───
func detectTimezone(q string) *Widget {
	patterns := []string{"time in ", "what time in ", "current time in ", "time at "}
	for _, p := range patterns {
		if idx := strings.Index(q, p); idx >= 0 {
			city := strings.TrimSpace(q[idx+len(p):])
			city = strings.TrimRight(city, "?!.")
			if city == "" { continue }

			// Map common cities to timezones
			tzMap := map[string]string{
				"tokyo": "Asia/Tokyo", "london": "Europe/London", "new york": "America/New_York",
				"paris": "Europe/Paris", "dubai": "Asia/Dubai", "singapore": "Asia/Singapore",
				"sydney": "Australia/Sydney", "mumbai": "Asia/Kolkata", "delhi": "Asia/Kolkata",
				"chennai": "Asia/Kolkata", "bangalore": "Asia/Kolkata", "kolkata": "Asia/Kolkata",
				"shanghai": "Asia/Shanghai", "beijing": "Asia/Shanghai", "hong kong": "Asia/Hong_Kong",
				"seoul": "Asia/Seoul", "berlin": "Europe/Berlin", "moscow": "Europe/Moscow",
				"los angeles": "America/Los_Angeles", "chicago": "America/Chicago",
				"toronto": "America/Toronto", "sao paulo": "America/Sao_Paulo",
				"cairo": "Africa/Cairo", "lagos": "Africa/Lagos", "nairobi": "Africa/Nairobi",
			}
			tz, ok := tzMap[strings.ToLower(city)]
			if !ok { return nil }

			loc, err := time.LoadLocation(tz)
			if err != nil { return nil }
			now := time.Now().In(loc)

			return &Widget{Type: "stat", Params: map[string]any{
				"value": now.Format("3:04 PM"),
				"label": fmt.Sprintf("%s · %s", city, now.Format("Mon, Jan 2")),
			}}
		}
	}
	return nil
}

// ─── Currency Conversion ───
var currencyRe = regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)\s*(usd|eur|gbp|inr|jpy|aud|cad|sgd|aed|sar|cny|krw|brl|mxn|thb|myr|php|idr|vnd|try|zar|ngn|egp|pkr|bdt|lkr)\s+(?:to|in)\s+(usd|eur|gbp|inr|jpy|aud|cad|sgd|aed|sar|cny|krw|brl|mxn|thb|myr|php|idr|vnd|try|zar|ngn|egp|pkr|bdt|lkr)`)

func detectCurrency(q string) *Widget {
	m := currencyRe.FindStringSubmatch(q)
	if m == nil { return nil }
	amount, _ := strconv.ParseFloat(m[1], 64)
	from := strings.ToUpper(m[2])
	to := strings.ToUpper(m[3])

	// Fetch rate from free API
	resp, err := http.Get(fmt.Sprintf("https://open.er-api.com/v6/latest/%s", from))
	if err != nil { return nil }
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	var data struct {
		Rates map[string]float64 `json:"rates"`
	}
	json.Unmarshal(b, &data)
	rate, ok := data.Rates[to]
	if !ok { return nil }

	converted := amount * rate
	return &Widget{Type: "stat", Params: map[string]any{
		"value": fmt.Sprintf("%.2f %s", converted, to),
		"label": fmt.Sprintf("%.2f %s → %s (rate: %.4f)", amount, from, to, rate),
	}}
}

// ─── Unit Conversion ───
var unitRe = regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)\s*(miles?|km|kilometers?|kg|kilograms?|lbs?|pounds?|feet|ft|meters?|m|inches?|in|cm|centimeters?|celsius|fahrenheit|°[cf]|gallons?|liters?|litres?|oz|ounces?|grams?|g)\s+(?:to|in)\s+(\w+)`)

func detectUnit(q string) *Widget {
	m := unitRe.FindStringSubmatch(q)
	if m == nil { return nil }
	val, _ := strconv.ParseFloat(m[1], 64)
	from := strings.ToLower(m[2])
	to := strings.ToLower(m[3])

	conversions := map[string]map[string]float64{
		"miles": {"km": 1.60934, "kilometers": 1.60934, "m": 1609.34, "meters": 1609.34, "feet": 5280, "ft": 5280},
		"km": {"miles": 0.621371, "m": 1000, "meters": 1000, "feet": 3280.84, "ft": 3280.84},
		"kg": {"lbs": 2.20462, "pounds": 2.20462, "g": 1000, "grams": 1000, "oz": 35.274, "ounces": 35.274},
		"lbs": {"kg": 0.453592, "g": 453.592, "grams": 453.592, "oz": 16, "ounces": 16},
		"feet": {"m": 0.3048, "meters": 0.3048, "cm": 30.48, "inches": 12, "in": 12},
		"celsius": {"fahrenheit": 0}, // special
		"fahrenheit": {"celsius": 0}, // special
		"gallons": {"liters": 3.78541, "litres": 3.78541},
		"liters": {"gallons": 0.264172},
	}

	// Normalize
	fromNorm := strings.TrimSuffix(from, "s")
	if fromNorm == "mile" { fromNorm = "miles" }
	if fromNorm == "pound" || fromNorm == "lb" { fromNorm = "lbs" }
	if fromNorm == "kilogram" { fromNorm = "kg" }
	if fromNorm == "kilometer" { fromNorm = "km" }
	if fromNorm == "meter" || fromNorm == "m" { fromNorm = "m" }
	if fromNorm == "foot" || fromNorm == "ft" { fromNorm = "feet" }
	if strings.Contains(fromNorm, "celsius") || fromNorm == "°c" { fromNorm = "celsius" }
	if strings.Contains(fromNorm, "fahrenheit") || fromNorm == "°f" { fromNorm = "fahrenheit" }

	var result float64
	if fromNorm == "celsius" {
		result = val*9/5 + 32
		to = "°F"
	} else if fromNorm == "fahrenheit" {
		result = (val - 32) * 5 / 9
		to = "°C"
	} else if rates, ok := conversions[fromNorm]; ok {
		if rate, ok := rates[to]; ok {
			result = val * rate
		} else { return nil }
	} else { return nil }

	return &Widget{Type: "stat", Params: map[string]any{
		"value": fmt.Sprintf("%.2f %s", result, to),
		"label": fmt.Sprintf("%.2f %s", val, from),
	}}
}

// ─── YouTube URL Detection ───
var youtubeRe = regexp.MustCompile(`(?:youtube\.com/watch\?v=|youtu\.be/|youtube\.com/embed/)([a-zA-Z0-9_-]{11})`)

func detectYouTube(text string) *Widget {
	m := youtubeRe.FindStringSubmatch(text)
	if m == nil { return nil }
	return &Widget{Type: "youtube", Params: map[string]any{"url": "https://www.youtube.com/watch?v=" + m[1]}}
}

// ─── Stock/Crypto Detection ───
var stockRe = regexp.MustCompile(`(?i)(?:stock|price|share)\s+(?:of\s+)?([A-Z]{1,5})|([A-Z]{1,5})\s+(?:stock|price|share)|(?:price of|how is)\s+([A-Z]{1,5})`)
var cryptoRe = regexp.MustCompile(`(?i)(bitcoin|btc|ethereum|eth|solana|sol|dogecoin|doge|xrp)\s+price|price\s+(?:of\s+)?(bitcoin|btc|ethereum|eth|solana|sol|dogecoin|doge|xrp)`)

func DetectStock(q string) *Widget {
	// Crypto
	m := cryptoRe.FindStringSubmatch(q)
	if m != nil {
		coin := strings.ToLower(m[1] + m[2])
		coinMap := map[string]string{"bitcoin": "bitcoin", "btc": "bitcoin", "ethereum": "ethereum", "eth": "ethereum", "solana": "solana", "sol": "solana", "dogecoin": "dogecoin", "doge": "dogecoin", "xrp": "ripple"}
		id, ok := coinMap[coin]
		if !ok { return nil }
		resp, err := http.Get(fmt.Sprintf("https://api.coingecko.com/api/v3/simple/price?ids=%s&vs_currencies=usd&include_24hr_change=true", id))
		if err != nil { return nil }
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		var data map[string]map[string]float64
		json.Unmarshal(b, &data)
		if d, ok := data[id]; ok {
			change := d["usd_24h_change"]
			changeType := "up"
			if change < 0 { changeType = "down" }
			return &Widget{Type: "stat", Params: map[string]any{
				"value": fmt.Sprintf("$%.2f", d["usd"]),
				"label": strings.ToUpper(coin) + " Price",
				"change": fmt.Sprintf("%.1f%% (24h)", change),
				"changeType": changeType,
			}}
		}
	}
	return nil
}

// ─── Definition Detection ───
func DetectDefinition(ctx context.Context, q string) *Widget {
	lower := strings.ToLower(q)
	var word string
	for _, p := range []string{"define ", "definition of ", "what does ", "meaning of ", "what is the meaning of "} {
		if idx := strings.Index(lower, p); idx >= 0 {
			word = strings.TrimSpace(q[idx+len(p):])
			word = strings.TrimRight(word, "?!.")
			break
		}
	}
	if word == "" || strings.Contains(word, " ") && len(strings.Fields(word)) > 3 { return nil }

	resp, err := http.Get(fmt.Sprintf("https://api.dictionaryapi.dev/api/v2/entries/en/%s", strings.Fields(word)[0]))
	if err != nil { return nil }
	defer resp.Body.Close()
	if resp.StatusCode != 200 { return nil }
	var entries []struct {
		Word     string `json:"word"`
		Meanings []struct {
			PartOfSpeech string `json:"partOfSpeech"`
			Definitions  []struct {
				Definition string `json:"definition"`
				Example    string `json:"example"`
			} `json:"definitions"`
		} `json:"meanings"`
	}
	b, _ := io.ReadAll(resp.Body)
	json.Unmarshal(b, &entries)
	if len(entries) == 0 || len(entries[0].Meanings) == 0 { return nil }

	e := entries[0]
	defs := []string{}
	for _, m := range e.Meanings {
		if len(m.Definitions) > 0 {
			defs = append(defs, fmt.Sprintf("*%s* — %s", m.PartOfSpeech, m.Definitions[0].Definition))
		}
	}
	return &Widget{Type: "callout", Params: map[string]any{
		"type": "info", "title": e.Word, "content": strings.Join(defs, "\n"),
	}}
}

// ─── Link Preview Detection ───
var urlRe = regexp.MustCompile(`https?://[^\s<>"]+`)

func DetectLinks(text string) []Widget {
	urls := urlRe.FindAllString(text, 3)
	widgets := []Widget{}
	for _, u := range urls {
		if youtubeRe.MatchString(u) { continue } // YouTube handled separately
		widgets = append(widgets, Widget{Type: "link", Params: map[string]any{"url": u}})
	}
	return widgets
}
