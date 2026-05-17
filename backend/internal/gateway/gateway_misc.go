// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package gateway

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/qorvenai/qorven/internal/channels"
	"github.com/qorvenai/qorven/internal/memory"
	socialqor "github.com/qorvenai/qorven/internal/qor/social"
	"github.com/qorvenai/qorven/internal/realtime"
	"github.com/qorvenai/qorven/internal/tools"
)

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
		for _, post := range due {
			results := publisher.PublishToAll(ctx, store, &post)
			allOK := true
			for _, r := range results {
				if !r.Success {
					allOK = false
					slog.Warn("social.scheduler.publish_failed", "post", post.ID, "platform", r.Platform, "error", r.Error)
				} else {
					slog.Info("social.scheduler.published", "post", post.ID, "platform", r.Platform, "url", r.PostURL)
				}
			}
			if allOK {
				store.MarkPublished(ctx, post.ID)
			} else {
				store.UpdatePostStatus(ctx, post.ID, socialqor.PostFailed)
			}
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
