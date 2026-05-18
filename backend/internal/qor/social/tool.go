// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package social

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/qorvenai/qorven/internal/tools"
)

// tool.go — Qorven-Social tool for agent use.

type SocialTool struct {
	store     *Store
	publisher *Publisher
}

func NewSocialTool(store *Store) *SocialTool {
	return &SocialTool{store: store, publisher: NewPublisher()}
}

func (t *SocialTool) Name() string { return "qorven_social" }
func (t *SocialTool) Description() string {
	return "Manage social media: create posts, schedule publishing, list integrations, view analytics. Supports Twitter, LinkedIn, Instagram, Facebook, Reddit."
}

func (t *SocialTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type": "string",
				"enum": []string{"create_post", "schedule_post", "publish_now", "list_posts", "delete_post", "list_integrations", "list_autoposts", "create_autopost"},
			},
			"content":    map[string]any{"type": "string", "description": "Post content text"},
			"platforms":  map[string]any{"type": "string", "description": "Comma-separated: twitter,linkedin,facebook"},
			"schedule":   map[string]any{"type": "string", "description": "ISO datetime for scheduling (e.g. 2026-04-10T09:00:00Z)"},
			"post_id":    map[string]any{"type": "string", "description": "Post ID for publish/delete"},
			"status":     map[string]any{"type": "string", "description": "Filter: draft, scheduled, published"},
			"name":       map[string]any{"type": "string", "description": "AutoPost rule name"},
			"source_url": map[string]any{"type": "string", "description": "RSS/webhook URL for autopost"},
			"cron":       map[string]any{"type": "string", "description": "Cron expression for autopost schedule"},
		},
		"required": []string{"action"},
	}
}

func (t *SocialTool) Execute(ctx context.Context, args map[string]any) *tools.Result {
	action, _ := args["action"].(string)
	agentID, _ := ctx.Value("agent_id").(string)
	if agentID == "" {
		agentID = "default"
	}

	switch action {
	case "create_post":
		content, _ := args["content"].(string)
		if content == "" { return tools.ErrorResult("content required") }
		platforms := parsePlatforms(args)
		post := &Post{Content: content, Platforms: platforms, AgentID: agentID, Status: PostDraft}
		id, err := t.store.CreatePost(ctx, post)
		if err != nil { return tools.ErrorResult("create failed: " + err.Error()) }
		return tools.SuccessResult(fmt.Sprintf("📝 Post created (draft): %s\nPlatforms: %v\nUse publish_now to send it.", id, platforms))

	case "schedule_post":
		content, _ := args["content"].(string)
		schedule, _ := args["schedule"].(string)
		if content == "" || schedule == "" { return tools.ErrorResult("content and schedule required") }
		platforms := parsePlatforms(args)
		schedTime, parseErr := parseScheduleTime(schedule)
		if parseErr != nil { return tools.ErrorResult("invalid schedule time: " + parseErr.Error() + ". Use ISO 8601 format: 2026-04-20T09:00:00Z") }
		if schedTime.Before(time.Now()) { return tools.ErrorResult("schedule time is in the past") }
		post := &Post{Content: content, Platforms: platforms, AgentID: agentID, Status: PostScheduled, ScheduledAt: &schedTime}
		id, err := t.store.CreatePost(ctx, post)
		if err != nil { return tools.ErrorResult("schedule failed: " + err.Error()) }
		return tools.SuccessResult(fmt.Sprintf("📅 Post scheduled: %s for %s\nPlatforms: %v\nThe post will be automatically published at the scheduled time.", id, schedTime.Format(time.RFC3339), platforms))

	case "publish_now":
		postID, _ := args["post_id"].(string)
		if postID == "" { return tools.ErrorResult("post_id required") }
		post, err := t.store.GetPost(ctx, postID)
		if err != nil { return tools.ErrorResult("post not found: " + err.Error()) }
		results := t.publisher.PublishToAll(ctx, t.store, post)
		t.store.MarkPublished(ctx, postID)
		data, _ := json.MarshalIndent(results, "", "  ")
		return tools.SuccessResult(fmt.Sprintf("🚀 Published to %d platforms:\n%s", len(results), string(data)))

	case "list_posts":
		status := PostStatus(strVal(args, "status"))
		posts, err := t.store.ListPosts(ctx, agentID, status, 20, 0)
		if err != nil { return tools.ErrorResult(err.Error()) }
		if len(posts) == 0 { return tools.SuccessResult("No posts found.") }
		var sb strings.Builder
		for _, p := range posts {
			sb.WriteString(fmt.Sprintf("• [%s] %s — %s (%v)\n", p.Status, p.ID[:8], truncate(p.Content, 60), p.Platforms))
		}
		return tools.SuccessResult(sb.String())

	case "delete_post":
		postID, _ := args["post_id"].(string)
		if postID == "" { return tools.ErrorResult("post_id required") }
		if err := t.store.DeletePost(ctx, postID); err != nil { return tools.ErrorResult(err.Error()) }
		return tools.SuccessResult("🗑️ Post deleted: " + postID)

	case "list_integrations":
		integrations, err := t.store.ListIntegrations(ctx, agentID)
		if err != nil { return tools.ErrorResult(err.Error()) }
		if len(integrations) == 0 { return tools.SuccessResult("No social media accounts connected.") }
		var sb strings.Builder
		for _, i := range integrations {
			status := "✅"
			if !i.Active { status = "❌" }
			sb.WriteString(fmt.Sprintf("%s %s: %s (@%s)\n", status, i.Platform, i.AccountName, i.AccountID))
		}
		return tools.SuccessResult(sb.String())

	case "list_autoposts":
		autoposts, err := t.store.ListAutoPosts(ctx, agentID)
		if err != nil { return tools.ErrorResult(err.Error()) }
		if len(autoposts) == 0 { return tools.SuccessResult("No autopost rules configured.") }
		var sb strings.Builder
		for _, a := range autoposts {
			status := "🟢"
			if !a.Active { status = "⏸️" }
			sb.WriteString(fmt.Sprintf("%s %s — %s (%s) → %v\n", status, a.Name, a.Source, a.Schedule, a.Platforms))
		}
		return tools.SuccessResult(sb.String())

	case "create_autopost":
		name, _ := args["name"].(string)
		sourceURL, _ := args["source_url"].(string)
		cron, _ := args["cron"].(string)
		if name == "" { return tools.ErrorResult("name required") }
		platforms := parsePlatforms(args)
		source := "manual"
		if sourceURL != "" { source = "rss" }
		ap := AutoPost{Name: name, Source: source, SourceURL: sourceURL, Platforms: platforms, Schedule: cron, Active: true, AgentID: agentID}
		id, err := t.store.CreateAutoPost(ctx, ap)
		if err != nil { return tools.ErrorResult(err.Error()) }
		return tools.SuccessResult(fmt.Sprintf("🤖 AutoPost created: %s (%s → %v every %s)", id, source, platforms, cron))

	default:
		return tools.ErrorResult("unknown action: " + action)
	}
}

func parsePlatforms(args map[string]any) []Platform {
	raw, _ := args["platforms"].(string)
	if raw == "" { return []Platform{PlatformTwitter} }
	var platforms []Platform
	for _, p := range strings.Split(raw, ",") {
		platforms = append(platforms, Platform(strings.TrimSpace(p)))
	}
	return platforms
}

func strVal(m map[string]any, key string) string { v, _ := m[key].(string); return v }

// parseScheduleTime tries multiple datetime formats for human flexibility.
func parseScheduleTime(s string) (time.Time, error) {
	formats := []string{
		time.RFC3339,
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
		"2006-01-02",
		"Jan 2, 2006 3:04 PM",
		"Jan 2 2006 15:04",
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("cannot parse %q", s)
}

