// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package agent

import (
	"strings"
	"unicode"
)

// NormalizeQuery cleans up user input before processing — fixes common typos,
// normalizes whitespace, and corrects STT artifacts.
// This runs before the LLM sees the message, improving search and tool accuracy.
func NormalizeQuery(input string) string {
	if input == "" { return input }

	s := input

	// 1. Fix common STT artifacts
	sttFixes := map[string]string{
		"weather in new york":  "weather in New York",
		"whether":              "weather",  // common STT confusion
		"there vs their":       "there vs their",
		"its vs it's":          "its vs it's",
	}
	lower := strings.ToLower(s)
	for pattern, fix := range sttFixes {
		if strings.Contains(lower, pattern) {
			s = strings.Replace(strings.ToLower(s), pattern, fix, 1)
		}
	}

	// 2. Fix doubled spaces and punctuation
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	s = strings.ReplaceAll(s, " ,", ",")
	s = strings.ReplaceAll(s, " .", ".")
	s = strings.ReplaceAll(s, " ?", "?")
	s = strings.ReplaceAll(s, " !", "!")

	// 3. Fix common keyboard typos (adjacent key swaps)
	typoFixes := map[string]string{
		"teh ":    "the ",
		"taht ":   "that ",
		"adn ":    "and ",
		"hte ":    "the ",
		"wiht ":   "with ",
		"fo ":     "of ",
		"si ":     "is ",
		"ti ":     "it ",
		"nto ":    "not ",
		"waht ":   "what ",
		"hwo ":    "how ",
		"woudl ":  "would ",
		"shoudl ": "should ",
		"coudl ":  "could ",
		"dont ":   "don't ",
		"doesnt ": "doesn't ",
		"cant ":   "can't ",
		"wont ":   "won't ",
		"didnt ":  "didn't ",
		"isnt ":   "isn't ",
		"wasnt ":  "wasn't ",
		"havent ": "haven't ",
		"hasnt ":  "hasn't ",
		"wouldnt ":"wouldn't ",
		"shouldnt ":"shouldn't ",
		"couldnt ":"couldn't ",
		"im ":     "I'm ",
		"ive ":    "I've ",
		"id ":     "I'd ",
		"ill ":    "I'll ",
		"youre ":  "you're ",
		"theyre ": "they're ",
		"theres ": "there's ",
		"whats ":  "what's ",
		"hows ":   "how's ",
		"whos ":   "who's ",
		"wheres ": "where's ",
		"thats ":  "that's ",
		"lets ":   "let's ",
		"heres ":  "here's ",
	}
	for typo, fix := range typoFixes {
		s = strings.ReplaceAll(s, typo, fix)
		// Also fix at start of string
		if strings.HasPrefix(s, typo) {
			s = fix + s[len(typo):]
		}
	}

	// 4. Capitalize first letter of sentences
	s = capitalizeFirst(s)

	return strings.TrimSpace(s)
}

func capitalizeFirst(s string) string {
	if s == "" { return s }
	runes := []rune(s)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}

// NormalizeSearchQuery is a stricter version for search — removes filler words
// and normalizes the query for better search engine results.
func NormalizeSearchQuery(query string) string {
	q := NormalizeQuery(query)

	// Remove filler/hedge words that hurt search
	fillers := []string{
		"please ", "can you ", "could you ", "would you ",
		"i want to ", "i need to ", "i'd like to ",
		"help me ", "show me ", "tell me ",
		"just ", "basically ", "actually ",
		"um ", "uh ", "like ",
	}
	lower := strings.ToLower(q)
	for _, f := range fillers {
		if strings.HasPrefix(lower, f) {
			q = q[len(f):]
			lower = strings.ToLower(q)
		}
	}

	return capitalizeAfterStrip(q)
}

// capitalizeAfterStrip ensures the first letter is capitalized after filler removal.
func capitalizeAfterStrip(s string) string {
	s = strings.TrimSpace(s)
	if s == "" { return s }
	return capitalizeFirst(s)
}
