// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

package agent

import (
	"github.com/qorvenai/qorven/internal/pii"
)

// piiAdapter wraps a pii.Config into the agent.PIIRedactor interface
// so Loop doesn't import internal/pii directly. Keeps the dependency
// graph clean: agent stays tool-level + provider-level, and pii stays
// a standalone utility package.
type piiAdapter struct {
	cfg pii.Config
}

func (p *piiAdapter) Redact(text string) string {
	return pii.Redact(text, p.cfg)
}

// NewPIIRedactor builds a redactor with the default placeholder shape
// ("{{PII:kind}}"). Pass 0 kinds to get a no-op redactor; use
// pii.All for everything.
//
// Intended wiring:
//
//	cfg := gw.sysConfig.Get(ctx, tenant, "services.pii_redaction")
//	if on, _ := cfg["enabled"].(bool); on {
//	    kinds := kindsFromConfig(cfg)     // parse enabled categories
//	    gw.agentLoop.SetPIIRedactor(agent.NewPIIRedactor(kinds))
//	}
func NewPIIRedactor(kinds pii.Kind) PIIRedactor {
	return &piiAdapter{cfg: pii.Config{Kinds: kinds}}
}
