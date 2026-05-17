// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package providers

import (
	"fmt"
	"strings"
	"sync"
)

// Registry manages provider instances by ID.
type Registry struct {
	mu        sync.RWMutex
	providers map[string]Provider
	configs   map[string]ProviderConfig
	sem       *Semaphore
}

func NewRegistry() *Registry {
	return &Registry{
		providers: make(map[string]Provider),
		configs:   make(map[string]ProviderConfig),
		sem:       NewSemaphore(5), // max 5 concurrent LLM calls
	}
}

// Register creates a provider instance from config and stores it.
func (r *Registry) Register(cfg ProviderConfig) error {
	p, err := NewProvider(cfg)
	if err != nil {
		return err
	}
	wrapped := NewRateLimitedProvider(p, r.sem)
	r.mu.Lock()
	r.providers[cfg.ID] = wrapped
	r.configs[cfg.ID] = cfg
	r.mu.Unlock()
	return nil
}

// RegisterProvider registers a pre-built provider (e.g. Bedrock).
func (r *Registry) RegisterProvider(name string, p Provider) {
	wrapped := NewRateLimitedProvider(p, r.sem)
	r.mu.Lock()
	r.providers[name] = wrapped
	r.configs[name] = ProviderConfig{ID: name, Name: name, Enabled: true}
	r.mu.Unlock()
}

// Get returns a provider by ID.
func (r *Registry) Get(id string) (Provider, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.providers[id]
	return p, ok
}

// GetByName returns the first provider matching the given name.
func (r *Registry) GetByName(name string) (Provider, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, cfg := range r.configs {
		if cfg.Name == name {
			return r.providers[cfg.ID], true
		}
	}
	return nil, false
}

// ProviderForModel picks the best provider for a given model ID by
// matching the ID's prefix / known patterns against the registered
// providers' type. Bedrock inference profiles (anthropic., us.anthropic.,
// deepseek., qwen., amazon.nova., nvidia.nemotron., etc.) route to the
// bedrock provider; everything else falls through to the first enabled
// provider whose model_registry entry lists this model, then to Default().
//
// Used by council and any other call-site that needs to map models to
// providers without the caller hard-coding the dispatch table.
func (r *Registry) ProviderForModel(model string) Provider {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// 1. Bedrock: recognise the inference-profile prefixes that the
	//    Bedrock provider knows how to invoke. Keep the list close to
	//    BedrockCuratedModels — unknown prefixes fall through so the
	//    user's configured OpenAI/Anthropic key still gets a shot.
	isBedrock := func(m string) bool {
		bedrockPrefixes := []string{
			"anthropic.", "us.anthropic.", "global.anthropic.", "eu.anthropic.",
			"deepseek.", "us.deepseek.",
			"qwen.", "amazon.nova", "amazon.titan",
			"nvidia.nemotron", "meta.llama", "us.meta.",
			"moonshotai.", "minimax.", "zai.glm", "mistral.devstral",
			"cohere.command",
		}
		for _, p := range bedrockPrefixes {
			if len(m) >= len(p) && m[:len(p)] == p { return true }
		}
		return false
	}
	if isBedrock(model) {
		for _, cfg := range r.configs {
			if cfg.ProviderType == TypeBedrock && cfg.Enabled {
				return r.providers[cfg.ID]
			}
		}
	}

	// 2. Exact-name match: if one of the registered providers is named
	//    e.g. "openai" and the model starts with "gpt-", prefer it.
	if spec, ok := ModelRegistry[model]; ok && spec.Provider != "" {
		for _, cfg := range r.configs {
			if cfg.Enabled && (cfg.Name == spec.Provider || cfg.ProviderType == spec.Provider) {
				return r.providers[cfg.ID]
			}
		}
	}

	// 3. Fallback: first enabled provider.
	for id, cfg := range r.configs {
		if cfg.Enabled { return r.providers[id] }
	}
	return nil
}

// Remove removes a provider by ID.
func (r *Registry) Remove(id string) {
	r.mu.Lock()
	delete(r.providers, id)
	delete(r.configs, id)
	r.mu.Unlock()
}

// List returns all registered provider configs.
func (r *Registry) List() []ProviderConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]ProviderConfig, 0, len(r.configs))
	for _, cfg := range r.configs {
		out = append(out, cfg)
	}
	return out
}

// ListCapable returns providers that have all the requested capability flags set.
// If no requirements are set (all false), it behaves identically to List().
func (r *Registry) ListCapable(need ProviderCapabilityFlags) []ProviderConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []ProviderConfig
	for _, cfg := range r.configs {
		if !cfg.Enabled { continue }
		c := cfg.Capabilities
		if need.Streaming && !c.Streaming { continue }
		if need.Caching && !c.Caching { continue }
		if need.Thinking && !c.Thinking { continue }
		if need.Vision && !c.Vision { continue }
		if need.Tools && !c.Tools { continue }
		if need.ParallelTools && !c.ParallelTools { continue }
		out = append(out, cfg)
	}
	return out
}

// GetModelContextWindow returns the actual context window for a model.
// Returns 0 if the model is unknown (caller should fall back to the agent's configured window).
func (r *Registry) GetModelContextWindow(modelID string) int {
	if s, ok := ModelRegistry[modelID]; ok {
		return s.MaxInputTokens
	}
	return 0
}

// HasModel checks if a model is known (exists in the model registry).
func (r *Registry) HasModel(model string) bool {
	spec := GetModelSpec(model)
	// GetModelSpec returns a default with MaxInputTokens=128000 for unknown models
	// A real model will have specific costs
	return spec.InputCostPer1M != 1.00 || spec.OutputCostPer1M != 3.00
}

// Default returns the first enabled provider, or nil.
func (r *Registry) Default() Provider {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for id, cfg := range r.configs {
		if cfg.Enabled {
			return r.providers[id]
		}
	}
	// Fallback: return any provider
	for _, p := range r.providers {
		return p
	}
	return nil
}

// RotateKey finds an alternative provider that can serve the same model.
// Called when the current provider returns 401/429/402 — rotates to a different
// API key (different provider config) without changing the model or losing context.
// Returns (true, newProvider) if rotation succeeded, (false, nil) otherwise.
func (r *Registry) RotateKey(current Provider, model string) (bool, Provider) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Find the ID of the current provider so we can skip it
	currentID := ""
	for id, p := range r.providers {
		if p == current {
			currentID = id
			break
		}
	}

	// Find another enabled provider — prefer ones that explicitly support this model
	for id, cfg := range r.configs {
		if id == currentID || !cfg.Enabled {
			continue
		}
		p, ok := r.providers[id]
		if !ok {
			continue
		}
		// Accept providers of the same type (same base URL means same API, different key)
		// or any OpenAI-compat provider that could proxy the model
		currentCfg, hasCurrent := r.configs[currentID]
		sameType := hasCurrent && cfg.ProviderType == currentCfg.ProviderType
		isCompat := cfg.ProviderType == TypeOpenAICompat || cfg.ProviderType == TypeOpenRouter
		if sameType || isCompat {
			return true, p
		}
	}
	return false, nil
}

// LoadAll registers multiple provider configs (e.g., from DB on boot).
func (r *Registry) LoadAll(configs []ProviderConfig) error {
	for _, cfg := range configs {
		if !cfg.Enabled {
			continue
		}
		if err := r.Register(cfg); err != nil {
			return fmt.Errorf("register %s: %w", cfg.Name, err)
		}
	}
	return nil
}

// NewProvider creates a Provider instance from config.
// The driver is resolved via DriverForProviderType so any provider_type that
// maps to a known driver works — no hardcoded switch cases needed for new providers.
func NewProvider(cfg ProviderConfig) (Provider, error) {
	// Fill api_base from manifest if caller didn't supply one
	if cfg.APIBase == "" {
		cfg.APIBase = DefaultAPIBase(cfg.ProviderType)
	}

	driver := DriverForProviderType(cfg.ProviderType)
	switch driver {
	case TypeAnthropicNative:
		return NewAnthropic(cfg), nil
	case TypeGeminiNative:
		// Strip the "/openai" OpenAI-compat suffix if the caller sent the
		// wrong base URL — the native Gemini client uses generateContent URLs.
		cfg.APIBase = strings.TrimSuffix(cfg.APIBase, "/openai")
		return NewGemini(cfg), nil
	case TypeDashScope:
		return NewDashScope(cfg.Name, cfg.APIKey, ""), nil
	case TypeBedrock:
		region := cfg.APIBase; if region == "" { region = "us-east-1" }
		var c BedrockCreds
		if cfg.AWSAccessKey != "" {
			c = BedrockCreds{AccessKey: cfg.AWSAccessKey, SecretKey: cfg.AWSSecretKey, SessionToken: cfg.AWSSessionToken}
		}
		p, err := NewBedrockProvider(cfg.Name, "", region, c)
		if err != nil { return nil, err }
		return p, nil
	case TypeBedrockConverse:
		p, err := NewBedrockConverseDriver(cfg, false)
		if err != nil { return nil, err }
		return p, nil
	case TypeBedrockMantle:
		p, err := NewBedrockConverseDriver(cfg, true)
		if err != nil { return nil, err }
		return p, nil
	case TypeSageMaker:
		p, err := NewSageMakerProvider(cfg)
		if err != nil { return nil, err }
		return p, nil
	default:
		// openai_compat handles all OpenAI-compatible endpoints (90% of providers)
		return NewOpenAI(cfg), nil
	}
}
