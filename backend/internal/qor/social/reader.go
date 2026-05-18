// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package social

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// reader.go — Pluggable platform reader interface for Qorven Social Intelligence.

// PlatformReader reads content from a social/web platform.
type PlatformReader interface {
	Platform() Platform
	Search(ctx context.Context, query string, opts SearchOpts) ([]ScoredResult, error)
	Read(ctx context.Context, id string) (*ScoredResult, error)
	Available() bool
}

// SearchOpts controls search behavior.
type SearchOpts struct {
	MaxResults int
	TimeRange  time.Duration
	SortBy     string // "relevance", "engagement", "recent"
}

func (o SearchOpts) maxResults() int {
	if o.MaxResults <= 0 { return 10 }
	if o.MaxResults > 50 { return 50 }
	return o.MaxResults
}

func (o SearchOpts) afterTime() time.Time {
	d := o.TimeRange
	if d <= 0 { d = 30 * 24 * time.Hour }
	return time.Now().Add(-d)
}

// ScoredResult is a single result from any platform, with engagement scoring.
type ScoredResult struct {
	Platform    Platform  `json:"platform"`
	Title       string    `json:"title"`
	Content     string    `json:"content"`
	URL         string    `json:"url"`
	Author      string    `json:"author"`
	PublishedAt time.Time `json:"published_at"`
	Upvotes     int       `json:"upvotes,omitempty"`
	Downvotes   int       `json:"downvotes,omitempty"`
	Likes       int       `json:"likes,omitempty"`
	Comments    int       `json:"comments,omitempty"`
	Shares      int       `json:"shares,omitempty"`
	Views       int       `json:"views,omitempty"`
	Subreddit   string    `json:"subreddit,omitempty"`
	Tags        []string  `json:"tags,omitempty"`
	Extra       map[string]string `json:"extra,omitempty"`
	EngagementScore float64 `json:"engagement_score"`
	FinalScore      float64 `json:"final_score"`
}

// ReaderRegistry holds all configured platform readers.
type ReaderRegistry struct {
	readers map[Platform]PlatformReader
	client  *http.Client
}

func NewReaderRegistry() *ReaderRegistry {
	return &ReaderRegistry{
		readers: make(map[Platform]PlatformReader),
		client:  &http.Client{Timeout: 15 * time.Second},
	}
}

func (r *ReaderRegistry) Register(reader PlatformReader)            { r.readers[reader.Platform()] = reader }
func (r *ReaderRegistry) Get(p Platform) (PlatformReader, bool)     { rd, ok := r.readers[p]; return rd, ok }
func (r *ReaderRegistry) All() map[Platform]PlatformReader          { return r.readers }

func (r *ReaderRegistry) Available() []Platform {
	var out []Platform
	for p, rd := range r.readers {
		if rd.Available() { out = append(out, p) }
	}
	return out
}

// httpGet is a shared helper for all readers.
func httpGet(ctx context.Context, client *http.Client, rawURL string, headers map[string]string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", rawURL, nil)
	if err != nil { return nil, err }
	req.Header.Set("User-Agent", "Qorven/1.0 (Social Intelligence)")
	for k, v := range headers { req.Header.Set(k, v) }
	resp, err := client.Do(req)
	if err != nil { return nil, err }
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}
	return io.ReadAll(io.LimitReader(resp.Body, 2<<20))
}

func truncate(s string, n int) string {
	if len(s) <= n { return s }
	return s[:n] + "..."
}

// normalizeEngagement computes a 0-1 score from raw engagement metrics.
func normalizeEngagement(upvotes, comments, views int) float64 {
	raw := float64(upvotes) + float64(comments)*2.0 + float64(views)*0.1
	if raw <= 0 { return 0 }
	score := math.Log(raw) / math.Log(100000)
	if score > 1 { score = 1 }
	return score
}

// ── Reddit Reader ──

type RedditReader struct{ client *http.Client }

func NewRedditReader(client *http.Client) *RedditReader { return &RedditReader{client: client} }
func (r *RedditReader) Platform() Platform              { return PlatformReddit }
func (r *RedditReader) Available() bool                 { return true }

func (r *RedditReader) Search(ctx context.Context, query string, opts SearchOpts) ([]ScoredResult, error) {
	u := fmt.Sprintf("https://www.reddit.com/search.json?q=%s&sort=relevance&t=month&limit=%d",
		url.QueryEscape(query), opts.maxResults())
	data, err := httpGet(ctx, r.client, u, map[string]string{"Accept": "application/json"})
	if err != nil { return nil, fmt.Errorf("reddit: %w", err) }

	var resp struct {
		Data struct {
			Children []struct {
				Data struct {
					Title       string  `json:"title"`
					Selftext    string  `json:"selftext"`
					Subreddit   string  `json:"subreddit"`
					Author      string  `json:"author"`
					Ups         int     `json:"ups"`
					NumComments int     `json:"num_comments"`
					Permalink   string  `json:"permalink"`
					URL         string  `json:"url"`
					CreatedUTC  float64 `json:"created_utc"`
				} `json:"data"`
			} `json:"children"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &resp); err != nil { return nil, err }

	var results []ScoredResult
	for _, c := range resp.Data.Children {
		d := c.Data
		created := time.Unix(int64(d.CreatedUTC), 0)
		if created.Before(opts.afterTime()) { continue }
		content := d.Selftext
		if content == "" { content = d.URL }
		results = append(results, ScoredResult{
			Platform: PlatformReddit, Title: d.Title, Content: truncate(content, 2000),
			URL: "https://reddit.com" + d.Permalink, Author: d.Author, PublishedAt: created,
			Upvotes: d.Ups, Comments: d.NumComments, Subreddit: d.Subreddit,
			EngagementScore: normalizeEngagement(d.Ups, d.NumComments, 0),
		})
	}
	return results, nil
}

func (r *RedditReader) Read(ctx context.Context, id string) (*ScoredResult, error) {
	u := id
	if !strings.HasPrefix(u, "http") {
		u = "https://www.reddit.com/comments/" + id + ".json"
	} else {
		u = strings.TrimSuffix(u, "/") + ".json"
	}
	data, err := httpGet(ctx, r.client, u, map[string]string{"Accept": "application/json"})
	if err != nil { return nil, err }

	var listings []struct {
		Data struct {
			Children []struct {
				Data struct {
					Title       string  `json:"title"`
					Selftext    string  `json:"selftext"`
					Author      string  `json:"author"`
					Ups         int     `json:"ups"`
					NumComments int     `json:"num_comments"`
					Subreddit   string  `json:"subreddit"`
					Permalink   string  `json:"permalink"`
					CreatedUTC  float64 `json:"created_utc"`
					Body        string  `json:"body"`
					Score       int     `json:"score"`
				} `json:"data"`
			} `json:"children"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &listings); err != nil { return nil, err }
	if len(listings) == 0 || len(listings[0].Data.Children) == 0 {
		return nil, fmt.Errorf("reddit: not found")
	}

	post := listings[0].Data.Children[0].Data
	content := post.Selftext

	// Collect top comments from second listing
	if len(listings) > 1 {
		var comments []string
		for i, c := range listings[1].Data.Children {
			if i >= 10 || c.Data.Body == "" { continue }
			comments = append(comments, fmt.Sprintf("[%d pts] %s", c.Data.Score, truncate(c.Data.Body, 300)))
		}
		if len(comments) > 0 {
			content += "\n\n--- Top Comments ---\n" + strings.Join(comments, "\n\n")
		}
	}

	return &ScoredResult{
		Platform: PlatformReddit, Title: post.Title, Content: truncate(content, 5000),
		URL: "https://reddit.com" + post.Permalink, Author: post.Author,
		PublishedAt: time.Unix(int64(post.CreatedUTC), 0),
		Upvotes: post.Ups, Comments: post.NumComments, Subreddit: post.Subreddit,
		EngagementScore: normalizeEngagement(post.Ups, post.NumComments, 0),
	}, nil
}

// ── Hacker News Reader ──

const PlatformHN Platform = "hackernews"

type HNReader struct{ client *http.Client }

func NewHNReader(client *http.Client) *HNReader { return &HNReader{client: client} }
func (r *HNReader) Platform() Platform          { return PlatformHN }
func (r *HNReader) Available() bool             { return true }

func (r *HNReader) Search(ctx context.Context, query string, opts SearchOpts) ([]ScoredResult, error) {
	after := opts.afterTime().Unix()
	u := fmt.Sprintf("https://hn.algolia.com/api/v1/search?query=%s&tags=story&numericFilters=created_at_i>%d&hitsPerPage=%d",
		url.QueryEscape(query), after, opts.maxResults())
	data, err := httpGet(ctx, r.client, u, nil)
	if err != nil { return nil, fmt.Errorf("hn: %w", err) }

	var resp struct {
		Hits []struct {
			Title       string `json:"title"`
			URL         string `json:"url"`
			Author      string `json:"author"`
			Points      int    `json:"points"`
			NumComments int    `json:"num_comments"`
			ObjectID    string `json:"objectID"`
			CreatedAt   string `json:"created_at"`
			StoryText   string `json:"story_text"`
		} `json:"hits"`
	}
	if err := json.Unmarshal(data, &resp); err != nil { return nil, err }

	var results []ScoredResult
	for _, h := range resp.Hits {
		created, _ := time.Parse(time.RFC3339, h.CreatedAt)
		content := h.StoryText
		if content == "" { content = h.URL }
		results = append(results, ScoredResult{
			Platform: PlatformHN, Title: h.Title, Content: truncate(content, 2000),
			URL: "https://news.ycombinator.com/item?id=" + h.ObjectID, Author: h.Author,
			PublishedAt: created, Upvotes: h.Points, Comments: h.NumComments,
			EngagementScore: normalizeEngagement(h.Points, h.NumComments, 0),
			Extra: map[string]string{"source_url": h.URL},
		})
	}
	return results, nil
}

func (r *HNReader) Read(ctx context.Context, id string) (*ScoredResult, error) {
	u := "https://hacker-news.firebaseio.com/v0/item/" + id + ".json"
	data, err := httpGet(ctx, r.client, u, nil)
	if err != nil { return nil, err }

	var item struct {
		ID    int    `json:"id"`
		Title string `json:"title"`
		Text  string `json:"text"`
		URL   string `json:"url"`
		By    string `json:"by"`
		Score int    `json:"score"`
		Desc  int    `json:"descendants"`
		Time  int64  `json:"time"`
		Kids  []int  `json:"kids"`
	}
	if err := json.Unmarshal(data, &item); err != nil { return nil, err }

	// Fetch top 5 comments
	content := item.Text
	if content == "" { content = item.URL }
	var comments []string
	for i, kid := range item.Kids {
		if i >= 5 { break }
		cu := fmt.Sprintf("https://hacker-news.firebaseio.com/v0/item/%d.json", kid)
		cdata, err := httpGet(ctx, r.client, cu, nil)
		if err != nil { continue }
		var c struct{ Text, By string }
		json.Unmarshal(cdata, &c)
		if c.Text != "" { comments = append(comments, fmt.Sprintf("[%s] %s", c.By, truncate(stripHTML(c.Text), 300))) }
	}
	if len(comments) > 0 { content += "\n\n--- Top Comments ---\n" + strings.Join(comments, "\n\n") }

	return &ScoredResult{
		Platform: PlatformHN, Title: item.Title, Content: truncate(content, 5000),
		URL: fmt.Sprintf("https://news.ycombinator.com/item?id=%d", item.ID),
		Author: item.By, PublishedAt: time.Unix(item.Time, 0),
		Upvotes: item.Score, Comments: item.Desc,
		EngagementScore: normalizeEngagement(item.Score, item.Desc, 0),
	}, nil
}

// ── GitHub Reader ──

const PlatformGitHubRead Platform = "github"

type GitHubReader struct {
	client *http.Client
	token  string
}

func NewGitHubReader(client *http.Client, token string) *GitHubReader {
	return &GitHubReader{client: client, token: token}
}

func (r *GitHubReader) Platform() Platform { return PlatformGitHubRead }
func (r *GitHubReader) Available() bool    { return true }

func (r *GitHubReader) Search(ctx context.Context, query string, opts SearchOpts) ([]ScoredResult, error) {
	after := opts.afterTime().Format("2006-01-02")
	u := fmt.Sprintf("https://api.github.com/search/repositories?q=%s+created:>%s&sort=stars&order=desc&per_page=%d",
		url.QueryEscape(query), after, opts.maxResults())
	headers := map[string]string{"Accept": "application/vnd.github.v3+json"}
	if r.token != "" { headers["Authorization"] = "Bearer " + r.token }

	data, err := httpGet(ctx, r.client, u, headers)
	if err != nil { return nil, fmt.Errorf("github: %w", err) }

	var resp struct {
		Items []struct {
			FullName    string   `json:"full_name"`
			Description string   `json:"description"`
			HTMLURL     string   `json:"html_url"`
			Stars       int      `json:"stargazers_count"`
			Forks       int      `json:"forks_count"`
			Issues      int      `json:"open_issues_count"`
			Language    string   `json:"language"`
			CreatedAt   string   `json:"created_at"`
			Topics      []string `json:"topics"`
			Owner       struct{ Login string `json:"login"` } `json:"owner"`
		} `json:"items"`
	}
	if err := json.Unmarshal(data, &resp); err != nil { return nil, err }

	var results []ScoredResult
	for _, it := range resp.Items {
		created, _ := time.Parse(time.RFC3339, it.CreatedAt)
		desc := it.Description
		if it.Language != "" { desc += " [" + it.Language + "]" }
		results = append(results, ScoredResult{
			Platform: PlatformGitHubRead, Title: it.FullName, Content: desc,
			URL: it.HTMLURL, Author: it.Owner.Login, PublishedAt: created,
			Likes: it.Stars, Shares: it.Forks, Comments: it.Issues, Tags: it.Topics,
			EngagementScore: normalizeEngagement(it.Stars, it.Forks+it.Issues, 0),
		})
	}
	return results, nil
}

func (r *GitHubReader) Read(ctx context.Context, id string) (*ScoredResult, error) {
	u := "https://api.github.com/repos/" + id
	headers := map[string]string{"Accept": "application/vnd.github.v3+json"}
	if r.token != "" { headers["Authorization"] = "Bearer " + r.token }
	data, err := httpGet(ctx, r.client, u, headers)
	if err != nil { return nil, err }

	var repo struct {
		FullName string `json:"full_name"`
		Desc     string `json:"description"`
		HTMLURL  string `json:"html_url"`
		Stars    int    `json:"stargazers_count"`
		Forks    int    `json:"forks_count"`
		Issues   int    `json:"open_issues_count"`
		Lang     string `json:"language"`
		Created  string `json:"created_at"`
		Topics   []string `json:"topics"`
		Owner    struct{ Login string `json:"login"` } `json:"owner"`
	}
	if err := json.Unmarshal(data, &repo); err != nil { return nil, err }

	content := repo.Desc + "\n"
	if repo.Lang != "" { content += "Language: " + repo.Lang + "\n" }
	content += fmt.Sprintf("Stars: %d | Forks: %d | Issues: %d", repo.Stars, repo.Forks, repo.Issues)

	// Fetch README
	headers["Accept"] = "application/vnd.github.v3.raw"
	readme, _ := httpGet(ctx, r.client, u+"/readme", headers)
	if len(readme) > 0 { content += "\n\n--- README ---\n" + truncate(string(readme), 3000) }

	created, _ := time.Parse(time.RFC3339, repo.Created)
	return &ScoredResult{
		Platform: PlatformGitHubRead, Title: repo.FullName, Content: content,
		URL: repo.HTMLURL, Author: repo.Owner.Login, PublishedAt: created,
		Likes: repo.Stars, Shares: repo.Forks, Comments: repo.Issues, Tags: repo.Topics,
		EngagementScore: normalizeEngagement(repo.Stars, repo.Forks, 0),
	}, nil
}

// ── RSS Reader ──

type RSSReader struct{ client *http.Client }

func NewRSSReader(client *http.Client) *RSSReader { return &RSSReader{client: client} }
func (r *RSSReader) Platform() Platform           { return PlatformRSS }
func (r *RSSReader) Available() bool              { return true }

const PlatformRSS Platform = "rss"

func (r *RSSReader) Search(_ context.Context, _ string, _ SearchOpts) ([]ScoredResult, error) {
	return nil, fmt.Errorf("RSS: use ReadFeed with a feed URL")
}

func (r *RSSReader) Read(ctx context.Context, feedURL string) (*ScoredResult, error) {
	items, err := r.ReadFeed(ctx, feedURL, 1)
	if err != nil { return nil, err }
	if len(items) == 0 { return nil, fmt.Errorf("empty feed") }
	return &items[0], nil
}

func (r *RSSReader) ReadFeed(ctx context.Context, feedURL string, max int) ([]ScoredResult, error) {
	data, err := httpGet(ctx, r.client, feedURL, nil)
	if err != nil { return nil, err }
	items := parseRSSItems(data)
	if max > 0 && len(items) > max { items = items[:max] }
	return items, nil
}

func parseRSSItems(data []byte) []ScoredResult {
	content := string(data)
	var results []ScoredResult
	itemTag, endTag := "<item>", "</item>"
	if strings.Contains(content, "<entry>") { itemTag, endTag = "<entry>", "</entry>" }

	for {
		start := strings.Index(content, itemTag)
		if start == -1 { break }
		end := strings.Index(content[start:], endTag)
		if end == -1 { break }
		item := content[start : start+end+len(endTag)]
		content = content[start+end+len(endTag):]

		title := xmlTag(item, "title")
		link := xmlTag(item, "link")
		if link == "" { link = xmlAttr(item, "link", "href") }
		desc := xmlTag(item, "description")
		if desc == "" { desc = xmlTag(item, "summary") }
		author := xmlTag(item, "author")
		if author == "" { author = xmlTag(item, "dc:creator") }
		pub := xmlTag(item, "pubDate")
		if pub == "" { pub = xmlTag(item, "published") }

		results = append(results, ScoredResult{
			Platform: PlatformRSS, Title: title, Content: truncate(stripHTML(desc), 2000),
			URL: link, Author: author, PublishedAt: parseFlexTime(pub),
		})
	}
	return results
}

func xmlTag(xml, tag string) string {
	open := "<" + tag
	start := strings.Index(xml, open)
	if start == -1 { return "" }
	tagEnd := strings.Index(xml[start:], ">")
	if tagEnd == -1 { return "" }
	cs := start + tagEnd + 1
	end := strings.Index(xml[cs:], "</"+tag+">")
	if end == -1 { return "" }
	val := xml[cs : cs+end]
	val = strings.TrimPrefix(val, "<![CDATA[")
	val = strings.TrimSuffix(val, "]]>")
	return strings.TrimSpace(val)
}

func xmlAttr(xml, tag, attr string) string {
	open := "<" + tag
	start := strings.Index(xml, open)
	if start == -1 { return "" }
	tagEnd := strings.Index(xml[start:], ">")
	if tagEnd == -1 { return "" }
	tc := xml[start : start+tagEnd]
	as := strings.Index(tc, attr+"=\"")
	if as == -1 { return "" }
	vs := as + len(attr) + 2
	ve := strings.Index(tc[vs:], "\"")
	if ve == -1 { return "" }
	return tc[vs : vs+ve]
}

func stripHTML(s string) string {
	var b strings.Builder
	inTag := false
	for _, r := range s {
		if r == '<' { inTag = true } else if r == '>' { inTag = false } else if !inTag { b.WriteRune(r) }
	}
	return strings.TrimSpace(b.String())
}

func parseFlexTime(s string) time.Time {
	for _, f := range []string{time.RFC1123Z, time.RFC1123, time.RFC3339, "2006-01-02T15:04:05Z", "2006-01-02"} {
		if t, err := time.Parse(f, s); err == nil { return t }
	}
	return time.Time{}
}
