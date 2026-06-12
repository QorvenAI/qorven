// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"text/template"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/qorvenai/qorven/internal/realtime"
	socialqor "github.com/qorvenai/qorven/internal/qor/social"
)

func (gw *Gateway) socialStore() *socialqor.Store {
	if gw.db == nil { return nil }
	return socialqor.NewStore(gw.db.Pool)
}

func (gw *Gateway) socialRelayRouter() *socialqor.RelayRouter {
	return socialqor.NewRelayRouter(gw.socialRelayStore, gw.decryptIntegrationKey)
}

// startSocialAnalyticsWorker launches the background analytics polling goroutine.
func (gw *Gateway) startSocialAnalyticsWorker(ctx context.Context) {
	store := gw.socialStore()
	if store == nil {
		return
	}
	worker := socialqor.NewAnalyticsWorker(store, func(ctx context.Context, agentID string, platform socialqor.Platform) (string, error) {
		token, _, err := store.GetIntegrationToken(ctx, agentID, platform)
		if err != nil {
			return "", err
		}
		if plain, decErr := gw.decryptIntegrationKey(token); decErr == nil && plain != "" {
			return plain, nil
		}
		return token, nil
	})
	worker.Start(ctx)
}

// startSocialTokenRefreshWorker launches the background token refresh goroutine.
func (gw *Gateway) startSocialTokenRefreshWorker(ctx context.Context) {
	store := gw.socialStore()
	if store == nil {
		return
	}
	worker := socialqor.NewTokenRefreshWorker(
		store,
		gw.encryptIntegrationKey,
		gw.decryptIntegrationKey,
		func(platform socialqor.Platform) (string, string) {
			return gw.socialOAuthCreds(string(platform))
		},
	)
	worker.Start(ctx)
}

// startScheduledPostDispatcher checks every minute for posts whose scheduled_at
// has passed and publishes them via the normal publisher pipeline.
func (gw *Gateway) startScheduledPostDispatcher(ctx context.Context) {
	store := gw.socialStore()
	if store == nil {
		return
	}
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			gw.dispatchDuePosts(ctx, store)
		}
	}
}

func (gw *Gateway) dispatchDuePosts(ctx context.Context, store *socialqor.Store) {
	posts, err := store.ListScheduledDue(ctx)
	if err != nil {
		slog.Warn("social.scheduled_dispatch: list failed", "error", err)
		return
	}
	now := time.Now()
	for i := range posts {
		post := &posts[i]
		// Filter out platforms whose integration has posting-hour or posting-day
		// restrictions that exclude the current time, or that are paused.
		// Platforms without a matching integration (e.g. token-based) are always allowed.
		allowedPlatforms, skipAll := store.FilterAllowedPlatforms(ctx, post.AgentID, post.Platforms, now)
		if skipAll {
			// All target platforms are outside their allowed posting window — leave
			// the post scheduled; the next tick will retry when a window opens.
			slog.Info("social.scheduled_dispatch.deferred", "post", post.ID, "reason", "outside posting window")
			continue
		}
		publishPost := *post
		publishPost.Platforms = allowedPlatforms

		results := socialqor.NewPublisher().PublishToAllVia(ctx, store, gw.socialRelayRouter(), &publishPost)
		allOK := true
		platformIDs := map[string]string{}
		for _, res := range results {
			if !res.Success {
				allOK = false
				slog.Warn("social.scheduled_dispatch.publish_failed", "post", post.ID, "platform", res.Platform, "error", res.Error)
			} else {
				slog.Info("social.scheduled_dispatch.published", "post", post.ID, "platform", res.Platform, "url", res.PostURL)
				if res.PostID != "" {
					platformIDs[string(res.Platform)] = res.PostID
				}
			}
		}
		if allOK {
			store.MarkPublished(ctx, post.ID)
		} else {
			store.UpdatePostStatus(ctx, post.ID, socialqor.PostFailed)
		}
		if len(platformIDs) > 0 {
			store.StorePlatformPostIDs(ctx, post.ID, platformIDs)
		}
		// In-app notification
		gw.emitSocialPublishNotification(post.AgentID, allOK, results)
		// Outgoing webhooks
		webhookEvent := "post.published"
		if !allOK {
			webhookEvent = "post.failed"
		}
		gw.fireSocialWebhooks(ctx, post.AgentID, "", webhookEvent, SocialWebhookPayload{
			Event:     webhookEvent,
			Timestamp: now,
			AgentID:   post.AgentID,
			Post:      map[string]any{"id": post.ID, "content": post.Content, "platforms": post.Platforms},
			Results:   results,
		})
		// Broadcast to web UI
		if gw.rtHub != nil {
			gw.rtHub.Broadcast(realtime.Event{
				Type: "social_published",
				Data: map[string]any{"post_id": post.ID, "results": results},
			})
		}
		slog.Info("social.scheduled_dispatch: published", "post_id", post.ID, "ok", allOK)
	}
}

// startAutoPostWorker evaluates active autopost rules on their cron schedule,
// fetches RSS content when applicable, and publishes generated posts.
func (gw *Gateway) startAutoPostWorker(ctx context.Context) {
	store := gw.socialStore()
	if store == nil {
		return
	}
	rssReader := socialqor.NewRSSReader(nil)
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	// Track last fire time per autopost rule to avoid double-firing.
	lastFired := map[string]time.Time{}

	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			rules, err := store.ListAutoPosts(ctx, "")
			if err != nil {
				slog.Warn("social.autopost: list failed", "error", err)
				continue
			}
			for _, rule := range rules {
				if !rule.Active || rule.Schedule == "" {
					continue
				}
				if !cronMatches(rule.Schedule, now) {
					continue
				}
				// Debounce: only fire once per minute window.
				if last, ok := lastFired[rule.ID]; ok && now.Sub(last) < 55*time.Second {
					continue
				}
				lastFired[rule.ID] = now
				go gw.fireAutoPost(ctx, store, rssReader, rule)
			}
		}
	}
}

// cronMatches returns true if t matches the cron expression (minute precision).
// Supports 5-field standard cron: min hour dom month dow.
func cronMatches(expr string, t time.Time) bool {
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return false
	}
	checks := []struct{ field string; val, max int }{
		{fields[0], t.Minute(), 59},
		{fields[1], t.Hour(), 23},
		{fields[2], t.Day(), 31},
		{fields[3], int(t.Month()), 12},
		{fields[4], int(t.Weekday()), 6},
	}
	for _, c := range checks {
		if !cronFieldMatches(c.field, c.val, c.max) {
			return false
		}
	}
	return true
}

func cronFieldMatches(field string, val, max int) bool {
	if field == "*" {
		return true
	}
	// Handle */n step syntax
	if strings.HasPrefix(field, "*/") {
		step := 0
		fmt.Sscanf(field[2:], "%d", &step)
		return step > 0 && val%step == 0
	}
	// Handle comma-separated list
	for _, part := range strings.Split(field, ",") {
		// Handle range a-b
		if strings.Contains(part, "-") {
			var lo, hi int
			fmt.Sscanf(part, "%d-%d", &lo, &hi)
			if val >= lo && val <= hi {
				return true
			}
			continue
		}
		var n int
		fmt.Sscanf(part, "%d", &n)
		if n == val {
			return true
		}
	}
	return false
}

func (gw *Gateway) fireAutoPost(ctx context.Context, store *socialqor.Store, rssReader *socialqor.RSSReader, rule socialqor.AutoPost) {
	var content string

	switch rule.Source {
	case "rss":
		if rule.SourceURL == "" {
			return
		}
		items, err := rssReader.ReadFeed(ctx, rule.SourceURL, 1)
		if err != nil || len(items) == 0 {
			slog.Warn("social.autopost: rss fetch failed", "rule", rule.ID, "url", rule.SourceURL, "error", err)
			return
		}
		item := items[0]
		if rule.Template != "" {
			tmpl, err := template.New("").Parse(rule.Template)
			if err == nil {
				var b strings.Builder
				tmpl.Execute(&b, map[string]any{
					"Title":   item.Title,
					"Content": item.Content,
					"URL":     item.URL,
					"Author":  item.Author,
				})
				content = b.String()
			}
		}
		if content == "" {
			content = item.Title
			if item.URL != "" {
				content += "\n" + item.URL
			}
		}
	default:
		// manual or webhook source — content must be provided in the template
		if rule.Template == "" {
			return
		}
		content = rule.Template
	}

	if content == "" {
		return
	}

	post := socialqor.Post{
		Content:   content,
		Platforms: rule.Platforms,
		AgentID:   rule.AgentID,
		Status:    socialqor.PostDraft,
	}
	id, err := store.CreatePost(ctx, &post)
	if err != nil {
		slog.Warn("social.autopost: create post failed", "rule", rule.ID, "error", err)
		return
	}
	post.ID = id

	// Autopost content is agent-authored — route it through the CMO approval
	// gate. If the rule's agent requires approval, hold the post
	// pending_approval (the dispatcher publishes it once approved) rather than
	// posting unreviewed on the cron.
	if post.DepartmentID == "" {
		if dept, _ := store.ResolveMarketingDepartment(ctx, defaultTenant); dept != "" {
			post.DepartmentID = dept
			store.SetPostDepartment(ctx, post.ID, dept)
		}
	}
	if status := gw.applySocialApprovalGate(ctx, &post, false); status == socialqor.PostPendingApproval {
		store.UpdatePostStatus(ctx, post.ID, socialqor.PostPendingApproval)
		store.SetApprovalStatus(ctx, post.ID, "pending")
		slog.Info("social.autopost: held for CMO approval", "rule", rule.ID, "post_id", post.ID)
		return
	}

	results := socialqor.NewPublisher().PublishToAllVia(ctx, store, gw.socialRelayRouter(), &post)
	allOK := true
	for _, res := range results {
		if !res.Success {
			allOK = false
		}
	}
	if allOK {
		store.MarkPublished(ctx, post.ID)
	} else {
		store.UpdatePostStatus(ctx, post.ID, socialqor.PostFailed)
	}
	gw.emitSocialPublishNotification(rule.AgentID, allOK, results)
	slog.Info("social.autopost: fired", "rule", rule.ID, "post_id", post.ID, "ok", allOK)
}

func (gw *Gateway) handleListSocialPosts(w http.ResponseWriter, r *http.Request) {
	store := gw.socialStore()
	if store == nil { writeJSON(w, 503, map[string]string{"error": "database not configured"}); return }
	agentID := r.URL.Query().Get("agent_id")
	status := socialqor.PostStatus(r.URL.Query().Get("status"))
	posts, err := store.ListPosts(r.Context(), agentID, status, 50, 0)
	if err != nil { writeJSON(w, 500, map[string]string{"error": err.Error()}); return }
	writeJSON(w, 200, posts)
}

func (gw *Gateway) handleCreateSocialPost(w http.ResponseWriter, r *http.Request) {
	store := gw.socialStore()
	if store == nil { writeJSON(w, 503, map[string]string{"error": "database not configured"}); return }
	var post socialqor.Post
	if json.NewDecoder(r.Body).Decode(&post) != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid JSON"})
		return
	}
	if post.Content == "" { writeJSON(w, 400, map[string]string{"error": "content required"}); return }
	if post.Status == "" { post.Status = socialqor.PostDraft }
	id, err := store.CreatePost(r.Context(), &post)
	if err != nil { writeJSON(w, 500, map[string]string{"error": err.Error()}); return }
	post.ID = id

	// Gate: only scheduled posts (not drafts) go through the approval gate.
	// Drafts stay at PostDraft status — no gate, no department assignment needed yet.
	if post.Status == socialqor.PostScheduled {
		// Default department to Marketing so the CMO inbox can filter by dept.
		if post.DepartmentID == "" {
			if dept, _ := store.ResolveMarketingDepartment(r.Context(), defaultTenant); dept != "" {
				post.DepartmentID = dept
				store.SetPostDepartment(r.Context(), id, dept)
			}
		}
		u := userFromContext(r.Context())
		humanAdmin := u != nil && u.Role == "admin"
		status := gw.applySocialApprovalGate(r.Context(), &post, humanAdmin)
		store.UpdatePostStatus(r.Context(), id, status)
		if status == socialqor.PostPendingApproval {
			store.SetApprovalStatus(r.Context(), id, "pending")
		}
		post.Status = status
	}

	// Fire post.scheduled webhook if the post is being (or remained) scheduled
	if post.Status == socialqor.PostScheduled {
		user := userFromContext(r.Context())
		if user != nil {
			gw.fireSocialWebhooks(r.Context(), post.AgentID, user.TenantID, "post.scheduled", SocialWebhookPayload{
				Event:     "post.scheduled",
				Timestamp: time.Now(),
				AgentID:   post.AgentID,
				Post:      map[string]any{"id": id, "content": post.Content, "platforms": post.Platforms, "scheduled_at": post.ScheduledAt},
			})
		}
	}

	writeJSON(w, 201, post)
}

func (gw *Gateway) handleGetSocialPost(w http.ResponseWriter, r *http.Request) {
	store := gw.socialStore()
	if store == nil { writeJSON(w, 503, map[string]string{"error": "database not configured"}); return }
	post, err := store.GetPost(r.Context(), chi.URLParam(r, "id"))
	if err != nil { writeJSON(w, 404, map[string]string{"error": "not found"}); return }
	writeJSON(w, 200, post)
}

func (gw *Gateway) handleDeleteSocialPost(w http.ResponseWriter, r *http.Request) {
	store := gw.socialStore()
	if store == nil { writeJSON(w, 503, map[string]string{"error": "database not configured"}); return }
	postID := chi.URLParam(r, "id")
	// Capture post info before deletion so webhook payload is meaningful
	post, _ := store.GetPost(r.Context(), postID)
	store.DeletePost(r.Context(), postID)

	// Fire post.deleted webhook
	if post != nil {
		user := userFromContext(r.Context())
		if user != nil {
			gw.fireSocialWebhooks(r.Context(), post.AgentID, user.TenantID, "post.deleted", SocialWebhookPayload{
				Event:     "post.deleted",
				Timestamp: time.Now(),
				AgentID:   post.AgentID,
				Post:      map[string]any{"id": postID, "content": post.Content, "platforms": post.Platforms},
			})
		}
	}
	writeJSON(w, 200, map[string]string{"status": "deleted"})
}

func (gw *Gateway) handlePublishSocialPost(w http.ResponseWriter, r *http.Request) {
	store := gw.socialStore()
	if store == nil { writeJSON(w, 503, map[string]string{"error": "database not configured"}); return }
	postID := chi.URLParam(r, "id")
	post, err := store.GetPost(r.Context(), postID)
	if err != nil { writeJSON(w, 404, map[string]string{"error": "post not found"}); return }
	results := socialqor.NewPublisher().PublishToAllVia(r.Context(), store, gw.socialRelayRouter(), post)
	allOK := true
	platformIDs := map[string]string{}
	for _, res := range results {
		if !res.Success { allOK = false }
		if res.PostID != "" {
			platformIDs[string(res.Platform)] = res.PostID
		}
	}
	if allOK {
		store.MarkPublished(r.Context(), postID)
	} else {
		store.UpdatePostStatus(r.Context(), postID, socialqor.PostFailed)
	}
	// Store platform post IDs for analytics polling (best-effort)
	if len(platformIDs) > 0 {
		store.StorePlatformPostIDs(r.Context(), postID, platformIDs)
	}
	// Emit in-app notification
	gw.emitSocialPublishNotification(post.AgentID, allOK, results)

	// Fire outgoing webhooks (async, best-effort)
	user := userFromContext(r.Context())
	if user != nil {
		event := "post.published"
		if !allOK {
			event = "post.failed"
		}
		webhookPayload := SocialWebhookPayload{
			Event:     event,
			Timestamp: time.Now(),
			AgentID:   post.AgentID,
			Post:      map[string]any{"id": postID, "content": post.Content, "platforms": post.Platforms},
			Results:   results,
		}
		gw.fireSocialWebhooks(r.Context(), post.AgentID, user.TenantID, event, webhookPayload)
	}

	writeJSON(w, 200, map[string]any{"results": results})
}

// handleGetSocialPostMetrics returns the latest engagement metrics for a post.
func (gw *Gateway) handleGetSocialPostMetrics(w http.ResponseWriter, r *http.Request) {
	store := gw.socialStore()
	if store == nil { writeJSON(w, 503, map[string]string{"error": "database not configured"}); return }
	metrics, err := store.GetMetrics(r.Context(), chi.URLParam(r, "id"))
	if err != nil { writeJSON(w, 500, map[string]string{"error": err.Error()}); return }
	writeJSON(w, 200, metrics)
}

// handleSocialAnalyticsSummary returns aggregated metrics for an agent across all platforms.
func (gw *Gateway) handleSocialAnalyticsSummary(w http.ResponseWriter, r *http.Request) {
	store := gw.socialStore()
	if store == nil { writeJSON(w, 503, map[string]string{"error": "database not configured"}); return }
	agentID := r.URL.Query().Get("agent_id")
	days := 30
	since := time.Now().AddDate(0, 0, -days)
	byPlatform, err := store.GetAggregateMetrics(r.Context(), agentID, since)
	if err != nil { writeJSON(w, 500, map[string]string{"error": err.Error()}); return }
	topPosts, _ := store.GetTopPosts(r.Context(), agentID, 10, since)
	writeJSON(w, 200, map[string]any{
		"by_platform": byPlatform,
		"top_posts":   topPosts,
		"days":        days,
	})
}

func (gw *Gateway) handleListSocialIntegrations(w http.ResponseWriter, r *http.Request) {
	store := gw.socialStore()
	if store == nil { writeJSON(w, 503, map[string]string{"error": "database not configured"}); return }
	agentID := r.URL.Query().Get("agent_id")
	integrations, err := store.ListIntegrations(r.Context(), agentID)
	if err != nil { writeJSON(w, 500, map[string]string{"error": err.Error()}); return }
	writeJSON(w, 200, integrations)
}

func (gw *Gateway) handleSaveSocialIntegration(w http.ResponseWriter, r *http.Request) {
	store := gw.socialStore()
	if store == nil { writeJSON(w, 503, map[string]string{"error": "database not configured"}); return }
	var integration socialqor.Integration
	if json.NewDecoder(r.Body).Decode(&integration) != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid JSON"})
		return
	}
	id, err := store.SaveIntegration(r.Context(), integration)
	if err != nil { writeJSON(w, 500, map[string]string{"error": err.Error()}); return }
	writeJSON(w, 201, map[string]string{"id": id})
}

func (gw *Gateway) handleDeleteSocialIntegration(w http.ResponseWriter, r *http.Request) {
	store := gw.socialStore()
	if store == nil { writeJSON(w, 503, map[string]string{"error": "database not configured"}); return }
	store.DeleteIntegration(r.Context(), chi.URLParam(r, "id"))
	writeJSON(w, 200, map[string]string{"status": "deleted"})
}

func (gw *Gateway) handleUpdateSocialIntegrationSettings(w http.ResponseWriter, r *http.Request) {
	store := gw.socialStore()
	if store == nil { writeJSON(w, 503, map[string]string{"error": "database not configured"}); return }
	integrationID := chi.URLParam(r, "id")
	var body struct {
		Nickname   string `json:"nickname"`
		AvatarURL  string `json:"avatar_url"`
		GroupName  string `json:"group_name"`
		PostHours  []int  `json:"post_hours"`
		PostDays   []int  `json:"post_days"`
		Paused     bool   `json:"paused"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid JSON"})
		return
	}
	if err := store.UpdateIntegrationSettings(r.Context(), integrationID, body.Nickname, body.AvatarURL, body.GroupName, body.PostHours, body.PostDays, body.Paused); err != nil {
		writeJSON(w, 500, map[string]string{"error": sanitizeError(err)})
		return
	}
	writeJSON(w, 200, map[string]string{"status": "updated"})
}

func (gw *Gateway) handleListSocialAutoPosts(w http.ResponseWriter, r *http.Request) {
	store := gw.socialStore()
	if store == nil { writeJSON(w, 503, map[string]string{"error": "database not configured"}); return }
	agentID := r.URL.Query().Get("agent_id")
	autoposts, err := store.ListAutoPosts(r.Context(), agentID)
	if err != nil { writeJSON(w, 500, map[string]string{"error": err.Error()}); return }
	writeJSON(w, 200, autoposts)
}

func (gw *Gateway) handleCreateSocialAutoPost(w http.ResponseWriter, r *http.Request) {
	store := gw.socialStore()
	if store == nil { writeJSON(w, 503, map[string]string{"error": "database not configured"}); return }
	var autopost socialqor.AutoPost
	if json.NewDecoder(r.Body).Decode(&autopost) != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid JSON"})
		return
	}
	id, err := store.CreateAutoPost(r.Context(), autopost)
	if err != nil { writeJSON(w, 500, map[string]string{"error": err.Error()}); return }
	writeJSON(w, 201, map[string]string{"id": id})
}

func (gw *Gateway) handleDeleteSocialAutoPost(w http.ResponseWriter, r *http.Request) {
	store := gw.socialStore()
	if store == nil { writeJSON(w, 503, map[string]string{"error": "database not configured"}); return }
	store.DeleteAutoPost(r.Context(), chi.URLParam(r, "id"))
	writeJSON(w, 200, map[string]string{"status": "deleted"})
}

func (gw *Gateway) handleToggleSocialAutoPost(w http.ResponseWriter, r *http.Request) {
	store := gw.socialStore()
	if store == nil { writeJSON(w, 503, map[string]string{"error": "database not configured"}); return }
	var body struct{ Active bool `json:"active"` }
	json.NewDecoder(r.Body).Decode(&body)
	if err := store.ToggleAutoPost(r.Context(), chi.URLParam(r, "id"), body.Active); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

// handleSocialCalendar returns posts grouped by date for the content calendar view.
func (gw *Gateway) handleSocialCalendar(w http.ResponseWriter, r *http.Request) {
	store := gw.socialStore()
	if store == nil { writeJSON(w, 503, map[string]string{"error": "database not configured"}); return }
	agentID := r.URL.Query().Get("agent_id")

	// Get all non-deleted posts for the calendar
	scheduled, _ := store.ListPosts(r.Context(), agentID, socialqor.PostScheduled, 100, 0)
	published, _ := store.ListPosts(r.Context(), agentID, socialqor.PostPublished, 100, 0)
	drafts, _ := store.ListPosts(r.Context(), agentID, socialqor.PostDraft, 50, 0)

	// Group by date
	type calendarEntry struct {
		Date  string            `json:"date"` // YYYY-MM-DD
		Posts []socialqor.Post  `json:"posts"`
	}
	byDate := map[string][]socialqor.Post{}
	for _, post := range append(append(scheduled, published...), drafts...) {
		date := post.CreatedAt.Format("2006-01-02")
		if post.ScheduledAt != nil {
			date = post.ScheduledAt.Format("2006-01-02")
		}
		if post.PublishedAt != nil {
			date = post.PublishedAt.Format("2006-01-02")
		}
		byDate[date] = append(byDate[date], post)
	}

	// Sort and return
	entries := []calendarEntry{}
	for date, posts := range byDate {
		entries = append(entries, calendarEntry{Date: date, Posts: posts})
	}

	writeJSON(w, 200, map[string]any{
		"entries": entries,
		"total": len(scheduled) + len(published) + len(drafts),
		"stats": map[string]int{
			"scheduled": len(scheduled),
			"published": len(published),
			"drafts":    len(drafts),
		},
	})
}

// emitSocialPublishNotification fires an in-app notification after a publish attempt.
func (gw *Gateway) emitSocialPublishNotification(agentID string, allOK bool, results []socialqor.PostResult) {
	var ok, failed []string
	for _, r := range results {
		if r.Success {
			ok = append(ok, string(r.Platform))
		} else {
			failed = append(failed, string(r.Platform))
		}
	}

	var title, highlight, nType string
	if allOK {
		nType = "social_published"
		title = "Post published"
		if len(ok) == 1 {
			highlight = "Published to " + ok[0]
		} else {
			highlight = fmt.Sprintf("Published to %d platforms", len(ok))
		}
	} else if len(ok) == 0 {
		nType = "social_failed"
		title = "Post failed"
		if len(failed) == 1 {
			highlight = "Failed on " + failed[0]
		} else {
			highlight = fmt.Sprintf("Failed on %d platforms", len(failed))
		}
	} else {
		nType = "social_partial"
		title = "Post partially published"
		highlight = fmt.Sprintf("%d succeeded, %d failed", len(ok), len(failed))
	}

	gw.writeNotification(agentID, "", "", nType, title, highlight, "social", "")
}

// Unused import silencer
var _ = time.Now
