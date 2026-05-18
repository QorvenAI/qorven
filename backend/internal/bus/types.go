// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package bus

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/google/uuid"
)

// MediaFile represents an inbound media file with its MIME type.
type MediaFile struct {
	Path     string `json:"path"`
	MimeType string `json:"mime_type,omitempty"`
}

// InboundMessage represents a message received from a channel.
type InboundMessage struct {
	Channel      string            `json:"channel"`
	SenderID     string            `json:"sender_id"`
	ChatID       string            `json:"chat_id"`
	Content      string            `json:"content"`
	Media        []MediaFile       `json:"media,omitempty"`
	SessionKey   string            `json:"session_key"`
	PeerKind     string            `json:"peer_kind,omitempty"`     // "direct" or "group"
	TenantID     uuid.UUID         `json:"tenant_id,omitempty"`
	AgentID      string            `json:"agent_id,omitempty"`
	UserID       string            `json:"user_id,omitempty"`
	HistoryLimit int               `json:"history_limit,omitempty"`
	ToolAllow    []string          `json:"tool_allow,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

// OutboundMessage represents a message to be sent to a channel.
type OutboundMessage struct {
	Channel  string            `json:"channel"`
	ChatID   string            `json:"chat_id"`
	Content  string            `json:"content"`
	Media    []MediaAttachment `json:"media,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// MediaAttachment represents a media file to be sent with a message.
type MediaAttachment struct {
	URL         string `json:"url"`
	ContentType string `json:"content_type,omitempty"`
	Caption     string `json:"caption,omitempty"`
}

// Event represents a server-side event to broadcast to WebSocket clients.
type Event struct {
	Name     string    `json:"name"`
	Payload  any       `json:"payload,omitempty"`
	TenantID uuid.UUID `json:"-"` // not serialized to clients
}

// Cache invalidation kind constants.
const (
	CacheKindAgent            = "agent"
	CacheKindBootstrap        = "bootstrap"
	CacheKindSkills           = "skills"
	CacheKindCron             = "cron"
	CacheKindChannelInstances = "channel_instances"
	CacheKindBuiltinTools     = "builtin_tools"
	CacheKindTeam             = "team"
	CacheKindUserWorkspace    = "user_workspace"
	CacheKindMCP              = "mcp"
	CacheKindProvider         = "provider"
	CacheKindHeartbeat        = "heartbeat"
)

// Topic constants for Subscribe/Broadcast.
const (
	TopicCacheBootstrap        = "cache:bootstrap"
	TopicCacheAgent            = "cache:agent"
	TopicCacheSkills           = "cache:skills"
	TopicCacheCron             = "cache:cron"
	TopicCacheBuiltinTools     = "cache:builtin_tools"
	TopicCacheTeam             = "cache:team"
	TopicCacheUserWorkspace    = "cache:user_workspace"
	TopicCacheChannelInstances = "cache:channel_instances"
	TopicCacheMCP              = "cache:mcp"
	TopicCacheProvider         = "cache:provider"
	TopicCacheHeartbeat        = "cache:heartbeat"
	TopicAudit                 = "audit"
	TopicTeamTaskAudit         = "team-task-audit"
	TopicChannelStreaming      = "channel-streaming"
	TopicConfigChanged         = "config:changed"
	TopicAgentStatusChanged    = "agent:status_changed"
	TopicAgentDeleted          = "agent:deleted"
)

// AuditEventPayload carries audit log data.
type AuditEventPayload struct {
	ActorType  string          `json:"actor_type"`
	ActorID    string          `json:"actor_id"`
	Action     string          `json:"action"`
	EntityType string          `json:"entity_type"`
	EntityID   string          `json:"entity_id"`
	IPAddress  string          `json:"ip_address,omitempty"`
	Details    json.RawMessage `json:"details,omitempty"`
	TenantID   uuid.UUID       `json:"tenant_id,omitempty"`
}

// CacheInvalidatePayload signals cache layers to evict stale entries.
type CacheInvalidatePayload struct {
	Kind string `json:"kind"`
	Key  string `json:"key"` // empty = invalidate all
}

// AgentStatusChangedPayload carries agent status transition info.
type AgentStatusChangedPayload struct {
	AgentID   string `json:"agent_id"`
	OldStatus string `json:"old_status"`
	NewStatus string `json:"new_status"`
}

// AgentDeletedPayload carries agent deletion info.
type AgentDeletedPayload struct {
	AgentKey string    `json:"agent_key"`
	Provider string    `json:"provider,omitempty"`
	TenantID uuid.UUID `json:"tenant_id,omitempty"`
}

// MessageHandler handles an inbound message from a specific channel.
type MessageHandler func(InboundMessage) error

// EventHandler handles a broadcast event.
type EventHandler func(Event)

// EventPublisher abstracts event broadcast + subscription.
type EventPublisher interface {
	Subscribe(id string, handler EventHandler)
	Unsubscribe(id string)
	Broadcast(event Event)
}

// MessageRouter abstracts inbound/outbound message routing.
type MessageRouter interface {
	PublishInbound(msg InboundMessage)
	ConsumeInbound(ctx context.Context) (InboundMessage, bool)
	PublishOutbound(msg OutboundMessage)
	SubscribeOutbound(ctx context.Context) (OutboundMessage, bool)
}

// IsInternalSender returns true if the senderID belongs to an internal system component.
func IsInternalSender(senderID string) bool {
	return strings.HasPrefix(senderID, "system:") ||
		strings.HasPrefix(senderID, "notification:") ||
		strings.HasPrefix(senderID, "teammate:") ||
		strings.HasPrefix(senderID, "ticker:") ||
		senderID == "session_send_tool"
}
