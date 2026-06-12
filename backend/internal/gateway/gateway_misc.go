// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/qorvenai/qorven/internal/agent"
	"github.com/qorvenai/qorven/internal/channels"
	"github.com/qorvenai/qorven/internal/memory"
	socialqor "github.com/qorvenai/qorven/internal/qor/social"
	"github.com/qorvenai/qorven/internal/realtime"
	"github.com/qorvenai/qorven/internal/tasks"
	"github.com/qorvenai/qorven/internal/tools"
)

// taskStoreAdapter wraps tasks.Store to satisfy tools.TeamTasksBackend.
type taskStoreAdapter struct{ store *tasks.Store }

func (a *taskStoreAdapter) ListAll(ctx context.Context, tenantID, status string, limit int) ([]tools.TeamTaskRow, error) {
	rows, err := a.store.ListAll(ctx, tenantID, status, limit)
	if err != nil {
		return nil, err
	}
	out := make([]tools.TeamTaskRow, len(rows))
	for i, r := range rows {
		out[i] = tools.TeamTaskRow{ID: r.ID, Title: r.Title, Status: r.Status, AssignedTo: r.AssignedTo}
	}
	return out, nil
}

func (a *taskStoreAdapter) CreateTask(ctx context.Context, tenantID, title string) (string, error) {
	return a.store.Create(ctx, tenantID, tasks.Task{Title: title})
}

func (a *taskStoreAdapter) Transition(ctx context.Context, id, newStatus string) error {
	return a.store.Transition(ctx, id, newStatus)
}

func (a *taskStoreAdapter) CompleteTask(ctx context.Context, id, result string) error {
	return a.store.Complete(ctx, id, result, 0)
}

// runtimeMgrAdapter wraps agent.RuntimeManager to satisfy tools.TeamMessageRuntime.
type runtimeMgrAdapter struct{ mgr *agent.RuntimeManager }

func (a *runtimeMgrAdapter) WakeAgentWithMessage(agentID, message string) bool {
	return a.mgr.WakeAgent(agentID, agent.WakeupSignal{
		Source:  agent.WakeupChannelMessage,
		Message: message,
		Context: map[string]any{"type": "team_message", "content": message},
	})
}

// lazyChannelSender wraps the gateway's chanMgr, which is set after the
// TaskCoordinator is created. Using a closure/lazy wrapper avoids a nil pointer
// at the point of wiring (chanMgr is assigned later in gateway init).
type lazyChannelSender struct{ gw *Gateway }

func (l *lazyChannelSender) SendToChannel(ctx context.Context, channelName, chatID, content string) error {
	if l.gw.chanMgr == nil {
		return fmt.Errorf("channel manager not available")
	}
	return l.gw.chanMgr.SendToChannel(ctx, channelName, chatID, content)
}

func (gw *Gateway) getAnnounceMu(sessionKey string) *sync.Mutex {
	v, _ := gw.announceMu.LoadOrStore(sessionKey, &sync.Mutex{})
	return v.(*sync.Mutex)
}

func (gw *Gateway) getEmbeddingURL() string {
	if providers := gw.providerReg.List(); len(providers) > 0 {
		return providers[0].APIBase
	}
	return "https://api.openai.com/v1"
}

func (gw *Gateway) resolveEmbeddingClient() *memory.EmbeddingClient {
	model := "text-embedding-3-small"
	dims := 1536 // pgvector schema requires 1536

	// Try DB providers first
	if gw.providerStore != nil {
		providers, err := gw.providerStore.List(context.Background(), defaultTenant)
		if err == nil {
			for _, p := range providers {
				if !p.Enabled || p.APIKey == "" {
					continue
				}
				apiBase := p.APIBase
				if apiBase == "" {
					apiBase = "https://api.openai.com/v1"
				}
				slog.Info("embedding.resolved", "provider", p.Name, "model", model)
				return memory.NewEmbeddingClient(apiBase, model).WithAPIKey(p.APIKey).WithDimensions(dims)
			}
		}
	}

	// Fallback: use first registered provider's URL (no auth)
	url := gw.getEmbeddingURL()
	return memory.NewEmbeddingClient(url, model).WithDimensions(dims)
}

type chanAdapter struct{ gw *Gateway }

func (a *chanAdapter) List() []map[string]any {
	if a.gw.chanMgr == nil {
		return nil
	}
	return a.gw.chanMgr.List()
}

func (a *chanAdapter) Send(ctx context.Context, instanceID string, msg tools.OutboundMessage) error {
	if a.gw.chanMgr == nil {
		return fmt.Errorf("channel manager not initialized")
	}
	return a.gw.chanMgr.Send(ctx, instanceID, channels.OutboundMessage{RecipientID: msg.RecipientID, Content: msg.Content})
}

func (gw *Gateway) runSocialScheduler(store *socialqor.Store) {
	publisher := socialqor.NewPublisher()
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	slog.Info("social.scheduler.started")

	for range ticker.C {
		ctx := context.Background()
		due, err := store.ListScheduledDue(ctx)
		if err != nil {
			slog.Warn("social.scheduler.list_error", "error", err)
			continue
		}
		now := time.Now()
		for _, post := range due {
			// Filter out platforms whose integration has posting-hour or posting-day restrictions.
			// Platforms without a matching integration (e.g. token-based) are always allowed.
			allowedPlatforms, skipAll := store.FilterAllowedPlatforms(ctx, post.AgentID, post.Platforms, now)
			if skipAll {
				// All platforms are outside their allowed window — leave the post scheduled,
				// it will be picked up again on the next tick when the window opens.
				slog.Info("social.scheduler.deferred", "post", post.ID, "reason", "outside posting window")
				continue
			}
			publishPost := post
			publishPost.Platforms = allowedPlatforms

			results := publisher.PublishToAllVia(ctx, store, gw.socialRelayRouter(), &publishPost)
			allOK := true
			platformIDs := map[string]string{}
			for _, r := range results {
				if !r.Success {
					allOK = false
					slog.Warn("social.scheduler.publish_failed", "post", post.ID, "platform", r.Platform, "error", r.Error)
				} else {
					slog.Info("social.scheduler.published", "post", post.ID, "platform", r.Platform, "url", r.PostURL)
					if r.PostID != "" {
						platformIDs[string(r.Platform)] = r.PostID
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
			event := "post.published"
			if !allOK {
				event = "post.failed"
			}
			gw.fireSocialWebhooks(ctx, post.AgentID, "", event, SocialWebhookPayload{
				Event:     event,
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
		}
	}
}
