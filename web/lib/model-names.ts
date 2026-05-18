// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

// Static display-name lookup — mirrors backend/internal/llm/model_catalog.go.
// Falls back to the raw ID when not found.

const catalog: Record<string, string> = {
  // Anthropic
  'claude-opus-4-5': 'Claude Opus 4.5',
  'claude-opus-4-1': 'Claude Opus 4.1',
  'claude-sonnet-4-5': 'Claude Sonnet 4.5',
  'claude-haiku-4-5': 'Claude Haiku 4.5',
  'claude-3-7-sonnet-20250219': 'Claude 3.7 Sonnet',
  'claude-3-5-sonnet-20241022': 'Claude 3.5 Sonnet',
  'claude-3-5-haiku-20241022': 'Claude 3.5 Haiku',
  'claude-3-opus-20240229': 'Claude 3 Opus',
  'claude-3-sonnet-20240229': 'Claude 3 Sonnet',
  'claude-3-haiku-20240307': 'Claude 3 Haiku',

  // OpenAI
  'gpt-4o': 'GPT-4o',
  'gpt-4o-mini': 'GPT-4o Mini',
  'gpt-4-turbo': 'GPT-4 Turbo',
  'gpt-4': 'GPT-4',
  'gpt-3.5-turbo': 'GPT-3.5 Turbo',
  'o1': 'o1',
  'o1-mini': 'o1 Mini',
  'o3': 'o3',
  'o3-mini': 'o3 Mini',
  'o4-mini': 'o4 Mini',

  // Google Gemini
  'gemini-2.5-pro-preview-05-06': 'Gemini 2.5 Pro Preview',
  'gemini-2.5-flash-preview-04-17': 'Gemini 2.5 Flash Preview',
  'gemini-2.0-flash': 'Gemini 2.0 Flash',
  'gemini-2.0-flash-lite': 'Gemini 2.0 Flash Lite',
  'gemini-1.5-pro-002': 'Gemini 1.5 Pro',
  'gemini-1.5-flash-002': 'Gemini 1.5 Flash',
  'gemma-3-27b-it': 'Gemma 3 27B',

  // Mistral
  'mistral-large-latest': 'Mistral Large',
  'mistral-small-latest': 'Mistral Small',
  'codestral-latest': 'Codestral',
  'ministral-3-3b-2512': 'Ministral 3B',
  'ministral-3-8b-2512': 'Ministral 8B',
  'devstral-small-2507': 'Devstral Small',
  'pixtral-large-2411': 'Pixtral Large',
  'pixtral-12b-2409': 'Pixtral 12B',

  // Meta Llama (Groq / direct)
  'llama-3.3-70b-versatile': 'Llama 3.3 70B',
  'llama-3.1-8b-instant': 'Llama 3.1 8B',
  'llama3-70b-8192': 'Llama 3 70B',
  'llama3-8b-8192': 'Llama 3 8B',
  'meta.llama3-70b-instruct-v1:0': 'Llama 3 70B (Bedrock)',
  'meta.llama3-8b-instruct-v1:0': 'Llama 3 8B (Bedrock)',
  'meta.llama3-1-70b-instruct-v1:0': 'Llama 3.1 70B (Bedrock)',
  'meta.llama3-1-405b-instruct-v1:0': 'Llama 3.1 405B (Bedrock)',

  // Amazon Bedrock (Anthropic)
  'anthropic.claude-3-7-sonnet-20250219-v1:0': 'Claude 3.7 Sonnet (Bedrock)',
  'anthropic.claude-3-5-sonnet-20241022-v2:0': 'Claude 3.5 Sonnet v2 (Bedrock)',
  'anthropic.claude-3-5-haiku-20241022-v1:0': 'Claude 3.5 Haiku (Bedrock)',
  'anthropic.claude-3-opus-20240229-v1:0': 'Claude 3 Opus (Bedrock)',

  // Amazon Bedrock (Amazon)
  'amazon.nova-pro-v1:0': 'Amazon Nova Pro',
  'amazon.nova-lite-v1:0': 'Amazon Nova Lite',
  'amazon.nova-micro-v1:0': 'Amazon Nova Micro',
  'amazon.titan-text-premier-v1:0': 'Titan Text Premier',

  // xAI
  'grok-3': 'Grok 3',
  'grok-3-mini': 'Grok 3 Mini',
  'grok-2-1212': 'Grok 2',

  // DeepSeek
  'deepseek-chat': 'DeepSeek Chat',
  'deepseek-reasoner': 'DeepSeek Reasoner',

  // Cohere
  'command-r-plus-08-2024': 'Command R+',
  'command-r-08-2024': 'Command R',
  'command-a-03-2025': 'Command A',
};

/** Returns the human-readable display name for a model ID, falling back to the raw ID. */
export function modelDisplayName(id: string | undefined | null): string {
  if (!id) return '';
  return catalog[id] ?? id;
}
