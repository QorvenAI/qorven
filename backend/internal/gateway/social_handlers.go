// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	socialqor "github.com/qorvenai/qorven/internal/qor/social"
)

func (gw *Gateway) socialStore() *socialqor.Store {
	if gw.db == nil { return nil }
	return socialqor.NewStore(gw.db.Pool)
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
	store.DeletePost(r.Context(), chi.URLParam(r, "id"))
	writeJSON(w, 200, map[string]string{"status": "deleted"})
}

func (gw *Gateway) handlePublishSocialPost(w http.ResponseWriter, r *http.Request) {
	store := gw.socialStore()
	if store == nil { writeJSON(w, 503, map[string]string{"error": "database not configured"}); return }
	postID := chi.URLParam(r, "id")
	post, err := store.GetPost(r.Context(), postID)
	if err != nil { writeJSON(w, 404, map[string]string{"error": "post not found"}); return }
	publisher := socialqor.NewPublisher()
	results := publisher.PublishToAll(r.Context(), store, post)
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
