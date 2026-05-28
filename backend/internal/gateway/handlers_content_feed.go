// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/qorvenai/qorven/internal/connectors"
	socialqor "github.com/qorvenai/qorven/internal/qor/social"
)

// ContentFeedItem is the enriched view of an outbound_queue entry for the content approval feed.
type ContentFeedItem struct {
	ID          string          `json:"id"`
	AgentID     string          `json:"agent_id"`
	AgentName   string          `json:"agent_name"`
	ActionType  string          `json:"action_type"`
	Content     string          `json:"content"`
	Platforms   []string        `json:"platforms,omitempty"`
	Channel     string          `json:"channel,omitempty"`
	Status      string          `json:"status"`
	RequestedAt time.Time       `json:"requested_at"`
	ExpiresAt   time.Time       `json:"expires_at"`
	Metadata    json.RawMessage `json:"metadata,omitempty"`
}

// handleListContentFeed returns the content approval feed with optional filters.
// GET /v1/content-feed?status=pending&channel=&agent_id=&limit=50
func (gw *Gateway) handleListContentFeed(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}

	status := r.URL.Query().Get("status")
	channel := r.URL.Query().Get("channel")
	agentID := r.URL.Query().Get("agent_id")
	limitStr := r.URL.Query().Get("limit")

	limit := 50
	if limitStr != "" {
		if n, err := strconv.Atoi(limitStr); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}

	// Build dynamic query
	query := `SELECT oq.id, oq.agent_id, COALESCE(a.display_name, a.name, ''), oq.action_type,
	                 oq.payload, oq.status, oq.requested_at, oq.expires_at
	          FROM outbound_queue oq
	          LEFT JOIN agents a ON a.id = oq.agent_id
	          WHERE 1=1`
	args := []any{}
	argIdx := 1

	if status != "" {
		query += ` AND oq.status = $` + strconv.Itoa(argIdx)
		args = append(args, status)
		argIdx++
	} else {
		// Default to pending
		query += ` AND oq.status = $` + strconv.Itoa(argIdx)
		args = append(args, "pending")
		argIdx++
	}

	if agentID != "" {
		query += ` AND oq.agent_id = $` + strconv.Itoa(argIdx)
		args = append(args, agentID)
		argIdx++
	}

	if channel != "" {
		query += ` AND oq.action_type = $` + strconv.Itoa(argIdx)
		args = append(args, channel)
		argIdx++
	}

	query += ` ORDER BY oq.requested_at DESC LIMIT $` + strconv.Itoa(argIdx)
	args = append(args, limit)

	rows, err := gw.db.Pool.Query(r.Context(), query, args...)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}
	defer rows.Close()

	items := []ContentFeedItem{}
	for rows.Next() {
		var item ContentFeedItem
		var payload json.RawMessage
		if err := rows.Scan(&item.ID, &item.AgentID, &item.AgentName, &item.ActionType,
			&payload, &item.Status, &item.RequestedAt, &item.ExpiresAt); err != nil {
			continue
		}

		// Parse payload to extract content and platforms for the preview
		var payloadMap map[string]any
		if json.Unmarshal(payload, &payloadMap) == nil {
			if c, ok := payloadMap["content"].(string); ok {
				item.Content = c
			}
			if p, ok := payloadMap["platforms"].([]any); ok {
				for _, v := range p {
					if s, ok := v.(string); ok {
						item.Platforms = append(item.Platforms, s)
					}
				}
			}
			if ch, ok := payloadMap["channel"].(string); ok {
				item.Channel = ch
			}
		}
		item.Metadata = payload
		items = append(items, item)
	}

	writeJSON(w, http.StatusOK, map[string]any{"items": items, "count": len(items)})
}

// handleApproveContent approves a content feed item and publishes it.
// POST /v1/content-feed/{id}/approve
func (gw *Gateway) handleApproveContent(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}

	id := chi.URLParam(r, "id")
	ctx := r.Context()

	// 1. Fetch the outbound_queue entry
	var agentID, actionType string
	var payload json.RawMessage
	var status string
	err := gw.db.Pool.QueryRow(ctx,
		`SELECT agent_id, action_type, payload, status FROM outbound_queue WHERE id = $1`, id,
	).Scan(&agentID, &actionType, &payload, &status)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "content item not found"})
		return
	}
	if status != "pending" {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "item already " + status})
		return
	}

	// 2. Mark as approved
	_, err = gw.db.Pool.Exec(ctx,
		`UPDATE outbound_queue SET status = 'approved', reviewed_by = 'user', reviewed_at = NOW() WHERE id = $1`, id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}

	// 3. Parse payload to extract content + platforms
	var payloadMap map[string]any
	json.Unmarshal(payload, &payloadMap)

	content, _ := payloadMap["content"].(string)

	// Route by action type: social posts vs articles/SEO recommendations
	switch actionType {
	case "article_draft":
		// CMS publishing: try WordPress or Webflow via connector executor
		title, _ := payloadMap["title"].(string)
		if title == "" {
			title = "Untitled Article"
		}
		cmsResult := gw.publishToCMS(ctx, title, content, payloadMap)
		writeJSON(w, http.StatusOK, map[string]any{"status": "approved", "cms": cmsResult})
		return

	case "seo_fix":
		// SEO recommendations are just approved — no auto-publish
		writeJSON(w, http.StatusOK, map[string]any{"status": "approved", "type": "seo_fix"})
		return

	case "reddit_reply":
		// Reddit replies: attempt to post via connector
		subreddit, _ := payloadMap["subreddit"].(string)
		threadTitle, _ := payloadMap["thread_title"].(string)
		replyResult := gw.publishRedditReply(ctx, subreddit, threadTitle, content, payloadMap)
		writeJSON(w, http.StatusOK, map[string]any{"status": "approved", "reddit": replyResult})
		return
	}

	// Default: social post flow
	var platforms []socialqor.Platform
	if p, ok := payloadMap["platforms"].([]any); ok {
		for _, v := range p {
			if s, ok := v.(string); ok {
				platforms = append(platforms, socialqor.Platform(s))
			}
		}
	}

	// 4. Create a social post
	store := gw.socialStore()
	if store == nil {
		writeJSON(w, http.StatusOK, map[string]any{"status": "approved", "note": "social store unavailable, post not created"})
		return
	}

	post := &socialqor.Post{
		Content:   content,
		Platforms: platforms,
		AgentID:   agentID,
		Status:    socialqor.PostDraft,
	}

	// Check if there's a scheduled_at in payload
	if scheduledStr, ok := payloadMap["scheduled_at"].(string); ok && scheduledStr != "" {
		if t, err := time.Parse(time.RFC3339, scheduledStr); err == nil {
			post.ScheduledAt = &t
			post.Status = socialqor.PostScheduled
		}
	}

	postID, err := store.CreatePost(ctx, post)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}

	// 5. If not scheduled, publish immediately
	if post.Status == socialqor.PostDraft {
		publisher := socialqor.NewPublisher()
		results := publisher.PublishToAll(ctx, store, post)
		allOK := true
		for _, res := range results {
			if !res.Success {
				allOK = false
			}
		}
		if allOK {
			store.MarkPublished(ctx, postID)
			writeJSON(w, http.StatusOK, map[string]any{"status": "published", "post_id": postID})
			return
		}
		store.UpdatePostStatus(ctx, postID, socialqor.PostFailed)
		writeJSON(w, http.StatusOK, map[string]any{"status": "publish_failed", "post_id": postID, "results": results})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"status": "scheduled", "post_id": postID})
}

// handleRejectContent rejects a content feed item.
// POST /v1/content-feed/{id}/reject
func (gw *Gateway) handleRejectContent(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}

	id := chi.URLParam(r, "id")
	var body struct {
		Reason string `json:"reason"`
	}
	json.NewDecoder(r.Body).Decode(&body)

	// Verify item exists and is pending
	var status string
	err := gw.db.Pool.QueryRow(r.Context(),
		`SELECT status FROM outbound_queue WHERE id = $1`, id).Scan(&status)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "content item not found"})
		return
	}
	if status != "pending" {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "item already " + status})
		return
	}

	_, err = gw.db.Pool.Exec(r.Context(),
		`UPDATE outbound_queue SET status = 'rejected', review_notes = $1, reviewed_by = 'user', reviewed_at = NOW() WHERE id = $2`,
		body.Reason, id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"status": "rejected"})
}

// handleEditContent allows editing the content of a pending item before approval.
// PUT /v1/content-feed/{id}
func (gw *Gateway) handleEditContent(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}

	id := chi.URLParam(r, "id")
	var body struct {
		Content   string   `json:"content"`
		Platforms []string `json:"platforms,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if body.Content == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "content required"})
		return
	}

	ctx := r.Context()

	// Verify item exists and is pending
	var status string
	var payload json.RawMessage
	err := gw.db.Pool.QueryRow(ctx,
		`SELECT status, payload FROM outbound_queue WHERE id = $1`, id).Scan(&status, &payload)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "content item not found"})
		return
	}
	if status != "pending" {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "can only edit pending items"})
		return
	}

	// Update the payload JSON
	var payloadMap map[string]any
	if json.Unmarshal(payload, &payloadMap) != nil {
		payloadMap = map[string]any{}
	}
	payloadMap["content"] = body.Content
	if len(body.Platforms) > 0 {
		payloadMap["platforms"] = body.Platforms
	}

	updatedPayload, _ := json.Marshal(payloadMap)
	_, err = gw.db.Pool.Exec(ctx,
		`UPDATE outbound_queue SET payload = $1 WHERE id = $2`, updatedPayload, id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"status": "updated", "id": id, "content": body.Content})
}

func (gw *Gateway) connectorExecutor() *connectors.Executor {
	return gw.connExec
}

// publishToCMS attempts to publish an article to the connected CMS (WordPress or Webflow).
func (gw *Gateway) publishToCMS(ctx context.Context, title, content string, payload map[string]any) map[string]any {
	executor := gw.connectorExecutor()
	if executor == nil {
		return map[string]any{"published": false, "reason": "connector executor not available"}
	}

	// Try WordPress first
	wpResult, err := executor.Execute(ctx, "wordpress", "create_post", map[string]any{
		"title":   title,
		"content": content,
		"status":  "draft",
	})
	if err == nil && wpResult != "" {
		return map[string]any{"published": true, "platform": "wordpress", "result": wpResult}
	}

	// Try Webflow
	collectionID, _ := payload["collection_id"].(string)
	if collectionID != "" {
		wfResult, err := executor.Execute(ctx, "webflow", "create_item", map[string]any{
			"collection_id": collectionID,
			"fields": map[string]any{
				"name": title,
				"slug": strings.ToLower(strings.ReplaceAll(title, " ", "-")),
				"body": content,
			},
		})
		if err == nil && wfResult != "" {
			return map[string]any{"published": true, "platform": "webflow", "result": wfResult}
		}
	}

	// Try Contentful
	spaceID, _ := payload["space_id"].(string)
	if spaceID != "" {
		cfResult, err := executor.Execute(ctx, "contentful", "create_entry", map[string]any{
			"space_id":     spaceID,
			"content_type": "blogPost",
			"fields": map[string]any{
				"title": map[string]any{"en-US": title},
				"body":  map[string]any{"en-US": content},
			},
		})
		if err == nil && cfResult != "" {
			return map[string]any{"published": true, "platform": "contentful", "result": cfResult}
		}
	}

	return map[string]any{"published": false, "reason": "no CMS connector available or configured"}
}

// publishRedditReply attempts to post a reply to Reddit via the connector.
func (gw *Gateway) publishRedditReply(ctx context.Context, subreddit, threadTitle, content string, payload map[string]any) map[string]any {
	executor := gw.connectorExecutor()
	if executor == nil {
		return map[string]any{"posted": false, "reason": "connector executor not available"}
	}

	params := map[string]any{
		"sr":    subreddit,
		"title": threadTitle,
		"text":  content,
	}
	if url, ok := payload["url"].(string); ok && url != "" {
		params["url"] = url
	}

	result, err := executor.Execute(ctx, "reddit", "submit_post", params)
	if err != nil {
		return map[string]any{"posted": false, "reason": err.Error()}
	}
	return map[string]any{"posted": true, "result": result}
}

// handleContentFeedStats returns aggregate stats for the content feed.
// GET /v1/content-feed/stats
func (gw *Gateway) handleContentFeedStats(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}

	ctx := r.Context()
	now := time.Now()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	thirtyDaysAgo := now.AddDate(0, 0, -30)

	var pending, approvedToday, rejectedToday, total30d int

	// Pending count
	gw.db.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM outbound_queue WHERE status = 'pending' AND expires_at > NOW()`).Scan(&pending)

	// Approved today
	gw.db.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM outbound_queue WHERE status = 'approved' AND reviewed_at >= $1`, todayStart).Scan(&approvedToday)

	// Rejected today
	gw.db.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM outbound_queue WHERE status = 'rejected' AND reviewed_at >= $1`, todayStart).Scan(&rejectedToday)

	// Total in last 30 days
	gw.db.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM outbound_queue WHERE requested_at >= $1`, thirtyDaysAgo).Scan(&total30d)

	writeJSON(w, http.StatusOK, map[string]any{
		"pending":        pending,
		"approved_today": approvedToday,
		"rejected_today": rejectedToday,
		"total_30d":      total30d,
	})
}
