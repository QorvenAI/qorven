// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package reach

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// channels.go — All platform channel readers for Qorven Reach.

var defaultClient = &http.Client{Timeout: 15 * time.Second}

func httpGet(ctx context.Context, rawURL string, headers map[string]string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", rawURL, nil)
	if err != nil { return nil, err }
	req.Header.Set("User-Agent", "Qorven-Reach/1.0")
	for k, v := range headers { req.Header.Set(k, v) }
	resp, err := defaultClient.Do(req)
	if err != nil { return nil, err }
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(b))
	}
	return io.ReadAll(io.LimitReader(resp.Body, 4<<20))
}

func trunc(s string, n int) string { if len(s) <= n { return s }; return s[:n] + "..." }

// ── Web (Jina Reader) ──

type WebChannel struct{}
func (c *WebChannel) Name() string          { return "web" }
func (c *WebChannel) Check() ChannelStatus  { return StatusReady }
func (c *WebChannel) Search(_ string, _ int) ([]ReadResult, error) { return nil, fmt.Errorf("web: use Read with URL") }
func (c *WebChannel) Read(rawURL string) (*ReadResult, error) {
	data, err := httpGet(context.Background(), "https://r.jina.ai/"+rawURL, map[string]string{"Accept": "text/plain"})
	if err != nil { return nil, err }
	return &ReadResult{Platform: "web", Title: rawURL, Content: string(data), URL: rawURL}, nil
}

// ── Reddit ──

type RedditChannel struct{}
func (c *RedditChannel) Name() string         { return "reddit" }
func (c *RedditChannel) Check() ChannelStatus { return StatusReady }
func (c *RedditChannel) Search(query string, limit int) ([]ReadResult, error) {
	if limit <= 0 { limit = 10 }
	u := fmt.Sprintf("https://www.reddit.com/search.json?q=%s&sort=relevance&t=month&limit=%d", url.QueryEscape(query), limit)
	data, err := httpGet(context.Background(), u, map[string]string{"Accept": "application/json"})
	if err != nil { return nil, err }
	var resp struct{ Data struct{ Children []struct{ Data struct {
		Title, Selftext, Subreddit, Author, Permalink string; Ups, NumComments int; CreatedUTC float64
	} `json:"data"` } `json:"children"` } `json:"data"` }
	json.Unmarshal(data, &resp)
	var out []ReadResult
	for _, c := range resp.Data.Children {
		d := c.Data
		out = append(out, ReadResult{Platform: "reddit", Title: d.Title, Content: trunc(d.Selftext, 2000),
			URL: "https://reddit.com" + d.Permalink, Author: d.Author,
			PublishedAt: time.Unix(int64(d.CreatedUTC), 0),
			Engagement: map[string]int{"upvotes": d.Ups, "comments": d.NumComments}})
	}
	return out, nil
}
func (c *RedditChannel) Read(id string) (*ReadResult, error) {
	u := id; if !strings.HasPrefix(u, "http") { u = "https://www.reddit.com/comments/" + id + ".json" } else { u = strings.TrimSuffix(u, "/") + ".json" }
	data, err := httpGet(context.Background(), u, map[string]string{"Accept": "application/json"})
	if err != nil { return nil, err }
	var listings []struct{ Data struct{ Children []struct{ Data struct {
		Title, Selftext, Author, Permalink, Body string; Ups, NumComments, Score int; CreatedUTC float64
	} `json:"data"` } `json:"children"` } `json:"data"` }
	json.Unmarshal(data, &listings)
	if len(listings) == 0 || len(listings[0].Data.Children) == 0 { return nil, fmt.Errorf("not found") }
	p := listings[0].Data.Children[0].Data
	content := p.Selftext
	if len(listings) > 1 { for i, c := range listings[1].Data.Children { if i >= 10 || c.Data.Body == "" { continue }; content += fmt.Sprintf("\n\n[%d pts] %s", c.Data.Score, trunc(c.Data.Body, 300)) } }
	return &ReadResult{Platform: "reddit", Title: p.Title, Content: trunc(content, 5000), URL: "https://reddit.com" + p.Permalink, Author: p.Author, PublishedAt: time.Unix(int64(p.CreatedUTC), 0), Engagement: map[string]int{"upvotes": p.Ups, "comments": p.NumComments}}, nil
}

// ── YouTube ──

type YouTubeChannel struct{}
func (c *YouTubeChannel) Name() string         { return "youtube" }
func (c *YouTubeChannel) Check() ChannelStatus { return StatusReady }
func (c *YouTubeChannel) Search(query string, limit int) ([]ReadResult, error) {
	// Uses Invidious public API (no key needed)
	if limit <= 0 { limit = 10 }
	u := fmt.Sprintf("https://vid.puffyan.us/api/v1/search?q=%s&type=video&sort_by=relevance&page=1", url.QueryEscape(query))
	data, err := httpGet(context.Background(), u, nil)
	if err != nil { return nil, err }
	var vids []struct{ Title, VideoID, Author string; ViewCount, LengthSeconds int; PublishedText string }
	json.Unmarshal(data, &vids)
	var out []ReadResult
	for i, v := range vids { if i >= limit { break }; out = append(out, ReadResult{Platform: "youtube", Title: v.Title, Content: fmt.Sprintf("By %s | %d views", v.Author, v.ViewCount), URL: "https://youtube.com/watch?v=" + v.VideoID, Author: v.Author, Engagement: map[string]int{"views": v.ViewCount}}) }
	return out, nil
}
func (c *YouTubeChannel) Read(videoURL string) (*ReadResult, error) {
	return &ReadResult{Platform: "youtube", Title: videoURL, Content: "Use yt-dlp for transcript: yt-dlp --write-sub --skip-download " + videoURL, URL: videoURL}, nil
}

// ── GitHub ──

type GitHubChannel struct{ Token string }
func (c *GitHubChannel) Name() string         { return "github" }
func (c *GitHubChannel) Check() ChannelStatus { return StatusReady }
func (c *GitHubChannel) Search(query string, limit int) ([]ReadResult, error) {
	if limit <= 0 { limit = 10 }
	h := map[string]string{"Accept": "application/vnd.github.v3+json"}
	if c.Token != "" { h["Authorization"] = "Bearer " + c.Token }
	u := fmt.Sprintf("https://api.github.com/search/repositories?q=%s&sort=stars&per_page=%d", url.QueryEscape(query), limit)
	data, err := httpGet(context.Background(), u, h)
	if err != nil { return nil, err }
	var resp struct {
		Items []struct {
			FullName    string `json:"full_name"`
			Description string `json:"description"`
			HTMLURL     string `json:"html_url"`
			Language    string `json:"language"`
			Stars       int    `json:"stargazers_count"`
			Forks       int    `json:"forks_count"`
			Owner       struct{ Login string `json:"login"` } `json:"owner"`
		} `json:"items"`
	}
	json.Unmarshal(data, &resp)
	var out []ReadResult
	for _, r := range resp.Items { out = append(out, ReadResult{Platform: "github", Title: r.FullName, Content: r.Description + " [" + r.Language + "]", URL: r.HTMLURL, Author: r.Owner.Login, Engagement: map[string]int{"stars": r.Stars, "forks": r.Forks}}) }
	return out, nil
}
func (c *GitHubChannel) Read(repo string) (*ReadResult, error) {
	h := map[string]string{"Accept": "application/vnd.github.v3+json"}
	if c.Token != "" { h["Authorization"] = "Bearer " + c.Token }
	data, err := httpGet(context.Background(), "https://api.github.com/repos/"+repo, h)
	if err != nil { return nil, err }
	var r struct {
		FullName    string `json:"full_name"`
		Description string `json:"description"`
		HTMLURL     string `json:"html_url"`
		Language    string `json:"language"`
		Stars       int    `json:"stargazers_count"`
		Forks       int    `json:"forks_count"`
		Issues      int    `json:"open_issues_count"`
		Owner       struct{ Login string `json:"login"` } `json:"owner"`
	}
	json.Unmarshal(data, &r)
	h["Accept"] = "application/vnd.github.v3.raw"
	readme, _ := httpGet(context.Background(), "https://api.github.com/repos/"+repo+"/readme", h)
	content := fmt.Sprintf("%s [%s]\nStars: %d | Forks: %d | Issues: %d\n\n%s", r.Description, r.Language, r.Stars, r.Forks, r.Issues, trunc(string(readme), 3000))
	return &ReadResult{Platform: "github", Title: r.FullName, Content: content, URL: r.HTMLURL, Author: r.Owner.Login, Engagement: map[string]int{"stars": r.Stars, "forks": r.Forks}}, nil
}

// ── Hacker News ──

type HNChannel struct{}
func (c *HNChannel) Name() string         { return "hackernews" }
func (c *HNChannel) Check() ChannelStatus { return StatusReady }
func (c *HNChannel) Search(query string, limit int) ([]ReadResult, error) {
	if limit <= 0 { limit = 10 }
	u := fmt.Sprintf("https://hn.algolia.com/api/v1/search?query=%s&tags=story&hitsPerPage=%d", url.QueryEscape(query), limit)
	data, err := httpGet(context.Background(), u, nil)
	if err != nil { return nil, err }
	var resp struct{ Hits []struct{ Title, URL, Author, ObjectID, CreatedAt string; Points int `json:"points"`; NumComments int `json:"num_comments"` } `json:"hits"` }
	json.Unmarshal(data, &resp)
	var out []ReadResult
	for _, h := range resp.Hits { t, _ := time.Parse(time.RFC3339, h.CreatedAt); out = append(out, ReadResult{Platform: "hackernews", Title: h.Title, Content: h.URL, URL: "https://news.ycombinator.com/item?id=" + h.ObjectID, Author: h.Author, PublishedAt: t, Engagement: map[string]int{"points": h.Points, "comments": h.NumComments}}) }
	return out, nil
}
func (c *HNChannel) Read(id string) (*ReadResult, error) {
	data, err := httpGet(context.Background(), "https://hacker-news.firebaseio.com/v0/item/"+id+".json", nil)
	if err != nil { return nil, err }
	var item struct{ ID int; Title, Text, URL, By string; Score, Descendants int; Time int64; Kids []int }
	json.Unmarshal(data, &item)
	content := item.Text; if content == "" { content = item.URL }
	return &ReadResult{Platform: "hackernews", Title: item.Title, Content: trunc(content, 3000), URL: fmt.Sprintf("https://news.ycombinator.com/item?id=%d", item.ID), Author: item.By, PublishedAt: time.Unix(item.Time, 0), Engagement: map[string]int{"points": item.Score, "comments": item.Descendants}}, nil
}

// ── V2EX ──

type V2EXChannel struct{}
func (c *V2EXChannel) Name() string         { return "v2ex" }
func (c *V2EXChannel) Check() ChannelStatus { return StatusReady }
func (c *V2EXChannel) Search(query string, _ int) ([]ReadResult, error) {
	// V2EX hot topics
	data, err := httpGet(context.Background(), "https://www.v2ex.com/api/topics/hot.json", nil)
	if err != nil { return nil, err }
	var topics []struct{ ID int; Title, Content string; Member struct{ Username string }; Replies int; Created int64 }
	json.Unmarshal(data, &topics)
	var out []ReadResult
	q := strings.ToLower(query)
	for _, t := range topics {
		if !strings.Contains(strings.ToLower(t.Title+t.Content), q) { continue }
		out = append(out, ReadResult{Platform: "v2ex", Title: t.Title, Content: trunc(t.Content, 1000), URL: fmt.Sprintf("https://www.v2ex.com/t/%d", t.ID), Author: t.Member.Username, PublishedAt: time.Unix(t.Created, 0), Engagement: map[string]int{"replies": t.Replies}})
	}
	return out, nil
}
func (c *V2EXChannel) Read(id string) (*ReadResult, error) {
	data, err := httpGet(context.Background(), "https://www.v2ex.com/api/topics/show.json?id="+id, nil)
	if err != nil { return nil, err }
	var topics []struct{ ID int; Title, Content string; Member struct{ Username string }; Replies int; Created int64 }
	json.Unmarshal(data, &topics)
	if len(topics) == 0 { return nil, fmt.Errorf("not found") }
	t := topics[0]
	return &ReadResult{Platform: "v2ex", Title: t.Title, Content: t.Content, URL: fmt.Sprintf("https://www.v2ex.com/t/%d", t.ID), Author: t.Member.Username, PublishedAt: time.Unix(t.Created, 0), Engagement: map[string]int{"replies": t.Replies}}, nil
}

// ── Weibo ──

type WeiboChannel struct{}
func (c *WeiboChannel) Name() string         { return "weibo" }
func (c *WeiboChannel) Check() ChannelStatus { return StatusReady }
func (c *WeiboChannel) Search(query string, limit int) ([]ReadResult, error) {
	// Weibo hot search (public, no auth)
	data, err := httpGet(context.Background(), "https://weibo.com/ajax/side/hotSearch", nil)
	if err != nil { return nil, err }
	var resp struct{ Data struct{ Realtime []struct{ Word string; Num int; RawHot int } } }
	json.Unmarshal(data, &resp)
	var out []ReadResult
	q := strings.ToLower(query)
	for _, h := range resp.Data.Realtime {
		if query != "" && !strings.Contains(strings.ToLower(h.Word), q) { continue }
		out = append(out, ReadResult{Platform: "weibo", Title: h.Word, Content: fmt.Sprintf("Hot rank: %d", h.RawHot), URL: "https://s.weibo.com/weibo?q=" + url.QueryEscape(h.Word), Engagement: map[string]int{"hot": h.RawHot}})
		if len(out) >= limit { break }
	}
	return out, nil
}
func (c *WeiboChannel) Read(id string) (*ReadResult, error) { return nil, fmt.Errorf("weibo: read requires cookie auth") }

// ── RSS ──

type RSSChannel struct{}
func (c *RSSChannel) Name() string         { return "rss" }
func (c *RSSChannel) Check() ChannelStatus { return StatusReady }
func (c *RSSChannel) Search(_ string, _ int) ([]ReadResult, error) { return nil, fmt.Errorf("rss: use Read with feed URL") }
func (c *RSSChannel) Read(feedURL string) (*ReadResult, error) {
	data, err := httpGet(context.Background(), feedURL, nil)
	if err != nil { return nil, err }
	// Simple XML extraction
	content := string(data)
	title := xmlExtract(content, "title")
	desc := xmlExtract(content, "description")
	if desc == "" { desc = xmlExtract(content, "summary") }
	return &ReadResult{Platform: "rss", Title: title, Content: trunc(desc, 2000), URL: feedURL}, nil
}

func xmlExtract(xml, tag string) string {
	open := "<" + tag; close := "</" + tag + ">"
	s := strings.Index(xml, open); if s == -1 { return "" }
	e := strings.Index(xml[s:], ">"); if e == -1 { return "" }
	cs := s + e + 1; ce := strings.Index(xml[cs:], close); if ce == -1 { return "" }
	v := xml[cs : cs+ce]; v = strings.TrimPrefix(v, "<![CDATA["); v = strings.TrimSuffix(v, "]]>")
	return strings.TrimSpace(v)
}

// ── Exa Search ──

type ExaChannel struct{ APIKey string }
func (c *ExaChannel) Name() string         { return "exa" }
func (c *ExaChannel) Check() ChannelStatus { if c.APIKey == "" { return StatusNeedsSetup }; return StatusReady }
func (c *ExaChannel) Search(query string, limit int) ([]ReadResult, error) {
	if c.APIKey == "" { return nil, fmt.Errorf("exa: API key required") }
	if limit <= 0 { limit = 10 }
	body := fmt.Sprintf(`{"query":"%s","numResults":%d,"type":"auto","useAutoprompt":true}`, query, limit)
	req, _ := http.NewRequest("POST", "https://api.exa.ai/search", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.APIKey)
	resp, err := defaultClient.Do(req)
	if err != nil { return nil, err }
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	var result struct{ Results []struct{ Title, URL, Text, Author, PublishedDate string; Score float64 } }
	json.Unmarshal(data, &result)
	var out []ReadResult
	for _, r := range result.Results { t, _ := time.Parse("2006-01-02", r.PublishedDate); out = append(out, ReadResult{Platform: "exa", Title: r.Title, Content: trunc(r.Text, 2000), URL: r.URL, Author: r.Author, PublishedAt: t}) }
	return out, nil
}
func (c *ExaChannel) Read(u string) (*ReadResult, error) { return nil, fmt.Errorf("exa: use Search") }

// ── LinkedIn (via Jina Reader) ──

type LinkedInChannel struct{}
func (c *LinkedInChannel) Name() string         { return "linkedin" }
func (c *LinkedInChannel) Check() ChannelStatus { return StatusReady }
func (c *LinkedInChannel) Search(_ string, _ int) ([]ReadResult, error) { return nil, fmt.Errorf("linkedin: use Read with profile URL") }
func (c *LinkedInChannel) Read(profileURL string) (*ReadResult, error) {
	data, err := httpGet(context.Background(), "https://r.jina.ai/"+profileURL, map[string]string{"Accept": "text/plain"})
	if err != nil { return nil, err }
	return &ReadResult{Platform: "linkedin", Title: profileURL, Content: string(data), URL: profileURL}, nil
}

// ── Bilibili ──

type BilibiliChannel struct{}
func (c *BilibiliChannel) Name() string         { return "bilibili" }
func (c *BilibiliChannel) Check() ChannelStatus { return StatusReady }
func (c *BilibiliChannel) Search(query string, limit int) ([]ReadResult, error) {
	if limit <= 0 { limit = 10 }
	u := fmt.Sprintf("https://api.bilibili.com/x/web-interface/search/type?search_type=video&keyword=%s&page=1&pagesize=%d", url.QueryEscape(query), limit)
	data, err := httpGet(context.Background(), u, nil)
	if err != nil { return nil, err }
	var resp struct{ Data struct{ Result []struct{ Title, Author, Bvid, Description string; Play, Danmaku int } } }
	json.Unmarshal(data, &resp)
	var out []ReadResult
	for _, v := range resp.Data.Result { out = append(out, ReadResult{Platform: "bilibili", Title: v.Title, Content: v.Description, URL: "https://www.bilibili.com/video/" + v.Bvid, Author: v.Author, Engagement: map[string]int{"views": v.Play, "danmaku": v.Danmaku}}) }
	return out, nil
}
func (c *BilibiliChannel) Read(bvid string) (*ReadResult, error) {
	u := fmt.Sprintf("https://api.bilibili.com/x/web-interface/view?bvid=%s", bvid)
	data, err := httpGet(context.Background(), u, nil)
	if err != nil { return nil, err }
	var resp struct{ Data struct{ Title, Desc string; Owner struct{ Name string }; Stat struct{ View, Like, Reply, Danmaku int } } }
	json.Unmarshal(data, &resp)
	d := resp.Data
	return &ReadResult{Platform: "bilibili", Title: d.Title, Content: d.Desc, URL: "https://www.bilibili.com/video/" + bvid, Author: d.Owner.Name, Engagement: map[string]int{"views": d.Stat.View, "likes": d.Stat.Like, "replies": d.Stat.Reply, "danmaku": d.Stat.Danmaku}}, nil
}
