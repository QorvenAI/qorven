// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package trends

import (
	"math"
	"time"
)

// signals.go — Engagement signal extraction and source quality scoring.
// Rewritten from last30days signals.py (221 lines).

// SourceQualityWeights maps each source to a base quality score.
// Grounding (web search) is 1.0 baseline; social platforms discounted for noise.
var SourceQualityWeights = map[string]float64{
	"polymarket":   0.95, // real money = highest signal
	"hackernews":   0.85,
	"github":       0.85,
	"youtube":      0.80,
	"reddit":       0.70,
	"bluesky":      0.65,
	"x":            0.65,
	"instagram":    0.55,
	"tiktok":       0.55,
	"threads":      0.50,
	"pinterest":    0.45,
	"xiaohongshu":  0.60,
	"perplexity":   0.90,
	"truthsocial":  0.30,
}

// AnnotateItem computes signal fields on a SourceItem.
func AnnotateItem(item *SourceItem, topic string) {
	// Local relevance
	rel := tokenOverlapRelevance(topic, item.Title+" "+item.Body)
	item.LocalRelevance = &rel

	// Freshness (0-100)
	fresh := computeFreshness(item.PublishedAt)
	item.Freshness = &fresh

	// Engagement score
	eng := computeEngagementScore(item.Engagement)
	item.EngagementScore = &eng

	// Source quality
	sq := sourceQuality(item.Source)
	item.SourceQuality = &sq

	// Combined local rank score
	localRank := rel*0.3 + eng*0.35 + float64(fresh)*0.01*0.2 + sq*0.15
	item.LocalRankScore = &localRank
}

// AnnotateStream annotates all items in a retrieval bundle.
func AnnotateStream(bundle *RetrievalBundle, topic string) {
	for source := range bundle.ItemsBySource {
		for i := range bundle.ItemsBySource[source] {
			AnnotateItem(&bundle.ItemsBySource[source][i], topic)
		}
	}
}

func computeEngagementScore(engagement map[string]float64) float64 {
	if len(engagement) == 0 { return 0 }

	// Platform-specific weighting
	weighted := 0.0
	weights := map[string]float64{
		"upvotes": 1.0, "points": 1.0, "likes": 1.0, "stars": 1.5,
		"comments": 2.5, "replies": 2.5, "num_comments": 2.5,
		"retweets": 2.0, "reposts": 2.0, "shares": 2.0, "forks": 2.0,
		"views": 0.01, "volume": 0.001, // Polymarket volume in dollars
		"bookmarks": 0.5, "awards": 3.0,
	}

	for key, val := range engagement {
		w := weights[key]
		if w == 0 { w = 1.0 }
		weighted += val * w
	}

	if weighted <= 0 { return 0 }
	score := math.Log(weighted) / math.Log(100000)
	if score > 1 { score = 1 }
	return score
}

func computeFreshness(t *time.Time) int {
	if t == nil { return 0 }
	hours := time.Since(*t).Hours()
	switch {
	case hours < 6:   return 100
	case hours < 24:  return 90
	case hours < 72:  return 80
	case hours < 168: return 60 // 7 days
	case hours < 336: return 40 // 14 days
	case hours < 720: return 25 // 30 days
	default:          return 10
	}
}

func sourceQuality(source string) float64 {
	if q, ok := SourceQualityWeights[source]; ok { return q }
	return 0.50
}
