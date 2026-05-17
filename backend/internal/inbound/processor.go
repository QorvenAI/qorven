// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

package inbound

import (
	"context"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qorvenai/qorven/internal/agent"
	"github.com/qorvenai/qorven/internal/channels"
	"github.com/qorvenai/qorven/internal/plugin"
	"github.com/qorvenai/qorven/internal/session"
)

// ActionMode controls what happens after classification.
type ActionMode string

const (
	ModeFullyAutonomous ActionMode = "fully_autonomous"
	ModeDraftAndApprove ActionMode = "draft_and_approve"
	ModeDraftOnly       ActionMode = "draft_only"
	ModeContextOnly     ActionMode = "context_only"
	ModeDrop            ActionMode = "drop"
)

// AgentConfig holds per-agent inbound automation settings.
type AgentConfig struct {
	AgentID             string     `json:"agent_id"`
	TenantID            uuid.UUID  `json:"tenant_id"`
	DefaultMode         ActionMode `json:"default_mode"`
	UnknownSenderMode   ActionMode `json:"unknown_sender_mode"`
	SpamAction          ActionMode `json:"spam_action"`
	NotificationChannel string     `json:"notification_channel"`
	NotificationTarget  string     `json:"notification_target"`
	BriefingEnabled     bool       `json:"briefing_enabled"`
	BriefingTime        string     `json:"briefing_time"`
	BriefingTimezone    string     `json:"briefing_timezone"`
}

// ReplyFunc sends a reply back to the originating channel.
// Gateway wires this up so the inbound processor can deliver autonomous responses.
type ReplyFunc func(ctx context.Context, agentID, channelType, chatID, content string, metadata map[string]string)

// Processor is the intelligence layer between channel receipt and agent loop.
type Processor struct {
	pool       *pgxpool.Pool
	sessions   *session.Store
	agentLoop  *agent.Loop
	classifier *Classifier
	draftQueue *DraftQueue
	pluginMgr  *plugin.Manager // nil-safe; set via SetPluginManager after construction
	reply      ReplyFunc       // nil-safe; set via SetReplyFunc after construction
}

// SetPluginManager injects the plugin manager so apps can hook into the pipeline.
func (p *Processor) SetPluginManager(mgr *plugin.Manager) { p.pluginMgr = mgr }

// SetReplyFunc wires the outbound delivery callback.
func (p *Processor) SetReplyFunc(fn ReplyFunc) { p.reply = fn }

// NewProcessor creates a Processor.
func NewProcessor(pool *pgxpool.Pool, sessions *session.Store, agentLoop *agent.Loop) *Processor {
	return &Processor{
		pool:       pool,
		sessions:   sessions,
		agentLoop:  agentLoop,
		classifier: &Classifier{},
		draftQueue: &DraftQueue{pool: pool},
	}
}

// Process runs the five-stage inbound pipeline.
func (p *Processor) Process(ctx context.Context, msg channels.InboundMessage) {
	// Stage 1: classify
	label := p.classifier.Classify(ctx, msg)

	// Fire message_receive hook so apps can observe every inbound message.
	if p.pluginMgr != nil {
		p.pluginMgr.FireHook(ctx, plugin.HookMessageReceive, map[string]any{
			"agent_id":  msg.AgentID,
			"sender_id": msg.SenderID,
			"channel":   msg.ChannelType,
			"content":   msg.Content,
			"label":     string(label),
		})
	}

	// Stage 2: load agent config
	cfg := p.loadAgentConfig(ctx, msg.AgentID)

	// Stage 3: determine action mode
	var mode ActionMode
	switch label {
	case LabelSpam:
		mode = cfg.SpamAction
	case LabelAutomated:
		mode = ModeContextOnly
	default:
		rules := p.loadRules(ctx, msg.AgentID)
		re := &RulesEngine{rules: rules}
		matched := re.Match(msg.SenderID, msg.ChannelType, msg.Content)
		if matched == "" {
			matched = cfg.UnknownSenderMode
		}
		mode = matched
	}

	if mode == ModeDrop {
		slog.Info("inbound.dropped", "agent", msg.AgentID, "sender", msg.SenderID)
		return
	}

	// Stage 4: build history summary
	historySummary := p.buildHistorySummary(ctx, msg)

	// Stage 5: act
	p.dispatch(ctx, msg, mode, cfg, historySummary)
}

// DraftQueue exposes the queue for gateway handlers.
func (p *Processor) DraftQueue() *DraftQueue { return p.draftQueue }
