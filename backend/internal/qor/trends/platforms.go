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
	"strings"
	"time"
)

// platforms.go — Instagram, TikTok, Threads, Perplexity, Pinterest, XiaoHongShu readers.

// ── Instagram (via ScrapeCreators API or public embed) ──

type InstagramSource struct {
	client *HTTPClient
	apiKey string // ScrapeCreators API key
}

func NewInstagramSource(apiKey string) *InstagramSource {
	return &InstagramSource{client: NewHTTPClient(15 * time.Second), apiKey: apiKey}
}

func (s *InstagramSource) Name() string { return "instagram" }

func (s *InstagramSource) Search(ctx context.Context, topic string, depth string) ([]SourceItem, error) {
	if s.apiKey == "" { return nil, fmt.Errorf("instagram: API key required (ScrapeCreators)") }
	limit := 10
	if depth == "deep" { limit = 25 }

	u := fmt.Sprintf("https://api.scrapecreators.com/v2/instagram/search?q=%s&limit=%d",
		url.QueryEscape(extractCoreSubject(topic)), limit)
	data, err := s.client.Get(ctx, u, map[string]string{"x-api-key": s.apiKey})
	if err != nil { return nil, err }

	var resp struct {
		Posts []struct {
			ID        string `json:"id"`
			Caption   string `json:"caption"`
			Likes     int    `json:"likes"`
			Comments  int    `json:"comments"`
			Views     int    `json:"views"`
			Username  string `json:"username"`
			Timestamp string `json:"timestamp"`
			URL       string `json:"url"`
		} `json:"posts"`
	}
	if err := json.Unmarshal(data, &resp); err != nil { return nil, err }

	var items []SourceItem
	for _, p := range resp.Posts {
		published := parseTime(p.Timestamp)
		eng := map[string]float64{"likes": float64(p.Likes), "comments": float64(p.Comments), "views": float64(p.Views)}
		total := float64(p.Likes) + float64(p.Comments)*3 + float64(p.Views)*0.01
		engScore := logNorm(total)

		items = append(items, SourceItem{
			ItemID: "ig_" + p.ID, Source: "instagram",
			Title: truncateStr(p.Caption, 120), Body: p.Caption,
			URL: p.URL, Author: "@" + p.Username, PublishedAt: published,
			DateConfidence: "high", Engagement: eng, EngagementScore: &engScore,
			RelevanceHint: tokenOverlapRelevance(topic, p.Caption),
		})
	}
	sort.Slice(items, func(i, j int) bool { return *items[i].EngagementScore > *items[j].EngagementScore })
	return items, nil
}

// ── TikTok (via ScrapeCreators API) ──

type TikTokSource struct {
	client *HTTPClient
	apiKey string
}

func NewTikTokSource(apiKey string) *TikTokSource {
	return &TikTokSource{client: NewHTTPClient(15 * time.Second), apiKey: apiKey}
}

func (s *TikTokSource) Name() string { return "tiktok" }

func (s *TikTokSource) Search(ctx context.Context, topic string, depth string) ([]SourceItem, error) {
	if s.apiKey == "" { return nil, fmt.Errorf("tiktok: API key required (ScrapeCreators)") }
	limit := 10
	if depth == "deep" { limit = 25 }

	u := fmt.Sprintf("https://api.scrapecreators.com/v2/tiktok/search?q=%s&limit=%d",
		url.QueryEscape(extractCoreSubject(topic)), limit)
	data, err := s.client.Get(ctx, u, map[string]string{"x-api-key": s.apiKey})
	if err != nil { return nil, err }

	var resp struct {
		Videos []struct {
			ID       string `json:"id"`
			Desc     string `json:"desc"`
			Likes    int    `json:"likes"`
			Comments int    `json:"comments"`
			Shares   int    `json:"shares"`
			Views    int    `json:"views"`
			Author   string `json:"author"`
			Created  string `json:"createTime"`
			URL      string `json:"url"`
		} `json:"videos"`
	}
	if err := json.Unmarshal(data, &resp); err != nil { return nil, err }

	var items []SourceItem
	for _, v := range resp.Videos {
		published := parseTime(v.Created)
		eng := map[string]float64{"likes": float64(v.Likes), "comments": float64(v.Comments), "shares": float64(v.Shares), "views": float64(v.Views)}
		total := float64(v.Likes) + float64(v.Comments)*3 + float64(v.Shares)*2 + float64(v.Views)*0.001
		engScore := logNorm(total)

		items = append(items, SourceItem{
			ItemID: "tt_" + v.ID, Source: "tiktok",
			Title: truncateStr(v.Desc, 120), Body: v.Desc,
			URL: v.URL, Author: "@" + v.Author, PublishedAt: published,
			DateConfidence: "high", Engagement: eng, EngagementScore: &engScore,
			RelevanceHint: tokenOverlapRelevance(topic, v.Desc),
		})
	}
	sort.Slice(items, func(i, j int) bool { return *items[i].EngagementScore > *items[j].EngagementScore })
	return items, nil
}

// ── Threads (Meta Threads API) ──

type ThreadsSource struct {
	client *HTTPClient
	apiKey string
}

func NewThreadsSource(apiKey string) *ThreadsSource {
	return &ThreadsSource{client: NewHTTPClient(15 * time.Second), apiKey: apiKey}
}

func (s *ThreadsSource) Name() string { return "threads" }

func (s *ThreadsSource) Search(ctx context.Context, topic string, depth string) ([]SourceItem, error) {
	if s.apiKey == "" { return nil, fmt.Errorf("threads: API key required (ScrapeCreators)") }
	limit := 10
	if depth == "deep" { limit = 20 }

	u := fmt.Sprintf("https://api.scrapecreators.com/v2/threads/search?q=%s&limit=%d",
		url.QueryEscape(extractCoreSubject(topic)), limit)
	data, err := s.client.Get(ctx, u, map[string]string{"x-api-key": s.apiKey})
	if err != nil { return nil, err }

	var resp struct {
		Posts []struct {
			ID       string `json:"id"`
			Text     string `json:"text"`
			Likes    int    `json:"likes"`
			Replies  int    `json:"replies"`
			Reposts  int    `json:"reposts"`
			Username string `json:"username"`
			Created  string `json:"timestamp"`
		} `json:"posts"`
	}
	if err := json.Unmarshal(data, &resp); err != nil { return nil, err }

	var items []SourceItem
	for _, p := range resp.Posts {
		published := parseTime(p.Created)
		eng := map[string]float64{"likes": float64(p.Likes), "replies": float64(p.Replies), "reposts": float64(p.Reposts)}
		total := float64(p.Likes) + float64(p.Replies)*3 + float64(p.Reposts)*2
		engScore := logNorm(total)

		items = append(items, SourceItem{
			ItemID: "th_" + p.ID, Source: "threads",
			Title: truncateStr(p.Text, 120), Body: p.Text,
			URL: fmt.Sprintf("https://threads.net/@%s/post/%s", p.Username, p.ID),
			Author: "@" + p.Username, PublishedAt: published,
			DateConfidence: "high", Engagement: eng, EngagementScore: &engScore,
			RelevanceHint: tokenOverlapRelevance(topic, p.Text),
		})
	}
	return items, nil
}

// ── Perplexity (Sonar Pro via OpenRouter) ──

type PerplexitySource struct {
	client    *HTTPClient
	apiKey    string // OpenRouter API key
}

func NewPerplexitySource(apiKey string) *PerplexitySource {
	return &PerplexitySource{client: NewHTTPClient(30 * time.Second), apiKey: apiKey}
}

func (s *PerplexitySource) Name() string { return "perplexity" }

func (s *PerplexitySource) Search(ctx context.Context, topic string, depth string) ([]SourceItem, error) {
	if s.apiKey == "" { return nil, fmt.Errorf("perplexity: OpenRouter API key required") }

	body := fmt.Sprintf(`{"model":"perplexity/sonar-pro","messages":[{"role":"user","content":"Search the web for recent information about: %s. Return factual findings with sources."}],"max_tokens":2000}`, topic)
	data, err := s.client.Do(ctx, "POST", "https://openrouter.ai/api/v1/chat/completions",
		map[string]string{"Authorization": "Bearer " + s.apiKey, "Content-Type": "application/json"},
		strings.NewReader(body))
	if err != nil { return nil, err }

	var resp struct {
		Choices []struct{ Message struct{ Content string } }
	}
	if err := json.Unmarshal(data, &resp); err != nil { return nil, err }
	if len(resp.Choices) == 0 { return nil, fmt.Errorf("no response") }

	content := resp.Choices[0].Message.Content
	now := time.Now()
	engScore := 0.9 // Perplexity is high-quality grounded search

	return []SourceItem{{
		ItemID: "pplx_" + fmt.Sprintf("%d", now.UnixMilli()),
		Source: "perplexity", Title: "Perplexity: " + truncateStr(topic, 80),
		Body: content, URL: "https://perplexity.ai",
		PublishedAt: &now, DateConfidence: "high",
		EngagementScore: &engScore, RelevanceHint: 0.95,
	}}, nil
}

// ── Pinterest ──

type PinterestSource struct {
	client *HTTPClient
	apiKey string
}

func NewPinterestSource(apiKey string) *PinterestSource {
	return &PinterestSource{client: NewHTTPClient(15 * time.Second), apiKey: apiKey}
}

func (s *PinterestSource) Name() string { return "pinterest" }

func (s *PinterestSource) Search(ctx context.Context, topic string, depth string) ([]SourceItem, error) {
	if s.apiKey == "" { return nil, fmt.Errorf("pinterest: API key required (ScrapeCreators)") }
	limit := 10
	if depth == "deep" { limit = 20 }

	u := fmt.Sprintf("https://api.scrapecreators.com/v2/pinterest/search?q=%s&limit=%d",
		url.QueryEscape(extractCoreSubject(topic)), limit)
	data, err := s.client.Get(ctx, u, map[string]string{"x-api-key": s.apiKey})
	if err != nil { return nil, err }

	var resp struct {
		Pins []struct {
			ID       string `json:"id"`
			Title    string `json:"title"`
			Desc     string `json:"description"`
			Saves    int    `json:"saves"`
			Comments int    `json:"comments"`
			URL      string `json:"url"`
			Creator  string `json:"creator"`
		} `json:"pins"`
	}
	if err := json.Unmarshal(data, &resp); err != nil { return nil, err }

	var items []SourceItem
	for _, p := range resp.Pins {
		eng := map[string]float64{"saves": float64(p.Saves), "comments": float64(p.Comments)}
		total := float64(p.Saves) + float64(p.Comments)*3
		engScore := logNorm(total)

		items = append(items, SourceItem{
			ItemID: "pin_" + p.ID, Source: "pinterest",
			Title: p.Title, Body: p.Desc, URL: p.URL, Author: p.Creator,
			Engagement: eng, EngagementScore: &engScore,
			RelevanceHint: tokenOverlapRelevance(topic, p.Title+" "+p.Desc),
		})
	}
	return items, nil
}

// ── XiaoHongShu / RedNote ──

type XiaoHongShuSource struct {
	client *HTTPClient
	apiKey string
}

func NewXiaoHongShuSource(apiKey string) *XiaoHongShuSource {
	return &XiaoHongShuSource{client: NewHTTPClient(15 * time.Second), apiKey: apiKey}
}

func (s *XiaoHongShuSource) Name() string { return "xiaohongshu" }

func (s *XiaoHongShuSource) Search(ctx context.Context, topic string, depth string) ([]SourceItem, error) {
	if s.apiKey == "" { return nil, fmt.Errorf("xiaohongshu: API key required") }
	limit := 10
	if depth == "deep" { limit = 20 }

	u := fmt.Sprintf("https://api.scrapecreators.com/v2/xiaohongshu/search?q=%s&limit=%d",
		url.QueryEscape(extractCoreSubject(topic)), limit)
	data, err := s.client.Get(ctx, u, map[string]string{"x-api-key": s.apiKey})
	if err != nil { return nil, err }

	var resp struct {
		Notes []struct {
			ID       string `json:"id"`
			Title    string `json:"title"`
			Content  string `json:"content"`
			Likes    int    `json:"likes"`
			Comments int    `json:"comments"`
			Collects int    `json:"collects"`
			Author   string `json:"author"`
			URL      string `json:"url"`
			Created  string `json:"timestamp"`
		} `json:"notes"`
	}
	if err := json.Unmarshal(data, &resp); err != nil { return nil, err }

	var items []SourceItem
	for _, n := range resp.Notes {
		published := parseTime(n.Created)
		eng := map[string]float64{"likes": float64(n.Likes), "comments": float64(n.Comments), "collects": float64(n.Collects)}
		total := float64(n.Likes) + float64(n.Comments)*3 + float64(n.Collects)*2
		engScore := logNorm(total)

		items = append(items, SourceItem{
			ItemID: "xhs_" + n.ID, Source: "xiaohongshu",
			Title: n.Title, Body: n.Content, URL: n.URL, Author: n.Author,
			PublishedAt: published, DateConfidence: "high",
			Engagement: eng, EngagementScore: &engScore,
			RelevanceHint: tokenOverlapRelevance(topic, n.Title+" "+n.Content),
		})
	}
	return items, nil
}

func logNorm(raw float64) float64 {
	if raw <= 0 { return 0 }
	score := math.Log(raw) / math.Log(100000)
	if score > 1 { score = 1 }
	return score
}
