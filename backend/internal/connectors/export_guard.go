package connectors

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

type ExportGuard struct {
	relayStore *RelayStore
}

func NewExportGuard(rs *RelayStore) *ExportGuard {
	return &ExportGuard{relayStore: rs}
}

var categoryExportPolicy = map[string]int{
	"social":        0,
	"email":         1,
	"messaging":     1,
	"calendar":      1,
	"productivity":  1,
	"storage":       1,
	"crm":           1,
	"development":   1,
	"marketing":     0,
	"communication": 1,
	"ecommerce":     1,
	"payments":      1,
}

func (g *ExportGuard) Check(ctx context.Context, tenantID, agentID, platformID, actionKey, category string, props map[string]any) error {
	if g.relayStore != nil && agentID != "" {
		allowed, err := g.relayStore.CheckPermission(ctx, tenantID, agentID, platformID, actionKey)
		if err != nil {
			return fmt.Errorf("permission check failed: %w", err)
		}
		if !allowed {
			return &PermissionDenied{AgentID: agentID, PlatformID: platformID, ActionKey: actionKey}
		}
	}

	findings := ScanPII(props)
	if len(findings) > 0 {
		maxLevel, ok := categoryExportPolicy[category]
		if !ok {
			maxLevel = 1
		}
		if maxLevel == 0 {
			return &PIIBlocked{Findings: findings, Action: platformID + "." + actionKey}
		}
	}

	return nil
}

type PIIFinding struct {
	Type  string `json:"type"`
	Field string `json:"field"`
}

var piiPatterns = map[string]*regexp.Regexp{
	"email":       regexp.MustCompile(`[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`),
	"phone":       regexp.MustCompile(`(\+?1[-.\s]?)?\(?\d{3}\)?[-.\s]?\d{3}[-.\s]?\d{4}`),
	"ssn":         regexp.MustCompile(`\b\d{3}-\d{2}-\d{4}\b`),
	"credit_card": regexp.MustCompile(`\b(?:\d{4}[-\s]?){3}\d{4}\b`),
	"api_key":     regexp.MustCompile(`\b(sk-[a-zA-Z0-9]{20,}|AIza[a-zA-Z0-9_\-]{35}|ghp_[a-zA-Z0-9]{36}|gsk_[a-zA-Z0-9]{20,})\b`),
}

func ScanPII(props map[string]any) []PIIFinding {
	var findings []PIIFinding
	for field, val := range props {
		text, ok := val.(string)
		if !ok {
			continue
		}
		for kind, re := range piiPatterns {
			if re.MatchString(text) {
				findings = append(findings, PIIFinding{Type: kind, Field: field})
			}
		}
	}
	return findings
}

type PermissionDenied struct {
	AgentID    string
	PlatformID string
	ActionKey  string
}

func (e *PermissionDenied) Error() string {
	return fmt.Sprintf("Agent not permitted to use %s.%s. Enable in Settings → Integrations → Permissions.", e.PlatformID, e.ActionKey)
}

type PIIBlocked struct {
	Findings []PIIFinding
	Action   string
}

func (e *PIIBlocked) Error() string {
	types := make([]string, len(e.Findings))
	for i, f := range e.Findings {
		types[i] = f.Type + " in '" + f.Field + "'"
	}
	return fmt.Sprintf("PII detected: %s. Cannot send to %s with personal data.", strings.Join(types, ", "), e.Action)
}
