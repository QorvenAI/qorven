// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// SocialMonitorTool monitors social platforms for mentions, trends, and content.
type SocialMonitorTool struct{}

func NewSocialMonitorTool() *SocialMonitorTool { return &SocialMonitorTool{} }

func (t *SocialMonitorTool) Name() string { return "social_monitor" }
func (t *SocialMonitorTool) Description() string {
	return "Monitor social platforms (GitHub, Reddit, HackerNews, Twitter) for mentions, trends, or specific topics. Use for brand monitoring, competitor analysis, or trend research."
}
func (t *SocialMonitorTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"platform": map[string]any{"type": "string", "enum": []string{"github", "reddit", "hackernews", "twitter"}, "description": "Platform to monitor"},
			"query":    map[string]any{"type": "string", "description": "Search query or topic"},
			"limit":    map[string]any{"type": "integer", "description": "Max results (default 10)"},
		},
		"required": []string{"platform", "query"},
	}
}

func (t *SocialMonitorTool) Execute(ctx context.Context, args map[string]any) *Result {
	platform, _ := args["platform"].(string)
	query, _ := args["query"].(string)
	limit := 10
	if l, ok := args["limit"].(float64); ok && l > 0 {
		limit = int(l)
	}

	switch platform {
	case "github":
		return t.searchGitHub(ctx, query, limit)
	case "reddit":
		return t.searchReddit(ctx, query, limit)
	case "hackernews":
		return t.searchHackerNews(ctx, query, limit)
	default:
		return ErrorResult("platform not supported: " + platform)
	}
}

func (t *SocialMonitorTool) searchGitHub(ctx context.Context, query string, limit int) *Result {
	url := fmt.Sprintf("https://api.github.com/search/repositories?q=%s&sort=stars&per_page=%d",
		strings.ReplaceAll(query, " ", "+"), limit)
	body, err := fetchJSON(ctx, url)
	if err != nil {
		return ErrorResult("github search: " + err.Error())
	}
	var result struct {
		Items []struct {
			FullName    string `json:"full_name"`
			Description string `json:"description"`
			Stars       int    `json:"stargazers_count"`
			URL         string `json:"html_url"`
			Language    string `json:"language"`
			UpdatedAt   string `json:"updated_at"`
		} `json:"items"`
	}
	json.Unmarshal(body, &result)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("GitHub: %d results for '%s'\n\n", len(result.Items), query))
	for i, r := range result.Items {
		desc := r.Description
		if len(desc) > 100 {
			desc = desc[:100] + "..."
		}
		sb.WriteString(fmt.Sprintf("%d. **%s** ⭐%d [%s]\n   %s\n   %s\n\n", i+1, r.FullName, r.Stars, r.Language, desc, r.URL))
	}
	return TextResult(sb.String())
}

func (t *SocialMonitorTool) searchReddit(ctx context.Context, query string, limit int) *Result {
	url := fmt.Sprintf("https://www.reddit.com/search.json?q=%s&sort=relevance&limit=%d",
		strings.ReplaceAll(query, " ", "+"), limit)
	body, err := fetchJSON(ctx, url)
	if err != nil {
		return ErrorResult("reddit search: " + err.Error())
	}
	var result struct {
		Data struct {
			Children []struct {
				Data struct {
					Title     string  `json:"title"`
					Subreddit string  `json:"subreddit"`
					Score     int     `json:"score"`
					URL       string  `json:"url"`
					NumComments int   `json:"num_comments"`
					Created   float64 `json:"created_utc"`
				} `json:"data"`
			} `json:"children"`
		} `json:"data"`
	}
	json.Unmarshal(body, &result)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Reddit: %d results for '%s'\n\n", len(result.Data.Children), query))
	for i, c := range result.Data.Children {
		r := c.Data
		sb.WriteString(fmt.Sprintf("%d. r/%s — %s (⬆%d, 💬%d)\n   %s\n\n", i+1, r.Subreddit, r.Title, r.Score, r.NumComments, r.URL))
	}
	return TextResult(sb.String())
}

func (t *SocialMonitorTool) searchHackerNews(ctx context.Context, query string, limit int) *Result {
	url := fmt.Sprintf("https://hn.algolia.com/api/v1/search?query=%s&hitsPerPage=%d",
		strings.ReplaceAll(query, " ", "+"), limit)
	body, err := fetchJSON(ctx, url)
	if err != nil {
		return ErrorResult("hackernews search: " + err.Error())
	}
	var result struct {
		Hits []struct {
			Title    string `json:"title"`
			URL      string `json:"url"`
			Points   int    `json:"points"`
			Comments int    `json:"num_comments"`
			Author   string `json:"author"`
		} `json:"hits"`
	}
	json.Unmarshal(body, &result)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("HackerNews: %d results for '%s'\n\n", len(result.Hits), query))
	for i, h := range result.Hits {
		sb.WriteString(fmt.Sprintf("%d. %s (⬆%d, 💬%d by %s)\n   %s\n\n", i+1, h.Title, h.Points, h.Comments, h.Author, h.URL))
	}
	return TextResult(sb.String())
}

func fetchJSON(ctx context.Context, url string) ([]byte, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	req.Header.Set("User-Agent", "Qorven/1.0")
	req.Header.Set("Accept", "application/json")
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var buf [256 * 1024]byte // 512KB max
	n, _ := resp.Body.Read(buf[:])
	return buf[:n], nil
}
