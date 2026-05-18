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
	"strings"
	"time"
)

type RedditSource struct {
	client *HTTPClient
}

func NewRedditSource() *RedditSource {
	return &RedditSource{client: NewHTTPClient(15 * time.Second)}
}

func (r *RedditSource) Name() string { return "reddit" }

func (r *RedditSource) Search(ctx context.Context, topic string, depth string) ([]SourceItem, error) {
	queries := expandQueries(topic)
	limit := 25
	if depth == "deep" { limit = 50 }

	var allItems []SourceItem
	seen := map[string]bool{}

	for _, q := range queries {
		items, err := r.searchQuery(ctx, q, limit)
		if err != nil { continue }
		for _, it := range items {
			if !seen[it.ItemID] {
				seen[it.ItemID] = true
				it.RelevanceHint = tokenOverlapRelevance(topic, it.Title+" "+it.Body)
				allItems = append(allItems, it)
			}
		}
	}

	sort.Slice(allItems, func(i, j int) bool {
		return *allItems[i].EngagementScore > *allItems[j].EngagementScore
	})
	cap := 15
	if depth == "deep" { cap = 30 }
	if len(allItems) > cap { allItems = allItems[:cap] }

	// Enrich top results with comments
	enrichLimit := 5
	if depth == "deep" { enrichLimit = 10 }
	for i := range allItems {
		if i >= enrichLimit { break }
		r.enrichWithComments(ctx, &allItems[i])
	}

	return allItems, nil
}

func (r *RedditSource) searchQuery(ctx context.Context, query string, limit int) ([]SourceItem, error) {
	u := fmt.Sprintf("https://www.reddit.com/search.json?q=%s&sort=relevance&t=month&limit=%d",
		url.QueryEscape(query), limit)
	data, err := r.client.Get(ctx, u, map[string]string{"Accept": "application/json"})
	if err != nil { return nil, err }

	var resp struct {
		Data struct {
			Children []struct {
				Data redditPost `json:"data"`
			} `json:"children"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &resp); err != nil { return nil, err }

	var items []SourceItem
	for _, c := range resp.Data.Children {
		p := c.Data
		created := time.Unix(int64(p.CreatedUTC), 0)
		body := p.Selftext
		if body == "" { body = p.URL }

		eng := map[string]float64{
			"upvotes":  float64(p.Ups),
			"comments": float64(p.NumComments),
			"ratio":    p.UpvoteRatio,
			"awards":   float64(p.TotalAwards),
		}
		total := float64(p.Ups) + float64(p.NumComments)*3
		engScore := 0.0
		if total > 0 { engScore = math.Log(total) / math.Log(100000) }
		if engScore > 1 { engScore = 1 }

		items = append(items, SourceItem{
			ItemID:          "rd_" + p.ID,
			Source:          "reddit",
			Title:           p.Title,
			Body:            truncateStr(body, 2000),
			URL:             "https://reddit.com" + p.Permalink,
			Author:          p.Author,
			Container:       "r/" + p.Subreddit,
			PublishedAt:     &created,
			DateConfidence:  "high",
			Engagement:      eng,
			EngagementScore: &engScore,
		})
	}
	return items, nil
}

func (r *RedditSource) enrichWithComments(ctx context.Context, item *SourceItem) {
	u := item.URL + ".json?limit=10&sort=top"
	data, err := r.client.Get(ctx, u, map[string]string{"Accept": "application/json"})
	if err != nil { return }

	var listings []struct {
		Data struct {
			Children []struct {
				Data struct {
					Body  string  `json:"body"`
					Score int     `json:"score"`
					Author string `json:"author"`
				} `json:"data"`
			} `json:"children"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &listings); err != nil || len(listings) < 2 { return }

	var comments []string
	for i, c := range listings[1].Data.Children {
		if i >= 10 || c.Data.Body == "" { continue }
		comments = append(comments, fmt.Sprintf("[%d pts, u/%s] %s",
			c.Data.Score, c.Data.Author, truncateStr(c.Data.Body, 300)))
	}
	if len(comments) > 0 {
		item.Body += "\n\n--- Top Comments ---\n" + strings.Join(comments, "\n\n")
	}
}

type redditPost struct {
	ID          string  `json:"id"`
	Title       string  `json:"title"`
	Selftext    string  `json:"selftext"`
	Subreddit   string  `json:"subreddit"`
	Author      string  `json:"author"`
	Ups         int     `json:"ups"`
	NumComments int     `json:"num_comments"`
	Permalink   string  `json:"permalink"`
	URL         string  `json:"url"`
	CreatedUTC  float64 `json:"created_utc"`
	UpvoteRatio float64 `json:"upvote_ratio"`
	TotalAwards int     `json:"total_awards_received"`
}
