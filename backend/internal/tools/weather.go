// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// WeatherTool queries Open-Meteo for current conditions + multi-day
// forecast. No API key required — Open-Meteo is free for non-commercial
// and reasonable commercial use.
//
// Why Open-Meteo (not OpenWeatherMap, WeatherAPI, etc.):
//   - Zero-config: no signup, no key, no dashboard
//   - No rate limit for reasonable use (10k req/day free)
//   - Covers global weather + forecast + geocoding in one provider
//
// Two upstream endpoints:
//   - https://geocoding-api.open-meteo.com/v1/search  — city → lat/lon
//   - https://api.open-meteo.com/v1/forecast          — lat/lon → weather
//
// The tool accepts either a `location` (plain English — "Tokyo", "New
// York, NY", "Paris, France") OR raw `lat`/`lon`. If a location is
// given, we geocode first. We always return a compact JSON-ish summary
// that the LLM can consume directly — no nested structures, no unit
// conversion tricks.
type WeatherTool struct {
	http        *http.Client
	geocodeURL  string
	forecastURL string
}

const (
	weatherGeocodeURL  = "https://geocoding-api.open-meteo.com/v1/search"
	weatherForecastURL = "https://api.open-meteo.com/v1/forecast"
)

// NewWeatherTool returns a tool that uses the default HTTP client with
// a 10s timeout — Open-Meteo's median latency is ~200ms, so 10s is
// comfortable headroom that also catches network blips without
// hanging an agent turn.
func NewWeatherTool() *WeatherTool {
	return &WeatherTool{
		http:        &http.Client{Timeout: 10 * time.Second},
		geocodeURL:  weatherGeocodeURL,
		forecastURL: weatherForecastURL,
	}
}

func (t *WeatherTool) Name() string { return "weather" }

func (t *WeatherTool) Description() string {
	return "Current weather and multi-day forecast for any location on Earth. " +
		"Provide either a location name (e.g. \"Tokyo\", \"Paris, France\") or " +
		"explicit latitude+longitude. Returns temperature, conditions, wind, " +
		"humidity, and a 3-day outlook. Uses Open-Meteo (free, no API key)."
}

func (t *WeatherTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"location": map[string]any{
				"type":        "string",
				"description": "Place name — city, city+state, or city+country. Examples: \"Tokyo\", \"Austin, TX\", \"Lisbon, Portugal\". Leave empty if passing lat/lon directly.",
			},
			"lat": map[string]any{
				"type":        "number",
				"description": "Latitude in decimal degrees (-90 to 90). Use only when `location` is empty.",
			},
			"lon": map[string]any{
				"type":        "number",
				"description": "Longitude in decimal degrees (-180 to 180). Use only when `location` is empty.",
			},
			"units": map[string]any{
				"type":        "string",
				"description": "Temperature + wind units. \"metric\" = Celsius + km/h (default), \"imperial\" = Fahrenheit + mph.",
				"enum":        []string{"metric", "imperial"},
			},
			"forecast_days": map[string]any{
				"type":        "integer",
				"description": "Number of forecast days (0-7). Default 3. Set to 0 for current-weather-only.",
			},
		},
	}
}

func (t *WeatherTool) Execute(ctx context.Context, args map[string]any) *Result {
	location, _ := args["location"].(string)
	location = strings.TrimSpace(location)
	lat, hasLat := toFloat(args["lat"])
	lon, hasLon := toFloat(args["lon"])

	unitsRaw, _ := args["units"].(string)
	units := strings.ToLower(strings.TrimSpace(unitsRaw))
	if units != "imperial" {
		units = "metric"
	}

	forecastDays := 3
	if n, ok := toInt(args["forecast_days"]); ok {
		if n < 0 {
			n = 0
		}
		if n > 7 {
			n = 7
		}
		forecastDays = n
	}

	// Either a location OR an explicit coord pair — never both ignored.
	var placeLabel, country string
	if location != "" {
		geo, err := t.geocode(ctx, location)
		if err != nil {
			return ErrorResult(fmt.Sprintf("could not find location %q: %v", location, err))
		}
		lat, lon = geo.Lat, geo.Lon
		placeLabel = geo.Label
		country = geo.Country
	} else if !hasLat || !hasLon {
		return ErrorResult("either `location` or both `lat` and `lon` are required")
	} else {
		if lat < -90 || lat > 90 {
			return ErrorResult("lat must be between -90 and 90")
		}
		if lon < -180 || lon > 180 {
			return ErrorResult("lon must be between -180 and 180")
		}
		placeLabel = fmt.Sprintf("%.3f,%.3f", lat, lon)
	}

	fc, err := t.fetchForecast(ctx, lat, lon, units, forecastDays)
	if err != nil {
		return ErrorResult(fmt.Sprintf("weather lookup failed: %v", err))
	}

	out := formatWeather(placeLabel, country, lat, lon, units, fc, forecastDays)

	// Emit a structured widget alongside the text. The frontend's
	// WeatherCard renders these fields (location, temperature,
	// condition, humidity, wind, and a daily subfield that mirrors
	// Open-Meteo's shape). The text return value remains the same
	// so agents that aren't widget-aware (CLI, channels without
	// rich rendering) still get a readable answer.
	widgetData := map[string]any{
		"location":    placeLabel,
		"temperature": fc.Current.Temperature2m,
		"feels_like":  fc.Current.ApparentTemp,
		"condition":   describeCode(fc.Current.WeatherCode),
		"icon":        "", // frontend derives from weather_code
		"humidity":    fc.Current.RelativeHumidity,
		"wind_speed":  fc.Current.WindSpeed,
	}
	if forecastDays > 0 && len(fc.Daily.Time) > 0 {
		widgetData["daily"] = map[string]any{
			"time":                             fc.Daily.Time,
			"temperature_2m_max":               fc.Daily.TempMax,
			"temperature_2m_min":               fc.Daily.TempMin,
			"weather_code":                     fc.Daily.WeatherCode,
			"precipitation_probability_max":    fc.Daily.PrecipitationSum,
		}
	}
	return &Result{
		ForLLM:  out,
		ForUser: out,
		Widget:  &Widget{Type: "weather", Data: widgetData},
	}
}

// --- Open-Meteo response types ---

type openMeteoGeo struct {
	Results []struct {
		Name      string  `json:"name"`
		Admin1    string  `json:"admin1"`
		Country   string  `json:"country"`
		Latitude  float64 `json:"latitude"`
		Longitude float64 `json:"longitude"`
	} `json:"results"`
}

type geoHit struct {
	Lat     float64
	Lon     float64
	Label   string
	Country string
}

func (t *WeatherTool) geocode(ctx context.Context, q string) (*geoHit, error) {
	u := t.geocodeURL + "?count=1&language=en&format=json&name=" + url.QueryEscape(q)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	res, err := t.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		return nil, fmt.Errorf("geocoding HTTP %d: %s", res.StatusCode, truncateErr(string(body), 200))
	}
	var payload openMeteoGeo
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("geocoding decode: %w", err)
	}
	if len(payload.Results) == 0 {
		return nil, fmt.Errorf("no results")
	}
	g := payload.Results[0]
	// Label: "Tokyo, Tokyo, Japan" is redundant, collapse duplicates.
	label := g.Name
	if g.Admin1 != "" && !strings.EqualFold(g.Admin1, g.Name) {
		label += ", " + g.Admin1
	}
	return &geoHit{Lat: g.Latitude, Lon: g.Longitude, Label: label, Country: g.Country}, nil
}

type openMeteoForecast struct {
	Current struct {
		Time              string  `json:"time"`
		Temperature2m     float64 `json:"temperature_2m"`
		ApparentTemp      float64 `json:"apparent_temperature"`
		RelativeHumidity  float64 `json:"relative_humidity_2m"`
		WindSpeed         float64 `json:"wind_speed_10m"`
		WindDirection     float64 `json:"wind_direction_10m"`
		Precipitation     float64 `json:"precipitation"`
		WeatherCode       int     `json:"weather_code"`
		IsDay             int     `json:"is_day"`
	} `json:"current"`
	Daily struct {
		Time             []string  `json:"time"`
		WeatherCode      []int     `json:"weather_code"`
		TempMax          []float64 `json:"temperature_2m_max"`
		TempMin          []float64 `json:"temperature_2m_min"`
		PrecipitationSum []float64 `json:"precipitation_sum"`
		WindSpeedMax     []float64 `json:"wind_speed_10m_max"`
	} `json:"daily"`
	CurrentUnits struct {
		Temperature2m string `json:"temperature_2m"`
		WindSpeed     string `json:"wind_speed_10m"`
		Precipitation string `json:"precipitation"`
	} `json:"current_units"`
	Timezone string `json:"timezone"`
}

func (t *WeatherTool) fetchForecast(ctx context.Context, lat, lon float64, units string, days int) (*openMeteoForecast, error) {
	v := url.Values{}
	v.Set("latitude", fmt.Sprintf("%.4f", lat))
	v.Set("longitude", fmt.Sprintf("%.4f", lon))
	v.Set("timezone", "auto")
	v.Set("current", "temperature_2m,apparent_temperature,relative_humidity_2m,wind_speed_10m,wind_direction_10m,precipitation,weather_code,is_day")
	if days > 0 {
		v.Set("daily", "weather_code,temperature_2m_max,temperature_2m_min,precipitation_sum,wind_speed_10m_max")
		v.Set("forecast_days", fmt.Sprintf("%d", days))
	}
	if units == "imperial" {
		v.Set("temperature_unit", "fahrenheit")
		v.Set("wind_speed_unit", "mph")
		v.Set("precipitation_unit", "inch")
	}

	u := t.forecastURL + "?" + v.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	res, err := t.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		return nil, fmt.Errorf("forecast HTTP %d: %s", res.StatusCode, truncateErr(string(body), 200))
	}
	var out openMeteoForecast
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("forecast decode: %w", err)
	}
	return &out, nil
}

// --- formatting ---

// weatherCodeText maps WMO weather codes to human-legible text.
// Source: https://open-meteo.com/en/docs — "Weather variable documentation"
// Kept compact: every code users will actually see is listed, the gaps
// return "unusual conditions" instead of silently showing an integer.
var weatherCodeText = map[int]string{
	0: "clear sky",
	1: "mainly clear", 2: "partly cloudy", 3: "overcast",
	45: "fog", 48: "depositing rime fog",
	51: "light drizzle", 53: "drizzle", 55: "heavy drizzle",
	56: "light freezing drizzle", 57: "freezing drizzle",
	61: "light rain", 63: "rain", 65: "heavy rain",
	66: "light freezing rain", 67: "freezing rain",
	71: "light snow", 73: "snow", 75: "heavy snow",
	77: "snow grains",
	80: "light rain showers", 81: "rain showers", 82: "violent rain showers",
	85: "light snow showers", 86: "heavy snow showers",
	95: "thunderstorm",
	96: "thunderstorm with light hail", 99: "thunderstorm with heavy hail",
}

func describeCode(code int) string {
	if s, ok := weatherCodeText[code]; ok {
		return s
	}
	return fmt.Sprintf("unusual conditions (code %d)", code)
}

func compassDirection(deg float64) string {
	// 16-point compass; 22.5° per slice. Rounded so 11° → N, not NNE.
	points := []string{"N", "NNE", "NE", "ENE", "E", "ESE", "SE", "SSE",
		"S", "SSW", "SW", "WSW", "W", "WNW", "NW", "NNW"}
	idx := int((deg+11.25)/22.5) % 16
	if idx < 0 {
		idx += 16
	}
	return points[idx]
}

func formatWeather(place, country string, lat, lon float64, units string, fc *openMeteoForecast, days int) string {
	var sb strings.Builder

	header := place
	if country != "" {
		header = header + ", " + country
	}
	sb.WriteString("# Weather for " + header + "\n")
	sb.WriteString(fmt.Sprintf("_%.3f, %.3f · %s_\n\n", lat, lon, fc.Timezone))

	// Current
	cu := fc.CurrentUnits
	c := fc.Current
	dayNight := "day"
	if c.IsDay == 0 {
		dayNight = "night"
	}
	sb.WriteString("## Current (" + dayNight + ")\n")
	sb.WriteString(fmt.Sprintf("- **Conditions**: %s\n", describeCode(c.WeatherCode)))
	sb.WriteString(fmt.Sprintf("- **Temperature**: %.1f%s (feels like %.1f%s)\n",
		c.Temperature2m, cu.Temperature2m, c.ApparentTemp, cu.Temperature2m))
	sb.WriteString(fmt.Sprintf("- **Humidity**: %.0f%%\n", c.RelativeHumidity))
	sb.WriteString(fmt.Sprintf("- **Wind**: %.1f %s from the %s\n",
		c.WindSpeed, cu.WindSpeed, compassDirection(c.WindDirection)))
	if c.Precipitation > 0 {
		sb.WriteString(fmt.Sprintf("- **Precipitation**: %.1f %s\n", c.Precipitation, cu.Precipitation))
	}
	sb.WriteString(fmt.Sprintf("- **Observed**: %s\n", c.Time))

	// Forecast
	if days > 0 && len(fc.Daily.Time) > 0 {
		sb.WriteString("\n## Forecast\n")
		for i := range fc.Daily.Time {
			if i >= days {
				break
			}
			sb.WriteString(fmt.Sprintf("- **%s** — %s, high %.1f%s / low %.1f%s",
				fc.Daily.Time[i],
				describeCode(fc.Daily.WeatherCode[i]),
				fc.Daily.TempMax[i], cu.Temperature2m,
				fc.Daily.TempMin[i], cu.Temperature2m))
			if fc.Daily.PrecipitationSum[i] > 0 {
				sb.WriteString(fmt.Sprintf(", %.1f %s precip",
					fc.Daily.PrecipitationSum[i], cu.Precipitation))
			}
			sb.WriteString(fmt.Sprintf(", wind up to %.1f %s",
				fc.Daily.WindSpeedMax[i], cu.WindSpeed))
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

// --- small helpers ---

func toFloat(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case float32:
		return float64(x), true
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	case json.Number:
		f, err := x.Float64()
		return f, err == nil
	case string:
		var f float64
		if _, err := fmt.Sscanf(strings.TrimSpace(x), "%f", &f); err == nil {
			return f, true
		}
	}
	return 0, false
}

// truncateErr shortens HTTP error bodies so a 500-line stack trace
// doesn't blow up the agent's context. Renamed to avoid colliding
// with self_improve.go's `truncate`.
func truncateErr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
