// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

package providers

// AuthKind describes the credential shape for a provider.
type AuthKind string

const (
	AuthKindAPIKey      AuthKind = "api_key"       // Bearer or x-api-key header
	AuthKindAWSCreds    AuthKind = "aws_creds"      // region + access_key + secret_key
	AuthKindAzureAPIKey AuthKind = "azure_api_key"  // api-key header + endpoint + api-version
	AuthKindOAuth2      AuthKind = "oauth2"         // access_token from OAuth flow
	AuthKindNone        AuthKind = "none"           // local endpoint, no credentials
)

// AuthField defines a single credential field shown in the provider key wizard.
type AuthField struct {
	Key         string   `json:"key"`                   // internal field name, e.g. "api_key"
	Label       string   `json:"label"`                 // UI label, e.g. "API Key"
	Type        string   `json:"type"`                  // "password" | "text" | "select" | "oauth"
	Required    bool     `json:"required"`
	Placeholder string   `json:"placeholder,omitempty"` // example format hint
	HelpText    string   `json:"help_text,omitempty"`   // shown below the field
	HelpURL     string   `json:"help_url,omitempty"`    // "Get key ↗" link destination
	KeyPrefixes []string `json:"key_prefixes,omitempty"` // validated on save; empty = no check
	Options     []string `json:"options,omitempty"`     // for type "select"
}

// ProviderAuthProfile is the complete credential spec for one provider type.
type ProviderAuthProfile struct {
	Kind   AuthKind    `json:"kind"`
	Fields []AuthField `json:"fields"`
}

// AuthProfiles maps provider type constant → its credential profile.
// The frontend fetches this at /v1/providers/auth-profiles and renders
// the add-key wizard fields dynamically — no hardcoded PRESETS needed.
var AuthProfiles = map[string]ProviderAuthProfile{
	// ── API-key providers ─────────────────────────────────────────────────────
	TypeAnthropicNative: {Kind: AuthKindAPIKey, Fields: []AuthField{{
		Key: "api_key", Label: "API Key", Type: "password", Required: true,
		Placeholder: "sk-ant-…",
		HelpURL:     "https://console.anthropic.com/settings/keys",
		KeyPrefixes: []string{"sk-ant-"},
	}}},

	TypeOpenAICompat: {Kind: AuthKindAPIKey, Fields: []AuthField{{
		Key: "api_key", Label: "API Key", Type: "password", Required: true,
		Placeholder: "sk-proj-… or sk-…",
		HelpURL:     "https://platform.openai.com/api-keys",
		KeyPrefixes: []string{"sk-proj-", "sk-"},
	}}},

	TypeGeminiNative: {Kind: AuthKindAPIKey, Fields: []AuthField{{
		Key: "api_key", Label: "API Key", Type: "password", Required: true,
		Placeholder: "AIza…",
		HelpURL:     "https://aistudio.google.com/app/apikey",
		KeyPrefixes: []string{"AIza"},
	}}},

	TypeGroq: {Kind: AuthKindAPIKey, Fields: []AuthField{{
		Key: "api_key", Label: "API Key", Type: "password", Required: true,
		Placeholder: "gsk_…",
		HelpURL:     "https://console.groq.com/keys",
		KeyPrefixes: []string{"gsk_"},
	}}},

	TypeDeepSeek: {Kind: AuthKindAPIKey, Fields: []AuthField{{
		Key: "api_key", Label: "API Key", Type: "password", Required: true,
		Placeholder: "sk-…",
		HelpURL:     "https://platform.deepseek.com/api_keys",
		KeyPrefixes: []string{"sk-"},
	}}},

	TypeMistral: {Kind: AuthKindAPIKey, Fields: []AuthField{{
		Key: "api_key", Label: "API Key", Type: "password", Required: true,
		Placeholder: "…",
		HelpURL:     "https://console.mistral.ai/api-keys",
	}}},

	TypeXAI: {Kind: AuthKindAPIKey, Fields: []AuthField{{
		Key: "api_key", Label: "API Key", Type: "password", Required: true,
		Placeholder: "xai-…",
		HelpURL:     "https://console.x.ai/team/default/api-keys",
		KeyPrefixes: []string{"xai-"},
	}}},

	TypeOpenRouter: {Kind: AuthKindAPIKey, Fields: []AuthField{{
		Key: "api_key", Label: "API Key", Type: "password", Required: true,
		Placeholder: "sk-or-v1-…",
		HelpURL:     "https://openrouter.ai/keys",
		KeyPrefixes: []string{"sk-or-"},
	}}},

	TypeTogether: {Kind: AuthKindAPIKey, Fields: []AuthField{{
		Key: "api_key", Label: "API Key", Type: "password", Required: true,
		Placeholder: "…",
		HelpURL:     "https://api.together.ai/settings/api-keys",
	}}},

	TypeFireworks: {Kind: AuthKindAPIKey, Fields: []AuthField{{
		Key: "api_key", Label: "API Key", Type: "password", Required: true,
		Placeholder: "fw_…",
		HelpURL:     "https://fireworks.ai/account/api-keys",
		KeyPrefixes: []string{"fw_"},
	}}},

	TypeCohere: {Kind: AuthKindAPIKey, Fields: []AuthField{{
		Key: "api_key", Label: "API Key", Type: "password", Required: true,
		Placeholder: "…",
		HelpURL:     "https://dashboard.cohere.com/api-keys",
	}}},

	TypePerplexity: {Kind: AuthKindAPIKey, Fields: []AuthField{{
		Key: "api_key", Label: "API Key", Type: "password", Required: true,
		Placeholder: "pplx-…",
		HelpURL:     "https://www.perplexity.ai/settings/api",
		KeyPrefixes: []string{"pplx-"},
	}}},

	TypeMiniMax: {Kind: AuthKindAPIKey, Fields: []AuthField{{
		Key: "api_key", Label: "API Key", Type: "password", Required: true,
		Placeholder: "…",
		HelpURL:     "https://platform.minimaxi.com/user-center/basic-information/interface-key",
	}}},

	TypeMoonshot: {Kind: AuthKindAPIKey, Fields: []AuthField{{
		Key: "api_key", Label: "API Key", Type: "password", Required: true,
		Placeholder: "sk-…",
		HelpURL:     "https://platform.moonshot.cn/console/api-keys",
		KeyPrefixes: []string{"sk-"},
	}}},

	TypeZhipu: {Kind: AuthKindAPIKey, Fields: []AuthField{{
		Key: "api_key", Label: "API Key", Type: "password", Required: true,
		Placeholder: "…",
		HelpURL:     "https://open.bigmodel.cn/usercenter/apikeys",
	}}},

	TypeDashScope: {Kind: AuthKindAPIKey, Fields: []AuthField{{
		Key: "api_key", Label: "API Key", Type: "password", Required: true,
		Placeholder: "sk-…",
		HelpURL:     "https://help.aliyun.com/zh/dashscope/developer-reference/api-key-management",
		KeyPrefixes: []string{"sk-"},
	}}},

	TypeSnowflake: {Kind: AuthKindAPIKey, Fields: []AuthField{{
		Key: "api_key", Label: "API Key", Type: "password", Required: true,
		Placeholder: "…",
		HelpURL:     "https://docs.snowflake.com/en/user-guide/snowflake-cortex/cortex-llm-rest-api",
	}}},

	TypeCloudflare: {Kind: AuthKindAPIKey, Fields: []AuthField{{
		Key: "api_key", Label: "API Token", Type: "password", Required: true,
		Placeholder: "…",
		HelpURL:     "https://developers.cloudflare.com/fundamentals/api/get-started/create-token/",
	}}},

	// ── AWS providers ─────────────────────────────────────────────────────────
	TypeBedrock: {Kind: AuthKindAWSCreds, Fields: []AuthField{
		{
			Key: "region", Label: "AWS Region", Type: "text", Required: true,
			Placeholder: "us-east-1",
			HelpURL:     "https://docs.aws.amazon.com/general/latest/gr/bedrock.html",
		},
		{
			Key: "access_key", Label: "Access Key ID", Type: "text", Required: true,
			Placeholder: "AKIA…",
			KeyPrefixes: []string{"AKIA", "ASIA"},
		},
		{
			Key: "secret_key", Label: "Secret Access Key", Type: "password", Required: true,
			Placeholder: "…",
		},
	}},

	TypeBedrockConverse: {Kind: AuthKindAWSCreds, Fields: []AuthField{
		{
			Key: "region", Label: "AWS Region", Type: "text", Required: true,
			Placeholder: "us-east-1",
			HelpURL:     "https://docs.aws.amazon.com/general/latest/gr/bedrock.html",
		},
		{
			Key: "access_key", Label: "Access Key ID", Type: "text", Required: true,
			Placeholder: "AKIA…",
			KeyPrefixes: []string{"AKIA", "ASIA"},
		},
		{
			Key: "secret_key", Label: "Secret Access Key", Type: "password", Required: true,
			Placeholder: "…",
		},
	}},

	TypeBedrockMantle: {Kind: AuthKindAWSCreds, Fields: []AuthField{
		{
			Key: "region", Label: "AWS Region", Type: "text", Required: true,
			Placeholder: "us-east-1",
			HelpURL:     "https://docs.aws.amazon.com/general/latest/gr/bedrock.html",
		},
		{
			Key: "access_key", Label: "Access Key ID", Type: "text", Required: true,
			Placeholder: "AKIA…",
			KeyPrefixes: []string{"AKIA", "ASIA"},
		},
		{
			Key: "secret_key", Label: "Secret Access Key", Type: "password", Required: true,
			Placeholder: "…",
		},
	}},

	TypeSageMaker: {Kind: AuthKindAWSCreds, Fields: []AuthField{
		{
			Key: "region", Label: "AWS Region", Type: "text", Required: true,
			Placeholder: "us-east-1",
		},
		{
			Key: "access_key", Label: "Access Key ID", Type: "text", Required: true,
			Placeholder: "AKIA…",
			KeyPrefixes: []string{"AKIA", "ASIA"},
		},
		{
			Key: "secret_key", Label: "Secret Access Key", Type: "password", Required: true,
			Placeholder: "…",
		},
		{
			Key: "endpoint_name", Label: "Endpoint Name", Type: "text", Required: true,
			Placeholder: "my-llm-endpoint",
			HelpURL:     "https://docs.aws.amazon.com/sagemaker/latest/dg/deploy-model.html",
		},
	}},

	// ── Azure ─────────────────────────────────────────────────────────────────
	TypeAzureOpenAI: {Kind: AuthKindAzureAPIKey, Fields: []AuthField{
		{
			Key: "api_key", Label: "API Key", Type: "password", Required: true,
			Placeholder: "…",
			HelpURL:     "https://portal.azure.com/#view/Microsoft_Azure_ProjectOxford/CognitiveServicesHub",
		},
		{
			Key: "endpoint", Label: "Endpoint URL", Type: "text", Required: true,
			Placeholder: "https://<resource>.openai.azure.com/",
		},
		{
			Key: "api_version", Label: "API Version", Type: "text", Required: false,
			Placeholder: "2024-02-01",
		},
	}},

	TypeAzureAI: {Kind: AuthKindAzureAPIKey, Fields: []AuthField{
		{
			Key: "api_key", Label: "API Key", Type: "password", Required: true,
			Placeholder: "…",
			HelpURL:     "https://ai.azure.com/",
		},
		{
			Key: "endpoint", Label: "Endpoint URL", Type: "text", Required: true,
			Placeholder: "https://<resource>.services.ai.azure.com/models",
		},
	}},

	// ── Local / no-auth endpoints ─────────────────────────────────────────────
	TypeOllama: {Kind: AuthKindNone, Fields: []AuthField{{
		Key: "base_url", Label: "Ollama URL", Type: "text", Required: true,
		Placeholder: "http://localhost:11434",
		HelpURL:     "https://ollama.com/",
		HelpText:    "No API key needed — Ollama runs locally.",
	}}},

	TypeLlamaSwap: {Kind: AuthKindNone, Fields: []AuthField{{
		Key: "base_url", Label: "Server URL", Type: "text", Required: true,
		Placeholder: "http://localhost:8080",
		HelpText:    "No API key needed — llama-swap runs locally.",
	}}},
}

// KeyPrefixesForProvider returns the expected key prefixes for the primary
// api_key field of the given provider type, or nil if the provider has no
// prefix constraint (local endpoints, multi-field auth, etc.).
func KeyPrefixesForProvider(providerType string) []string {
	profile, ok := AuthProfiles[providerType]
	if !ok {
		return nil
	}
	for _, f := range profile.Fields {
		if f.Key == "api_key" && len(f.KeyPrefixes) > 0 {
			return f.KeyPrefixes
		}
	}
	return nil
}
