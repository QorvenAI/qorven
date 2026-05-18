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
	"sort"
	
	"time"
)

// twitter.go — Twitter/X search via API v2 and cookie-based auth.

type TwitterSource struct {
	client    *HTTPClient
	bearerToken string // API v2 bearer token
	authToken   string // cookie auth_token (for GraphQL)
	ct0         string // cookie ct0 (for GraphQL)
}

func NewTwitterSource(bearerToken, authToken, ct0 string) *TwitterSource {
	return &TwitterSource{
		client: NewHTTPClient(15 * time.Second),
		bearerToken: bearerToken, authToken: authToken, ct0: ct0,
	}
}

func (t *TwitterSource) Name() string { return "twitter" }

func (t *TwitterSource) Available() bool {
	return t.bearerToken != "" || (t.authToken != "" && t.ct0 != "")
}

// Search queries Twitter for recent tweets matching the topic.
func (t *TwitterSource) Search(ctx context.Context, topic string, depth string) ([]SourceItem, error) {
	if t.bearerToken != "" {
		return t.searchV2(ctx, topic, depth)
	}
	return nil, fmt.Errorf("twitter: no credentials configured")
}

func (t *TwitterSource) searchV2(ctx context.Context, topic string, depth string) ([]SourceItem, error) {
	core := extractCoreSubject(topic)
	maxResults := 20
	if depth == "deep" { maxResults = 50 }

	params := url.Values{
		"query":        {core + " -is:retweet lang:en"},
		"max_results":  {fmt.Sprintf("%d", maxResults)},
		"sort_order":   {"relevancy"},
		"tweet.fields": {"created_at,public_metrics,author_id,conversation_id"},
		"user.fields":  {"username,name,public_metrics"},
		"expansions":   {"author_id"},
	}

	u := "https://api.twitter.com/2/tweets/search/recent?" + params.Encode()
	headers := map[string]string{"Authorization": "Bearer " + t.bearerToken}

	data, err := t.client.Get(ctx, u, headers)
	if err != nil { return nil, fmt.Errorf("twitter v2: %w", err) }

	var resp struct {
		Data []struct {
			ID        string `json:"id"`
			Text      string `json:"text"`
			CreatedAt string `json:"created_at"`
			AuthorID  string `json:"author_id"`
			Metrics   struct {
				Retweets  int `json:"retweet_count"`
				Replies   int `json:"reply_count"`
				Likes     int `json:"like_count"`
				Quotes    int `json:"quote_count"`
				Bookmarks int `json:"bookmark_count"`
				Views     int `json:"impression_count"`
			} `json:"public_metrics"`
		} `json:"data"`
		Includes struct {
			Users []struct {
				ID       string `json:"id"`
				Username string `json:"username"`
				Name     string `json:"name"`
				Metrics  struct {
					Followers int `json:"followers_count"`
				} `json:"public_metrics"`
			} `json:"users"`
		} `json:"includes"`
	}
	if err := json.Unmarshal(data, &resp); err != nil { return nil, err }

	// Build user lookup
	users := map[string]string{}
	for _, u := range resp.Includes.Users { users[u.ID] = u.Username }

	var items []SourceItem
	for _, tw := range resp.Data {
		published := parseTime(tw.CreatedAt)
		username := users[tw.AuthorID]
		engagement := map[string]float64{
			"likes":     float64(tw.Metrics.Likes),
			"retweets":  float64(tw.Metrics.Retweets),
			"replies":   float64(tw.Metrics.Replies),
			"quotes":    float64(tw.Metrics.Quotes),
			"bookmarks": float64(tw.Metrics.Bookmarks),
			"views":     float64(tw.Metrics.Views),
		}

		totalEng := float64(tw.Metrics.Likes + tw.Metrics.Retweets*2 + tw.Metrics.Replies*3)
		engScore := normalizeEng(totalEng)

		items = append(items, SourceItem{
			ItemID:          "tw_" + tw.ID,
			Source:          "twitter",
			Title:           truncateStr(tw.Text, 120),
			Body:            tw.Text,
			URL:             fmt.Sprintf("https://x.com/%s/status/%s", username, tw.ID),
			Author:          "@" + username,
			PublishedAt:     published,
			DateConfidence:  "high",
			Engagement:      engagement,
			EngagementScore: &engScore,
			RelevanceHint:   tokenOverlapRelevance(topic, tw.Text),
		})
	}

	sort.Slice(items, func(i, j int) bool {
		return *items[i].EngagementScore > *items[j].EngagementScore
	})
	return items, nil
}

func truncateStr(s string, n int) string {
	if len(s) <= n { return s }
	return s[:n] + "..."
}

func normalizeEng(raw float64) float64 {
	if raw <= 0 { return 0 }
	
	score := math.Log(raw) / math.Log(100000)
	if score > 1 { score = 1 }
	return score
}
