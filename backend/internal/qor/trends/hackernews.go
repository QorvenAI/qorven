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

type HackerNewsSource struct {
	client *HTTPClient
}

func NewHackerNewsSource() *HackerNewsSource {
	return &HackerNewsSource{client: NewHTTPClient(15 * time.Second)}
}

func (h *HackerNewsSource) Name() string { return "hackernews" }

func (h *HackerNewsSource) Search(ctx context.Context, topic string, depth string) ([]SourceItem, error) {
	hitsPerPage := 20
	if depth == "deep" { hitsPerPage = 40 }
	after := time.Now().Add(-30 * 24 * time.Hour).Unix()

	u := fmt.Sprintf("https://hn.algolia.com/api/v1/search?query=%s&tags=story&numericFilters=created_at_i>%d&hitsPerPage=%d",
		url.QueryEscape(extractCoreSubject(topic)), after, hitsPerPage)

	data, err := h.client.Get(ctx, u, nil)
	if err != nil { return nil, fmt.Errorf("hn: %w", err) }

	var resp struct {
		Hits []hnHit `json:"hits"`
	}
	if err := json.Unmarshal(data, &resp); err != nil { return nil, err }

	var items []SourceItem
	for _, hit := range resp.Hits {
		created, _ := time.Parse(time.RFC3339, hit.CreatedAt)
		body := hit.StoryText
		if body == "" && hit.URL != "" { body = hit.URL }

		eng := map[string]float64{
			"points":   float64(hit.Points),
			"comments": float64(hit.NumComments),
		}
		total := float64(hit.Points) + float64(hit.NumComments)*3
		engScore := 0.0
		if total > 0 { engScore = math.Log(total) / math.Log(100000) }
		if engScore > 1 { engScore = 1 }

		items = append(items, SourceItem{
			ItemID:          "hn_" + hit.ObjectID,
			Source:          "hackernews",
			Title:           hit.Title,
			Body:            truncateStr(body, 2000),
			URL:             "https://news.ycombinator.com/item?id=" + hit.ObjectID,
			Author:          hit.Author,
			PublishedAt:     &created,
			DateConfidence:  "high",
			Engagement:      eng,
			EngagementScore: &engScore,
			RelevanceHint:   tokenOverlapRelevance(topic, hit.Title+" "+body),
			Metadata:        map[string]any{"source_url": hit.URL},
		})
	}

	// Enrich top 5 with comments
	for i := range items {
		if i >= 5 { break }
		h.enrichComments(ctx, &items[i])
	}

	sort.Slice(items, func(i, j int) bool { return *items[i].EngagementScore > *items[j].EngagementScore })
	return items, nil
}

func (h *HackerNewsSource) enrichComments(ctx context.Context, item *SourceItem) {
	id := strings.TrimPrefix(item.ItemID, "hn_")
	u := fmt.Sprintf("https://hn.algolia.com/api/v1/items/%s", id)
	data, err := h.client.Get(ctx, u, nil)
	if err != nil { return }

	var resp struct {
		Children []struct {
			Text   string `json:"text"`
			Author string `json:"author"`
			Points int    `json:"points"`
		} `json:"children"`
	}
	if err := json.Unmarshal(data, &resp); err != nil { return }

	var comments []string
	for i, c := range resp.Children {
		if i >= 5 || c.Text == "" { continue }
		clean := stripHTMLTags(c.Text)
		comments = append(comments, fmt.Sprintf("[%s] %s", c.Author, truncateStr(clean, 300)))
	}
	if len(comments) > 0 {
		item.Body += "\n\n--- Top Comments ---\n" + strings.Join(comments, "\n\n")
	}
}

func stripHTMLTags(s string) string {
	var b strings.Builder
	inTag := false
	for _, r := range s {
		if r == '<' { inTag = true } else if r == '>' { inTag = false } else if !inTag { b.WriteRune(r) }
	}
	return strings.TrimSpace(b.String())
}

type hnHit struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Author      string `json:"author"`
	Points      int    `json:"points"`
	NumComments int    `json:"num_comments"`
	ObjectID    string `json:"objectID"`
	CreatedAt   string `json:"created_at"`
	StoryText   string `json:"story_text"`
}
