// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package store

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
)

const (
	ProviderAnthropicNative = "anthropic_native"
	ProviderOpenAICompat    = "openai_compat"
	ProviderGeminiNative    = "gemini_native"
	ProviderOpenRouter      = "openrouter"
	ProviderGroq            = "groq"
	ProviderDeepSeek        = "deepseek"
	ProviderMistral         = "mistral"
	ProviderXAI             = "xai"
	ProviderMiniMax         = "minimax_native"
	ProviderCohere          = "cohere"
	ProviderPerplexity      = "perplexity"
	ProviderDashScope       = "dashscope"
	ProviderOllama          = "ollama"
	ProviderOllamaCloud     = "ollama_cloud"
	ProviderACP             = "acp"
	ProviderNovita          = "novita"
	NovitaDefaultAPIBase    = "https://api.novita.ai/openai"
	NovitaDefaultModel      = "moonshotai/kimi-k2.5"
)

var ValidProviderTypes = map[string]bool{
	ProviderAnthropicNative: true, ProviderOpenAICompat: true, ProviderGeminiNative: true,
	ProviderOpenRouter: true, ProviderGroq: true, ProviderDeepSeek: true,
	ProviderMistral: true, ProviderXAI: true, ProviderMiniMax: true,
	ProviderCohere: true, ProviderPerplexity: true, ProviderDashScope: true,
	ProviderOllama: true, ProviderOllamaCloud: true, ProviderACP: true, ProviderNovita: true,
}

type LLMProviderData struct {
	BaseModel
	TenantID     uuid.UUID       `json:"tenant_id,omitempty"`
	Name         string          `json:"name"`
	DisplayName  string          `json:"display_name,omitempty"`
	ProviderType string          `json:"provider_type"`
	APIBase      string          `json:"api_base,omitempty"`
	APIKey       string          `json:"api_key,omitempty"`
	Enabled      bool            `json:"enabled"`
	Settings     json.RawMessage `json:"settings,omitempty"`
}

const RequiredMemoryEmbeddingDimensions = 1536

type EmbeddingSettings struct {
	Enabled    bool   `json:"enabled"`
	Model      string `json:"model,omitempty"`
	APIBase    string `json:"api_base,omitempty"`
	Dimensions int    `json:"dimensions,omitempty"`
}

type ProviderReasoningConfig struct {
	Effort   string `json:"effort,omitempty"`
	Fallback string `json:"fallback,omitempty"`
}

func ParseEmbeddingSettings(settings json.RawMessage) *EmbeddingSettings {
	if len(settings) == 0 { return nil }
	var s struct { Embedding *EmbeddingSettings `json:"embedding"` }
	if json.Unmarshal(settings, &s) != nil || s.Embedding == nil { return nil }
	return s.Embedding
}

func ParseProviderReasoningConfig(settings json.RawMessage) *ProviderReasoningConfig {
	if len(settings) == 0 { return nil }
	var raw struct { ReasoningDefaults *ProviderReasoningConfig `json:"reasoning_defaults"` }
	if json.Unmarshal(settings, &raw) != nil || raw.ReasoningDefaults == nil { return nil }
	return raw.ReasoningDefaults
}

var NoEmbeddingTypes = map[string]bool{
	ProviderAnthropicNative: true, ProviderACP: true,
}

type ProviderStore interface {
	CreateProvider(ctx context.Context, p *LLMProviderData) error
	GetProvider(ctx context.Context, id uuid.UUID) (*LLMProviderData, error)
	GetProviderByName(ctx context.Context, name string) (*LLMProviderData, error)
	ListProviders(ctx context.Context) ([]LLMProviderData, error)
	ListAllProviders(ctx context.Context) ([]LLMProviderData, error)
	UpdateProvider(ctx context.Context, id uuid.UUID, updates map[string]any) error
	DeleteProvider(ctx context.Context, id uuid.UUID) error
}
