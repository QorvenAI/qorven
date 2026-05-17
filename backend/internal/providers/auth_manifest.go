// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package providers

// ProviderAuth describes how to authenticate with a provider and which
// internal driver to use. Keys are litellm_provider strings (the "Provider"
// field in ModelRegistry) so any model in the registry can be resolved to
// its auth config via LookupProviderForModel.
//
// AuthType values:
//   bearer        — Authorization: Bearer {api_key}
//   x-api-key     — x-api-key: {api_key} header (Anthropic)
//   query_key     — ?key={api_key} query param (Gemini native)
//   aws_sigv4     — AWS SigV4 signing (Bedrock)
//   none          — no auth (local endpoints)
//
// Driver values map to the NewProvider() switch:
//   openai_compat    — openai.go (handles all OpenAI-compatible endpoints)
//   anthropic_native — anthropic.go
//   gemini_native    — gemini.go
//   bedrock          — bedrock.go
//   dashscope        — dashscope.go
type ProviderAuth struct {
	AuthType string // bearer, x-api-key, query_key, aws_sigv4, none
	BaseURL  string // default API endpoint; empty = driver default
	Driver   string // openai_compat | anthropic_native | gemini_native | bedrock | dashscope
	// Icon is the filename stem under /icons/providers/ (e.g. "openai" → openai.webp)
	Icon string
	// DisplayName is shown in the UI when different from the map key
	DisplayName string
	// Category groups providers in the UI
	Category string // cloud | openai_compat | local | enterprise
}

// ProviderAuthManifest maps litellm_provider identifiers to their auth config.
// Sources: LiteLLM provider list + individual provider docs.
// Update this when a new provider launches; no other code change needed.
var ProviderAuthManifest = map[string]ProviderAuth{
	// ── Major cloud LLMs ──────────────────────────────────────────────────────
	"openai": {
		AuthType: "bearer", BaseURL: "https://api.openai.com/v1",
		Driver: TypeOpenAICompat, Icon: "openai", DisplayName: "OpenAI", Category: "cloud",
	},
	"anthropic": {
		AuthType: "x-api-key", BaseURL: "https://api.anthropic.com",
		Driver: TypeAnthropicNative, Icon: "anthropic", DisplayName: "Anthropic", Category: "cloud",
	},
	"gemini": {
		// Driver sends x-goog-api-key header (not ?key= query param — avoids key in logs).
		AuthType: "query_key", BaseURL: "https://generativelanguage.googleapis.com/v1beta",
		Driver: TypeGeminiNative, Icon: "google", DisplayName: "Google Gemini", Category: "cloud",
	},
	"bedrock": {
		AuthType: "aws_sigv4", BaseURL: "",
		Driver: TypeBedrock, Icon: "aws", DisplayName: "AWS Bedrock", Category: "cloud",
	},
	"bedrock_converse": {
		AuthType: "aws_sigv4", BaseURL: "",
		Driver: TypeBedrock, Icon: "aws", DisplayName: "AWS Bedrock", Category: "cloud",
	},
	"bedrock_mantle": {
		AuthType: "aws_sigv4", BaseURL: "",
		Driver: TypeBedrock, Icon: "aws", DisplayName: "AWS Bedrock", Category: "cloud",
	},
	"vertex_ai": {
		AuthType: "bearer", BaseURL: "https://us-central1-aiplatform.googleapis.com/v1",
		Driver: TypeOpenAICompat, Icon: "google", DisplayName: "Google Vertex AI", Category: "cloud",
	},
	"azure": {
		// Azure OpenAI uses api-key header, not Authorization: Bearer.
		// Base URL is customer-specific: https://{resource}.openai.azure.com/openai/deployments/{deployment}
		AuthType: "api-key", BaseURL: "",
		Driver: TypeOpenAICompat, Icon: "azure", DisplayName: "Azure OpenAI", Category: "cloud",
	},
	"azure_ai": {
		AuthType: "bearer", BaseURL: "",
		Driver: TypeOpenAICompat, Icon: "azure", DisplayName: "Azure AI Foundry", Category: "cloud",
	},

	// ── Fast inference / OpenAI-compat ────────────────────────────────────────
	"groq": {
		AuthType: "bearer", BaseURL: "https://api.groq.com/openai/v1",
		Driver: TypeOpenAICompat, Icon: "groq", DisplayName: "Groq", Category: "openai_compat",
	},
	"deepseek": {
		AuthType: "bearer", BaseURL: "https://api.deepseek.com/v1",
		Driver: TypeOpenAICompat, Icon: "deepseek", DisplayName: "DeepSeek", Category: "openai_compat",
	},
	"mistral": {
		AuthType: "bearer", BaseURL: "https://api.mistral.ai/v1",
		Driver: TypeOpenAICompat, Icon: "mistral", DisplayName: "Mistral", Category: "openai_compat",
	},
	"xai": {
		AuthType: "bearer", BaseURL: "https://api.x.ai/v1",
		Driver: TypeOpenAICompat, Icon: "xai", DisplayName: "xAI (Grok)", Category: "openai_compat",
	},
	"openrouter": {
		AuthType: "bearer", BaseURL: "https://openrouter.ai/api/v1",
		Driver: TypeOpenAICompat, Icon: "openrouter", DisplayName: "OpenRouter", Category: "openai_compat",
	},
	"together_ai": {
		AuthType: "bearer", BaseURL: "https://api.together.xyz/v1",
		Driver: TypeOpenAICompat, Icon: "together", DisplayName: "Together AI", Category: "openai_compat",
	},
	"fireworks_ai": {
		AuthType: "bearer", BaseURL: "https://api.fireworks.ai/inference/v1",
		Driver: TypeOpenAICompat, Icon: "fireworks", DisplayName: "Fireworks AI", Category: "openai_compat",
	},
	"perplexity": {
		AuthType: "bearer", BaseURL: "https://api.perplexity.ai",
		Driver: TypeOpenAICompat, Icon: "perplexity", DisplayName: "Perplexity", Category: "openai_compat",
	},
	"cohere_chat": {
		AuthType: "bearer", BaseURL: "https://api.cohere.ai/compatibility/v1",
		Driver: TypeOpenAICompat, Icon: "cohere", DisplayName: "Cohere", Category: "openai_compat",
	},
	"cerebras": {
		AuthType: "bearer", BaseURL: "https://api.cerebras.ai/v1",
		Driver: TypeOpenAICompat, Icon: "cerebras", DisplayName: "Cerebras", Category: "openai_compat",
	},
	"sambanova": {
		AuthType: "bearer", BaseURL: "https://api.sambanova.ai/v1",
		Driver: TypeOpenAICompat, Icon: "sambanova", DisplayName: "SambaNova", Category: "openai_compat",
	},
	"deepinfra": {
		AuthType: "bearer", BaseURL: "https://api.deepinfra.com/v1/openai",
		Driver: TypeOpenAICompat, Icon: "deepinfra", DisplayName: "DeepInfra", Category: "openai_compat",
	},
	"hyperbolic": {
		AuthType: "bearer", BaseURL: "https://api.hyperbolic.xyz/v1",
		Driver: TypeOpenAICompat, Icon: "hyperbolic", DisplayName: "Hyperbolic", Category: "openai_compat",
	},
	"lambda_ai": {
		AuthType: "bearer", BaseURL: "https://api.lambdalabs.com/v1",
		Driver: TypeOpenAICompat, Icon: "lambda", DisplayName: "Lambda", Category: "openai_compat",
	},
	"novita": {
		AuthType: "bearer", BaseURL: "https://api.novita.ai/v3/openai",
		Driver: TypeOpenAICompat, Icon: "novita", DisplayName: "Novita AI", Category: "openai_compat",
	},
	"nebius": {
		AuthType: "bearer", BaseURL: "https://api.studio.nebius.ai/v1",
		Driver: TypeOpenAICompat, Icon: "nebius", DisplayName: "Nebius AI Studio", Category: "openai_compat",
	},
	"featherless_ai": {
		AuthType: "bearer", BaseURL: "https://api.featherless.ai/v1",
		Driver: TypeOpenAICompat, Icon: "featherless", DisplayName: "Featherless AI", Category: "openai_compat",
	},
	"friendliai": {
		AuthType: "bearer", BaseURL: "https://inference.friendli.ai/v1",
		Driver: TypeOpenAICompat, Icon: "friendliai", DisplayName: "FriendliAI", Category: "openai_compat",
	},

	// ── Regional / specialised ────────────────────────────────────────────────
	"dashscope": {
		AuthType: "bearer", BaseURL: "https://dashscope-intl.aliyuncs.com/compatible-mode/v1",
		Driver: TypeDashScope, Icon: "alibaba", DisplayName: "Alibaba (Qwen)", Category: "openai_compat",
	},
	"moonshot": {
		AuthType: "bearer", BaseURL: "https://api.moonshot.cn/v1",
		Driver: TypeOpenAICompat, Icon: "moonshot", DisplayName: "Moonshot (Kimi)", Category: "openai_compat",
	},
	"minimax": {
		AuthType: "bearer", BaseURL: "https://api.minimax.io/v1",
		Driver: TypeOpenAICompat, Icon: "minimax", DisplayName: "MiniMax", Category: "openai_compat",
	},
	"zhipu": {
		AuthType: "bearer", BaseURL: "https://open.bigmodel.cn/api/paas/v4",
		Driver: TypeOpenAICompat, Icon: "zhipu", DisplayName: "Zhipu (GLM)", Category: "openai_compat",
	},
	"ai21": {
		AuthType: "bearer", BaseURL: "https://api.ai21.com/studio/v1",
		Driver: TypeOpenAICompat, Icon: "ai21", DisplayName: "AI21 Labs", Category: "openai_compat",
	},
	"cohere": {
		AuthType: "bearer", BaseURL: "https://api.cohere.ai/compatibility/v1",
		Driver: TypeOpenAICompat, Icon: "cohere", DisplayName: "Cohere", Category: "openai_compat",
	},
	"replicate": {
		AuthType: "bearer", BaseURL: "https://api.replicate.com/v1",
		Driver: TypeOpenAICompat, Icon: "replicate", DisplayName: "Replicate", Category: "openai_compat",
	},
	"anyscale": {
		AuthType: "bearer", BaseURL: "https://api.endpoints.anyscale.com/v1",
		Driver: TypeOpenAICompat, Icon: "anyscale", DisplayName: "Anyscale", Category: "openai_compat",
	},
	"cloudflare": {
		// BaseURL is account-specific: https://api.cloudflare.com/client/v4/accounts/{ACCOUNT_ID}/ai/v1
		// Leave empty so users are prompted to supply their full endpoint or set settings.account_id.
		AuthType: "bearer", BaseURL: "",
		Driver: TypeCloudflare, Icon: "cloudflare", DisplayName: "Cloudflare AI", Category: "openai_compat",
	},
	"databricks": {
		AuthType: "bearer", BaseURL: "",
		Driver: TypeOpenAICompat, Icon: "databricks", DisplayName: "Databricks", Category: "enterprise",
	},
	"snowflake": {
		// Snowflake Cortex uses: Authorization: Snowflake Token="$JWT" + X-Snowflake-Authorization-Token-Type: KEYPAIR_JWT
		// API key field stores the signed JWT generated from the user's RSA private key.
		AuthType: "snowflake", BaseURL: "",
		Driver: TypeOpenAICompat, Icon: "snowflake", DisplayName: "Snowflake Arctic", Category: "enterprise",
	},
	"watsonx": {
		AuthType: "bearer", BaseURL: "https://us-south.ml.cloud.ibm.com",
		Driver: TypeOpenAICompat, Icon: "ibm", DisplayName: "IBM watsonx", Category: "enterprise",
	},
	"sagemaker": {
		AuthType: "aws_sigv4", BaseURL: "",
		Driver: TypeOpenAICompat, Icon: "aws", DisplayName: "AWS SageMaker", Category: "enterprise",
	},
	"oci": {
		AuthType: "bearer", BaseURL: "",
		Driver: TypeOpenAICompat, Icon: "oracle", DisplayName: "Oracle OCI", Category: "enterprise",
	},
	"volcengine": {
		AuthType: "bearer", BaseURL: "https://ark.cn-beijing.volces.com/api/v3",
		Driver: TypeOpenAICompat, Icon: "volcengine", DisplayName: "ByteDance (Doubao)", Category: "openai_compat",
	},
	"morph": {
		AuthType: "bearer", BaseURL: "https://api.morphllm.com/v1",
		Driver: TypeOpenAICompat, Icon: "morph", DisplayName: "Morph", Category: "openai_compat",
	},

	// ── Local / self-hosted ───────────────────────────────────────────────────
	"ollama": {
		AuthType: "none", BaseURL: "http://localhost:11434/v1",
		Driver: TypeOpenAICompat, Icon: "ollama", DisplayName: "Ollama", Category: "local",
	},
	"meta_llama": {
		AuthType: "bearer", BaseURL: "https://api.llama.com/compat/v1",
		Driver: TypeOpenAICompat, Icon: "meta", DisplayName: "Meta Llama API", Category: "openai_compat",
	},

	// ── Vertex AI sub-providers (all route through openai_compat) ─────────────
	"vertex_ai-language-models": {
		AuthType: "bearer", BaseURL: "https://us-central1-aiplatform.googleapis.com/v1",
		Driver: TypeOpenAICompat, Icon: "google", DisplayName: "Google Vertex AI", Category: "cloud",
	},
	"vertex_ai-anthropic_models": {
		// Vertex AI uses Google OAuth bearer tokens for all hosted models, including Anthropic.
		// The endpoint speaks the Anthropic Messages API wire format, so keep AnthropicNative driver.
		AuthType: "bearer", BaseURL: "https://us-central1-aiplatform.googleapis.com/v1",
		Driver: TypeAnthropicNative, Icon: "google", DisplayName: "Anthropic on Vertex", Category: "cloud",
	},
	"vertex_ai-openai_models": {
		AuthType: "bearer", BaseURL: "https://us-central1-aiplatform.googleapis.com/v1",
		Driver: TypeOpenAICompat, Icon: "google", DisplayName: "OpenAI on Vertex", Category: "cloud",
	},
	"vertex_ai-mistral_models": {
		AuthType: "bearer", BaseURL: "https://us-central1-aiplatform.googleapis.com/v1",
		Driver: TypeOpenAICompat, Icon: "google", DisplayName: "Mistral on Vertex", Category: "cloud",
	},
	"vertex_ai-llama_models": {
		AuthType: "bearer", BaseURL: "https://us-central1-aiplatform.googleapis.com/v1",
		Driver: TypeOpenAICompat, Icon: "google", DisplayName: "Llama on Vertex", Category: "cloud",
	},
	"vertex_ai-deepseek_models": {
		AuthType: "bearer", BaseURL: "https://us-central1-aiplatform.googleapis.com/v1",
		Driver: TypeOpenAICompat, Icon: "google", DisplayName: "DeepSeek on Vertex", Category: "cloud",
	},
	"vertex_ai-ai21_models": {
		AuthType: "bearer", BaseURL: "https://us-central1-aiplatform.googleapis.com/v1",
		Driver: TypeOpenAICompat, Icon: "google", DisplayName: "AI21 on Vertex", Category: "cloud",
	},
	"vertex_ai-minimax_models": {
		AuthType: "bearer", BaseURL: "https://us-central1-aiplatform.googleapis.com/v1",
		Driver: TypeOpenAICompat, Icon: "google", DisplayName: "MiniMax on Vertex", Category: "cloud",
	},
	"vertex_ai-moonshot_models": {
		AuthType: "bearer", BaseURL: "https://us-central1-aiplatform.googleapis.com/v1",
		Driver: TypeOpenAICompat, Icon: "google", DisplayName: "Moonshot on Vertex", Category: "cloud",
	},
	"vertex_ai-qwen_models": {
		AuthType: "bearer", BaseURL: "https://us-central1-aiplatform.googleapis.com/v1",
		Driver: TypeOpenAICompat, Icon: "google", DisplayName: "Qwen on Vertex", Category: "cloud",
	},
	"vertex_ai-zai_models": {
		AuthType: "bearer", BaseURL: "https://us-central1-aiplatform.googleapis.com/v1",
		Driver: TypeOpenAICompat, Icon: "google", DisplayName: "ZAI on Vertex", Category: "cloud",
	},
	"palm": {
		AuthType: "query_key", BaseURL: "https://generativelanguage.googleapis.com/v1beta",
		Driver: TypeGeminiNative, Icon: "google", DisplayName: "Google PaLM (legacy)", Category: "cloud",
	},
}

// LookupProviderForModel resolves a model name to its ProviderAuth config.
// Chains ModelRegistry (model → litellm_provider) → ProviderAuthManifest (provider → auth).
func LookupProviderForModel(model string) (ProviderAuth, bool) {
	spec, ok := ModelRegistry[model]
	if !ok {
		return ProviderAuth{}, false
	}
	auth, ok := ProviderAuthManifest[spec.Provider]
	return auth, ok
}

// DriverForProviderType returns the canonical driver string for a provider_type constant.
// Used when resolving from a stored provider_type rather than a model name.
func DriverForProviderType(pt string) string {
	switch pt {
	case TypeAnthropicNative:
		return TypeAnthropicNative
	case TypeGeminiNative:
		return TypeGeminiNative
	case TypeBedrock:
		return TypeBedrock
	case TypeBedrockConverse:
		return TypeBedrockConverse
	case TypeBedrockMantle:
		return TypeBedrockMantle
	case TypeSageMaker:
		return TypeSageMaker
	case TypeDashScope:
		return TypeDashScope
	case TypeLlamaSwap:
		return TypeLlamaSwap
	default:
		return TypeOpenAICompat
	}
}

// IsValidProviderType returns true if the given string is an accepted provider_type.
// Replaces the old hardcoded ValidProviderTypes map — now driven from the manifest
// plus the explicit driver types that don't have a litellm_provider equivalent.
func IsValidProviderType(pt string) bool {
	// Explicit driver types always valid
	switch pt {
	case TypeOpenAICompat, TypeAnthropicNative, TypeGeminiNative,
		TypeBedrock, TypeBedrockConverse, TypeBedrockMantle, TypeSageMaker,
		TypeDashScope, TypeLlamaSwap:
		return true
	}
	// Any manifest-listed provider type is also valid
	for _, a := range ProviderAuthManifest {
		if a.Driver == pt {
			return true
		}
	}
	// Legacy named types (groq, deepseek, etc.) that map to openai_compat driver
	// are kept valid for backwards-compat with existing DB rows.
	legacyTypes := map[string]bool{
		TypeOpenRouter: true, TypeGroq: true, TypeDeepSeek: true, TypeMistral: true,
		TypeXAI: true, TypeOllama: true, TypeTogether: true, TypeFireworks: true,
		TypeCohere: true, TypePerplexity: true, TypeMiniMax: true, TypeMoonshot: true,
		TypeZhipu: true, TypeAzureOpenAI: true, TypeAzureAI: true, TypeSnowflake: true, TypeCloudflare: true,
	}
	return legacyTypes[pt]
}

// ProviderAuthForType resolves a stored provider_type string to a ProviderAuth.
// Used when loading an existing provider from DB (which has provider_type, not model).
func ProviderAuthForType(pt string) (ProviderAuth, bool) {
	// Direct type → manifest lookup by searching for matching driver+baseurl
	// For named types like "groq", "deepseek" etc. we have explicit entries.
	// Map legacy type constants to manifest keys.
	legacyToManifestKey := map[string]string{
		TypeOpenRouter:  "openrouter",
		TypeGroq:        "groq",
		TypeDeepSeek:    "deepseek",
		TypeMistral:     "mistral",
		TypeXAI:         "xai",
		TypeOllama:      "ollama",
		TypeTogether:    "together_ai",
		TypeFireworks:   "fireworks_ai",
		TypeCohere:      "cohere_chat",
		TypePerplexity:  "perplexity",
		TypeMiniMax:     "minimax",
		TypeMoonshot:    "moonshot",
		TypeZhipu:       "zhipu",
		TypeDashScope:   "dashscope",
		TypeBedrock:     "bedrock",
		TypeAzureOpenAI: "azure",
		TypeAzureAI:     "azure_ai",
		TypeSnowflake:   "snowflake",
		TypeCloudflare:  "cloudflare",
	}
	if key, ok := legacyToManifestKey[pt]; ok {
		auth, ok := ProviderAuthManifest[key]
		return auth, ok
	}
	// For openai_compat, anthropic_native etc. return synthetic entries
	switch pt {
	case TypeOpenAICompat:
		return ProviderAuth{AuthType: "bearer", Driver: TypeOpenAICompat, Category: "openai_compat"}, true
	case TypeAnthropicNative:
		auth, ok := ProviderAuthManifest["anthropic"]
		return auth, ok
	case TypeGeminiNative:
		auth, ok := ProviderAuthManifest["gemini"]
		return auth, ok
	}
	return ProviderAuth{}, false
}
