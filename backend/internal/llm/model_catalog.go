// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package llm

// ModelMeta holds display metadata for a known model ID.
type ModelMeta struct {
	ID            string
	DisplayName   string
	Provider      string
	ContextWindow int
}

// ModelCatalog maps canonical model IDs to their display metadata.
// Used by GetModelName() to humanise raw IDs in logs, UI, and analytics.
var ModelCatalog = map[string]ModelMeta{
	// ── Anthropic ──────────────────────────────────────────────────────────
	"claude-opus-4-5":                    {ID: "claude-opus-4-5", DisplayName: "Claude Opus 4.5", Provider: "anthropic", ContextWindow: 200000},
	"claude-opus-4-1":                    {ID: "claude-opus-4-1", DisplayName: "Claude Opus 4.1", Provider: "anthropic", ContextWindow: 200000},
	"claude-sonnet-4-5":                  {ID: "claude-sonnet-4-5", DisplayName: "Claude Sonnet 4.5", Provider: "anthropic", ContextWindow: 200000},
	"claude-haiku-4-5":                   {ID: "claude-haiku-4-5", DisplayName: "Claude Haiku 4.5", Provider: "anthropic", ContextWindow: 200000},
	"claude-3-7-sonnet-20250219":         {ID: "claude-3-7-sonnet-20250219", DisplayName: "Claude 3.7 Sonnet", Provider: "anthropic", ContextWindow: 200000},
	"claude-3-5-sonnet-20241022":         {ID: "claude-3-5-sonnet-20241022", DisplayName: "Claude 3.5 Sonnet", Provider: "anthropic", ContextWindow: 200000},
	"claude-3-5-haiku-20241022":          {ID: "claude-3-5-haiku-20241022", DisplayName: "Claude 3.5 Haiku", Provider: "anthropic", ContextWindow: 200000},
	"claude-3-opus-20240229":             {ID: "claude-3-opus-20240229", DisplayName: "Claude 3 Opus", Provider: "anthropic", ContextWindow: 200000},
	"claude-3-sonnet-20240229":           {ID: "claude-3-sonnet-20240229", DisplayName: "Claude 3 Sonnet", Provider: "anthropic", ContextWindow: 200000},
	"claude-3-haiku-20240307":            {ID: "claude-3-haiku-20240307", DisplayName: "Claude 3 Haiku", Provider: "anthropic", ContextWindow: 200000},

	// ── OpenAI ─────────────────────────────────────────────────────────────
	"gpt-4o":                             {ID: "gpt-4o", DisplayName: "GPT-4o", Provider: "openai", ContextWindow: 128000},
	"gpt-4o-mini":                        {ID: "gpt-4o-mini", DisplayName: "GPT-4o Mini", Provider: "openai", ContextWindow: 128000},
	"gpt-4-turbo":                        {ID: "gpt-4-turbo", DisplayName: "GPT-4 Turbo", Provider: "openai", ContextWindow: 128000},
	"gpt-4":                              {ID: "gpt-4", DisplayName: "GPT-4", Provider: "openai", ContextWindow: 8192},
	"gpt-3.5-turbo":                      {ID: "gpt-3.5-turbo", DisplayName: "GPT-3.5 Turbo", Provider: "openai", ContextWindow: 16385},
	"o1":                                 {ID: "o1", DisplayName: "o1", Provider: "openai", ContextWindow: 200000},
	"o1-mini":                            {ID: "o1-mini", DisplayName: "o1 Mini", Provider: "openai", ContextWindow: 128000},
	"o3":                                 {ID: "o3", DisplayName: "o3", Provider: "openai", ContextWindow: 200000},
	"o3-mini":                            {ID: "o3-mini", DisplayName: "o3 Mini", Provider: "openai", ContextWindow: 200000},
	"o4-mini":                            {ID: "o4-mini", DisplayName: "o4 Mini", Provider: "openai", ContextWindow: 200000},

	// ── Google Gemini ───────────────────────────────────────────────────────
	"gemini-2.5-pro-preview-05-06":       {ID: "gemini-2.5-pro-preview-05-06", DisplayName: "Gemini 2.5 Pro Preview", Provider: "gemini", ContextWindow: 1048576},
	"gemini-2.5-flash-preview-04-17":     {ID: "gemini-2.5-flash-preview-04-17", DisplayName: "Gemini 2.5 Flash Preview", Provider: "gemini", ContextWindow: 1048576},
	"gemini-2.0-flash":                   {ID: "gemini-2.0-flash", DisplayName: "Gemini 2.0 Flash", Provider: "gemini", ContextWindow: 1048576},
	"gemini-2.0-flash-lite":              {ID: "gemini-2.0-flash-lite", DisplayName: "Gemini 2.0 Flash Lite", Provider: "gemini", ContextWindow: 1048576},
	"gemini-1.5-pro-002":                 {ID: "gemini-1.5-pro-002", DisplayName: "Gemini 1.5 Pro 002", Provider: "gemini", ContextWindow: 2097152},
	"gemini-1.5-flash-002":               {ID: "gemini-1.5-flash-002", DisplayName: "Gemini 1.5 Flash 002", Provider: "gemini", ContextWindow: 1048576},
	"gemma-3-27b-it":                     {ID: "gemma-3-27b-it", DisplayName: "Gemma 3 27B IT", Provider: "gemini", ContextWindow: 131072},

	// ── Mistral ─────────────────────────────────────────────────────────────
	"mistral-large-latest":               {ID: "mistral-large-latest", DisplayName: "Mistral Large", Provider: "mistral", ContextWindow: 131072},
	"mistral-small-latest":               {ID: "mistral-small-latest", DisplayName: "Mistral Small", Provider: "mistral", ContextWindow: 131072},
	"codestral-latest":                   {ID: "codestral-latest", DisplayName: "Codestral", Provider: "mistral", ContextWindow: 262144},
	"ministral-3-3b-2512":                {ID: "ministral-3-3b-2512", DisplayName: "Ministral 3B", Provider: "mistral", ContextWindow: 131072},
	"ministral-3-8b-2512":                {ID: "ministral-3-8b-2512", DisplayName: "Ministral 8B", Provider: "mistral", ContextWindow: 131072},
	"devstral-small-2507":                {ID: "devstral-small-2507", DisplayName: "Devstral Small", Provider: "mistral", ContextWindow: 131072},
	"pixtral-large-2411":                 {ID: "pixtral-large-2411", DisplayName: "Pixtral Large", Provider: "mistral", ContextWindow: 131072},
	"pixtral-12b-2409":                   {ID: "pixtral-12b-2409", DisplayName: "Pixtral 12B", Provider: "mistral", ContextWindow: 131072},

	// ── Meta Llama (Groq / direct) ──────────────────────────────────────────
	"llama-3.3-70b-versatile":            {ID: "llama-3.3-70b-versatile", DisplayName: "Llama 3.3 70B", Provider: "groq", ContextWindow: 128000},
	"llama-3.1-8b-instant":               {ID: "llama-3.1-8b-instant", DisplayName: "Llama 3.1 8B", Provider: "groq", ContextWindow: 128000},
	"llama3-70b-8192":                    {ID: "llama3-70b-8192", DisplayName: "Llama 3 70B", Provider: "groq", ContextWindow: 8192},
	"llama3-8b-8192":                     {ID: "llama3-8b-8192", DisplayName: "Llama 3 8B", Provider: "groq", ContextWindow: 8192},
	"meta.llama3-70b-instruct-v1:0":      {ID: "meta.llama3-70b-instruct-v1:0", DisplayName: "Llama 3 70B (Bedrock)", Provider: "bedrock", ContextWindow: 8192},
	"meta.llama3-8b-instruct-v1:0":       {ID: "meta.llama3-8b-instruct-v1:0", DisplayName: "Llama 3 8B (Bedrock)", Provider: "bedrock", ContextWindow: 8192},
	"meta.llama3-1-70b-instruct-v1:0":    {ID: "meta.llama3-1-70b-instruct-v1:0", DisplayName: "Llama 3.1 70B (Bedrock)", Provider: "bedrock", ContextWindow: 128000},
	"meta.llama3-1-405b-instruct-v1:0":   {ID: "meta.llama3-1-405b-instruct-v1:0", DisplayName: "Llama 3.1 405B (Bedrock)", Provider: "bedrock", ContextWindow: 128000},

	// ── Amazon Bedrock (Anthropic) ──────────────────────────────────────────
	"anthropic.claude-3-7-sonnet-20250219-v1:0": {ID: "anthropic.claude-3-7-sonnet-20250219-v1:0", DisplayName: "Claude 3.7 Sonnet (Bedrock)", Provider: "bedrock", ContextWindow: 200000},
	"anthropic.claude-3-5-sonnet-20241022-v2:0": {ID: "anthropic.claude-3-5-sonnet-20241022-v2:0", DisplayName: "Claude 3.5 Sonnet v2 (Bedrock)", Provider: "bedrock", ContextWindow: 200000},
	"anthropic.claude-3-5-haiku-20241022-v1:0":  {ID: "anthropic.claude-3-5-haiku-20241022-v1:0", DisplayName: "Claude 3.5 Haiku (Bedrock)", Provider: "bedrock", ContextWindow: 200000},
	"anthropic.claude-3-opus-20240229-v1:0":      {ID: "anthropic.claude-3-opus-20240229-v1:0", DisplayName: "Claude 3 Opus (Bedrock)", Provider: "bedrock", ContextWindow: 200000},

	// ── Amazon Bedrock (Amazon) ─────────────────────────────────────────────
	"amazon.nova-pro-v1:0":               {ID: "amazon.nova-pro-v1:0", DisplayName: "Amazon Nova Pro", Provider: "bedrock", ContextWindow: 300000},
	"amazon.nova-lite-v1:0":              {ID: "amazon.nova-lite-v1:0", DisplayName: "Amazon Nova Lite", Provider: "bedrock", ContextWindow: 300000},
	"amazon.nova-micro-v1:0":             {ID: "amazon.nova-micro-v1:0", DisplayName: "Amazon Nova Micro", Provider: "bedrock", ContextWindow: 128000},
	"amazon.titan-text-premier-v1:0":     {ID: "amazon.titan-text-premier-v1:0", DisplayName: "Titan Text Premier", Provider: "bedrock", ContextWindow: 32000},

	// ── xAI ────────────────────────────────────────────────────────────────
	"grok-3":                             {ID: "grok-3", DisplayName: "Grok 3", Provider: "xai", ContextWindow: 131072},
	"grok-3-mini":                        {ID: "grok-3-mini", DisplayName: "Grok 3 Mini", Provider: "xai", ContextWindow: 131072},
	"grok-2-1212":                        {ID: "grok-2-1212", DisplayName: "Grok 2", Provider: "xai", ContextWindow: 131072},

	// ── DeepSeek ───────────────────────────────────────────────────────────
	"deepseek-chat":                      {ID: "deepseek-chat", DisplayName: "DeepSeek Chat", Provider: "deepseek", ContextWindow: 65536},
	"deepseek-reasoner":                  {ID: "deepseek-reasoner", DisplayName: "DeepSeek Reasoner", Provider: "deepseek", ContextWindow: 65536},

	// ── Cohere ─────────────────────────────────────────────────────────────
	"command-r-plus-08-2024":             {ID: "command-r-plus-08-2024", DisplayName: "Command R+ (Aug 2024)", Provider: "cohere", ContextWindow: 128000},
	"command-r-08-2024":                  {ID: "command-r-08-2024", DisplayName: "Command R (Aug 2024)", Provider: "cohere", ContextWindow: 128000},
	"command-a-03-2025":                  {ID: "command-a-03-2025", DisplayName: "Command A", Provider: "cohere", ContextWindow: 256000},
}

// GetModelName returns the human-readable display name for a model ID,
// falling back to the raw ID if no entry exists.
func GetModelName(id string) string {
	if m, ok := ModelCatalog[id]; ok {
		return m.DisplayName
	}
	return id
}
