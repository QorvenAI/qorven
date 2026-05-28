// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgxpool"
)

type ValidationSeverity string

const (
	SeverityBlock ValidationSeverity = "block"
	SeverityWarn  ValidationSeverity = "warn"
	SeverityInfo  ValidationSeverity = "info"
)

type ValidationIssue struct {
	Rule     string             `json:"rule"`
	Severity ValidationSeverity `json:"severity"`
	Message  string             `json:"message"`
}

type ValidationResult struct {
	Passed bool              `json:"passed"`
	Score  float64           `json:"score"`
	Issues []ValidationIssue `json:"issues"`
}

type ValidationRule struct {
	Name     string
	Severity ValidationSeverity
	Check    func(content, actionType string, metadata map[string]any) *ValidationIssue
}

type OutputValidator struct {
	rules []ValidationRule
	db    *pgxpool.Pool
}

func NewOutputValidator(db *pgxpool.Pool) *OutputValidator {
	v := &OutputValidator{db: db}
	v.rules = v.defaultRules()
	return v
}

func (v *OutputValidator) Validate(content, actionType string, metadata map[string]any) ValidationResult {
	var issues []ValidationIssue
	for _, rule := range v.rules {
		if issue := rule.Check(content, actionType, metadata); issue != nil {
			issues = append(issues, *issue)
		}
	}

	score := v.computeScore(issues)
	return ValidationResult{
		Passed: score >= 4.0,
		Score:  score,
		Issues: issues,
	}
}

func (v *OutputValidator) RecordOutput(ctx context.Context, tenantID, agentID, sessionID, channel, contentType, content string, metadata map[string]any, result ValidationResult) {
	if v.db == nil {
		return
	}
	hash := sha256.Sum256([]byte(content))
	hashStr := hex.EncodeToString(hash[:])

	preview := content
	if len(preview) > 500 {
		preview = preview[:500]
	}

	v.db.Exec(ctx, `
		INSERT INTO output_audit (tenant_id, agent_id, session_id, channel, content_type, content_hash, content_preview, full_content, metadata, quality_score, validation_result)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`, tenantID, agentID, sessionID, channel, contentType, hashStr, preview, content, metadata, result.Score, result.Issues)
}

func (v *OutputValidator) IsDuplicate(ctx context.Context, tenantID, content string) bool {
	if v.db == nil {
		return false
	}
	hash := sha256.Sum256([]byte(content))
	hashStr := hex.EncodeToString(hash[:])

	var count int
	err := v.db.QueryRow(ctx, `
		SELECT COUNT(*) FROM output_audit
		WHERE tenant_id = $1 AND content_hash = $2 AND delivered_at > NOW() - INTERVAL '30 days'
	`, tenantID, hashStr).Scan(&count)
	if err != nil {
		return false
	}
	return count > 0
}

func (v *OutputValidator) computeScore(issues []ValidationIssue) float64 {
	if len(issues) == 0 {
		return 10.0
	}
	score := 10.0
	for _, issue := range issues {
		switch issue.Severity {
		case SeverityBlock:
			score -= 4.0
		case SeverityWarn:
			score -= 2.0
		case SeverityInfo:
			score -= 0.5
		}
	}
	if score < 1.0 {
		score = 1.0
	}
	return score
}

func (v *OutputValidator) defaultRules() []ValidationRule {
	return []ValidationRule{
		{Name: "pii_email", Severity: SeverityBlock, Check: checkPIIEmail},
		{Name: "pii_phone", Severity: SeverityWarn, Check: checkPIIPhone},
		{Name: "pii_ssn", Severity: SeverityBlock, Check: checkPIISSN},
		{Name: "pii_credit_card", Severity: SeverityBlock, Check: checkPIICreditCard},
		{Name: "empty_content", Severity: SeverityBlock, Check: checkEmptyContent},
		{Name: "article_min_length", Severity: SeverityWarn, Check: checkArticleMinLength},
		{Name: "social_max_length", Severity: SeverityWarn, Check: checkSocialMaxLength},
		{Name: "excessive_length", Severity: SeverityInfo, Check: checkExcessiveLength},
	}
}

var (
	emailRegex      = regexp.MustCompile(`[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`)
	phoneRegex      = regexp.MustCompile(`\b(\+?1[-.\s]?)?(\(?\d{3}\)?[-.\s]?)?\d{3}[-.\s]?\d{4}\b`)
	ssnRegex        = regexp.MustCompile(`\b\d{3}-\d{2}-\d{4}\b`)
	creditCardRegex = regexp.MustCompile(`\b(?:\d{4}[-\s]?){3}\d{4}\b`)
)

func checkPIIEmail(content, actionType string, _ map[string]any) *ValidationIssue {
	if actionType == "email" || actionType == "email_draft" {
		return nil
	}
	matches := emailRegex.FindAllString(content, -1)
	for _, m := range matches {
		if strings.Contains(m, "@example.") || strings.Contains(m, "@test.") {
			continue
		}
		return &ValidationIssue{
			Rule:     "pii_email",
			Severity: SeverityBlock,
			Message:  fmt.Sprintf("contains email address: %s", m),
		}
	}
	return nil
}

func checkPIIPhone(content, _ string, _ map[string]any) *ValidationIssue {
	if phoneRegex.MatchString(content) {
		return &ValidationIssue{
			Rule:     "pii_phone",
			Severity: SeverityWarn,
			Message:  "contains possible phone number",
		}
	}
	return nil
}

func checkPIISSN(content, _ string, _ map[string]any) *ValidationIssue {
	if ssnRegex.MatchString(content) {
		return &ValidationIssue{
			Rule:     "pii_ssn",
			Severity: SeverityBlock,
			Message:  "contains possible SSN",
		}
	}
	return nil
}

func checkPIICreditCard(content, _ string, _ map[string]any) *ValidationIssue {
	if creditCardRegex.MatchString(content) {
		return &ValidationIssue{
			Rule:     "pii_credit_card",
			Severity: SeverityBlock,
			Message:  "contains possible credit card number",
		}
	}
	return nil
}

func checkEmptyContent(content, _ string, _ map[string]any) *ValidationIssue {
	trimmed := strings.TrimSpace(content)
	if len(trimmed) == 0 {
		return &ValidationIssue{
			Rule:     "empty_content",
			Severity: SeverityBlock,
			Message:  "output is empty",
		}
	}
	return nil
}

func checkArticleMinLength(content, actionType string, _ map[string]any) *ValidationIssue {
	if actionType != "article_draft" {
		return nil
	}
	words := len(strings.Fields(content))
	if words < 100 {
		return &ValidationIssue{
			Rule:     "article_min_length",
			Severity: SeverityWarn,
			Message:  fmt.Sprintf("article has only %d words (minimum 100)", words),
		}
	}
	return nil
}

func checkSocialMaxLength(content, actionType string, _ map[string]any) *ValidationIssue {
	if actionType != "social_post" {
		return nil
	}
	charCount := utf8.RuneCountInString(content)
	if charCount > 280 {
		return &ValidationIssue{
			Rule:     "social_max_length",
			Severity: SeverityWarn,
			Message:  fmt.Sprintf("social post is %d characters (X/Twitter limit is 280)", charCount),
		}
	}
	return nil
}

func checkExcessiveLength(content, _ string, _ map[string]any) *ValidationIssue {
	if len(content) > 50000 {
		return &ValidationIssue{
			Rule:     "excessive_length",
			Severity: SeverityInfo,
			Message:  fmt.Sprintf("output is %d bytes (unusually large)", len(content)),
		}
	}
	return nil
}
