// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package memory

import (
	"strings"

	"github.com/qorvenai/qorven/internal/pii"
)

// Classifier auto-assigns classification to knowledge items based on
// content signals, source patterns, and agent role.
type Classifier struct {
	piiCfg pii.Config
	rules  []ClassificationRule
}

type ClassificationRule struct {
	Level          Classification
	SourcePatterns []string
	ContentSignals []string
	AgentRoles     []string
	PIIRequired    bool
}

var defaultRules = []ClassificationRule{
	{Level: ClassRestricted, PIIRequired: true},
	{Level: ClassConfidential, SourcePatterns: []string{"telegram:", "email:", "whatsapp:", "sms:", "webchat:", "slack:"}},
	{Level: ClassConfidential, ContentSignals: []string{"revenue", "budget", "salary", "payment", "invoice"}, AgentRoles: []string{"cfo", "analyst"}},
	{Level: ClassConfidential, ContentSignals: []string{"contract", "nda", "compliance", "lawsuit", "regulation"}, AgentRoles: []string{"legal"}},
	{Level: ClassConfidential, ContentSignals: []string{"performance review", "termination", "hire", "compensation"}, AgentRoles: []string{"chro"}},
	{Level: ClassConfidential, ContentSignals: []string{"roadmap", "acquisition", "pivot", "competitive"}, AgentRoles: []string{"chief", "product"}},
}

func NewClassifier() *Classifier {
	return &Classifier{
		piiCfg: pii.Config{Kinds: pii.All},
		rules:  defaultRules,
	}
}

// Classify determines the classification level for content.
func (c *Classifier) Classify(content, source, agentRole string) Classification {
	if c.hasPII(content) {
		return ClassRestricted
	}

	highest := ClassPublic
	lower := strings.ToLower(content)

	for _, rule := range c.rules {
		if rule.PIIRequired {
			continue
		}
		if c.matchesRule(rule, lower, source, agentRole) && rule.Level > highest {
			highest = rule.Level
		}
	}

	if highest == ClassPublic {
		return ClassInternal
	}
	return highest
}

func (c *Classifier) hasPII(content string) bool {
	dets := pii.Scan(content, c.piiCfg)
	return len(dets) > 0
}

func (c *Classifier) matchesRule(rule ClassificationRule, lowerContent, source, agentRole string) bool {
	for _, p := range rule.SourcePatterns {
		if strings.HasPrefix(source, p) {
			return true
		}
	}

	if len(rule.ContentSignals) > 0 && len(rule.AgentRoles) > 0 {
		roleMatch := false
		for _, r := range rule.AgentRoles {
			if r == agentRole {
				roleMatch = true
				break
			}
		}
		if roleMatch {
			for _, sig := range rule.ContentSignals {
				if strings.Contains(lowerContent, sig) {
					return true
				}
			}
		}
		return false
	}

	for _, sig := range rule.ContentSignals {
		if strings.Contains(lowerContent, sig) {
			return true
		}
	}

	return false
}
