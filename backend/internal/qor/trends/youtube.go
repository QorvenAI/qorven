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

// youtube.go — YouTube search and transcript extraction.

type YouTubeSource struct {
	client *HTTPClient
	apiKey string // YouTube Data API v3 key
}

func NewYouTubeSource(apiKey string) *YouTubeSource {
	return &YouTubeSource{client: NewHTTPClient(15 * time.Second), apiKey: apiKey}
}

func (y *YouTubeSource) Name() string     { return "youtube" }
func (y *YouTubeSource) Available() bool   { return y.apiKey != "" }

func (y *YouTubeSource) Search(ctx context.Context, topic string, depth string) ([]SourceItem, error) {
	if y.apiKey == "" { return nil, fmt.Errorf("youtube: no API key") }

	maxResults := 10
	if depth == "deep" { maxResults = 25 }

	params := url.Values{
		"part":       {"snippet"},
		"q":          {extractCoreSubject(topic)},
		"type":       {"video"},
		"order":      {"relevance"},
		"maxResults": {fmt.Sprintf("%d", maxResults)},
		"key":        {y.apiKey},
		"publishedAfter": {time.Now().Add(-30 * 24 * time.Hour).Format(time.RFC3339)},
	}

	u := "https://www.googleapis.com/youtube/v3/search?" + params.Encode()
	data, err := y.client.Get(ctx, u, nil)
	if err != nil { return nil, fmt.Errorf("youtube search: %w", err) }

	var resp struct {
		Items []struct {
			ID      struct{ VideoID string `json:"videoId"` } `json:"id"`
			Snippet struct {
				Title       string `json:"title"`
				Description string `json:"description"`
				ChannelTitle string `json:"channelTitle"`
				PublishedAt string `json:"publishedAt"`
			} `json:"snippet"`
		} `json:"items"`
	}
	if err := json.Unmarshal(data, &resp); err != nil { return nil, err }

	// Get video statistics in batch
	var videoIDs []string
	for _, it := range resp.Items { videoIDs = append(videoIDs, it.ID.VideoID) }
	stats := y.getVideoStats(ctx, videoIDs)

	var items []SourceItem
	for _, it := range resp.Items {
		vid := it.ID.VideoID
		published := parseTime(it.Snippet.PublishedAt)
		engagement := map[string]float64{}
		if s, ok := stats[vid]; ok {
			engagement["views"] = s.Views
			engagement["likes"] = s.Likes
			engagement["comments"] = s.Comments
		}

		totalEng := engagement["likes"] + engagement["comments"]*3 + engagement["views"]*0.001
		engScore := 0.0
		if totalEng > 0 { engScore = math.Log(totalEng) / math.Log(100000) }
		if engScore > 1 { engScore = 1 }

		items = append(items, SourceItem{
			ItemID:          "yt_" + vid,
			Source:          "youtube",
			Title:           it.Snippet.Title,
			Body:            it.Snippet.Description,
			URL:             "https://youtube.com/watch?v=" + vid,
			Author:          it.Snippet.ChannelTitle,
			PublishedAt:     published,
			DateConfidence:  "high",
			Engagement:      engagement,
			EngagementScore: &engScore,
			RelevanceHint:   tokenOverlapRelevance(topic, it.Snippet.Title+" "+it.Snippet.Description),
		})
	}

	sort.Slice(items, func(i, j int) bool { return *items[i].EngagementScore > *items[j].EngagementScore })
	return items, nil
}

type videoStats struct {
	Views    float64
	Likes    float64
	Comments float64
}

func (y *YouTubeSource) getVideoStats(ctx context.Context, ids []string) map[string]videoStats {
	if len(ids) == 0 { return nil }
	params := url.Values{
		"part": {"statistics"},
		"id":   {strings.Join(ids, ",")},
		"key":  {y.apiKey},
	}
	u := "https://www.googleapis.com/youtube/v3/videos?" + params.Encode()
	data, err := y.client.Get(ctx, u, nil)
	if err != nil { return nil }

	var resp struct {
		Items []struct {
			ID    string `json:"id"`
			Stats struct {
				Views    string `json:"viewCount"`
				Likes    string `json:"likeCount"`
				Comments string `json:"commentCount"`
			} `json:"statistics"`
		} `json:"items"`
	}
	json.Unmarshal(data, &resp)

	out := map[string]videoStats{}
	for _, it := range resp.Items {
		out[it.ID] = videoStats{
			Views:    parseFloat(it.Stats.Views),
			Likes:    parseFloat(it.Stats.Likes),
			Comments: parseFloat(it.Stats.Comments),
		}
	}
	return out
}

func parseFloat(s string) float64 {
	var f float64
	fmt.Sscanf(s, "%f", &f)
	return f
}

// GetTranscript fetches captions for a YouTube video.
func (y *YouTubeSource) GetTranscript(ctx context.Context, videoID string) (string, error) {
	params := url.Values{
		"part":    {"snippet"},
		"videoId": {videoID},
		"key":     {y.apiKey},
	}
	u := "https://www.googleapis.com/youtube/v3/captions?" + params.Encode()
	data, err := y.client.Get(ctx, u, nil)
	if err != nil { return "", err }

	var resp struct {
		Items []struct {
			ID      string `json:"id"`
			Snippet struct {
				Language string `json:"language"`
			} `json:"snippet"`
		} `json:"items"`
	}
	json.Unmarshal(data, &resp)

	if len(resp.Items) == 0 { return "", fmt.Errorf("no captions available") }

	// Find English caption
	captionID := resp.Items[0].ID
	for _, c := range resp.Items {
		if c.Snippet.Language == "en" { captionID = c.ID; break }
	}

	// Download caption
	cu := fmt.Sprintf("https://www.googleapis.com/youtube/v3/captions/%s?key=%s&tfmt=srt", captionID, y.apiKey)
	caption, err := y.client.Get(ctx, cu, map[string]string{"Authorization": "Bearer " + y.apiKey})
	if err != nil { return "", err }
	return string(caption), nil
}
