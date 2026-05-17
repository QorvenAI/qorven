// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package social

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// publish.go — Publish posts to social media platforms via their APIs.

// Publisher sends posts to connected social platforms.
type Publisher struct {
	client *http.Client
}

func NewPublisher() *Publisher {
	return &Publisher{client: &http.Client{}}
}

// Publish sends a post to a specific platform using the provided access token.
func (p *Publisher) Publish(ctx context.Context, platform Platform, token, content string, mediaURLs []string) (*PostResult, error) {
	switch platform {
	case PlatformTwitter:
		return p.publishTwitter(ctx, token, content)
	case PlatformLinkedIn:
		return p.publishLinkedIn(ctx, token, content)
	case PlatformFacebook:
		return p.publishFacebook(ctx, token, content)
	case PlatformReddit:
		return p.publishReddit(ctx, token, content)
	case PlatformInstagram:
		return p.publishInstagram(ctx, token, content, mediaURLs)
	case PlatformThreads:
		return p.publishThreads(ctx, token, content)
	case PlatformTikTok:
		return p.publishTikTok(ctx, token, content, mediaURLs)
	case PlatformYouTube:
		return p.publishYouTube(ctx, token, content)
	case PlatformBluesky:
		return p.publishBluesky(ctx, token, content)
	case PlatformMastodon:
		return p.publishMastodon(ctx, token, content)
	case PlatformPinterest:
		return p.publishPinterest(ctx, token, content, mediaURLs)
	default:
		return &PostResult{Platform: platform, Success: false, Error: fmt.Sprintf("platform %s not supported yet", platform)}, nil
	}
}

func (p *Publisher) publishTwitter(ctx context.Context, token, content string) (*PostResult, error) {
	body, _ := json.Marshal(map[string]any{"text": content})
	req, _ := http.NewRequestWithContext(ctx, "POST", "https://api.twitter.com/2/tweets", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil { return &PostResult{Platform: PlatformTwitter, Success: false, Error: err.Error()}, nil }
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))

	if resp.StatusCode != 201 {
		return &PostResult{Platform: PlatformTwitter, Success: false, Error: fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(respBody))}, nil
	}

	var result struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	json.Unmarshal(respBody, &result)
	slog.Info("social.publish.twitter", "id", result.Data.ID)
	return &PostResult{Platform: PlatformTwitter, Success: true, PostID: result.Data.ID, PostURL: "https://twitter.com/i/status/" + result.Data.ID}, nil
}

func (p *Publisher) publishLinkedIn(ctx context.Context, token, content string) (*PostResult, error) {
	// Get user URN first
	urn, err := p.linkedInURN(ctx, token)
	if err != nil { return &PostResult{Platform: PlatformLinkedIn, Success: false, Error: err.Error()}, nil }

	body, _ := json.Marshal(map[string]any{
		"author":         urn,
		"lifecycleState": "PUBLISHED",
		"specificContent": map[string]any{
			"com.linkedin.ugc.ShareContent": map[string]any{
				"shareCommentary":   map[string]any{"text": content},
				"shareMediaCategory": "NONE",
			},
		},
		"visibility": map[string]any{"com.linkedin.ugc.MemberNetworkVisibility": "PUBLIC"},
	})

	req, _ := http.NewRequestWithContext(ctx, "POST", "https://api.linkedin.com/v2/ugcPosts", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Restli-Protocol-Version", "2.0.0")

	resp, err := p.client.Do(req)
	if err != nil { return &PostResult{Platform: PlatformLinkedIn, Success: false, Error: err.Error()}, nil }
	defer resp.Body.Close()

	if resp.StatusCode != 201 {
		respBody, _ := io.ReadAll(resp.Body)
		return &PostResult{Platform: PlatformLinkedIn, Success: false, Error: string(respBody)}, nil
	}

	postID := resp.Header.Get("X-RestLi-Id")
	slog.Info("social.publish.linkedin", "id", postID)
	return &PostResult{Platform: PlatformLinkedIn, Success: true, PostID: postID}, nil
}

func (p *Publisher) linkedInURN(ctx context.Context, token string) (string, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", "https://api.linkedin.com/v2/userinfo", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := p.client.Do(req)
	if err != nil { return "", err }
	defer resp.Body.Close()
	var result struct{ Sub string `json:"sub"` }
	json.NewDecoder(resp.Body).Decode(&result)
	return "urn:li:person:" + result.Sub, nil
}

func (p *Publisher) publishFacebook(ctx context.Context, token, content string) (*PostResult, error) {
	url := fmt.Sprintf("https://graph.facebook.com/v18.0/me/feed?message=%s&access_token=%s",
		strings.ReplaceAll(content, " ", "+"), token)
	req, _ := http.NewRequestWithContext(ctx, "POST", url, nil)
	resp, err := p.client.Do(req)
	if err != nil { return &PostResult{Platform: PlatformFacebook, Success: false, Error: err.Error()}, nil }
	defer resp.Body.Close()
	var result struct{ ID string `json:"id"` }
	json.NewDecoder(resp.Body).Decode(&result)
	if result.ID == "" {
		return &PostResult{Platform: PlatformFacebook, Success: false, Error: "no post ID returned"}, nil
	}
	return &PostResult{Platform: PlatformFacebook, Success: true, PostID: result.ID}, nil
}

func (p *Publisher) publishReddit(ctx context.Context, token, content string) (*PostResult, error) {
	return &PostResult{Platform: PlatformReddit, Success: false, Error: "Reddit posting requires subreddit — set subreddit in post metadata"}, nil
}

// ─── Instagram (Graph API) ─────────────────────────────────────────────────
// Requires a connected Facebook Page + Instagram Business Account.
// Text-only posts not supported by Instagram — requires media.
func (p *Publisher) publishInstagram(ctx context.Context, token, content string, mediaURLs []string) (*PostResult, error) {
	if len(mediaURLs) == 0 {
		return &PostResult{Platform: PlatformInstagram, Success: false, Error: "Instagram requires at least one image URL"}, nil
	}

	// Step 1: Create media container
	containerURL := fmt.Sprintf("https://graph.facebook.com/v18.0/me/media?image_url=%s&caption=%s&access_token=%s",
		mediaURLs[0], strings.ReplaceAll(content, " ", "+"), token)
	req, _ := http.NewRequestWithContext(ctx, "POST", containerURL, nil)
	resp, err := p.client.Do(req)
	if err != nil { return &PostResult{Platform: PlatformInstagram, Success: false, Error: err.Error()}, nil }
	defer resp.Body.Close()
	var container struct{ ID string `json:"id"` }
	json.NewDecoder(resp.Body).Decode(&container)
	if container.ID == "" {
		return &PostResult{Platform: PlatformInstagram, Success: false, Error: "failed to create media container"}, nil
	}

	// Step 2: Publish the container
	publishURL := fmt.Sprintf("https://graph.facebook.com/v18.0/me/media_publish?creation_id=%s&access_token=%s",
		container.ID, token)
	req2, _ := http.NewRequestWithContext(ctx, "POST", publishURL, nil)
	resp2, err := p.client.Do(req2)
	if err != nil { return &PostResult{Platform: PlatformInstagram, Success: false, Error: err.Error()}, nil }
	defer resp2.Body.Close()
	var result struct{ ID string `json:"id"` }
	json.NewDecoder(resp2.Body).Decode(&result)

	slog.Info("social.publish.instagram", "id", result.ID)
	return &PostResult{Platform: PlatformInstagram, Success: true, PostID: result.ID}, nil
}

// ─── Threads (Meta Graph API) ──────────────────────────────────────────────
// Uses same Graph API as Instagram but different endpoint.
func (p *Publisher) publishThreads(ctx context.Context, token, content string) (*PostResult, error) {
	// Step 1: Create media container
	body, _ := json.Marshal(map[string]any{
		"media_type": "TEXT",
		"text":       content,
	})
	url := fmt.Sprintf("https://graph.threads.net/v1.0/me/threads?access_token=%s", token)
	req, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.client.Do(req)
	if err != nil { return &PostResult{Platform: PlatformThreads, Success: false, Error: err.Error()}, nil }
	defer resp.Body.Close()
	var container struct{ ID string `json:"id"` }
	json.NewDecoder(resp.Body).Decode(&container)
	if container.ID == "" {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return &PostResult{Platform: PlatformThreads, Success: false, Error: "container failed: " + string(respBody)}, nil
	}

	// Step 2: Publish
	pubURL := fmt.Sprintf("https://graph.threads.net/v1.0/me/threads_publish?creation_id=%s&access_token=%s",
		container.ID, token)
	req2, _ := http.NewRequestWithContext(ctx, "POST", pubURL, nil)
	resp2, err := p.client.Do(req2)
	if err != nil { return &PostResult{Platform: PlatformThreads, Success: false, Error: err.Error()}, nil }
	defer resp2.Body.Close()
	var result struct{ ID string `json:"id"` }
	json.NewDecoder(resp2.Body).Decode(&result)

	slog.Info("social.publish.threads", "id", result.ID)
	return &PostResult{Platform: PlatformThreads, Success: true, PostID: result.ID}, nil
}

// ─── TikTok (Creator API v2) ───────────────────────────────────────────────
// TikTok requires video — text-only posts are not supported.
func (p *Publisher) publishTikTok(ctx context.Context, token, content string, mediaURLs []string) (*PostResult, error) {
	if len(mediaURLs) == 0 {
		return &PostResult{Platform: PlatformTikTok, Success: false, Error: "TikTok requires a video URL"}, nil
	}

	// Step 1: Init upload
	body, _ := json.Marshal(map[string]any{
		"post_info": map[string]any{
			"title":            content,
			"privacy_level":    "PUBLIC_TO_EVERYONE",
			"disable_duet":     false,
			"disable_comment":  false,
			"disable_stitch":   false,
		},
		"source_info": map[string]any{
			"source":    "PULL_FROM_URL",
			"video_url": mediaURLs[0],
		},
	})
	req, _ := http.NewRequestWithContext(ctx, "POST", "https://open.tiktokapis.com/v2/post/publish/video/init/", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json; charset=UTF-8")
	resp, err := p.client.Do(req)
	if err != nil { return &PostResult{Platform: PlatformTikTok, Success: false, Error: err.Error()}, nil }
	defer resp.Body.Close()

	var result struct {
		Data struct{ PublishID string `json:"publish_id"` } `json:"data"`
		Error struct{ Code string `json:"code"`; Message string `json:"message"` } `json:"error"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	if result.Error.Code != "" && result.Error.Code != "ok" {
		return &PostResult{Platform: PlatformTikTok, Success: false, Error: result.Error.Message}, nil
	}

	slog.Info("social.publish.tiktok", "publish_id", result.Data.PublishID)
	return &PostResult{Platform: PlatformTikTok, Success: true, PostID: result.Data.PublishID}, nil
}

// ─── YouTube (Data API v3) ─────────────────────────────────────────────────
// Creates a community post (text). Video upload requires separate flow.
func (p *Publisher) publishYouTube(ctx context.Context, token, content string) (*PostResult, error) {
	// YouTube community post via Data API v3
	body, _ := json.Marshal(map[string]any{
		"snippet": map[string]any{
			"description": content,
		},
	})
	req, _ := http.NewRequestWithContext(ctx, "POST", "https://www.googleapis.com/youtube/v3/activities?part=snippet", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.client.Do(req)
	if err != nil { return &PostResult{Platform: PlatformYouTube, Success: false, Error: err.Error()}, nil }
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return &PostResult{Platform: PlatformYouTube, Success: false, Error: fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(respBody))}, nil
	}

	var result struct{ ID string `json:"id"` }
	json.NewDecoder(resp.Body).Decode(&result)
	slog.Info("social.publish.youtube", "id", result.ID)
	return &PostResult{Platform: PlatformYouTube, Success: true, PostID: result.ID}, nil
}

// ─── Bluesky (AT Protocol / XRPC) ─────────────────────────────────────────
// Token format for Bluesky: "handle:app_password" (separated by colon).
// No OAuth needed — uses direct auth with handle + app password.
func (p *Publisher) publishBluesky(ctx context.Context, token, content string) (*PostResult, error) {
	// Parse handle:password from token
	parts := strings.SplitN(token, ":", 2)
	if len(parts) != 2 {
		return &PostResult{Platform: PlatformBluesky, Success: false, Error: "token must be handle:app_password format"}, nil
	}
	handle, password := parts[0], parts[1]

	// Step 1: Create session (get JWT)
	authBody, _ := json.Marshal(map[string]string{"identifier": handle, "password": password})
	authReq, _ := http.NewRequestWithContext(ctx, "POST", "https://bsky.social/xrpc/com.atproto.server.createSession", bytes.NewReader(authBody))
	authReq.Header.Set("Content-Type", "application/json")
	authResp, err := p.client.Do(authReq)
	if err != nil { return &PostResult{Platform: PlatformBluesky, Success: false, Error: err.Error()}, nil }
	defer authResp.Body.Close()
	var session struct{ DID string `json:"did"`; AccessJwt string `json:"accessJwt"` }
	json.NewDecoder(authResp.Body).Decode(&session)
	if session.AccessJwt == "" {
		return &PostResult{Platform: PlatformBluesky, Success: false, Error: "Bluesky auth failed"}, nil
	}

	// Step 2: Create post record
	now := strings.Replace(strings.Replace(time.Now().UTC().Format(time.RFC3339Nano), " ", "T", 1), " +0000 UTC", "Z", 1)
	postBody, _ := json.Marshal(map[string]any{
		"repo":       session.DID,
		"collection": "app.bsky.feed.post",
		"record": map[string]any{
			"$type":     "app.bsky.feed.post",
			"text":      content,
			"createdAt": now,
		},
	})
	postReq, _ := http.NewRequestWithContext(ctx, "POST", "https://bsky.social/xrpc/com.atproto.repo.createRecord", bytes.NewReader(postBody))
	postReq.Header.Set("Authorization", "Bearer "+session.AccessJwt)
	postReq.Header.Set("Content-Type", "application/json")
	postResp, err := p.client.Do(postReq)
	if err != nil { return &PostResult{Platform: PlatformBluesky, Success: false, Error: err.Error()}, nil }
	defer postResp.Body.Close()

	var result struct{ URI string `json:"uri"`; CID string `json:"cid"` }
	json.NewDecoder(postResp.Body).Decode(&result)
	if result.URI == "" {
		return &PostResult{Platform: PlatformBluesky, Success: false, Error: "post creation failed"}, nil
	}

	postURL := fmt.Sprintf("https://bsky.app/profile/%s/post/%s", handle, strings.Split(result.URI, "/")[len(strings.Split(result.URI, "/"))-1])
	slog.Info("social.publish.bluesky", "uri", result.URI)
	return &PostResult{Platform: PlatformBluesky, Success: true, PostID: result.URI, PostURL: postURL}, nil
}

// ─── Mastodon (ActivityPub) ────────────────────────────────────────────────
// Token format: "instance_url:access_token" (e.g. "mastodon.social:abc123").
// Supports any Mastodon-compatible instance.
func (p *Publisher) publishMastodon(ctx context.Context, token, content string) (*PostResult, error) {
	parts := strings.SplitN(token, ":", 2)
	instance := "mastodon.social"
	accessToken := token
	if len(parts) == 2 && strings.Contains(parts[0], ".") {
		instance = parts[0]
		accessToken = parts[1]
	}

	body, _ := json.Marshal(map[string]any{"status": content, "visibility": "public"})
	apiURL := fmt.Sprintf("https://%s/api/v1/statuses", instance)
	req, _ := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.client.Do(req)
	if err != nil { return &PostResult{Platform: PlatformMastodon, Success: false, Error: err.Error()}, nil }
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return &PostResult{Platform: PlatformMastodon, Success: false, Error: fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(respBody))}, nil
	}

	var result struct{ ID string `json:"id"`; URL string `json:"url"` }
	json.NewDecoder(resp.Body).Decode(&result)
	slog.Info("social.publish.mastodon", "id", result.ID, "instance", instance)
	return &PostResult{Platform: PlatformMastodon, Success: true, PostID: result.ID, PostURL: result.URL}, nil
}

// ─── Pinterest (API v5) ────────────────────────────────────────────────────
// Requires an image — text-only pins not supported.
func (p *Publisher) publishPinterest(ctx context.Context, token, content string, mediaURLs []string) (*PostResult, error) {
	if len(mediaURLs) == 0 {
		return &PostResult{Platform: PlatformPinterest, Success: false, Error: "Pinterest requires an image URL"}, nil
	}

	body, _ := json.Marshal(map[string]any{
		"title":       content[:min(len(content), 100)],
		"description": content,
		"board_id":    "", // must be set via metadata
		"media_source": map[string]any{
			"source_type": "image_url",
			"url":         mediaURLs[0],
		},
	})
	req, _ := http.NewRequestWithContext(ctx, "POST", "https://api.pinterest.com/v5/pins", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.client.Do(req)
	if err != nil { return &PostResult{Platform: PlatformPinterest, Success: false, Error: err.Error()}, nil }
	defer resp.Body.Close()

	if resp.StatusCode != 201 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return &PostResult{Platform: PlatformPinterest, Success: false, Error: fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(respBody))}, nil
	}

	var result struct{ ID string `json:"id"` }
	json.NewDecoder(resp.Body).Decode(&result)
	slog.Info("social.publish.pinterest", "id", result.ID)
	return &PostResult{Platform: PlatformPinterest, Success: true, PostID: result.ID}, nil
}

// PublishToAll publishes a post to all its target platforms.
func (p *Publisher) PublishToAll(ctx context.Context, store *Store, post *Post) []PostResult {
	results := []PostResult{}
	for _, platform := range post.Platforms {
		token, _, err := store.GetIntegrationToken(ctx, post.AgentID, platform)
		if err != nil {
			results = append(results, PostResult{Platform: platform, Success: false, Error: "no integration: " + err.Error()})
			continue
		}
		result, err := p.Publish(ctx, platform, token, post.Content, post.MediaURLs)
		if err != nil {
			results = append(results, PostResult{Platform: platform, Success: false, Error: err.Error()})
		} else {
			results = append(results, *result)
		}
	}
	return results
}
