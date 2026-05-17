// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package trends

import (
	"regexp"
	"strings"
)

// relevance.go — Token overlap relevance scoring.
// Rewritten from last30days relevance.py (148 lines).

var stopwords = map[string]bool{
	"the": true, "a": true, "an": true, "to": true, "for": true, "how": true,
	"is": true, "in": true, "of": true, "on": true, "and": true, "with": true,
	"from": true, "by": true, "at": true, "this": true, "that": true, "it": true,
	"my": true, "your": true, "their": true, "its": true, "was": true, "were": true,
	"be": true, "been": true, "being": true, "have": true, "has": true, "had": true,
	"do": true, "does": true, "did": true, "will": true, "would": true, "could": true,
	"should": true, "may": true, "might": true, "can": true, "not": true, "no": true,
	"or": true, "but": true, "if": true, "then": true, "so": true, "as": true,
	"about": true, "up": true, "out": true, "just": true, "also": true, "very": true,
	"what": true, "which": true, "who": true, "whom": true, "where": true, "when": true,
	"why": true, "all": true, "each": true, "every": true, "both": true, "few": true,
	"more": true, "most": true, "other": true, "some": true, "such": true, "than": true,
}

// LowSignalQueryTokens are words too generic to serve as sole topic-match signal.
var LowSignalQueryTokens = map[string]bool{
	"odds": true, "review": true, "news": true, "update": true, "latest": true,
	"best": true, "top": true, "new": true, "good": true, "bad": true,
}

var wordRe = regexp.MustCompile(`\w+`)

// TokenOverlapRelevance computes relevance as fraction of informative query tokens found in text.
func TokenOverlapRelevance(query, text string) float64 {
	return tokenOverlapRelevance(query, text)
}

// InformativeTokens extracts non-stopword tokens from text.
func InformativeTokens(text string) []string {
	words := wordRe.FindAllString(strings.ToLower(text), -1)
	var tokens []string
	for _, w := range words {
		if !stopwords[w] && len(w) > 1 { tokens = append(tokens, w) }
	}
	return tokens
}

// EntityOverlap checks if key entity tokens from query appear in text.
func EntityOverlap(query, text string) float64 {
	qTokens := InformativeTokens(query)
	if len(qTokens) == 0 { return 0.5 }

	tLower := strings.ToLower(text)
	matches := 0
	for _, t := range qTokens {
		if strings.Contains(tLower, t) { matches++ }
	}
	return float64(matches) / float64(len(qTokens))
}

// IsLowSignalMatch returns true if the match is based only on generic tokens.
func IsLowSignalMatch(query, text string) bool {
	qTokens := InformativeTokens(query)
	tLower := strings.ToLower(text)
	informativeMatches := 0
	for _, t := range qTokens {
		if !LowSignalQueryTokens[t] && strings.Contains(tLower, t) {
			informativeMatches++
		}
	}
	return informativeMatches == 0
}
