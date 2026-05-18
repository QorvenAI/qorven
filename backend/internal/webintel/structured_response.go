// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package webintel

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// StructuredWeatherResponse generates a rich, deterministic weather response.
// The LLM does NOT generate this — we do. No hallucination possible.
func StructuredWeatherResponse(data map[string]any) string {
	loc := str(data["location"])
	temp := toF(data["temperature"])
	feels := toF(data["feels_like"])
	cond := str(data["condition"])
	humid := toF(data["humidity"])
	wind := toF(data["wind_speed"])
	icon := str(data["icon"])

	// Parse daily forecast
	daily, _ := data["daily"].(map[string]any)
	times, _ := daily["time"].([]any)
	maxTemps, _ := daily["temperature_2m_max"].([]any)
	minTemps, _ := daily["temperature_2m_min"].([]any)
	precip, _ := daily["precipitation_probability_max"].([]any)
	codes, _ := daily["weather_code"].([]any)

	var sb strings.Builder

	// Current conditions
	sb.WriteString(fmt.Sprintf("## %s Weather %s\n\n", shortLoc(loc), icon))
	sb.WriteString(fmt.Sprintf("**Right now** it's **%.0f°C** and %s.\n\n", temp, strings.ToLower(cond)))

	// Details
	sb.WriteString("### Current Conditions\n")
	sb.WriteString(fmt.Sprintf("- 🌡️ **Temperature:** %.0f°C (feels like %.0f°C)\n", temp, feels))
	sb.WriteString(fmt.Sprintf("- 💧 **Humidity:** %.0f%%\n", humid))
	sb.WriteString(fmt.Sprintf("- 💨 **Wind:** %.0f km/h\n", wind))
	sb.WriteString(fmt.Sprintf("- ☁️ **Conditions:** %s\n", cond))

	// Today's high/low
	if len(maxTemps) > 0 && len(minTemps) > 0 {
		sb.WriteString(fmt.Sprintf("- 📊 **Today's range:** %.0f°C – %.0f°C\n", toF(minTemps[0]), toF(maxTemps[0])))
	}
	if len(precip) > 0 {
		sb.WriteString(fmt.Sprintf("- 🌧️ **Precipitation chance:** %.0f%%\n", toF(precip[0])))
	}

	// Multi-day forecast
	if len(times) > 1 {
		sb.WriteString("\n### Forecast\n")
		for i := 0; i < min(5, len(times)); i++ {
			day := "Today"
			if i > 0 {
				t, err := time.Parse("2006-01-02", str(times[i]))
				if err == nil { day = t.Format("Mon Jan 2") }
			}
			hi := "—"
			lo := "—"
			rain := ""
			wx := ""
			if i < len(maxTemps) { hi = fmt.Sprintf("%.0f°C", toF(maxTemps[i])) }
			if i < len(minTemps) { lo = fmt.Sprintf("%.0f°C", toF(minTemps[i])) }
			if i < len(precip) && toF(precip[i]) > 0 { rain = fmt.Sprintf(" 🌧️%.0f%%", toF(precip[i])) }
			if i < len(codes) { wx = " " + condFromCode(int(toF(codes[i]))) }
			sb.WriteString(fmt.Sprintf("- **%s:** %s / %s%s%s\n", day, hi, lo, wx, rain))
		}
	}

	// Advice
	sb.WriteString("\n### What to Know\n")
	if temp < 10 {
		sb.WriteString("- 🧥 Dress warmly — layers recommended\n")
	} else if temp < 20 {
		sb.WriteString("- 🧥 A light jacket would be comfortable\n")
	} else if temp > 30 {
		sb.WriteString("- ☀️ Stay hydrated and use sunscreen\n")
	} else {
		sb.WriteString("- 👕 Comfortable weather for outdoor activities\n")
	}
	if len(precip) > 0 && toF(precip[0]) > 40 {
		sb.WriteString("- ☂️ Bring an umbrella — rain is likely\n")
	}
	if wind > 30 {
		sb.WriteString("- 💨 Strong winds — secure loose items\n")
	}

	return sb.String()
}

func shortLoc(loc string) string {
	parts := strings.Split(loc, ",")
	if len(parts) >= 2 { return strings.TrimSpace(parts[0]) + ", " + strings.TrimSpace(parts[len(parts)-1]) }
	return loc
}

func str(v any) string {
	if s, ok := v.(string); ok { return s }
	return fmt.Sprintf("%v", v)
}

func toF(v any) float64 {
	switch n := v.(type) {
	case float64: return n
	case float32: return float64(n)
	case int: return float64(n)
	}
	return 0
}

func condFromCode(code int) string {
	switch {
	case code == 0: return "☀️ Clear"
	case code <= 3: return "⛅ Partly Cloudy"
	case code <= 48: return "🌫️ Fog"
	case code <= 57: return "🌦️ Drizzle"
	case code <= 67: return "🌧️ Rain"
	case code <= 77: return "❄️ Snow"
	case code <= 86: return "🌨️ Snow Showers"
	case code == 95: return "⛈️ Thunderstorm"
	default: return "⛈️ Severe Storm"
	}
}

func round(f float64) float64 { return math.Round(f) }
