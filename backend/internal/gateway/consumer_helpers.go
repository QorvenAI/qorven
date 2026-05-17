// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package gateway

import (
	"fmt"
	"mime"
	"path/filepath"
	"strings"

	"github.com/qorvenai/qorven/internal/channels"
)

// extractSessionMetadata builds a metadata map from channel InboundMessage metadata.
// Persists friendly names (display_name, username, chat_title) into sessions
// so the web UI can show human-readable labels.
func extractSessionMetadata(msg channels.InboundMessage) map[string]string {
	meta := make(map[string]string)

	if v := msg.Metadata["first_name"]; v != "" {
		meta["display_name"] = v
	} else if v := msg.Metadata["display_name"]; v != "" {
		meta["display_name"] = v
	}
	if v := msg.Metadata["username"]; v != "" {
		meta["username"] = v
	}
	if v := msg.Metadata["chat_title"]; v != "" {
		meta["chat_title"] = v
	}
	if msg.ChannelType != "" {
		meta["channel_type"] = msg.ChannelType
	}

	if len(meta) == 0 {
		return nil
	}
	return meta
}

// mediaToMarkdown converts media file paths to markdown image/link syntax
// using the /v1/files/ HTTP endpoint. Used for WebSocket channel where
// outbound media attachments are not supported.
func mediaToMarkdown(paths []string) string {
	if len(paths) == 0 {
		return ""
	}

	var parts []string
	for _, p := range paths {
		cleanPath := filepath.Clean(p)
		urlPath := strings.TrimPrefix(cleanPath, "/")
		if urlPath == "" {
			continue
		}

		var fileURL string
		if strings.HasPrefix(urlPath, "v1/files/") || strings.HasPrefix(urlPath, "v1/media/") {
			fileURL = "/" + strings.SplitN(urlPath, "?", 2)[0]
		} else {
			fileURL = "/v1/files/" + urlPath
		}

		ct := mime.TypeByExtension(filepath.Ext(p))
		if strings.HasPrefix(ct, "image/") {
			parts = append(parts, fmt.Sprintf("![image](%s)", fileURL))
		} else {
			parts = append(parts, fmt.Sprintf("[%s](%s)", filepath.Base(p), fileURL))
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return "\n\n" + strings.Join(parts, "\n")
}

// overrideSessionKeyForThread extracts topic/thread ID from composite local_key
// and returns the correct session key for forum topics or DM threads.
// If localKey is empty or has no suffix, returns the original sessionKey unchanged.
func overrideSessionKeyForThread(sessionKey, localKey, agentID, channel, chatID, peerKind string) string {
	if localKey == "" {
		return sessionKey
	}
	if idx := strings.Index(localKey, ":topic:"); idx > 0 && peerKind == "group" {
		var topicID int
		fmt.Sscanf(localKey[idx+7:], "%d", &topicID)
		if topicID > 0 {
			return fmt.Sprintf("%s:%s:topic:%d:%s", agentID, channel, topicID, chatID)
		}
	} else if idx := strings.Index(localKey, ":thread:"); idx > 0 && peerKind == "direct" {
		var threadID int
		fmt.Sscanf(localKey[idx+8:], "%d", &threadID)
		if threadID > 0 {
			return fmt.Sprintf("%s:%s:thread:%d:%s", agentID, channel, threadID, chatID)
		}
	}
	return sessionKey
}

// buildAnnounceOutMeta builds outbound metadata for announce messages so that
// Send() can route replies to the correct forum topic or DM thread.
func buildAnnounceOutMeta(localKey string) map[string]string {
	if localKey == "" {
		return nil
	}
	meta := map[string]string{"local_key": localKey}
	if idx := strings.Index(localKey, ":topic:"); idx > 0 {
		meta["message_thread_id"] = localKey[idx+7:]
	} else if idx := strings.Index(localKey, ":thread:"); idx > 0 {
		meta["message_thread_id"] = localKey[idx+8:]
	}
	return meta
}

// DeliveryChannels returns the set of channel identifiers a Qor reply must be
// sent to. The source channel always gets the reply; web and tui always receive
// a copy via the realtime hub (WebSocket broadcast). For web/tui sources the
// list is just ["web","tui"] since the hub already covers both.
func DeliveryChannels(sourceChannel string) []string {
	switch sourceChannel {
	case "web", "tui", "":
		return []string{"web", "tui"}
	default:
		return []string{sourceChannel, "web", "tui"}
	}
}

// resolveChannelType returns the platform type for a channel instance name.
func resolveChannelType(chanMgr *channels.Manager, name string) string {
	if chanMgr == nil || name == "" {
		return ""
	}
	for _, ch := range chanMgr.List() {
		if ch["name"] == name || ch["id"] == name {
			if t, ok := ch["type"].(string); ok {
				return t
			}
		}
	}
	return ""
}
