// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// weatherMockServer stands in for geocoding + forecast endpoints so the
// tests never touch the real Open-Meteo API. The handler routes by URL
// path — both endpoints live on the same test server for simplicity.
func weatherMockServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "/search"):
			// Return one hit for "Tokyo", zero hits for "NowheresVille".
			q := r.URL.Query().Get("name")
			if q == "NowheresVille" {
				_, _ = w.Write([]byte(`{"results":[]}`))
				return
			}
			_, _ = w.Write([]byte(`{"results":[{
				"name":"Tokyo","admin1":"Tokyo","country":"Japan",
				"latitude":35.6895,"longitude":139.6917
			}]}`))

		case strings.Contains(r.URL.Path, "/forecast"):
			// Shape matches open-meteo exactly — keep the test honest
			// about what we parse from upstream.
			_, _ = w.Write([]byte(`{
				"timezone":"Asia/Tokyo",
				"current":{
					"time":"2026-04-21T10:00",
					"temperature_2m":21.4,
					"apparent_temperature":20.1,
					"relative_humidity_2m":58,
					"wind_speed_10m":12.3,
					"wind_direction_10m":225,
					"precipitation":0,
					"weather_code":2,
					"is_day":1
				},
				"current_units":{
					"temperature_2m":"°C",
					"wind_speed_10m":"km/h",
					"precipitation":"mm"
				},
				"daily":{
					"time":["2026-04-21","2026-04-22","2026-04-23"],
					"weather_code":[2,61,3],
					"temperature_2m_max":[23.0,20.5,22.0],
					"temperature_2m_min":[15.0,14.8,16.2],
					"precipitation_sum":[0,4.5,0.1],
					"wind_speed_10m_max":[15.0,22.0,12.0]
				}
			}`))

		default:
			http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
		}
	}))
}

// newTestWeatherTool wires a WeatherTool at the mock server's base URL
// using the exported test hooks. Avoids a global to keep tests parallelisable.
func newTestWeatherTool(baseURL string) *WeatherTool {
	return &WeatherTool{
		http:        &http.Client{Timeout: 2 * time.Second},
		geocodeURL:  baseURL + "/v1/search",
		forecastURL: baseURL + "/v1/forecast",
	}
}

func TestWeather_ByLocation_Metric(t *testing.T) {
	srv := weatherMockServer(t)
	defer srv.Close()

	tool := newTestWeatherTool(srv.URL)
	r := tool.Execute(context.Background(), map[string]any{
		"location":      "Tokyo",
		"units":         "metric",
		"forecast_days": 3,
	})
	if r.IsError {
		t.Fatalf("unexpected error result: %s", r.ForLLM)
	}
	// Place header must mention Tokyo + Japan.
	if !strings.Contains(r.ForLLM, "Tokyo") || !strings.Contains(r.ForLLM, "Japan") {
		t.Errorf("place label missing: %q", r.ForLLM)
	}
	// Current temperature should render to 1 decimal with °C.
	if !strings.Contains(r.ForLLM, "21.4°C") {
		t.Errorf("expected 21.4°C in output, got: %s", r.ForLLM)
	}
	// Compass: wind direction 225 → SW.
	if !strings.Contains(r.ForLLM, "SW") {
		t.Errorf("expected SW wind direction, got: %s", r.ForLLM)
	}
	// Forecast block must list all three days.
	for _, d := range []string{"2026-04-21", "2026-04-22", "2026-04-23"} {
		if !strings.Contains(r.ForLLM, d) {
			t.Errorf("forecast day %s missing from output", d)
		}
	}
	// WMO code 2 → "partly cloudy"; 61 → "light rain".
	if !strings.Contains(r.ForLLM, "partly cloudy") {
		t.Errorf("wmo code 2 not mapped to partly cloudy")
	}
	if !strings.Contains(r.ForLLM, "light rain") {
		t.Errorf("wmo code 61 not mapped to light rain")
	}
}

func TestWeather_ByLatLon_ImperialUnits(t *testing.T) {
	// Imperial units must be propagated in the forecast query. We assert
	// via a custom server that inspects the query string.
	gotUnits := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/forecast") {
			gotUnits = r.URL.Query().Get("temperature_unit")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"timezone":"UTC",
			"current":{"time":"2026-04-21T10:00","temperature_2m":72.0,"apparent_temperature":70.0,"relative_humidity_2m":40,"wind_speed_10m":5.0,"wind_direction_10m":0,"precipitation":0,"weather_code":0,"is_day":1},
			"current_units":{"temperature_2m":"°F","wind_speed_10m":"mph","precipitation":"inch"},
			"daily":{"time":[],"weather_code":[],"temperature_2m_max":[],"temperature_2m_min":[],"precipitation_sum":[],"wind_speed_10m_max":[]}
		}`))
	}))
	defer srv.Close()

	tool := newTestWeatherTool(srv.URL)
	r := tool.Execute(context.Background(), map[string]any{
		"lat":   40.7128,
		"lon":   -74.0060,
		"units": "imperial",
	})
	if r.IsError {
		t.Fatalf("unexpected error: %s", r.ForLLM)
	}
	if gotUnits != "fahrenheit" {
		t.Errorf("expected temperature_unit=fahrenheit, got %q", gotUnits)
	}
	if !strings.Contains(r.ForLLM, "°F") {
		t.Errorf("imperial output should contain °F, got: %s", r.ForLLM)
	}
}

func TestWeather_LocationNotFound(t *testing.T) {
	srv := weatherMockServer(t)
	defer srv.Close()
	tool := newTestWeatherTool(srv.URL)

	r := tool.Execute(context.Background(), map[string]any{"location": "NowheresVille"})
	if !r.IsError {
		t.Fatalf("expected error for unresolvable location")
	}
	if !strings.Contains(r.ForLLM, "NowheresVille") {
		t.Errorf("error should cite the unresolvable location name")
	}
}

func TestWeather_MissingArgs(t *testing.T) {
	// No location AND no lat/lon → explicit error, not a silent pass-through.
	srv := weatherMockServer(t)
	defer srv.Close()
	tool := newTestWeatherTool(srv.URL)

	r := tool.Execute(context.Background(), map[string]any{})
	if !r.IsError {
		t.Fatal("expected error when both location and lat/lon are absent")
	}
	if !strings.Contains(r.ForLLM, "required") {
		t.Errorf("error message should mention 'required'; got %q", r.ForLLM)
	}
}

func TestWeather_OutOfRangeCoords(t *testing.T) {
	srv := weatherMockServer(t)
	defer srv.Close()
	tool := newTestWeatherTool(srv.URL)

	// Lat 200 is nonsense; the tool must reject before round-tripping.
	r := tool.Execute(context.Background(), map[string]any{"lat": 200.0, "lon": 0.0})
	if !r.IsError {
		t.Fatal("lat=200 should be rejected")
	}
}

func TestWeather_CurrentOnly_NoForecast(t *testing.T) {
	// forecast_days=0 should still succeed — user just wants "now".
	srv := weatherMockServer(t)
	defer srv.Close()
	tool := newTestWeatherTool(srv.URL)

	r := tool.Execute(context.Background(), map[string]any{
		"location":      "Tokyo",
		"forecast_days": 0,
	})
	if r.IsError {
		t.Fatalf("forecast_days=0 should be OK: %s", r.ForLLM)
	}
	// With days=0, the output must NOT include the Forecast header.
	if strings.Contains(r.ForLLM, "## Forecast") {
		t.Errorf("forecast_days=0 output unexpectedly contains forecast block")
	}
}

func TestWeather_ForecastDaysClamped(t *testing.T) {
	// forecast_days=99 must silently clamp to 7, not propagate.
	gotDays := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/forecast") {
			gotDays = r.URL.Query().Get("forecast_days")
		}
		if strings.Contains(r.URL.Path, "/search") {
			_, _ = w.Write([]byte(`{"results":[{"name":"X","country":"Y","latitude":0,"longitude":0}]}`))
			return
		}
		_, _ = w.Write([]byte(`{
			"timezone":"UTC",
			"current":{"time":"","temperature_2m":0,"apparent_temperature":0,"relative_humidity_2m":0,"wind_speed_10m":0,"wind_direction_10m":0,"precipitation":0,"weather_code":0,"is_day":1},
			"current_units":{"temperature_2m":"°C","wind_speed_10m":"km/h","precipitation":"mm"},
			"daily":{"time":[],"weather_code":[],"temperature_2m_max":[],"temperature_2m_min":[],"precipitation_sum":[],"wind_speed_10m_max":[]}
		}`))
	}))
	defer srv.Close()

	tool := newTestWeatherTool(srv.URL)
	_ = tool.Execute(context.Background(), map[string]any{
		"location":      "X",
		"forecast_days": 99,
	})
	if gotDays != "7" {
		t.Errorf("forecast_days=99 should clamp to 7, upstream saw %q", gotDays)
	}
}

func TestWeather_ToolMetadata(t *testing.T) {
	// Definition consistency — Registry builds schemas from these.
	tool := NewWeatherTool()
	if tool.Name() != "weather" {
		t.Errorf("name = %q, want weather", tool.Name())
	}
	params := tool.Parameters()
	props, _ := params["properties"].(map[string]any)
	for _, must := range []string{"location", "lat", "lon", "units", "forecast_days"} {
		if _, ok := props[must]; !ok {
			t.Errorf("missing parameter %q in schema", must)
		}
	}
}

func TestCompassDirection(t *testing.T) {
	// Boundary values for the 16-point compass — ensure the +11.25
	// offset lands the cardinal directions correctly.
	cases := []struct {
		deg  float64
		want string
	}{
		{0, "N"}, {10, "N"}, {11.24, "N"},
		{11.26, "NNE"},
		{45, "NE"},
		{90, "E"},
		{180, "S"},
		{225, "SW"},
		{270, "W"},
		{348.76, "N"}, // wrap around
		{360, "N"},
	}
	for _, c := range cases {
		if got := compassDirection(c.deg); got != c.want {
			t.Errorf("compass(%.2f) = %q, want %q", c.deg, got, c.want)
		}
	}
}

func TestDescribeCode(t *testing.T) {
	if got := describeCode(0); got != "clear sky" {
		t.Errorf("code 0 = %q, want clear sky", got)
	}
	if got := describeCode(999); !strings.Contains(got, "999") {
		t.Errorf("unknown code should surface the number; got %q", got)
	}
}

// Silence unused-import warning if json isn't referenced directly.
var _ = json.Number("")
