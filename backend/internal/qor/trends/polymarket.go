// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package trends

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"
)

// polymarket.go — Polymarket prediction market search via Gamma API.
// Free, no auth. Real money signals — odds backed by actual dollars.

const gammaSearchURL = "https://gamma-api.polymarket.com/public-search"

var (
	polyDepth = DepthConfig{Quick: 1, Default: 3, Deep: 4}
	polyCap   = DepthConfig{Quick: 5, Default: 15, Deep: 25}
)

type PolymarketSource struct {
	client *HTTPClient
}

func NewPolymarketSource() *PolymarketSource {
	return &PolymarketSource{client: NewHTTPClient(15 * time.Second)}
}

func (p *PolymarketSource) Name() string { return "polymarket" }

// Search queries Polymarket for prediction markets related to the topic.
func (p *PolymarketSource) Search(ctx context.Context, topic string, depth string) ([]SourceItem, error) {
	queries := expandQueries(topic)
	pages := polyDepth.Get(depth)
	cap := polyCap.Get(depth)

	var allEvents []polyEvent
	seen := map[string]bool{}

	for _, q := range queries {
		for page := 1; page <= pages; page++ {
			events, err := p.searchPage(ctx, q, page)
			if err != nil { continue }
			for _, e := range events {
				if !seen[e.ID] {
					seen[e.ID] = true
					allEvents = append(allEvents, e)
				}
			}
		}
	}

	// Score and filter by relevance
	var items []SourceItem
	for _, e := range allEvents {
		rel := tokenOverlapRelevance(topic, e.Title+" "+e.Description)
		if rel < 0.15 { continue }

		item := p.eventToItem(e, topic)
		hint := rel
		item.RelevanceHint = hint
		items = append(items, item)
	}

	// Sort by volume (real money = strongest signal)
	sort.Slice(items, func(i, j int) bool {
		vi := items[i].Engagement["volume"]
		vj := items[j].Engagement["volume"]
		return vi > vj
	})

	if len(items) > cap { items = items[:cap] }
	return items, nil
}

type polyEvent struct {
	ID          string       `json:"id"`
	Title       string       `json:"title"`
	Description string       `json:"description"`
	Slug        string       `json:"slug"`
	Active      bool         `json:"active"`
	Closed      bool         `json:"closed"`
	Volume      float64      `json:"volume"`
	Liquidity   float64      `json:"liquidity"`
	StartDate   string       `json:"startDate"`
	EndDate     string       `json:"endDate"`
	Markets     []polyMarket `json:"markets"`
	Tags        []polyTag    `json:"tags"`
}

type polyMarket struct {
	ID            string  `json:"id"`
	Question      string  `json:"question"`
	OutcomeYes    float64 `json:"outcomePriceYes,string"`
	OutcomeNo     float64 `json:"outcomePriceNo,string"`
	Volume        float64 `json:"volume,string"`
	Liquidity     float64 `json:"liquidity,string"`
	Active        bool    `json:"active"`
	Closed        bool    `json:"closed"`
}

type polyTag struct {
	Label string `json:"label"`
	Slug  string `json:"slug"`
}

func (p *PolymarketSource) searchPage(ctx context.Context, query string, page int) ([]polyEvent, error) {
	u := fmt.Sprintf("%s?query=%s&page=%d", gammaSearchURL, url.QueryEscape(query), page)
	data, err := p.client.Get(ctx, u, nil)
	if err != nil { return nil, err }

	var events []polyEvent
	if err := json.Unmarshal(data, &events); err != nil { return nil, err }
	return events, nil
}

func (p *PolymarketSource) eventToItem(e polyEvent, topic string) SourceItem {
	// Build body from markets
	var body strings.Builder
	body.WriteString(e.Description + "\n\n")

	totalVolume := 0.0
	for _, m := range e.Markets {
		if m.Closed { continue }
		yesOdds := m.OutcomeYes * 100
		noOdds := m.OutcomeNo * 100
		body.WriteString(fmt.Sprintf("📊 %s\n  YES: %.0f%% | NO: %.0f%% | Volume: $%.0f\n\n",
			m.Question, yesOdds, noOdds, m.Volume))
		totalVolume += m.Volume
	}

	// Confidence from volume
	confidence := "low"
	if totalVolume > 10000 { confidence = "med" }
	if totalVolume > 100000 { confidence = "high" }

	engagement := map[string]float64{
		"volume":    totalVolume,
		"liquidity": e.Liquidity,
		"markets":   float64(len(e.Markets)),
	}

	// Engagement score: log-scale volume
	engScore := 0.0
	if totalVolume > 0 { engScore = math.Log(totalVolume) / math.Log(1000000) }
	if engScore > 1 { engScore = 1 }

	published := parseTime(e.StartDate)

	return SourceItem{
		ItemID:         "pm_" + e.ID,
		Source:         "polymarket",
		Title:          e.Title,
		Body:           body.String(),
		URL:            "https://polymarket.com/event/" + e.Slug,
		PublishedAt:    published,
		DateConfidence: confidence,
		Engagement:     engagement,
		EngagementScore: &engScore,
		Metadata: map[string]any{
			"total_volume": totalVolume,
			"active":       e.Active,
			"num_markets":  len(e.Markets),
		},
	}
}

// ── Shared utilities ──

var prefixRe = regexp.MustCompile(`(?i)^(last \d+ days?\s+|what(?:'s| is| are) (?:people saying about|happening with)\s+|how (?:is|are)\s+|tell me about\s+|research\s+)`)

func extractCoreSubject(topic string) string {
	return strings.TrimSpace(prefixRe.ReplaceAllString(strings.TrimSpace(topic), ""))
}

var noiseWords = map[string]bool{
	"the": true, "a": true, "an": true, "in": true, "on": true, "at": true,
	"of": true, "for": true, "and": true, "or": true, "to": true, "is": true,
	"are": true, "was": true, "were": true, "will": true, "be": true,
}

func expandQueries(topic string) []string {
	core := extractCoreSubject(topic)
	queries := []string{core}
	words := strings.Fields(core)
	if len(words) >= 2 {
		for _, w := range words {
			if len(w) > 1 && !noiseWords[strings.ToLower(w)] {
				queries = append(queries, w)
			}
		}
	}
	if !strings.EqualFold(strings.TrimSpace(topic), core) {
		queries = append(queries, strings.TrimSpace(topic))
	}
	seen := map[string]bool{}
	var unique []string
	for _, q := range queries {
		low := strings.ToLower(strings.TrimSpace(q))
		if low != "" && !seen[low] {
			seen[low] = true
			unique = append(unique, strings.TrimSpace(q))
		}
	}
	if len(unique) > 6 { unique = unique[:6] }
	return unique
}

func tokenOverlapRelevance(query, text string) float64 {
	qTokens := strings.Fields(strings.ToLower(query))
	tLower := strings.ToLower(text)
	matches := 0
	for _, t := range qTokens {
		if !noiseWords[t] && strings.Contains(tLower, t) { matches++ }
	}
	informative := 0
	for _, t := range qTokens {
		if !noiseWords[t] { informative++ }
	}
	if informative == 0 { return 0.5 }
	return float64(matches) / float64(informative)
}

func parseTime(s string) *time.Time {
	for _, f := range []string{time.RFC3339, "2006-01-02T15:04:05Z", "2006-01-02"} {
		if t, err := time.Parse(f, s); err == nil { return &t }
	}
	return nil
}
