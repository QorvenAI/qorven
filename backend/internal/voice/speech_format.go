// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package voice

import (
	"regexp"
	"strings"
)

// FormatForSpeech cleans text for natural TTS output.
func FormatForSpeech(text string) string {
	s := text

	// Strip markdown
	s = regexp.MustCompile(`\*\*(.*?)\*\*`).ReplaceAllString(s, "$1")
	s = regexp.MustCompile(`\*(.*?)\*`).ReplaceAllString(s, "$1")
	s = regexp.MustCompile("`(.*?)`").ReplaceAllString(s, "$1")
	s = regexp.MustCompile(`#{1,6}\s`).ReplaceAllString(s, "")
	s = regexp.MustCompile(`\[([^\]]+)\]\([^)]+\)`).ReplaceAllString(s, "$1")

	// Temperature: "23°C" or "23 C" → "23 degrees Celsius"
	s = regexp.MustCompile(`(\d+)\s*°?\s*C\b`).ReplaceAllString(s, "$1 degrees Celsius")
	s = regexp.MustCompile(`(\d+)\s*°?\s*F\b`).ReplaceAllString(s, "$1 degrees Fahrenheit")

	// Speed: "9 km/h" → "9 kilometers per hour"
	s = regexp.MustCompile(`(\d+)\s*km/h`).ReplaceAllString(s, "$1 kilometers per hour")

	// Humidity: "77%" → "77 percent"
	s = regexp.MustCompile(`(\d+)%`).ReplaceAllString(s, "$1 percent")

	// Clean long addresses — keep only city/country
	// Match patterns like "Street, Area, City, Region, Country, Postcode"
	s = regexp.MustCompile(`[^,]+,\s*[^,]+,\s*[^,]+,\s*([^,]+),\s*[^,]*,\s*([^,]+),\s*[^,]*,\s*([^,]+),\s*[^,]*`).
		ReplaceAllString(s, "$1, $3")

	// Remove bullet points
	s = strings.ReplaceAll(s, "- ", "")
	s = strings.ReplaceAll(s, "• ", "")

	// Clean multiple newlines
	s = regexp.MustCompile(`\n{2,}`).ReplaceAllString(s, ". ")
	s = strings.ReplaceAll(s, "\n", ". ")

	// Clean double spaces
	s = regexp.MustCompile(`\s{2,}`).ReplaceAllString(s, " ")

	return strings.TrimSpace(s)
}
