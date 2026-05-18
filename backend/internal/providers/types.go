// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package providers

import (
	"context"
	"encoding/json"
)

// Provider is the interface all LLM drivers implement.
type Provider interface {
	Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error)
	ChatStream(ctx context.Context, req ChatRequest, onChunk func(StreamChunk)) (*ChatResponse, error)
	DefaultModel() string
	Name() string
}

// ThinkingCapable is optionally implemented by providers that support reasoning.
type ThinkingCapable interface {
	SupportsThinking() bool
}

// ChatRequest is the input for a Chat/ChatStream call.
type ChatRequest struct {
	Messages []Message        `json:"messages"`
	Tools      []ToolDefinition `json:"tools,omitempty"`
	ToolChoice string           `json:"tool_choice,omitempty"`
	Model    string           `json:"model,omitempty"`
	Options  map[string]any   `json:"options,omitempty"`
}

// ChatResponse is the result from an LLM call.
type ChatResponse struct {
	Content      string          `json:"content"`
	Thinking     string          `json:"thinking,omitempty"`
	ToolCalls    []ToolCall      `json:"tool_calls,omitempty"`
	FinishReason string          `json:"finish_reason"`
	Usage        *Usage          `json:"usage,omitempty"`
	RawContent   json.RawMessage `json:"-"`
	// Provider tracing fields — populated when available (currently OpenAI-compatible).
	RequestID          string `json:"request_id,omitempty"`
	RateLimitRemaining int    `json:"rate_limit_remaining,omitempty"`
}

// StreamChunk is a piece of a streaming response.
type StreamChunk struct {
	Content  string `json:"content,omitempty"`
	Thinking string `json:"thinking,omitempty"`
	Done     bool   `json:"done,omitempty"`
}

// Message represents a conversation message.
type Message struct {
	Role         string         `json:"role"`
	Content      string         `json:"content"`
	Thinking     string         `json:"thinking,omitempty"`
	Images       []ImageContent `json:"images,omitempty"`
	ToolCalls    []ToolCall     `json:"tool_calls,omitempty"`
	ToolCallID   string         `json:"tool_call_id,omitempty"`
	CacheControl string         `json:"cache_control,omitempty"` // "ephemeral" for Anthropic caching
}

type ImageContent struct {
	MimeType string `json:"mime_type"`
	Data     string `json:"data"`
}

type ToolCall struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	// ArgsParseError is non-empty when the raw arguments JSON failed
	// to decode (usually a truncated stream). Callers must treat this
	// as a failed tool call instead of executing with empty args —
	// see providers/toolcall_parse.go for the rationale.
	ArgsParseError string `json:"args_parse_error,omitempty"`
}

type ToolDefinition struct {
	Type     string             `json:"type"`
	Function ToolFunctionSchema `json:"function"`
}

type ToolFunctionSchema struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
	Strict      *bool          `json:"strict,omitempty"`
}

type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
	ThinkingTokens   int `json:"thinking_tokens,omitempty"`
	// Anthropic prompt-cache fields — zero on non-Anthropic providers
	CacheCreationTokens int `json:"cache_creation_tokens,omitempty"`
	CacheReadTokens     int `json:"cache_read_tokens,omitempty"`
}

// Provider type constants.
const (
	TypeOpenAICompat    = "openai_compat"
	TypeAnthropicNative = "anthropic_native"
	TypeGeminiNative    = "gemini_native"
	TypeOpenRouter      = "openrouter"
	TypeGroq            = "groq"
	TypeDeepSeek        = "deepseek"
	TypeMistral         = "mistral"
	TypeXAI             = "xai"
	TypeOllama          = "ollama"
	TypeTogether        = "together"
	TypeDashScope       = "dashscope"
	TypeLlamaSwap       = "llama_swap"
	TypeBedrock         = "bedrock"
	TypeFireworks       = "fireworks"
	TypeCohere          = "cohere"
	TypePerplexity      = "perplexity"
	TypeMiniMax         = "minimax"
	TypeMoonshot        = "moonshot"
	TypeZhipu           = "zhipu"
	TypeAzureOpenAI     = "azure"
	TypeAzureAI         = "azure_ai"
	TypeSnowflake       = "snowflake"
	TypeCloudflare      = "cloudflare"
	TypeSageMaker       = "sagemaker"
	TypeBedrockConverse = "bedrock_converse"
	TypeBedrockMantle   = "bedrock_mantle"
)

// ValidProviderTypes is kept for backwards-compatibility.
// New code should call IsValidProviderType() from auth_manifest.go instead,
// which is driven from ProviderAuthManifest and handles all 60+ providers.
var ValidProviderTypes = map[string]bool{
	TypeOpenAICompat: true, TypeAnthropicNative: true, TypeGeminiNative: true,
	TypeOpenRouter: true, TypeGroq: true, TypeDeepSeek: true, TypeMistral: true,
	TypeXAI: true, TypeOllama: true, TypeTogether: true, TypeFireworks: true,
	TypeCohere: true, TypePerplexity: true, TypeMiniMax: true, TypeMoonshot: true,
	TypeZhipu: true, TypeBedrock: true, TypeDashScope: true,
	TypeAzureOpenAI: true, TypeAzureAI: true, TypeSnowflake: true, TypeCloudflare: true,
	TypeSageMaker: true, TypeBedrockConverse: true, TypeBedrockMantle: true,
}

// IsValidName checks provider name is a safe slug (lowercase alphanumeric + hyphens).
func IsValidName(name string) bool {
	if name == "" || len(name) > 100 {
		return false
	}
	for _, c := range name {
		if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' || c == '_') {
			return false
		}
	}
	return true
}

// ProviderCapabilityFlags are stored in the providers.capabilities column.
// The smart router reads these when selecting a model for a given request.
type ProviderCapabilityFlags struct {
	Streaming     bool `json:"streaming"`
	Caching       bool `json:"caching"`       // prompt/context caching support
	Thinking      bool `json:"thinking"`       // extended reasoning / thinking tokens
	Vision        bool `json:"vision"`
	Tools         bool `json:"tools"`
	ParallelTools bool `json:"parallel_tools"`
}

// ProviderConfig is the DB/config representation.
type ProviderConfig struct {
	ID           string                  `json:"id"`
	Name         string                  `json:"name"`
	DisplayName  string                  `json:"display_name,omitempty"`
	ProviderType string                  `json:"provider_type"`
	APIBase      string                  `json:"api_base,omitempty"`
	APIKey       string                  `json:"api_key,omitempty"`
	Enabled      bool                    `json:"enabled"`
	Settings     json.RawMessage         `json:"settings,omitempty"`
	Capabilities ProviderCapabilityFlags `json:"capabilities"`
	// AWS static credentials — used when Bedrock is configured outside EC2
	// (no IMDS role). If empty, falls back to the standard AWS credential chain.
	AWSAccessKey    string `json:"aws_access_key,omitempty"`
	AWSSecretKey    string `json:"aws_secret_key,omitempty"`
	AWSSessionToken string `json:"aws_session_token,omitempty"`
}

// DefaultAPIBase returns the default API base URL for a provider type.
// Checks ProviderAuthForType (manifest-driven) first, then falls back to
// the legacy switch for any types not yet in the manifest.
func DefaultAPIBase(providerType string) string {
	if auth, ok := ProviderAuthForType(providerType); ok && auth.BaseURL != "" {
		return auth.BaseURL
	}
	// Fallback for driver types without a manifest entry
	switch providerType {
	case TypeOpenAICompat:
		return "https://api.openai.com/v1"
	case TypeAnthropicNative:
		return "https://api.anthropic.com"
	case TypeGeminiNative:
		return "https://generativelanguage.googleapis.com/v1beta"
	}
	return ""
}
