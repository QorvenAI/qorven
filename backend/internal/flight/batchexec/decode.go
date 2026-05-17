// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package batchexec

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// antiXSSI is the prefix Google prepends to JSON responses to prevent XSSI.
const antiXSSI = ")]}'"

// ErrEmptyResponse is returned when the response body is empty after stripping.
var ErrEmptyResponse = errors.New("empty response after stripping anti-XSSI prefix")

// StripAntiXSSI removes the ")]}\'" prefix and any leading whitespace/newlines.
func StripAntiXSSI(body []byte) []byte {
	body = bytes.TrimSpace(body)
	body = bytes.TrimPrefix(body, []byte(antiXSSI))
	return bytes.TrimSpace(body)
}

// DecodeFlightResponse parses a Google Flights API response.
//
// The response format after stripping anti-XSSI is a JSON array. The flight
// data is at: parsed[0][2] (a JSON string), which when parsed again yields
// the actual flight results at indices [2] and [3].
//
// Returns the raw parsed result (the inner JSON object) and any error.
func DecodeFlightResponse(body []byte) (any, error) {
	stripped := StripAntiXSSI(body)
	if len(stripped) == 0 {
		return nil, ErrEmptyResponse
	}

	// First parse: outer array
	var outer []any
	if err := json.Unmarshal(stripped, &outer); err != nil {
		return nil, fmt.Errorf("decode outer: %w", err)
	}

	if len(outer) == 0 {
		return nil, fmt.Errorf("outer array empty")
	}

	// outer[0] should be an array
	first, ok := outer[0].([]any)
	if !ok {
		return nil, fmt.Errorf("unexpected response format")
	}

	if len(first) < 3 {
		return nil, fmt.Errorf("outer[0] too short: %d elements", len(first))
	}

	// first[2] is a JSON string containing the actual flight data
	innerJSON, ok := first[2].(string)
	if !ok {
		// It might already be a parsed structure
		return first[2], nil
	}

	var inner any
	if err := json.Unmarshal([]byte(innerJSON), &inner); err != nil {
		return nil, fmt.Errorf("decode inner: %w", err)
	}

	return inner, nil
}

// DecodeBatchResponse parses a batchexecute response (used for Hotels).
//
// The batchexecute response format is length-prefixed lines after the anti-XSSI
// prefix. Each line starts with a decimal length, then a JSON array.
//
// Returns a slice of response entries, each being the parsed JSON.
func DecodeBatchResponse(body []byte) ([]any, error) {
	stripped := StripAntiXSSI(body)
	if len(stripped) == 0 {
		return nil, ErrEmptyResponse
	}

	// Try direct JSON parse first (sometimes it's just a JSON array)
	var direct []any
	if err := json.Unmarshal(stripped, &direct); err == nil {
		return direct, nil
	}

	// Otherwise, parse length-prefixed format:
	// Each entry is: \n<length>\n<json-array>\n
	var results []any
	text := string(stripped)

	for len(text) > 0 {
		text = strings.TrimSpace(text)
		if len(text) == 0 {
			break
		}

		// Read the length line
		nlIdx := strings.Index(text, "\n")
		if nlIdx < 0 {
			// Single remaining chunk, try to parse directly
			var chunk any
			if err := json.Unmarshal([]byte(text), &chunk); err == nil {
				results = append(results, chunk)
			}
			break
		}

		// Skip the length, read the JSON that follows
		text = text[nlIdx+1:]
		if len(text) == 0 {
			break
		}

		// Find the JSON array: starts with [
		arrStart := strings.Index(text, "[")
		if arrStart < 0 {
			break
		}
		text = text[arrStart:]

		// Parse the JSON array, letting Go's decoder find the end
		dec := json.NewDecoder(strings.NewReader(text))
		var entry any
		if err := dec.Decode(&entry); err != nil {
			// Try to skip past this malformed chunk
			nextNL := strings.Index(text, "\n")
			if nextNL < 0 {
				break
			}
			text = text[nextNL:]
			continue
		}

		results = append(results, entry)

		// Advance past what was consumed
		consumed := dec.InputOffset()
		text = text[consumed:]
	}

	if len(results) == 0 {
		return nil, fmt.Errorf("no parseable entries in batch response")
	}

	return results, nil
}

// ExtractFlightData extracts flight entries from a decoded flight response.
//
// After DecodeFlightResponse, the inner result should be an array where
// indices [2] and [3] contain flight data arrays. Each of those has
// sub-array [0] containing individual flight entries.
func ExtractFlightData(inner any) ([]any, error) {
	arr, ok := inner.([]any)
	if !ok {
		return nil, fmt.Errorf("unexpected flight data format")
	}

	var flights []any
	for _, idx := range []int{2, 3} {
		if idx >= len(arr) {
			continue
		}
		bucket, ok := arr[idx].([]any)
		if !ok || len(bucket) == 0 {
			continue
		}
		items, ok := bucket[0].([]any)
		if !ok {
			continue
		}
		flights = append(flights, items...)
	}

	if len(flights) == 0 {
		return nil, fmt.Errorf("no flight data at indices [2] or [3]")
	}

	return flights, nil
}
