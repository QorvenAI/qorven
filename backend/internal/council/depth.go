// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package council

// Depth controls the complexity dial — one slider that maps to
// model tier, council mode, search depth, and tool permissions.
type Depth string

const (
	DepthQuick    Depth = "quick"    // Eco model, no tools, no council, single search
	DepthBalanced Depth = "balanced" // Auto-routed model, standard tools, single model
	DepthDeep     Depth = "deep"     // Primary model, all tools, council for complex queries
	DepthMax      Depth = "max"      // Council always, deep research, extended tool budget
)

// DepthConfig maps a depth level to concrete system parameters.
type DepthConfig struct {
	Depth           Depth   `json:"depth"`
	ModelTier       string  `json:"model_tier"`        // "eco", "auto", "premium"
	ToolsEnabled    bool    `json:"tools_enabled"`
	CouncilEnabled  bool    `json:"council_enabled"`
	CouncilThreshold float64 `json:"council_threshold"` // complexity score to trigger council
	SearchPasses    int     `json:"search_passes"`      // 1 = single, 3 = deep
	MaxIterations   int     `json:"max_iterations"`     // agent loop iterations
	MaxTokens       int     `json:"max_tokens"`         // per-response token limit
}

// DepthConfigs maps each depth level to its configuration.
var DepthConfigs = map[Depth]DepthConfig{
	DepthQuick: {
		Depth:           DepthQuick,
		ModelTier:       "eco",
		ToolsEnabled:    false,
		CouncilEnabled:  false,
		CouncilThreshold: 999, // never triggers
		SearchPasses:    1,
		MaxIterations:   5,
		MaxTokens:       1024,
	},
	DepthBalanced: {
		Depth:           DepthBalanced,
		ModelTier:       "auto",
		ToolsEnabled:    true,
		CouncilEnabled:  false,
		CouncilThreshold: 999,
		SearchPasses:    1,
		MaxIterations:   15,
		MaxTokens:       4096,
	},
	DepthDeep: {
		Depth:           DepthDeep,
		ModelTier:       "auto",
		ToolsEnabled:    true,
		CouncilEnabled:  true,
		CouncilThreshold: 0.7, // council when complexity > 0.7
		SearchPasses:    2,
		MaxIterations:   20,
		MaxTokens:       8192,
	},
	DepthMax: {
		Depth:           DepthMax,
		ModelTier:       "premium",
		ToolsEnabled:    true,
		CouncilEnabled:  true,
		CouncilThreshold: 0.0, // council always
		SearchPasses:    3,
		MaxIterations:   30,
		MaxTokens:       16384,
	},
}

// GetDepthConfig returns the config for a depth level.
func GetDepthConfig(depth Depth) DepthConfig {
	if cfg, ok := DepthConfigs[depth]; ok {
		return cfg
	}
	return DepthConfigs[DepthBalanced] // default
}

// ShouldUseCouncil checks if council mode should activate for a given query complexity.
func ShouldUseCouncil(depth Depth, complexityScore float64) bool {
	cfg := GetDepthConfig(depth)
	return cfg.CouncilEnabled && complexityScore >= cfg.CouncilThreshold
}
