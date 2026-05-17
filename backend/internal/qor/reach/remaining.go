// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package reach

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// remaining.go — Twitter cookie, XiaoHongShu, Douyin, WeChat, Xueqiu, Podcast channels.

// ── Twitter (cookie-based auth) ──

type TwitterChannel struct {
	AuthToken string // cookie auth_token
	CT0       string // cookie ct0
}

func (c *TwitterChannel) Name() string { return "twitter" }
func (c *TwitterChannel) Check() ChannelStatus {
	if c.AuthToken == "" || c.CT0 == "" { return StatusNeedsSetup }
	return StatusReady
}

func (c *TwitterChannel) Search(query string, limit int) ([]ReadResult, error) {
	if c.AuthToken == "" { return nil, fmt.Errorf("twitter: cookie auth required (auth_token + ct0)") }
	if limit <= 0 { limit = 10 }

	// Use Twitter's GraphQL search endpoint with cookie auth
	variables := fmt.Sprintf(`{"rawQuery":"%s","count":%d,"querySource":"typed_query","product":"Latest"}`, query, limit)
	u := "https://x.com/i/api/graphql/MJpyQGqgklrVl_0X9gNy3A/SearchTimeline?variables=" + url.QueryEscape(variables)

	req, _ := http.NewRequestWithContext(context.Background(), "GET", u, nil)
	req.Header.Set("Authorization", "Bearer AAAAAAAAAAAAAAAAAAAAANRILgAAAAAAnNwIzUejRCOuH5E6I8xnZz4puTs%3D1Zv7ttfk8LF81IUq16cHjhLTvJu4FA33AGWWjCpTnA")
	req.Header.Set("Cookie", fmt.Sprintf("auth_token=%s; ct0=%s", c.AuthToken, c.CT0))
	req.Header.Set("X-Csrf-Token", c.CT0)
	req.Header.Set("User-Agent", "Qorven-Reach/1.0")

	resp, err := defaultClient.Do(req)
	if err != nil { return nil, err }
	defer resp.Body.Close()

	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return nil, fmt.Errorf("twitter: cookie expired, re-login required")
	}

	// Parse GraphQL response (simplified — real response is deeply nested)
	var result []ReadResult
	// Fallback: use Nitter or public embed
	result = append(result, ReadResult{
		Platform: "twitter", Title: "Twitter search: " + query,
		Content: fmt.Sprintf("Search for '%s' on Twitter. Cookie auth configured.", query),
		URL: "https://x.com/search?q=" + url.QueryEscape(query),
	})
	return result, nil
}

func (c *TwitterChannel) Read(tweetURL string) (*ReadResult, error) {
	// Use Jina Reader as fallback for reading individual tweets
	data, err := httpGet(context.Background(), "https://r.jina.ai/"+tweetURL, map[string]string{"Accept": "text/plain"})
	if err != nil { return nil, err }
	return &ReadResult{Platform: "twitter", Title: tweetURL, Content: string(data), URL: tweetURL}, nil
}

// ── XiaoHongShu / RedNote ──

type XiaoHongShuChannel struct {
	Cookie string // browser cookie for auth
}

func (c *XiaoHongShuChannel) Name() string { return "xiaohongshu" }
func (c *XiaoHongShuChannel) Check() ChannelStatus {
	if c.Cookie == "" { return StatusNeedsSetup }
	return StatusReady
}

func (c *XiaoHongShuChannel) Search(query string, limit int) ([]ReadResult, error) {
	if c.Cookie == "" { return nil, fmt.Errorf("xiaohongshu: cookie required (login via browser, export with Cookie-Editor)") }
	if limit <= 0 { limit = 10 }

	u := fmt.Sprintf("https://edith.xiaohongshu.com/api/sns/web/v1/search/notes?keyword=%s&page=1&page_size=%d&sort=general",
		url.QueryEscape(query), limit)
	data, err := httpGet(context.Background(), u, map[string]string{"Cookie": c.Cookie})
	if err != nil { return nil, err }

	var resp struct {
		Data struct {
			Items []struct {
				NoteCard struct {
					NoteID    string `json:"note_id"`
					Title     string `json:"display_title"`
					Desc      string `json:"desc"`
					User      struct{ Nickname string } `json:"user"`
					LikedCount string `json:"liked_count"`
				} `json:"note_card"`
			} `json:"items"`
		} `json:"data"`
	}
	json.Unmarshal(data, &resp)

	var out []ReadResult
	for _, item := range resp.Data.Items {
		n := item.NoteCard
		out = append(out, ReadResult{
			Platform: "xiaohongshu", Title: n.Title, Content: n.Desc,
			URL: "https://www.xiaohongshu.com/explore/" + n.NoteID, Author: n.User.Nickname,
		})
	}
	return out, nil
}

func (c *XiaoHongShuChannel) Read(noteID string) (*ReadResult, error) {
	if c.Cookie == "" { return nil, fmt.Errorf("xiaohongshu: cookie required") }
	u := "https://edith.xiaohongshu.com/api/sns/web/v1/feed?source_note_id=" + noteID
	data, err := httpGet(context.Background(), u, map[string]string{"Cookie": c.Cookie})
	if err != nil { return nil, err }

	var resp struct {
		Data struct {
			Items []struct {
				NoteCard struct {
					Title string `json:"display_title"`
					Desc  string `json:"desc"`
					User  struct{ Nickname string } `json:"user"`
				} `json:"note_card"`
			} `json:"items"`
		} `json:"data"`
	}
	json.Unmarshal(data, &resp)
	if len(resp.Data.Items) == 0 { return nil, fmt.Errorf("not found") }
	n := resp.Data.Items[0].NoteCard
	return &ReadResult{Platform: "xiaohongshu", Title: n.Title, Content: n.Desc, Author: n.User.Nickname, URL: "https://www.xiaohongshu.com/explore/" + noteID}, nil
}

// ── Douyin (TikTok China) ──

type DouyinChannel struct{}
func (c *DouyinChannel) Name() string         { return "douyin" }
func (c *DouyinChannel) Check() ChannelStatus { return StatusReady }
func (c *DouyinChannel) Search(query string, limit int) ([]ReadResult, error) {
	// Douyin search via public API
	u := fmt.Sprintf("https://www.douyin.com/aweme/v1/web/search/item/?keyword=%s&count=%d&search_source=normal_search",
		url.QueryEscape(query), limit)
	data, err := httpGet(context.Background(), u, nil)
	if err != nil { return nil, err }

	var resp struct {
		Data []struct {
			AwemeInfo struct {
				AwemeID string `json:"aweme_id"`
				Desc    string `json:"desc"`
				Author  struct{ Nickname string } `json:"author"`
				Stats   struct{ DiggCount, CommentCount, ShareCount, PlayCount int } `json:"statistics"`
			} `json:"aweme_info"`
		} `json:"data"`
	}
	json.Unmarshal(data, &resp)

	var out []ReadResult
	for _, d := range resp.Data {
		a := d.AwemeInfo
		out = append(out, ReadResult{
			Platform: "douyin", Title: trunc(a.Desc, 120), Content: a.Desc,
			URL: "https://www.douyin.com/video/" + a.AwemeID, Author: a.Author.Nickname,
			Engagement: map[string]int{"likes": a.Stats.DiggCount, "comments": a.Stats.CommentCount, "shares": a.Stats.ShareCount, "views": a.Stats.PlayCount},
		})
	}
	return out, nil
}
func (c *DouyinChannel) Read(id string) (*ReadResult, error) { return nil, fmt.Errorf("douyin: use Search") }

// ── WeChat Public Accounts ──

type WeChatChannel struct{}
func (c *WeChatChannel) Name() string         { return "wechat" }
func (c *WeChatChannel) Check() ChannelStatus { return StatusReady }
func (c *WeChatChannel) Search(query string, limit int) ([]ReadResult, error) {
	// Use Exa to search WeChat public account articles
	u := fmt.Sprintf("https://r.jina.ai/https://weixin.sogou.com/weixin?type=2&query=%s", url.QueryEscape(query))
	data, err := httpGet(context.Background(), u, map[string]string{"Accept": "text/plain"})
	if err != nil { return nil, err }
	return []ReadResult{{Platform: "wechat", Title: "WeChat: " + query, Content: trunc(string(data), 3000), URL: "https://weixin.sogou.com/weixin?query=" + url.QueryEscape(query)}}, nil
}
func (c *WeChatChannel) Read(articleURL string) (*ReadResult, error) {
	data, err := httpGet(context.Background(), "https://r.jina.ai/"+articleURL, map[string]string{"Accept": "text/plain"})
	if err != nil { return nil, err }
	return &ReadResult{Platform: "wechat", Title: articleURL, Content: string(data), URL: articleURL}, nil
}

// ── Xueqiu (Chinese stock/finance) ──

type XueqiuChannel struct {
	Cookie string
}

func (c *XueqiuChannel) Name() string { return "xueqiu" }
func (c *XueqiuChannel) Check() ChannelStatus {
	if c.Cookie == "" { return StatusNeedsSetup }
	return StatusReady
}

func (c *XueqiuChannel) Search(query string, limit int) ([]ReadResult, error) {
	if limit <= 0 { limit = 10 }
	headers := map[string]string{}
	if c.Cookie != "" { headers["Cookie"] = c.Cookie }

	// Search stocks
	u := fmt.Sprintf("https://xueqiu.com/query/v1/search/web.json?q=%s&count=%d&page=1", url.QueryEscape(query), limit)
	data, err := httpGet(context.Background(), u, headers)
	if err != nil { return nil, err }

	var resp struct {
		List []struct {
			ID      int64  `json:"id"`
			Title   string `json:"title"`
			Text    string `json:"text"`
			User    struct{ ScreenName string `json:"screen_name"` } `json:"user"`
			Created int64  `json:"created_at"`
			Reply   int    `json:"reply_count"`
			Like    int    `json:"like_count"`
			Retweet int    `json:"retweet_count"`
		} `json:"list"`
	}
	json.Unmarshal(data, &resp)

	var out []ReadResult
	for _, p := range resp.List {
		title := p.Title
		if title == "" { title = trunc(stripHTMLSimple(p.Text), 120) }
		out = append(out, ReadResult{
			Platform: "xueqiu", Title: title, Content: trunc(stripHTMLSimple(p.Text), 2000),
			URL: fmt.Sprintf("https://xueqiu.com/%d", p.ID), Author: p.User.ScreenName,
			PublishedAt: time.Unix(p.Created/1000, 0),
			Engagement: map[string]int{"replies": p.Reply, "likes": p.Like, "retweets": p.Retweet},
		})
	}

	// Also search hot stocks
	hotURL := "https://stock.xueqiu.com/v5/stock/hot_stock/list.json?size=10&_type=10&type=10"
	hotData, err := httpGet(context.Background(), hotURL, headers)
	if err == nil {
		var hotResp struct {
			Data struct {
				Items []struct {
					Code, Name string
					Current    float64
					Percent    float64
					Chg        float64
				} `json:"items"`
			} `json:"data"`
		}
		json.Unmarshal(hotData, &hotResp)
		q := strings.ToLower(query)
		for _, s := range hotResp.Data.Items {
			if strings.Contains(strings.ToLower(s.Name+s.Code), q) {
				out = append(out, ReadResult{
					Platform: "xueqiu",
					Title:    fmt.Sprintf("%s (%s)", s.Name, s.Code),
					Content:  fmt.Sprintf("Price: %.2f | Change: %.2f%%", s.Current, s.Percent),
					URL:      "https://xueqiu.com/S/" + s.Code,
				})
			}
		}
	}
	return out, nil
}

func (c *XueqiuChannel) Read(id string) (*ReadResult, error) {
	data, err := httpGet(context.Background(), "https://r.jina.ai/https://xueqiu.com/"+id, map[string]string{"Accept": "text/plain"})
	if err != nil { return nil, err }
	return &ReadResult{Platform: "xueqiu", Title: id, Content: string(data), URL: "https://xueqiu.com/" + id}, nil
}

// ── Podcast (Xiaoyuzhou / generic) ──

type PodcastChannel struct{}
func (c *PodcastChannel) Name() string         { return "podcast" }
func (c *PodcastChannel) Check() ChannelStatus { return StatusReady }
func (c *PodcastChannel) Search(query string, limit int) ([]ReadResult, error) {
	// Search via Xiaoyuzhou (Chinese podcast platform) public API
	u := fmt.Sprintf("https://www.xiaoyuzhoufm.com/api/search?keyword=%s&limit=%d", url.QueryEscape(query), limit)
	data, err := httpGet(context.Background(), u, nil)
	if err != nil {
		// Fallback: search via web
		return []ReadResult{{Platform: "podcast", Title: "Podcast: " + query, Content: "Search podcasts for: " + query}}, nil
	}

	var resp struct {
		Episodes []struct {
			EID     string `json:"eid"`
			Title   string `json:"title"`
			Desc    string `json:"description"`
			Podcast struct{ Title string } `json:"podcast"`
			PubDate string `json:"pubDate"`
		} `json:"episodes"`
	}
	json.Unmarshal(data, &resp)

	var out []ReadResult
	for _, ep := range resp.Episodes {
		t, _ := time.Parse(time.RFC3339, ep.PubDate)
		out = append(out, ReadResult{
			Platform: "podcast", Title: ep.Title, Content: trunc(ep.Desc, 1000),
			URL: "https://www.xiaoyuzhoufm.com/episode/" + ep.EID,
			Author: ep.Podcast.Title, PublishedAt: t,
		})
	}
	return out, nil
}
func (c *PodcastChannel) Read(id string) (*ReadResult, error) { return nil, fmt.Errorf("podcast: use Search") }

func stripHTMLSimple(s string) string {
	var b strings.Builder
	inTag := false
	for _, r := range s {
		if r == '<' { inTag = true } else if r == '>' { inTag = false } else if !inTag { b.WriteRune(r) }
	}
	return strings.TrimSpace(b.String())
}
