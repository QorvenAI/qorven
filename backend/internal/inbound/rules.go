package inbound

import (
	"context"
	"log/slog"
	"regexp"
	"strings"

	"github.com/google/uuid"
)


// MatchType constants.
type MatchType string

const (
	MatchContact MatchType = "contact"
	MatchDomain  MatchType = "domain"
	MatchChannel MatchType = "channel"
	MatchKeyword MatchType = "keyword"
	MatchDefault MatchType = "default"
)

// Rule is a single routing rule.
type Rule struct {
	ID              string
	Priority        int
	MatchType       MatchType
	MatchValue      string
	Mode            ActionMode
	compiledKeyword *regexp.Regexp // pre-compiled for MatchKeyword rules; nil for others
}

// RulesEngine evaluates rules and returns the first matching mode.
type RulesEngine struct {
	rules []Rule
}

// Match returns the action mode for the given sender/channel/content.
// Returns "" if no rule matches.
func (re *RulesEngine) Match(senderID, channelType, content string) ActionMode {
	sender := strings.ToLower(senderID)
	for _, r := range re.rules {
		switch r.MatchType {
		case MatchContact:
			if strings.ToLower(r.MatchValue) == sender {
				return r.Mode
			}
		case MatchDomain:
			domain := strings.TrimPrefix(strings.ToLower(r.MatchValue), "@")
			if strings.HasSuffix(sender, "@"+domain) || strings.HasSuffix(sender, domain) {
				return r.Mode
			}
		case MatchChannel:
			if strings.EqualFold(r.MatchValue, channelType) {
				return r.Mode
			}
		case MatchKeyword:
			if r.compiledKeyword != nil && r.compiledKeyword.MatchString(content) {
				return r.Mode
			}
		case MatchDefault:
			return r.Mode
		}
	}
	return ""
}

// loadRules fetches rules for an agent from DB, ordered by priority asc.
func (p *Processor) loadRules(ctx context.Context, agentID string) []Rule {
	rows, err := p.pool.Query(ctx,
		`SELECT id, priority, match_type, match_value, mode FROM inbound_rules
		 WHERE agent_id = $1 ORDER BY priority ASC`, agentID)
	if err != nil {
		slog.Warn("inbound.rules.load_failed", "agent", agentID, "err", err)
		return nil
	}
	defer rows.Close()
	var rules []Rule
	for rows.Next() {
		var r Rule
		var matchType, mode string
		if err := rows.Scan(&r.ID, &r.Priority, &matchType, &r.MatchValue, &mode); err != nil {
			continue
		}
		r.MatchType = MatchType(matchType)
		r.Mode = ActionMode(mode)
		if r.MatchType == MatchKeyword && r.MatchValue != "" {
			pat, err := regexp.Compile("(?i)" + r.MatchValue)
			if err != nil {
				slog.Warn("inbound.rules.invalid_keyword", "rule", r.ID, "pattern", r.MatchValue, "err", err)
				continue // skip rules with invalid patterns
			}
			r.compiledKeyword = pat
		}
		rules = append(rules, r)
	}
	return rules
}

// loadAgentConfig fetches or returns a default AgentConfig.
func (p *Processor) loadAgentConfig(ctx context.Context, agentID string) AgentConfig {
	cfg := AgentConfig{
		AgentID:           agentID,
		DefaultMode:       ModeDraftAndApprove,
		UnknownSenderMode: ModeContextOnly,
		SpamAction:        ModeDrop,
		BriefingTime:      "08:00",
		BriefingTimezone:  "Asia/Shanghai",
	}
	var tenantID uuid.UUID
	var defMode, unknownMode, spamAct string
	_ = p.pool.QueryRow(ctx,
		`SELECT tenant_id, default_mode, unknown_sender_mode, spam_action,
		        notification_channel, notification_target,
		        briefing_enabled, briefing_time, briefing_timezone
		 FROM inbound_agent_config WHERE agent_id = $1`, agentID,
	).Scan(
		&tenantID, &defMode, &unknownMode, &spamAct,
		&cfg.NotificationChannel, &cfg.NotificationTarget,
		&cfg.BriefingEnabled, &cfg.BriefingTime, &cfg.BriefingTimezone,
	)
	if tenantID != (uuid.UUID{}) {
		cfg.TenantID = tenantID
	}
	if defMode != "" {
		cfg.DefaultMode = ActionMode(defMode)
	}
	if unknownMode != "" {
		cfg.UnknownSenderMode = ActionMode(unknownMode)
	}
	if spamAct != "" {
		cfg.SpamAction = ActionMode(spamAct)
	}
	return cfg
}
