// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
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

// bluesky.go — Bluesky AT Protocol search.

type BlueskySource struct {
	client *HTTPClient
}

func NewBlueskySource() *BlueskySource {
	return &BlueskySource{client: NewHTTPClient(15 * time.Second)}
}

func (b *BlueskySource) Name() string   { return "bluesky" }
func (b *BlueskySource) Available() bool { return true } // public API

func (b *BlueskySource) Search(ctx context.Context, topic string, depth string) ([]SourceItem, error) {
	limit := 25
	if depth == "deep" { limit = 50 }

	u := fmt.Sprintf("https://public.api.bsky.app/xrpc/app.bsky.feed.searchPosts?q=%s&limit=%d&sort=top",
		url.QueryEscape(extractCoreSubject(topic)), limit)

	data, err := b.client.Get(ctx, u, nil)
	if err != nil { return nil, fmt.Errorf("bluesky: %w", err) }

	var resp struct {
		Posts []struct {
			URI    string `json:"uri"`
			CID    string `json:"cid"`
			Author struct {
				Handle string `json:"handle"`
				Name   string `json:"displayName"`
			} `json:"author"`
			Record struct {
				Text      string `json:"text"`
				CreatedAt string `json:"createdAt"`
			} `json:"record"`
			LikeCount    int `json:"likeCount"`
			RepostCount  int `json:"repostCount"`
			ReplyCount   int `json:"replyCount"`
			QuoteCount   int `json:"quoteCount"`
		} `json:"posts"`
	}
	if err := json.Unmarshal(data, &resp); err != nil { return nil, err }

	var items []SourceItem
	for _, p := range resp.Posts {
		published := parseTime(p.Record.CreatedAt)
		engagement := map[string]float64{
			"likes":   float64(p.LikeCount),
			"reposts": float64(p.RepostCount),
			"replies": float64(p.ReplyCount),
		}
		totalEng := float64(p.LikeCount + p.RepostCount*2 + p.ReplyCount*3)
		engScore := 0.0
		if totalEng > 0 { engScore = math.Log(totalEng) / math.Log(100000) }
		if engScore > 1 { engScore = 1 }

		// Extract post ID from URI: at://did:plc:xxx/app.bsky.feed.post/yyy
		postURL := fmt.Sprintf("https://bsky.app/profile/%s/post/%s", p.Author.Handle, extractPostID(p.URI))

		items = append(items, SourceItem{
			ItemID:          "bsky_" + p.CID[:12],
			Source:          "bluesky",
			Title:           truncateStr(p.Record.Text, 120),
			Body:            p.Record.Text,
			URL:             postURL,
			Author:          "@" + p.Author.Handle,
			PublishedAt:     published,
			DateConfidence:  "high",
			Engagement:      engagement,
			EngagementScore: &engScore,
			RelevanceHint:   tokenOverlapRelevance(topic, p.Record.Text),
		})
	}

	sort.Slice(items, func(i, j int) bool { return *items[i].EngagementScore > *items[j].EngagementScore })
	return items, nil
}

func extractPostID(uri string) string {
	// at://did:plc:xxx/app.bsky.feed.post/yyy → yyy
	parts := splitLast(uri, "/")
	return parts
}

func splitLast(s, sep string) string {
	i := len(s) - 1
	for i >= 0 && string(s[i]) != sep { i-- }
	if i < 0 { return s }
	return s[i+1:]
}
