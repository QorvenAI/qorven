// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package agent

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"sync/atomic"
	"unicode"
)

// defender.go — Tool Result Defender.
// Scans tool output for prompt injection before the LLM sees it.
// Two-tier: Tier 1 pattern detection (~1ms) + Tier 2 scoring.

type RiskLevel string

const (
	RiskLow      RiskLevel = "low"
	RiskMedium   RiskLevel = "medium"
	RiskHigh     RiskLevel = "high"
	RiskCritical RiskLevel = "critical"
)

type DefenseResult struct {
	Allowed         bool      `json:"allowed"`
	RiskLevel       RiskLevel `json:"risk_level"`
	Sanitized       string    `json:"sanitized"`
	Detections      []string  `json:"detections"`
	FieldsSanitized []string  `json:"fields_sanitized,omitempty"`
	Score           float64   `json:"score"` // 0.0=safe, 1.0=injection
}

type Defender struct {
	blockHighRisk bool
	idCounter     atomic.Int64
}

func NewDefender(blockHighRisk bool) *Defender {
	return &Defender{blockHighRisk: blockHighRisk}
}

// DefendToolResult scans a tool result string for prompt injection.
func (d *Defender) DefendToolResult(content, toolName string) DefenseResult {
	if content == "" {
		return DefenseResult{Allowed: true, RiskLevel: RiskLow, Sanitized: content}
	}

	result := DefenseResult{Allowed: true, RiskLevel: RiskMedium, Sanitized: content}

	// Tier 1: Pattern detection
	result.Sanitized, result.Detections, result.RiskLevel = d.tier1(content, toolName)

	// Tier 2: Heuristic scoring (no ML model — pure Go, no ONNX dependency)
	result.Score = d.tier2Score(result.Sanitized)
	if result.Score > 0.8 { result.RiskLevel = RiskCritical }
	if result.Score > 0.5 && result.RiskLevel == RiskMedium { result.RiskLevel = RiskHigh }

	// Block decision
	if d.blockHighRisk && (result.RiskLevel == RiskHigh || result.RiskLevel == RiskCritical) {
		result.Allowed = false
	}

	if len(result.Detections) == 0 && result.Score < 0.3 {
		result.RiskLevel = RiskLow
	}

	return result
}

// DefendFields scans specific fields of a structured tool result.
func (d *Defender) DefendFields(fields map[string]string, toolName string) (map[string]string, DefenseResult) {
	riskyFields := toolRiskyFields(toolName)
	sanitized := make(map[string]string, len(fields))
	combined := DefenseResult{Allowed: true, RiskLevel: RiskLow}

	for key, value := range fields {
		if !riskyFields[key] {
			sanitized[key] = value
			continue
		}
		r := d.DefendToolResult(value, toolName)
		sanitized[key] = r.Sanitized
		if riskSeverity(r.RiskLevel) > riskSeverity(combined.RiskLevel) { combined.RiskLevel = r.RiskLevel }
		if !r.Allowed { combined.Allowed = false }
		combined.Detections = append(combined.Detections, r.Detections...)
		if len(r.Detections) > 0 { combined.FieldsSanitized = append(combined.FieldsSanitized, key) }
	}
	combined.Sanitized = fmt.Sprintf("%v", sanitized)
	return sanitized, combined
}

// ── Tier 1: Pattern Detection ──

func (d *Defender) tier1(content, toolName string) (string, []string, RiskLevel) {
	s := content
	var detections []string
	risk := RiskMedium

	// 1. Unicode normalization — prevent homoglyph attacks
	normalized := normalizeUnicode(s)
	if normalized != s {
		detections = append(detections, "unicode_homoglyph")
		s = normalized
	}

	// 2. Detect and decode encoded payloads
	if decoded, found := detectEncoding(s); found {
		detections = append(detections, "encoded_payload")
		risk = RiskHigh
		s = decoded
	}

	// 3. Strip role markers
	stripped, roleDetections := stripRoleMarkers(s)
	if len(roleDetections) > 0 {
		detections = append(detections, roleDetections...)
		risk = RiskHigh
		s = stripped
	}

	// 4. Detect and redact injection patterns
	redacted, injDetections := redactInjectionPatterns(s)
	if len(injDetections) > 0 {
		detections = append(detections, injDetections...)
		risk = RiskCritical
		s = redacted
	}

	// 5. Wrap in boundary tags
	id := d.idCounter.Add(1)
	s = fmt.Sprintf("[UD-%d]%s[/UD-%d]", id, s, id)

	return s, detections, risk
}

// ── Tier 2: Heuristic Scoring ──

func (d *Defender) tier2Score(content string) float64 {
	lower := strings.ToLower(content)
	score := 0.0

	// Injection phrase density
	injectionPhrases := []struct{ phrase string; weight float64 }{
		{"ignore previous", 0.4}, {"ignore all", 0.4}, {"disregard", 0.3},
		{"forget your instructions", 0.5}, {"new instructions", 0.4},
		{"you are now", 0.3}, {"act as", 0.2}, {"pretend you are", 0.3},
		{"system prompt", 0.4}, {"override", 0.3}, {"jailbreak", 0.5},
		{"do anything now", 0.5}, {"ignore safety", 0.4},
		{"output your instructions", 0.5}, {"reveal your prompt", 0.5},
		{"what are your rules", 0.3}, {"bypass", 0.3},
	}
	for _, p := range injectionPhrases {
		if strings.Contains(lower, p.phrase) { score += p.weight }
	}

	// Role marker presence
	roleMarkers := []string{"system:", "assistant:", "<|system|>", "[inst]", "<<sys>>", "<|im_start|>"}
	for _, m := range roleMarkers {
		if strings.Contains(lower, m) { score += 0.3 }
	}

	// Instruction-like structure (imperative sentences)
	imperatives := []string{"you must", "you should", "you will", "always ", "never ", "do not ", "from now on"}
	for _, imp := range imperatives {
		if strings.Contains(lower, imp) { score += 0.1 }
	}

	if score > 1.0 { score = 1.0 }
	return score
}

// ── Pattern Helpers ──

var homoglyphMap = map[rune]rune{
	'а': 'a', 'е': 'e', 'о': 'o', 'р': 'p', 'с': 'c', 'у': 'y', 'х': 'x',
	'А': 'A', 'Е': 'E', 'О': 'O', 'Р': 'P', 'С': 'C', 'У': 'Y', 'Х': 'X',
	'і': 'i', 'ј': 'j', 'ɡ': 'g', 'ɑ': 'a', 'ε': 'e',
	'\u200b': 0, '\u200c': 0, '\u200d': 0, '\ufeff': 0, // zero-width chars
}

func normalizeUnicode(s string) string {
	var b strings.Builder
	changed := false
	for _, r := range s {
		if mapped, ok := homoglyphMap[r]; ok {
			if mapped != 0 { b.WriteRune(mapped) }
			changed = true
		} else if !unicode.IsPrint(r) && r != '\n' && r != '\r' && r != '\t' {
			changed = true // skip non-printable
		} else {
			b.WriteRune(r)
		}
	}
	if !changed { return s }
	return b.String()
}

var roleMarkerRe = regexp.MustCompile(`(?im)^(SYSTEM|ASSISTANT|USER|HUMAN|AI)\s*:\s*`)
var xmlRoleRe = regexp.MustCompile(`(?i)<\/?(system|assistant|user|instruction|prompt)>`)
var specialRoleRe = regexp.MustCompile(`(?i)(\[INST\]|\[/INST\]|<<SYS>>|<</SYS>>|<\|im_start\|>\s*(system|assistant)|<\|im_end\|>)`)

func stripRoleMarkers(s string) (string, []string) {
	var detections []string
	if roleMarkerRe.MatchString(s) { detections = append(detections, "role_marker"); s = roleMarkerRe.ReplaceAllString(s, "") }
	if xmlRoleRe.MatchString(s) { detections = append(detections, "xml_role_tag"); s = xmlRoleRe.ReplaceAllString(s, "") }
	if specialRoleRe.MatchString(s) { detections = append(detections, "special_token"); s = specialRoleRe.ReplaceAllString(s, "") }
	return s, detections
}

var injectionPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(ignore|disregard|forget|override|bypass)\s+(all\s+)?(your\s+|my\s+|previous|prior|above)?\s*(instructions?|rules?|prompts?|guidelines?|constraints?)`),
	regexp.MustCompile(`(?i)new\s+instructions?\s*:`),
	regexp.MustCompile(`(?i)(you are now|from now on you are|pretend you are|act as if you are)\s+`),
	regexp.MustCompile(`(?i)(output|reveal|show|print|display)\s+(your|the)\s+(system\s+)?(prompt|instructions?|rules?)`),
	regexp.MustCompile(`(?i)do\s+anything\s+now`),
	regexp.MustCompile(`(?i)(jailbreak|jail\s*break)`),
}

func redactInjectionPatterns(s string) (string, []string) {
	var detections []string
	for _, re := range injectionPatterns {
		if re.MatchString(s) {
			detections = append(detections, "injection_pattern")
			s = re.ReplaceAllString(s, "[REDACTED-INJECTION]")
		}
	}
	return s, detections
}

func detectEncoding(s string) (string, bool) {
	// Check for base64-encoded injection
	b64Re := regexp.MustCompile(`[A-Za-z0-9+/]{40,}={0,2}`)
	for _, match := range b64Re.FindAllString(s, 3) {
		decoded, err := base64.StdEncoding.DecodeString(match)
		if err == nil {
			lower := strings.ToLower(string(decoded))
			if strings.Contains(lower, "ignore") || strings.Contains(lower, "system") || strings.Contains(lower, "instruction") {
				return strings.ReplaceAll(s, match, "[ENCODED-INJECTION-REDACTED]"), true
			}
		}
	}
	// Check for URL-encoded injection
	if decoded, err := url.QueryUnescape(s); err == nil && decoded != s {
		lower := strings.ToLower(decoded)
		if strings.Contains(lower, "ignore") || strings.Contains(lower, "system") {
			return decoded, true
		}
	}
	return s, false
}

// ── Per-Tool Field Scanning ──

func toolRiskyFields(toolName string) map[string]bool {
	// Tool-specific risky fields (content that could contain injection)
	toolFields := map[string][]string{
		"gmail":     {"subject", "body", "snippet", "content"},
		"email":     {"subject", "body", "snippet", "content"},
		"documents": {"name", "description", "content", "title"},
		"github":    {"name", "title", "body", "description", "message"},
		"slack":     {"text", "message", "title"},
		"crm":       {"name", "description", "notes", "content"},
	}

	// Match by prefix
	for prefix, fields := range toolFields {
		if strings.HasPrefix(toolName, prefix) {
			m := make(map[string]bool, len(fields))
			for _, f := range fields { m[f] = true }
			return m
		}
	}

	// Default risky fields
	defaults := []string{"name", "description", "content", "title", "notes", "summary", "body", "text", "message", "comment", "subject"}
	m := make(map[string]bool, len(defaults))
	for _, f := range defaults { m[f] = true }
	return m
}

func riskSeverity(r RiskLevel) int {
	switch r {
	case RiskLow: return 0
	case RiskMedium: return 1
	case RiskHigh: return 2
	case RiskCritical: return 3
	default: return 0
	}
}
