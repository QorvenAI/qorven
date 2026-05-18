// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package providers

import "sort"

// ProviderManifest describes a supported provider type for the UI.
type ProviderManifest struct {
	ID             string          `json:"id"`
	Name           string          `json:"name"`
	Icon           string          `json:"icon"`
	Category       string          `json:"category"`
	AuthType       string          `json:"auth_type"` // api_key, aws_credentials, oauth2, cli_binary, none
	DefaultAPIBase string          `json:"default_api_base"`
	DefaultModel   string          `json:"default_model"`
	Models         []string        `json:"models"`
	Fields         []ManifestField `json:"fields"`
}

type ManifestField struct {
	Name        string `json:"name"`
	Label       string `json:"label"`
	Type        string `json:"type"` // password, text, url, select
	Required    bool   `json:"required"`
	Placeholder string `json:"placeholder,omitempty"`
}

// catalogSupplemental holds per-provider data that ProviderAuthManifest doesn't carry.
type catalogSupplemental struct {
	DefaultModel string
	Models       []string
	Fields       []ManifestField // overrides auto-generated fields when non-nil
	Skip         bool            // de-duplicates sub-entries (e.g. vertex_ai sub-providers)
}

var providerSupplemental = map[string]catalogSupplemental{
	// ── Major cloud ──────────────────────────────────────────────────────────
	// Source: litellm model_prices_and_context_window.json + llmgateway openai.ts
	"openai": {DefaultModel: "gpt-4o", Models: []string{
		// Flagship
		"gpt-4o", "gpt-4o-mini",
		// GPT-4.1 series
		"gpt-4.1", "gpt-4.1-mini", "gpt-4.1-nano",
		// Reasoning
		"o3", "o3-mini", "o4-mini", "o1", "o1-mini",
		// Legacy
		"gpt-4-turbo", "gpt-4", "gpt-3.5-turbo",
		// Codex
		"codex-mini-latest",
		// Search
		"gpt-4o-search-preview", "gpt-4o-mini-search-preview",
	}},
	// Source: litellm + llmgateway anthropic.ts
	"anthropic": {DefaultModel: "claude-sonnet-4-6", Models: []string{
		// Claude 4.x — aliases
		"claude-opus-4-5",
		"claude-opus-4-1",
		"claude-sonnet-4-5",
		"claude-haiku-4-5",
		// Claude 4.x — dated
		"claude-opus-4-7",
		"claude-opus-4-6",
		"claude-opus-4-5-20251101",
		"claude-opus-4-1-20250805",
		"claude-opus-4-20250514",
		"claude-sonnet-4-6",
		"claude-sonnet-4-5-20250929",
		"claude-sonnet-4-20250514",
		"claude-haiku-4-5-20251001",
		// Claude 3.x
		"claude-3-7-sonnet-20250219",
		"claude-3-opus-20240229",
		"claude-3-haiku-20240307",
	}},
	// Source: litellm gemini/ prefix + llmgateway google.ts
	"gemini": {DefaultModel: "gemini-2.5-pro", Models: []string{
		// Gemini 2.5
		"gemini-2.5-pro",
		"gemini-2.5-flash",
		"gemini-2.5-flash-lite",
		// Gemini 2.0
		"gemini-2.0-flash",
		"gemini-2.0-flash-lite",
		// Gemini 1.5
		"gemini-1.5-flash",
		// Gemma 3
		"gemma-3-27b-it",
	}},
	// Source: litellm bedrock/ prefix — model IDs as sent to Bedrock API
	"bedrock": {
		DefaultModel: "anthropic.claude-sonnet-4-6",
		Models: []string{
			// Claude 4.x on Bedrock
			"anthropic.claude-sonnet-4-6",
			"anthropic.claude-opus-4-7",
			"anthropic.claude-opus-4-6-v1",
			"anthropic.claude-opus-4-1-20250805-v1:0",
			"anthropic.claude-opus-4-20250514-v1:0",
			"anthropic.claude-sonnet-4-5-20250929-v1:0",
			"anthropic.claude-sonnet-4-20250514-v1:0",
			"anthropic.claude-haiku-4-5-20251001-v1:0",
			// Claude 3.x on Bedrock
			"anthropic.claude-3-7-sonnet-20250219-v1:0",
			"anthropic.claude-3-5-sonnet-20241022-v2:0",
			"anthropic.claude-3-5-haiku-20241022-v1:0",
			"anthropic.claude-3-opus-20240229-v1:0",
			"anthropic.claude-3-haiku-20240307-v1:0",
			// Amazon Nova
			"amazon.nova-pro-v1:0",
			"amazon.nova-lite-v1:0",
			"amazon.nova-micro-v1:0",
			// Meta Llama 4
			"meta.llama4-maverick-17b-instruct-v1:0",
			"meta.llama4-scout-17b-instruct-v1:0",
			// Meta Llama 3.x
			"meta.llama3-1-405b-instruct-v1:0",
			"meta.llama3-1-70b-instruct-v1:0",
			"meta.llama3-1-8b-instruct-v1:0",
			// Mistral on Bedrock
			"mistral.mistral-large-2407-v1:0",
			"mistral.mistral-small-2402-v1:0",
		},
		Fields: []ManifestField{
			{Name: "aws_access_key", Label: "Access Key ID", Type: "password", Required: false, Placeholder: "AKIA... (or use IAM role)"},
			{Name: "aws_secret_key", Label: "Secret Access Key", Type: "password", Required: false},
			{Name: "aws_region", Label: "Region", Type: "text", Required: true, Placeholder: "us-east-1"},
		},
	},
	"bedrock_converse": {Skip: true},
	"bedrock_mantle":   {Skip: true},
	"azure": {
		DefaultModel: "gpt-4o",
		Fields: []ManifestField{
			{Name: "api_base", Label: "Azure Endpoint", Type: "url", Required: true, Placeholder: "https://your-resource.openai.azure.com"},
			{Name: "api_key", Label: "API Key", Type: "password", Required: true},
			{Name: "api_version", Label: "API Version", Type: "text", Required: false, Placeholder: "2024-02-01"},
		},
	},
	"azure_ai": {
		DefaultModel: "gpt-4o",
		Fields: []ManifestField{
			{Name: "api_base", Label: "AI Foundry Endpoint", Type: "url", Required: true},
			{Name: "api_key", Label: "API Key", Type: "password", Required: true},
		},
	},
	"vertex_ai": {
		DefaultModel: "gemini-2.5-pro",
		Models: []string{
			"gemini-2.5-pro", "gemini-2.5-flash", "gemini-2.5-flash-lite",
			"gemini-2.0-flash", "gemini-1.5-pro", "gemini-1.5-flash",
		},
		Fields: []ManifestField{
			{Name: "api_base", Label: "Vertex AI Endpoint", Type: "url", Required: true, Placeholder: "https://us-central1-aiplatform.googleapis.com/v1"},
			{Name: "api_key", Label: "Service Account Token", Type: "password", Required: true},
		},
	},
	// vertex_ai sub-providers collapsed into the single "vertex_ai" entry
	"vertex_ai-language-models":  {Skip: true},
	"vertex_ai-anthropic_models": {Skip: true},
	"vertex_ai-openai_models":    {Skip: true},
	"vertex_ai-mistral_models":   {Skip: true},
	"vertex_ai-llama_models":     {Skip: true},
	"vertex_ai-deepseek_models":  {Skip: true},
	"vertex_ai-ai21_models":      {Skip: true},
	"vertex_ai-minimax_models":   {Skip: true},
	"vertex_ai-moonshot_models":  {Skip: true},
	"vertex_ai-qwen_models":      {Skip: true},
	"vertex_ai-zai_models":       {Skip: true},
	"palm":                       {Skip: true},

	// ── Fast inference ───────────────────────────────────────────────────────
	// Source: litellm groq/ + llmgateway (groq provider model names)
	"groq": {DefaultModel: "llama-3.3-70b-versatile", Models: []string{
		// Llama 4
		"meta-llama/llama-4-maverick-17b-128e-instruct",
		"meta-llama/llama-4-scout-17b-16e-instruct",
		// Llama 3.x
		"llama-3.3-70b-versatile",
		"llama-3.1-8b-instant",
		"llama3-70b-8192",
		"llama3-8b-8192",
		// OpenAI OSS on Groq (confirmed in Groq docs)
		"openai/gpt-oss-120b",
		"openai/gpt-oss-20b",
		// Qwen (confirmed preview)
		"qwen/qwen3-32b",
		// Other confirmed
		"mixtral-8x7b-32768",
	}},
	// Source: litellm deepseek/ + llmgateway deepseek.ts
	"deepseek": {DefaultModel: "deepseek-chat", Models: []string{
		"deepseek-chat",    // DeepSeek-V3.x series (latest)
		"deepseek-reasoner", // DeepSeek-R1 series
		"deepseek-coder",
	}},
	// Source: litellm mistral/ + llmgateway mistral.ts
	"mistral": {DefaultModel: "mistral-large-latest", Models: []string{
		"mistral-large-latest",
		"mistral-large-2512",
		"mistral-medium-latest",
		"mistral-small-2506",
		"codestral-2508",
		"codestral-latest",
		"devstral-2512",
		"devstral-small-2507",
		"devstral-medium-2507",
		"pixtral-large-2411",
		"ministral-3-14b-2512",
		"ministral-3-8b-2512",
		"ministral-3-3b-2512",
		"open-mistral-nemo",
		"open-codestral-mamba",
	}},
	// Source: litellm xai/ + llmgateway xai.ts
	"xai": {DefaultModel: "grok-3", Models: []string{
		// Grok 4
		"grok-4",
		"grok-4-0709",
		"grok-4-fast-reasoning",
		"grok-4-fast-non-reasoning",
		// Grok 3
		"grok-3",
		"grok-3-mini",
		"grok-3-fast",
		"grok-3-mini-fast",
		// Grok 2
		"grok-2-1212",
		"grok-2-vision-1212",
	}},
	// Source: litellm openrouter/ — uses provider/model format
	"openrouter": {DefaultModel: "anthropic/claude-sonnet-4-6", Models: []string{
		// Anthropic
		"anthropic/claude-sonnet-4-6",
		"anthropic/claude-opus-4-7",
		"anthropic/claude-3-5-sonnet-20241022",
		// OpenAI
		"openai/gpt-4o",
		"openai/gpt-4o-mini",
		"openai/gpt-4.1",
		// Google
		"google/gemini-2.5-pro",
		"google/gemini-2.5-flash",
		// Meta
		"meta-llama/llama-3.3-70b-instruct",
		"meta-llama/llama-4-maverick",
		// DeepSeek
		"deepseek/deepseek-chat",
		"deepseek/deepseek-r1",
		// xAI
		"x-ai/grok-3",
		"x-ai/grok-4",
		// Mistral
		"mistralai/mistral-large",
		// Qwen
		"qwen/qwen3-235b-a22b",
	}},
	// Source: litellm perplexity/ + llmgateway perplexity.ts
	"perplexity": {DefaultModel: "sonar-pro", Models: []string{
		"sonar-pro",
		"sonar",
		"sonar-reasoning-pro",
		"sonar-deep-research",
		"r1-1776",
	}},
	// Source: litellm cohere/ — correct model IDs from Cohere docs
	"cohere_chat": {DefaultModel: "command-a-03-2025", Models: []string{
		"command-a-03-2025",
		"command-r-plus-08-2024",
		"command-r-plus",
		"command-r-08-2024",
		"command-r",
		"command-light",
	}},
	"cohere": {Skip: true},
	// Source: litellm together_ai/ + llmgateway meta.ts/alibaba.ts (together provider names)
	"together_ai": {DefaultModel: "meta-llama/Llama-3.3-70B-Instruct-Turbo", Models: []string{
		// Meta Llama
		"meta-llama/Llama-3.3-70B-Instruct-Turbo",
		"meta-llama/Meta-Llama-3.1-8B-Instruct-Turbo",
		"meta-llama/Meta-Llama-3.1-405B-Instruct-Turbo",
		// DeepSeek
		"deepseek-ai/DeepSeek-V3",
		"deepseek-ai/DeepSeek-R1",
		// Qwen
		"Qwen/Qwen2.5-72B-Instruct-Turbo",
		"Qwen/Qwen3-235B-A22B",
		// Kimi
		"moonshotai/Kimi-K2.5",
		"moonshotai/Kimi-K2.6",
		// GLM
		"zai-org/GLM-5",
		"zai-org/GLM-5.1",
		// MiniMax
		"MiniMaxAI/MiniMax-M2.5",
	}},
	// Source: litellm fireworks_ai/ — uses accounts/fireworks/models/ prefix
	"fireworks_ai": {DefaultModel: "accounts/fireworks/models/llama-v3p3-70b-instruct", Models: []string{
		"accounts/fireworks/models/llama-v3p3-70b-instruct",
		"accounts/fireworks/models/llama-v3p2-3b-instruct",
		"accounts/fireworks/models/llama-v3p1-405b-instruct",
		"accounts/fireworks/models/deepseek-v3",
		"accounts/fireworks/models/deepseek-r1",
		"accounts/fireworks/models/qwen3-8b",
		"accounts/fireworks/models/qwen2p5-72b-instruct",
		"accounts/fireworks/models/mixtral-8x7b-instruct-hf",
	}},
	// Source: litellm deepinfra/
	"deepinfra": {DefaultModel: "meta-llama/Llama-3.3-70B-Instruct", Models: []string{
		"meta-llama/Llama-3.3-70B-Instruct",
		"meta-llama/Meta-Llama-3.1-405B-Instruct",
		"meta-llama/Llama-4-Maverick-17B-128E-Instruct",
		"deepseek-ai/DeepSeek-V3",
		"deepseek-ai/DeepSeek-R1",
		"Qwen/Qwen2.5-72B-Instruct",
		"Qwen/Qwen3-235B-A22B",
		"mistralai/Mistral-7B-Instruct-v0.2",
	}},
	// Source: litellm sambanova/
	"sambanova": {DefaultModel: "Llama-4-Maverick-17B-128E-Instruct", Models: []string{
		"Llama-4-Maverick-17B-128E-Instruct",
		"Meta-Llama-3.3-70B-Instruct",
		"Meta-Llama-3.1-405B-Instruct",
		"DeepSeek-R1",
		"DeepSeek-R1-Distill-Llama-70B",
		"Qwen3-235B-A22B",
	}},
	// Source: litellm cerebras/ + llmgateway (cerebras provider names)
	"cerebras": {DefaultModel: "llama-3.3-70b", Models: []string{
		"llama-3.3-70b",
		"llama3.1-8b",
		"qwen-3-235b-a22b-instruct-2507",
		"qwen-3-32b",
		"zai-glm-4.7",
		"gpt-oss-120b",
	}},
	// Source: litellm hyperbolic/ + llmgateway (nebius/novita provider names)
	"hyperbolic": {DefaultModel: "meta-llama/Llama-3.3-70B-Instruct", Models: []string{
		"meta-llama/Llama-3.3-70B-Instruct",
		"meta-llama/Meta-Llama-3.1-405B-Instruct",
		"deepseek-ai/DeepSeek-V3",
		"deepseek-ai/DeepSeek-R1",
		"deepseek-ai/DeepSeek-R1-0528",
		"Qwen/Qwen2.5-72B-Instruct",
		"Qwen/Qwen3-235B-A22B",
		"Qwen/QwQ-32B",
		"moonshotai/Kimi-K2-Instruct",
	}},
	// Source: litellm lambda_ai/
	"lambda_ai": {DefaultModel: "llama3.3-70b-instruct-fp8", Models: []string{
		"llama3.3-70b-instruct-fp8",
		"llama3.1-405b-instruct-fp8",
		"deepseek-r1-671b",
		"deepseek-v3-0324",
		"qwen3-32b-fp8",
		"llama-4-maverick-17b-128e-instruct-fp8",
		"hermes3-405b",
	}},
	// Source: litellm novita/ + llmgateway (novita provider names)
	"novita": {DefaultModel: "meta-llama/llama-3.3-70b-instruct", Models: []string{
		"meta-llama/llama-3.3-70b-instruct",
		"meta-llama/llama-4-maverick-17b-128e-instruct-fp8",
		"meta-llama/llama-4-scout-17b-16e-instruct",
		"deepseek/deepseek-v3.2",
		"deepseek/deepseek-r1-turbo",
		"deepseek/deepseek-v3-turbo",
		"deepseek/deepseek-v4-flash",
		"qwen/qwen3-235b-a22b-instruct-2507",
		"qwen/qwen3-32b",
		"qwen/qwen3-coder-480b-a35b-instruct",
		"moonshotai/kimi-k2-instruct",
		"moonshotai/kimi-k2.6",
		"minimax/minimax-m2.5",
		"google/gemma-3-27b-it",
		"mistralai/mistral-nemo",
	}},
	// Source: litellm nebius/ + llmgateway (nebius provider names)
	"nebius": {DefaultModel: "meta-llama/Llama-3.3-70B-Instruct", Models: []string{
		"meta-llama/Llama-3.3-70B-Instruct",
		"meta-llama/Meta-Llama-3.1-405B-Instruct",
		"deepseek-ai/DeepSeek-V3",
		"deepseek-ai/DeepSeek-V3.2",
		"deepseek-ai/DeepSeek-R1",
		"deepseek-ai/DeepSeek-R1-0528",
		"Qwen/Qwen3-235B-A22B-Instruct-2507",
		"Qwen/Qwen3-Coder-480B-A35B-Instruct",
		"Qwen/Qwen3-32B",
		"Qwen/Qwen2.5-72B-Instruct",
		"Qwen/QwQ-32B",
		"nvidia/Llama-3_1-Nemotron-Ultra-253B-v1",
		"moonshotai/Kimi-K2-Instruct",
		"moonshotai/Kimi-K2.5",
		"openai/gpt-oss-120b",
		"openai/gpt-oss-20b",
		"MiniMaxAI/MiniMax-M2.5",
		"google/gemma-2-9b-it-fast",
	}},

	// ── Regional / specialised ────────────────────────────────────────────────
	// Source: litellm dashscope/ + llmgateway alibaba.ts
	"dashscope": {DefaultModel: "qwen3-max", Models: []string{
		// Qwen 3.x
		"qwen3-235b-a22b",
		"qwen3-32b",
		"qwen3-30b-a3b",
		"qwen3-max",
		// Qwen Max/Plus/Flash
		"qwen-max",
		"qwen-max-latest",
		"qwen-plus",
		"qwen-plus-latest",
		"qwen-turbo",
		"qwen-turbo-latest",
		// QwQ
		"qwq-32b",
		"qwq-plus",
		// Coder
		"qwen3-coder-plus",
		"qwen-coder-plus",
		// VL
		"qwen-vl-max",
		"qwen-vl-plus",
	}},
	// Source: litellm moonshot/ + llmgateway moonshot.ts
	"moonshot": {DefaultModel: "kimi-k2.5", Models: []string{
		"kimi-k2.5",
		"kimi-k2.6",
		"kimi-k2",
		"kimi-k2-thinking",
		"kimi-latest",
		"moonshot-v1-128k",
		"moonshot-v1-32k",
		"moonshot-v1-8k",
	}},
	// Source: litellm minimax/ + llmgateway minimax.ts
	"minimax": {DefaultModel: "MiniMax-M2.5", Models: []string{
		"MiniMax-M2.7",
		"MiniMax-M2.5",
		"MiniMax-M2.1",
		"MiniMax-M2",
		"MiniMax-Text-01",
	}},
	// Source: llmgateway zai.ts (GLM models from Z AI / Zhipu)
	"zhipu": {DefaultModel: "glm-4.7", Models: []string{
		"glm-5.1",
		"glm-5",
		"glm-4.7",
		"glm-4.6",
		"glm-4.5",
		"glm-4.5-air",
		"glm-4-plus",
	}},
	// Source: litellm ai21/
	"ai21": {DefaultModel: "jamba-1.5-large", Models: []string{
		"jamba-1.5-large",
		"jamba-1.5-mini",
		"jamba-instruct",
	}},
	// ── Enterprise (need api_base) ────────────────────────────────────────────
	"databricks": {
		DefaultModel: "databricks-meta-llama-3-1-70b-instruct",
		Models: []string{
			"databricks-claude-3-7-sonnet",
			"databricks-meta-llama-3-1-70b-instruct",
			"databricks-meta-llama-3-1-405b-instruct",
			"databricks-dbrx-instruct",
			"databricks-mixtral-8x7b-instruct",
		},
		Fields: []ManifestField{
			{Name: "api_base", Label: "Databricks Host", Type: "url", Required: true},
			{Name: "api_key", Label: "Token", Type: "password", Required: true},
		},
	},
	"snowflake": {
		DefaultModel: "snowflake-arctic-instruct",
		Fields: []ManifestField{
			{Name: "api_base", Label: "Snowflake Endpoint", Type: "url", Required: true},
			{Name: "api_key", Label: "Token", Type: "password", Required: true},
		},
	},
	"oci": {
		DefaultModel: "cohere.command-r-plus",
		Fields: []ManifestField{
			{Name: "api_base", Label: "OCI Endpoint", Type: "url", Required: true},
			{Name: "api_key", Label: "Auth Token", Type: "password", Required: true},
		},
	},
	"watsonx": {
		DefaultModel: "ibm/granite-34b-code-instruct",
		Fields: []ManifestField{
			{Name: "api_base", Label: "watsonx Endpoint", Type: "url", Required: true, Placeholder: "https://us-south.ml.cloud.ibm.com"},
			{Name: "api_key", Label: "API Key", Type: "password", Required: true},
		},
	},
	"sagemaker": {
		DefaultModel: "meta.llama3-1-70b-instruct-v1:0",
		Fields: []ManifestField{
			{Name: "aws_access_key", Label: "Access Key ID", Type: "password", Required: true},
			{Name: "aws_secret_key", Label: "Secret Access Key", Type: "password", Required: true},
			{Name: "aws_region", Label: "Region", Type: "text", Required: true, Placeholder: "us-east-1"},
		},
	},

	// ── Local / self-hosted ───────────────────────────────────────────────────
	"ollama": {
		DefaultModel: "llama3.3",
		Fields: []ManifestField{
			{Name: "api_base", Label: "Ollama URL", Type: "url", Required: true, Placeholder: "http://localhost:11434/v1"},
		},
	},

	// ── Misc ──────────────────────────────────────────────────────────────────
	// Source: replicate.com/explore + litellm replicate/
	"replicate": {DefaultModel: "meta/llama-3.3-70b-instruct", Models: []string{
		"meta/llama-3.3-70b-instruct",
		"meta/llama-4-scout",
		"meta/llama-4-maverick",
		"anthropic/claude-3.7-sonnet",
		"mistralai/mixtral-8x7b-instruct-v0.1",
		"google-deepmind/gemma-3-27b-it",
	}, Fields: []ManifestField{
		{Name: "api_key", Label: "API Token", Type: "password", Required: true, Placeholder: "r8_..."},
	}},
	// Source: anyscale endpoints docs
	"anyscale": {DefaultModel: "meta-llama/Llama-3.3-70B-Instruct", Models: []string{
		"meta-llama/Llama-3.3-70B-Instruct",
		"meta-llama/Llama-3.1-8B-Instruct",
		"mistralai/Mixtral-8x7B-Instruct-v0.1",
		"mistralai/Mistral-7B-Instruct-v0.1",
	}},
	// Source: developers.cloudflare.com/workers-ai/models/
	"cloudflare": {DefaultModel: "@cf/meta/llama-3.3-70b-instruct-fp8-fast", Models: []string{
		"@cf/meta/llama-3.3-70b-instruct-fp8-fast",
		"@cf/meta/llama-4-scout-17b-16e-instruct",
		"@cf/meta/llama-3.1-70b-instruct",
		"@cf/meta/llama-3.1-8b-instruct",
		"@cf/deepseek-ai/deepseek-r1-distill-llama-70b",
		"@cf/mistral/mistral-7b-instruct-v0.2",
		"@cf/qwen/qwen2.5-coder-32b-instruct",
		"@cf/google/gemma-3-12b-it",
	}, Fields: []ManifestField{
		{Name: "account_id", Label: "Account ID", Type: "text", Required: true, Placeholder: "Your Cloudflare account ID"},
		{Name: "api_key", Label: "API Token", Type: "password", Required: true},
	}},
	// Source: llama.developer.meta.com/docs
	"meta_llama": {DefaultModel: "Llama-4-Scout-17B-16E-Instruct", Models: []string{
		"Llama-4-Maverick-17B-128E-Instruct-FP8",
		"Llama-4-Scout-17B-16E-Instruct",
		"Llama-3.3-70B-Instruct",
		"Llama-3.1-405B-Instruct-FP8",
		"Llama-3.1-70B-Instruct",
		"Llama-3.1-8B-Instruct",
	}},
	// Source: featherless.ai/models
	"featherless_ai": {DefaultModel: "meta-llama/Llama-3.3-70B-Instruct", Models: []string{
		"meta-llama/Llama-3.3-70B-Instruct",
		"meta-llama/Llama-3.1-405B-Instruct",
		"mistralai/Mistral-Large-Instruct-2411",
		"Qwen/Qwen2.5-72B-Instruct",
		"deepseek-ai/DeepSeek-V3",
	}},
	// Source: docs.friendli.ai
	"friendliai": {DefaultModel: "meta-llama-3.3-70b-instruct", Models: []string{
		"meta-llama-3.3-70b-instruct",
		"meta-llama-3.1-405b-instruct",
		"meta-llama-3.1-70b-instruct",
		"mixtral-8x7b-instruct-v0-1",
		"deepseek-v3",
	}},
	// Source: volcengine ark docs (doubao model IDs — endpoint-based)
	"volcengine": {DefaultModel: "doubao-pro-32k", Models: []string{
		"doubao-pro-32k",
		"doubao-pro-128k",
		"doubao-lite-4k",
		"doubao-lite-32k",
		"doubao-lite-128k",
		"deepseek-v3-241226",
		"deepseek-r1-250120",
	}},
	// morph — small focused provider
	"morph": {DefaultModel: "morph-v3", Models: []string{
		"morph-v3",
		"morph-v2",
	}},
	"sambanova_fast": {DefaultModel: "Meta-Llama-3.3-70B-Instruct"},
}

// StaticModelsForProviderType returns the curated static model list for a given
// provider_type string (as stored in DB). Falls back to empty slice.
// This is used when the live API call fails or returns nothing.
func StaticModelsForProviderType(providerType string) []ModelInfo {
	// Map DB provider_type strings to providerSupplemental keys.
	// TypeXxx constants match the DB values; supplemental keys match litellm/llmgateway IDs.
	key := providerType
	switch providerType {
	case TypeGeminiNative:
		key = "gemini"
	case TypeAnthropicNative:
		key = "anthropic"
	case TypeBedrock:
		key = "bedrock"
	case TypeDashScope:
		key = "dashscope"
	case TypeOpenAICompat:
		key = "openai" // generic OpenAI-compat: show OpenAI models as reference
	case TypeOpenRouter:
		key = "openrouter"
	case TypeGroq:
		key = "groq"
	case TypeDeepSeek:
		key = "deepseek"
	case TypeMistral:
		key = "mistral"
	case TypeXAI:
		key = "xai"
	case TypeTogether:
		key = "together_ai"
	case TypeFireworks:
		key = "fireworks_ai"
	case TypeCohere:
		key = "cohere_chat"
	case TypePerplexity:
		key = "perplexity"
	case TypeMiniMax:
		key = "minimax"
	case TypeMoonshot:
		key = "moonshot"
	case TypeZhipu:
		key = "zhipu"
	case TypeAzureOpenAI:
		key = "azure"
	case TypeAzureAI:
		key = "azure_ai"
	case TypeSnowflake:
		key = "snowflake"
	case TypeCloudflare:
		key = "cloudflare"
	case TypeOllama:
		return nil // Ollama models are instance-specific, can't enumerate statically
	}
	sup, ok := providerSupplemental[key]
	if !ok {
		return nil
	}
	models := make([]ModelInfo, 0, len(sup.Models))
	for _, id := range sup.Models {
		models = append(models, ModelInfo{ID: id, Name: id})
	}
	if len(models) == 0 && sup.DefaultModel != "" {
		models = append(models, ModelInfo{ID: sup.DefaultModel, Name: sup.DefaultModel})
	}
	return models
}

// categoryOrder defines sort priority for provider categories in the LLM section.
var categoryOrder = map[string]int{
	"cloud":         0,
	"openai_compat": 1,
	"local":         2,
	"enterprise":    3,
}

// authTypeToManifestAuthType converts ProviderAuth.AuthType to ProviderManifest.AuthType.
// Each distinct auth flow has its own value so the frontend can render the correct fields.
func authTypeToManifestAuthType(authType string) string {
	switch authType {
	case "aws_sigv4":
		return "aws_credentials"
	case "none":
		return "none"
	case "query_key":
		return "query_key" // key sent as ?key= param (Gemini, PaLM)
	case "api-key":
		return "api_key_header" // Azure: api-key header (not Authorization: Bearer)
	case "snowflake":
		return "snowflake" // Snowflake JWT: Authorization: Snowflake Token="$JWT"
	case "x-api-key":
		return "x_api_key" // Anthropic: x-api-key header
	default:
		return "api_key" // standard Authorization: Bearer
	}
}

// defaultFieldsForAuth generates default ManifestField list for an auth type.
func defaultFieldsForAuth(authType, baseURL string) []ManifestField {
	switch authType {
	case "aws_sigv4":
		return []ManifestField{
			{Name: "aws_access_key", Label: "Access Key ID", Type: "password", Required: false, Placeholder: "AKIA..."},
			{Name: "aws_secret_key", Label: "Secret Access Key", Type: "password", Required: false},
			{Name: "aws_region", Label: "Region", Type: "text", Required: true, Placeholder: "us-east-1"},
		}
	case "none":
		if baseURL != "" {
			return []ManifestField{
				{Name: "api_base", Label: "API Base URL", Type: "url", Required: true, Placeholder: baseURL},
			}
		}
		return nil
	case "api-key":
		// Azure OpenAI: api-key header + endpoint URL required
		return []ManifestField{
			{Name: "api_base", Label: "Azure Endpoint", Type: "url", Required: true, Placeholder: "https://your-resource.openai.azure.com"},
			{Name: "api_key", Label: "API Key", Type: "password", Required: true},
		}
	case "snowflake":
		return []ManifestField{
			{Name: "api_base", Label: "Snowflake Account URL", Type: "url", Required: true, Placeholder: "https://account.snowflakecomputing.com/api/v2/cortex/inference:complete"},
			{Name: "api_key", Label: "Signed JWT Token", Type: "password", Required: true},
		}
	default:
		return []ManifestField{
			{Name: "api_key", Label: "API Key", Type: "password", Required: true},
		}
	}
}

// ProviderCatalog returns all supported provider types.
// LLM providers are generated from ProviderAuthManifest; non-LLM services are hardcoded.
func ProviderCatalog() []ProviderManifest {
	llm := []ProviderManifest{}

	for id, auth := range ProviderAuthManifest {
		sup := providerSupplemental[id]
		if sup.Skip {
			continue
		}

		fields := sup.Fields
		if fields == nil {
			fields = defaultFieldsForAuth(auth.AuthType, auth.BaseURL)
		}

		name := auth.DisplayName
		if name == "" {
			name = id
		}

		llm = append(llm, ProviderManifest{
			ID:             id,
			Name:           name,
			Icon:           auth.Icon,
			Category:       auth.Category,
			AuthType:       authTypeToManifestAuthType(auth.AuthType),
			DefaultAPIBase: auth.BaseURL,
			DefaultModel:   sup.DefaultModel,
			Models:         sup.Models,
			Fields:         fields,
		})
	}

	// Sort by category priority, then alphabetically by name
	sort.Slice(llm, func(i, j int) bool {
		oi := categoryOrder[llm[i].Category]
		oj := categoryOrder[llm[j].Category]
		if oi != oj {
			return oi < oj
		}
		return llm[i].Name < llm[j].Name
	})

	// Special entries not in ProviderAuthManifest (prepended to the LLM list)
	special := []ProviderManifest{
		{
			ID: "custom", Name: "Custom / OpenAI-Compatible", Icon: "server", Category: "custom",
			AuthType: "api_key",
			Fields: []ManifestField{
				{Name: "api_base", Label: "API Endpoint", Type: "url", Required: true, Placeholder: "https://your-server.com/v1"},
				{Name: "api_key", Label: "API Key", Type: "password", Required: false, Placeholder: "sk-... (optional)"},
				{Name: "model", Label: "Model Name", Type: "text", Required: false, Placeholder: "auto-detected, or enter manually"},
				{Name: "input_price", Label: "Input Price ($/1M tokens)", Type: "text", Required: false, Placeholder: "0.00"},
				{Name: "output_price", Label: "Output Price ($/1M tokens)", Type: "text", Required: false, Placeholder: "0.00"},
			},
		},
		{
			ID: "nvidia", Name: "NVIDIA NIM", Icon: "nvidia", Category: "openai_compat",
			AuthType: "api_key", DefaultAPIBase: "https://integrate.api.nvidia.com/v1",
			DefaultModel: "nvidia/nemotron-ultra-253b",
			Models: []string{
				"nvidia/nemotron-ultra-253b", "nvidia/nemotron-3-super-120b-a12b",
				"nvidia/nemotron-nano-30b-a3b", "nvidia/deepseek-v3.2",
				"nvidia/mistral-large-3-675b", "nvidia/qwen3-coder-480b",
				"nvidia/devstral-2-123b", "nvidia/glm-4.7",
				"nvidia/llama-4-maverick", "nvidia/gpt-oss-120b", "nvidia/gpt-oss-20b",
			},
			Fields: []ManifestField{{Name: "api_key", Label: "NVIDIA API Key (free at build.nvidia.com)", Type: "password", Required: true}},
		},
		{
			ID: "lmstudio", Name: "LM Studio (Local)", Icon: "lmstudio", Category: "local",
			AuthType: "none", DefaultAPIBase: "http://localhost:1234/v1", DefaultModel: "local-model",
			Fields: []ManifestField{
				{Name: "api_base", Label: "LM Studio URL", Type: "url", Required: true, Placeholder: "http://localhost:1234/v1"},
			},
		},
		{
			ID: "vllm", Name: "vLLM (Self-hosted)", Icon: "vllm", Category: "local",
			AuthType: "none", DefaultAPIBase: "http://localhost:8000/v1", DefaultModel: "local-model",
			Fields: []ManifestField{
				{Name: "api_base", Label: "vLLM URL", Type: "url", Required: true, Placeholder: "http://localhost:8000/v1"},
			},
		},
	}

	return append(append(special, llm...), nonLLMCatalog()...)
}

// nonLLMCatalog returns search, voice, and data services (not LLM providers).
func nonLLMCatalog() []ProviderManifest {
	apiKeyField := func(label, placeholder string) []ManifestField {
		f := ManifestField{Name: "api_key", Label: label, Type: "password", Required: true}
		if placeholder != "" {
			f.Placeholder = placeholder
		}
		return []ManifestField{f}
	}
	return []ProviderManifest{
		// ─── Search ───
		{ID: "brave-search", Name: "Brave Search", Icon: "brave", Category: "search", AuthType: "api_key", DefaultAPIBase: "https://api.search.brave.com/res/v1/web/search", Fields: apiKeyField("Brave API Key", "BSA...")},
		{ID: "tavily", Name: "Tavily Search", Icon: "tavily", Category: "search", AuthType: "api_key", DefaultAPIBase: "https://api.tavily.com", Fields: apiKeyField("Tavily API Key", "tvly-...")},
		{ID: "exa", Name: "Exa Neural Search", Icon: "exa", Category: "search", AuthType: "api_key", DefaultAPIBase: "https://api.exa.ai", Fields: apiKeyField("Exa API Key", "exa-...")},
		{ID: "searxng", Name: "SearXNG (Self-hosted)", Icon: "searxng", Category: "search", AuthType: "none", DefaultAPIBase: "http://localhost:8080", Fields: []ManifestField{{Name: "api_base", Label: "SearXNG URL", Type: "url", Required: true, Placeholder: "http://localhost:8080"}}},
		{ID: "serper", Name: "Serper (Google Search)", Icon: "google", Category: "search", AuthType: "api_key", DefaultAPIBase: "https://google.serper.dev", Fields: apiKeyField("Serper API Key", "")},
		{ID: "perplexity-search", Name: "Perplexity Search", Icon: "perplexity", Category: "search", AuthType: "api_key", DefaultAPIBase: "https://api.perplexity.ai/search", Fields: apiKeyField("Perplexity API Key", "pplx-...")},
		// ─── Voice ───
		{ID: "elevenlabs", Name: "ElevenLabs TTS", Icon: "elevenlabs", Category: "voice", AuthType: "api_key", DefaultAPIBase: "https://api.elevenlabs.io/v1", Fields: apiKeyField("ElevenLabs API Key", "")},
		{ID: "deepgram", Name: "Deepgram STT", Icon: "deepgram", Category: "voice", AuthType: "api_key", DefaultAPIBase: "https://api.deepgram.com/v1", Fields: apiKeyField("Deepgram API Key", "")},
		// ─── Data & Enrichment ───
		{ID: "maxmind", Name: "MaxMind GeoIP", Icon: "maxmind", Category: "data", AuthType: "api_key", DefaultAPIBase: "https://geoip.maxmind.com/geoip/v2.1", Fields: apiKeyField("MaxMind License Key", "")},
		{ID: "perplexity-embeddings", Name: "Perplexity Embeddings", Icon: "perplexity", Category: "embeddings", AuthType: "api_key", DefaultAPIBase: "https://api.perplexity.ai/embeddings", Models: []string{"pplx-embed"}, Fields: apiKeyField("Perplexity API Key (same key)", "")},
		{ID: "jina", Name: "Jina Reader", Icon: "jina", Category: "data", AuthType: "api_key", DefaultAPIBase: "https://r.jina.ai", Fields: apiKeyField("Jina API Key", "jina_...")},
		{ID: "stability", Name: "Stability AI (Images)", Icon: "stability", Category: "media", AuthType: "api_key", DefaultAPIBase: "https://api.stability.ai/v1", Fields: apiKeyField("Stability API Key", "")},
	}
}
