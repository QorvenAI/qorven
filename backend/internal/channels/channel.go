// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package channels

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"sync"

	"github.com/google/uuid"
)

// InternalChannels are system channels excluded from outbound dispatch.
var InternalChannels = map[string]bool{
	"cli": true, "system": true, "subagent": true, "browser": true, "ws": true, "web": true, "webchat": true,
}

// IsInternalChannel checks if a channel name is internal.
func IsInternalChannel(name string) bool {
	return InternalChannels[name]
}

// DMPolicy controls how DMs from unknown senders are handled.
type DMPolicy string

const (
	DMPolicyPairing   DMPolicy = "pairing"
	DMPolicyAllowlist DMPolicy = "allowlist"
	DMPolicyOpen      DMPolicy = "open"
	DMPolicyDisabled  DMPolicy = "disabled"
)

// GroupPolicy controls how group messages are handled.
type GroupPolicy string

const (
	GroupPolicyOpen      GroupPolicy = "open"
	GroupPolicyAllowlist GroupPolicy = "allowlist"
	GroupPolicyDisabled  GroupPolicy = "disabled"
)

// Channel type constants.
const (
	TypeTelegram = "telegram"
	TypeDiscord  = "discord"
	TypeSlack    = "slack"
	TypeFeishu   = "feishu"
	TypeWhatsApp = "whatsapp"
	TypeEmail    = "email"
)

// Channel is the interface every adapter must implement.
type Channel interface {
	Name() string
	Type() string
	AgentID() string
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	Send(ctx context.Context, msg OutboundMessage) error
	IsRunning() bool
}

// AllowListChannel is optionally implemented by channels with allowlist support.
type AllowListChannel interface {
	Channel
	IsAllowed(senderID string) bool
}

// StreamingChannel extends Channel with real-time streaming preview support.
type StreamingChannel interface {
	Channel
	StreamEnabled(isGroup bool) bool
	CreateStream(ctx context.Context, chatID string, firstStream bool) (ChannelStream, error)
	FinalizeStream(ctx context.Context, chatID string, stream ChannelStream)
	ReasoningStreamEnabled() bool
}

// WebhookChannel extends Channel with an HTTP handler.
type WebhookChannel interface {
	Channel
	WebhookHandler() (path string, handler http.Handler)
}

// ReactionChannel extends Channel with status reaction support.
type ReactionChannel interface {
	Channel
	OnReactionEvent(ctx context.Context, chatID, messageID, status string) error
	ClearReaction(ctx context.Context, chatID, messageID string) error
}

// ChannelStream represents a streaming message handle.
type ChannelStream interface {
	Update(ctx context.Context, content string) error
	Stop(ctx context.Context) error
	MessageID() string
}

type InboundMessage struct {
	ChannelName  string
	ChannelType  string
	InstanceID   string
	AgentID      string
	TenantID     uuid.UUID
	SenderID     string
	SenderName   string
	ChatID       string
	Content      string
	Subject      string
	ReplyTo      string
	PeerKind     string // "direct" or "group"
	UserID       string
	HistoryLimit int
	ToolAllow    []string
	Metadata     map[string]string
	Media        []MediaFile
}

type MediaFile struct {
	Path     string
	MimeType string
}

type OutboundMessage struct {
	RecipientID string
	ChatID      string
	Content     string
	Subject     string
	Format      string
	ReplyTo     string
	Metadata    map[string]string
	Media       []MediaAttachment
}

type MediaAttachment struct {
	URL         string
	ContentType string
	Caption     string
}

// InboundHandler is called when a channel receives a message.
type InboundHandler func(ctx context.Context, msg InboundMessage)

// BaseChannel provides shared functionality for all channel implementations.
type BaseChannel struct {
	name        string
	channelType string
	agentID     string
	tenantID    uuid.UUID
	running     bool
	stateMu     sync.RWMutex
	allowList   []string
	handler     InboundHandler
}

// NewBaseChannel creates a new BaseChannel.
func NewBaseChannel(name, channelType string, allowList []string, handler InboundHandler) *BaseChannel {
	return &BaseChannel{
		name:        name,
		channelType: channelType,
		allowList:   allowList,
		handler:     handler,
	}
}

func (c *BaseChannel) Name() string        { return c.name }
func (c *BaseChannel) Type() string        { return c.channelType }
func (c *BaseChannel) AgentID() string     { return c.agentID }
func (c *BaseChannel) TenantID() uuid.UUID { return c.tenantID }

func (c *BaseChannel) SetAgentID(id string)       { c.agentID = id }
func (c *BaseChannel) SetTenantID(id uuid.UUID)   { c.tenantID = id }

func (c *BaseChannel) IsRunning() bool {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return c.running
}

func (c *BaseChannel) SetRunning(running bool) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	c.running = running
}

func (c *BaseChannel) IsAllowed(senderID string) bool {
	if len(c.allowList) == 0 {
		return true
	}
	idPart := senderID
	if idx := strings.Index(senderID, "|"); idx > 0 {
		idPart = senderID[:idx]
	}
	for _, allowed := range c.allowList {
		trimmed := strings.TrimPrefix(allowed, "@")
		if senderID == allowed || idPart == allowed || senderID == trimmed || idPart == trimmed {
			return true
		}
	}
	return false
}

func (c *BaseChannel) CheckPolicy(peerKind string, dmPolicy, groupPolicy DMPolicy, senderID string) bool {
	policy := dmPolicy
	if peerKind == "group" {
		policy = DMPolicy(groupPolicy)
	}
	if policy == "" {
		policy = DMPolicyOpen
	}
	switch policy {
	case DMPolicyDisabled:
		return false
	case DMPolicyAllowlist, DMPolicyPairing:
		return c.IsAllowed(senderID)
	default:
		return true
	}
}

func (c *BaseChannel) HandleMessage(msg InboundMessage) {
	if msg.PeerKind != "group" && !c.IsAllowed(msg.SenderID) {
		return
	}
	if c.handler != nil {
		c.handler(context.Background(), msg)
	}
}

// HasAllowList returns true if an allowlist is configured.
func (c *BaseChannel) HasAllowList() bool { return len(c.allowList) > 0 }

// MessageBus interface for outbound message publishing.
type MessageBus interface {
	PublishOutbound(msg OutboundMessage)
	SubscribeOutbound(ctx context.Context) (OutboundMessage, bool)
}

// Manager manages all channel instances.
type Manager struct {
	mu       sync.RWMutex
	channels map[string]Channel
	health   map[string]ChannelHealth
	handler  InboundHandler
	bus      MessageBus
	runs     sync.Map // runID → *RunContext
}

func NewManager(handler InboundHandler) *Manager {
	return &Manager{
		channels: make(map[string]Channel),
		health:   make(map[string]ChannelHealth),
		handler:  handler,
	}
}

// NewManagerWithBus creates a Manager with a message bus for outbound dispatch.
func NewManagerWithBus(handler InboundHandler, bus MessageBus) *Manager {
	return &Manager{
		channels: make(map[string]Channel),
		health:   make(map[string]ChannelHealth),
		handler:  handler,
		bus:      bus,
	}
}

// SetBus sets the message bus for outbound dispatch.
func (m *Manager) SetBus(bus MessageBus) { m.bus = bus }

func (m *Manager) Register(instanceID string, ch Channel) {
	m.mu.Lock()
	m.channels[instanceID] = ch
	m.mu.Unlock()
}

func (m *Manager) Start(ctx context.Context, instanceID string) error {
	m.mu.RLock()
	ch, ok := m.channels[instanceID]
	m.mu.RUnlock()
	if !ok {
		return nil
	}
	slog.Info("channel.start", "instance", instanceID, "type", ch.Type(), "agent", ch.AgentID())
	return ch.Start(context.Background())
}

func (m *Manager) Stop(ctx context.Context, instanceID string) error {
	m.mu.RLock()
	ch, ok := m.channels[instanceID]
	m.mu.RUnlock()
	if !ok {
		return nil
	}
	return ch.Stop(ctx)
}

func (m *Manager) Send(ctx context.Context, instanceID string, msg OutboundMessage) error {
	m.mu.RLock()
	ch, ok := m.channels[instanceID]
	m.mu.RUnlock()
	if !ok {
		return nil
	}
	return ch.Send(ctx, msg)
}

func (m *Manager) StartAll(ctx context.Context) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for id, ch := range m.channels {
		if err := ch.Start(ctx); err != nil {
			slog.Error("channel.start.failed", "instance", id, "error", err)
		}
	}
}

func (m *Manager) StopAll(ctx context.Context) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, ch := range m.channels {
		ch.Stop(ctx)
	}
}

func (m *Manager) Handler() InboundHandler { return m.handler }

func (m *Manager) GetChannel(instanceID string) Channel {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.channels[instanceID]
}

// Unregister removes a channel from the manager.
func (m *Manager) Unregister(instanceID string) {
	m.mu.Lock()
	delete(m.channels, instanceID)
	m.mu.Unlock()
}

func (m *Manager) List() []map[string]any {
	m.mu.RLock()
	defer m.mu.RUnlock()
	list := []map[string]any{}
	for id, ch := range m.channels {
		list = append(list, map[string]any{
			"id": id, "name": ch.Name(), "type": ch.Type(),
			"agent_id": ch.AgentID(), "running": ch.IsRunning(),
		})
	}
	return list
}

// RecordHealth stores runtime health for an instance.
func (m *Manager) RecordHealth(name string, snapshot ChannelHealth) {
	m.mu.Lock()
	defer m.mu.Unlock()
	prev := m.health[name]
	if snapshot.ChannelType == "" {
		if prev.ChannelType != "" {
			snapshot.ChannelType = prev.ChannelType
		} else if ch, ok := m.channels[name]; ok {
			snapshot.ChannelType = ch.Type()
		}
	}
	m.health[name] = mergeChannelHealth(prev, snapshot)
}

// RecordFailure stores a classified failure snapshot for an instance.
func (m *Manager) RecordFailure(name, summary string, err error) {
	m.RecordHealth(name, NewFailedChannelHealth(summary, err))
}

// GetStatus returns the running status of all channels.
func (m *Manager) GetStatus() map[string]any {
	m.mu.RLock()
	defer m.mu.RUnlock()
	status := make(map[string]any, len(m.health)+len(m.channels))
	for name, snapshot := range m.health {
		status[name] = snapshot
	}
	for name, ch := range m.channels {
		if _, ok := status[name]; !ok {
			state := ChannelHealthStateStopped
			summary := "Stopped"
			if ch.IsRunning() {
				state = ChannelHealthStateHealthy
				summary = "Connected"
			}
			status[name] = ChannelHealth{
				ChannelType: ch.Type(),
				Enabled:     true,
				Running:     ch.IsRunning(),
				State:       state,
				Summary:     summary,
			}
		}
	}
	return status
}

// ChannelTypeForName returns the platform type for a channel instance name.
func (m *Manager) ChannelTypeForName(name string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if ch, ok := m.channels[name]; ok {
		return ch.Type()
	}
	return ""
}

// ChannelTenantID returns the tenant ID for a channel instance.
func (m *Manager) ChannelTenantID(name string) (string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ch, ok := m.channels[name]
	if !ok {
		return "", false
	}
	if tc, ok := ch.(interface{ TenantID() string }); ok {
		return tc.TenantID(), true
	}
	return "", false
}
