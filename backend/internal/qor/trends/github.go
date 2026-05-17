// Copyright 2026 Tekky AI Academy LLP. All rights reserved.
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

type GitHubSource struct {
	client *HTTPClient
	token  string
}

func NewGitHubSource(token string) *GitHubSource {
	return &GitHubSource{client: NewHTTPClient(15 * time.Second), token: token}
}

func (g *GitHubSource) Name() string { return "github" }

func (g *GitHubSource) Search(ctx context.Context, topic string, depth string) ([]SourceItem, error) {
	queries := expandQueries(topic)
	perPage := 10
	if depth == "deep" { perPage = 25 }

	var allItems []SourceItem
	seen := map[string]bool{}

	// Search repos
	for _, q := range queries[:min(len(queries), 3)] {
		items, err := g.searchRepos(ctx, q, perPage)
		if err != nil { continue }
		for _, it := range items {
			if !seen[it.ItemID] { seen[it.ItemID] = true; allItems = append(allItems, it) }
		}
	}

	// Search discussions/issues for the core topic
	core := extractCoreSubject(topic)
	issues, _ := g.searchIssues(ctx, core, perPage)
	for _, it := range issues {
		if !seen[it.ItemID] { seen[it.ItemID] = true; allItems = append(allItems, it) }
	}

	sort.Slice(allItems, func(i, j int) bool { return *allItems[i].EngagementScore > *allItems[j].EngagementScore })
	cap := 20
	if depth == "deep" { cap = 40 }
	if len(allItems) > cap { allItems = allItems[:cap] }
	return allItems, nil
}

func (g *GitHubSource) headers() map[string]string {
	h := map[string]string{"Accept": "application/vnd.github.v3+json"}
	if g.token != "" { h["Authorization"] = "Bearer " + g.token }
	return h
}

func (g *GitHubSource) searchRepos(ctx context.Context, query string, perPage int) ([]SourceItem, error) {
	after := time.Now().Add(-30 * 24 * time.Hour).Format("2006-01-02")
	u := fmt.Sprintf("https://api.github.com/search/repositories?q=%s+pushed:>%s&sort=stars&order=desc&per_page=%d",
		url.QueryEscape(query), after, perPage)

	data, err := g.client.Get(ctx, u, g.headers())
	if err != nil { return nil, err }

	var resp struct {
		Items []ghRepo `json:"items"`
	}
	if err := json.Unmarshal(data, &resp); err != nil { return nil, err }

	var items []SourceItem
	for _, repo := range resp.Items {
		created, _ := time.Parse(time.RFC3339, repo.CreatedAt)
		pushed, _ := time.Parse(time.RFC3339, repo.PushedAt)

		desc := repo.Description
		if repo.Language != "" { desc += " [" + repo.Language + "]" }
		if len(repo.Topics) > 0 { desc += " #" + strings.Join(repo.Topics, " #") }

		eng := map[string]float64{
			"stars":  float64(repo.Stars),
			"forks":  float64(repo.Forks),
			"issues": float64(repo.Issues),
		}
		total := float64(repo.Stars) + float64(repo.Forks)*2 + float64(repo.Issues)
		engScore := 0.0
		if total > 0 { engScore = math.Log(total) / math.Log(100000) }
		if engScore > 1 { engScore = 1 }

		pub := pushed
		if pub.IsZero() { pub = created }

		items = append(items, SourceItem{
			ItemID:          "gh_" + repo.FullName,
			Source:          "github",
			Title:           repo.FullName,
			Body:            desc,
			URL:             repo.HTMLURL,
			Author:          repo.Owner.Login,
			PublishedAt:     &pub,
			DateConfidence:  "high",
			Engagement:      eng,
			EngagementScore: &engScore,
			RelevanceHint:   tokenOverlapRelevance(repo.FullName+" "+desc, extractCoreSubject("")),
		})
	}
	return items, nil
}

func (g *GitHubSource) searchIssues(ctx context.Context, query string, perPage int) ([]SourceItem, error) {
	u := fmt.Sprintf("https://api.github.com/search/issues?q=%s+type:issue+state:open&sort=reactions&order=desc&per_page=%d",
		url.QueryEscape(query), perPage)

	data, err := g.client.Get(ctx, u, g.headers())
	if err != nil { return nil, err }

	var resp struct {
		Items []struct {
			Title     string `json:"title"`
			Body      string `json:"body"`
			HTMLURL   string `json:"html_url"`
			User      struct{ Login string `json:"login"` } `json:"user"`
			Comments  int    `json:"comments"`
			Reactions struct {
				Total int `json:"total_count"`
				Plus  int `json:"+1"`
			} `json:"reactions"`
			CreatedAt string `json:"created_at"`
			Labels    []struct{ Name string `json:"name"` } `json:"labels"`
		} `json:"items"`
	}
	if err := json.Unmarshal(data, &resp); err != nil { return nil, err }

	var items []SourceItem
	for _, issue := range resp.Items {
		created, _ := time.Parse(time.RFC3339, issue.CreatedAt)
		eng := map[string]float64{
			"reactions": float64(issue.Reactions.Total),
			"comments":  float64(issue.Comments),
			"thumbsup":  float64(issue.Reactions.Plus),
		}
		total := float64(issue.Reactions.Total) + float64(issue.Comments)*2
		engScore := 0.0
		if total > 0 { engScore = math.Log(total) / math.Log(100000) }
		if engScore > 1 { engScore = 1 }

		items = append(items, SourceItem{
			ItemID:          "ghi_" + fmt.Sprintf("%d", hash(issue.HTMLURL)),
			Source:          "github",
			Title:           "[Issue] " + issue.Title,
			Body:            truncateStr(issue.Body, 1500),
			URL:             issue.HTMLURL,
			Author:          issue.User.Login,
			PublishedAt:     &created,
			DateConfidence:  "high",
			Engagement:      eng,
			EngagementScore: &engScore,
		})
	}
	return items, nil
}

func hash(s string) uint32 {
	var h uint32
	for _, c := range s { h = h*31 + uint32(c) }
	return h
}

func min(a, b int) int { if a < b { return a }; return b }

type ghRepo struct {
	FullName    string   `json:"full_name"`
	Description string   `json:"description"`
	HTMLURL     string   `json:"html_url"`
	Stars       int      `json:"stargazers_count"`
	Forks       int      `json:"forks_count"`
	Issues      int      `json:"open_issues_count"`
	Language    string   `json:"language"`
	CreatedAt   string   `json:"created_at"`
	PushedAt    string   `json:"pushed_at"`
	Topics      []string `json:"topics"`
	Owner       struct{ Login string `json:"login"` } `json:"owner"`
}
