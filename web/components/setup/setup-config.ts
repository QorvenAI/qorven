// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

export const TOTAL_STEPS = 5;

export function toVisualStep(step: number): number {
  if (step === 1) return 1;
  if (step === 2) return 2;
  if (step === 3) return 3;
  if (step === 4) return 4;
  return 5;
}

export const LANGUAGES = [
  'English', 'Spanish', 'French', 'German', 'Hindi',
  'Chinese', 'Japanese', 'Korean', 'Arabic', 'Portuguese',
];

export const AVATAR_GRADIENTS = [
  'from-violet-500 to-fuchsia-500',
  'from-blue-500 to-cyan-400',
  'from-emerald-500 to-teal-400',
  'from-amber-500 to-orange-500',
  'from-rose-500 to-pink-500',
  'from-slate-600 to-zinc-400',
];

export const BEDROCK_REGIONS = [
  { id: 'us-east-1', label: 'US East (N. Virginia)' },
  { id: 'us-west-2', label: 'US West (Oregon)' },
  { id: 'eu-central-1', label: 'EU (Frankfurt)' },
  { id: 'ap-northeast-1', label: 'Asia Pacific (Tokyo)' },
  { id: 'ap-south-1', label: 'Asia Pacific (Mumbai)' },
];

export const PROVIDER_TYPE_OVERRIDES: Record<string, string> = {
  gemini: 'gemini_native',
  anthropic: 'anthropic_native',
};

export type ProviderOption = {
  id: string;
  label: string;
  hint: string;
  providerType: string;
  fields: ('region' | 'api_key' | 'api_base' | 'aws_access_key')[];
  defaultApiBase?: string;
  category: 'cloud' | 'openai_compat' | 'local';
};

export const PROVIDER_OPTIONS_FALLBACK: ProviderOption[] = [
  { id: 'bedrock',    label: 'AWS Bedrock',    hint: 'Static credentials or IAM role.',                 providerType: 'bedrock',          fields: ['region', 'aws_access_key'], category: 'cloud' },
  { id: 'openai',     label: 'OpenAI',         hint: 'gpt-4o, gpt-5, o-series.',                       providerType: 'openai_compat',    fields: ['api_key'],             defaultApiBase: 'https://api.openai.com/v1', category: 'cloud' },
  { id: 'anthropic',  label: 'Anthropic',      hint: 'Claude models (direct API).',                    providerType: 'anthropic_native', fields: ['api_key'],             defaultApiBase: 'https://api.anthropic.com/v1', category: 'cloud' },
  { id: 'deepseek',   label: 'DeepSeek',       hint: 'DeepSeek V3, R1.',                               providerType: 'deepseek',         fields: ['api_key'],             defaultApiBase: 'https://api.deepseek.com/v1', category: 'openai_compat' },
  { id: 'gemini',     label: 'Google Gemini',  hint: 'Gemini 2.5 Pro / Flash.',                        providerType: 'gemini_native',    fields: ['api_key'],             defaultApiBase: 'https://generativelanguage.googleapis.com/v1beta', category: 'cloud' },
  { id: 'groq',       label: 'Groq',           hint: 'Ultra-fast Llama + Kimi — free tier.',           providerType: 'groq',             fields: ['api_key'],             defaultApiBase: 'https://api.groq.com/openai/v1', category: 'openai_compat' },
  { id: 'mistral',    label: 'Mistral',        hint: 'Mistral Large + Devstral.',                      providerType: 'mistral',          fields: ['api_key'],             defaultApiBase: 'https://api.mistral.ai/v1', category: 'openai_compat' },
  { id: 'xai',        label: 'xAI (Grok)',     hint: 'Grok 3 / 4 via OpenAI-compat API.',              providerType: 'xai',              fields: ['api_key'],             defaultApiBase: 'https://api.x.ai/v1', category: 'openai_compat' },
  { id: 'openrouter', label: 'OpenRouter',     hint: '200+ models behind one API key.',                providerType: 'openrouter',       fields: ['api_key'],             defaultApiBase: 'https://openrouter.ai/api/v1', category: 'openai_compat' },
  { id: 'together',   label: 'Together AI',    hint: 'Llama, Qwen, DeepSeek hosted.',                  providerType: 'together',         fields: ['api_key'],             defaultApiBase: 'https://api.together.xyz/v1', category: 'openai_compat' },
  { id: 'fireworks',  label: 'Fireworks AI',   hint: 'Fast Llama + DeepSeek inference.',               providerType: 'fireworks',        fields: ['api_key'],             defaultApiBase: 'https://api.fireworks.ai/inference/v1', category: 'openai_compat' },
  { id: 'cohere',     label: 'Cohere',         hint: 'Command R+ + Rerank.',                           providerType: 'cohere',           fields: ['api_key'],             defaultApiBase: 'https://api.cohere.ai/compatibility/v1', category: 'openai_compat' },
  { id: 'ollama',     label: 'Ollama (Local)', hint: 'Run models on your machine — no API key.',       providerType: 'ollama',           fields: ['api_base'],            defaultApiBase: 'http://localhost:11434/v1', category: 'local' },
  { id: 'custom',     label: 'Custom',         hint: 'Any OpenAI-compatible endpoint.',                providerType: 'openai_compat',    fields: ['api_base', 'api_key'], category: 'openai_compat' },
];

export const PRIME_ROLE_PRESETS = [
  { label: 'General Assistant',  value: 'A helpful, thoughtful AI assistant that handles a wide variety of tasks with clarity and care.' },
  { label: 'Code + Engineering', value: 'A senior software engineer. Prefers reading code before editing, writes minimal diffs, and keeps changes testable.' },
  { label: 'Research & Analysis', value: 'A rigorous researcher. Decomposes complex questions, cites sources, and produces concise structured summaries.' },
  { label: 'Marketing & Content', value: 'A clear, direct writer. Avoids buzzwords, varies sentence length, and leads with the point.' },
  { label: 'Customer Support',   value: 'A friendly, patient support agent. Asks one clarifying question at a time and drives issues to resolution.' },
  { label: 'Business Strategy',  value: 'A strategic advisor. Thinks in frameworks, surfaces tradeoffs, and recommends data-backed decisions.' },
  { label: 'Data & Analytics',   value: 'A data analyst. Translates business questions into precise queries, interprets results, and explains findings clearly.' },
  { label: 'Custom',             value: '' },
];

export function prettyModel(id: string): string {
  const map: Record<string, string> = {
    'us.anthropic.claude-opus-4-7': 'Claude Opus 4.7 (latest)',
    'us.anthropic.claude-sonnet-4-6': 'Claude Sonnet 4.6 (recommended)',
    'us.anthropic.claude-haiku-4-5-20251001-v1:0': 'Claude Haiku 4.5 (fast)',
    'us.anthropic.claude-opus-4-5-20251101-v1:0': 'Claude Opus 4.5',
    'us.anthropic.claude-sonnet-4-5-20250929-v1:0': 'Claude Sonnet 4.5',
    'deepseek.v3.2': 'DeepSeek V3.2 (coding)',
    'us.deepseek.r1-v1:0': 'DeepSeek R1 (reasoning)',
    'qwen.qwen3-coder-next': 'Qwen3 Coder Next',
    'qwen.qwen3-next-80b-a3b': 'Qwen3 Next 80B',
    'us.meta.llama4-maverick-17b-instruct-v1:0': 'Llama 4 Maverick',
    'us.meta.llama4-scout-17b-instruct-v1:0': 'Llama 4 Scout',
    'nvidia.nemotron-super-3-120b': 'Nemotron Super 3 (120B)',
    'moonshotai.kimi-k2.5': 'Kimi K2.5',
    'minimax.minimax-m2': 'MiniMax M2',
    'amazon.nova-pro-v1:0': 'Amazon Nova Pro',
    'amazon.nova-lite-v1:0': 'Amazon Nova Lite',
  };
  return map[id] ?? id;
}

export const RECOMMENDED_PRIMARY = 'us.anthropic.claude-sonnet-4-6';
export const RECOMMENDED_FAST    = 'us.anthropic.claude-haiku-4-5-20251001-v1:0';
export const RECOMMENDED_CODING  = 'deepseek.v3.2';

export const SPECIALIST_PRESETS = [
  {
    key: 'researcher',
    display_name: 'Researcher',
    role: 'researcher',
    title: 'Web research + analysis',
    model: RECOMMENDED_PRIMARY,
    system_prompt: 'You are a research specialist. Decompose complex questions, search the web, cite sources, and produce concise structured summaries.',
    temperature: 0.3,
  },
  {
    key: 'developer',
    display_name: 'Developer',
    role: 'developer',
    title: 'Go + TypeScript engineer',
    model: RECOMMENDED_CODING,
    system_prompt: 'You are a senior software engineer. Prefer reading code before editing, show diffs, and keep changes small and testable.',
    temperature: 0.1,
  },
  {
    key: 'writer',
    display_name: 'Writer',
    role: 'writer',
    title: 'Content + marketing copy',
    model: RECOMMENDED_PRIMARY,
    system_prompt: 'You are a clear, direct writer. Avoid buzzwords, vary sentence length, and lead with the point.',
    temperature: 0.6,
  },
  {
    key: 'support',
    display_name: 'Support Agent',
    role: 'support',
    title: 'Customer help + tickets',
    model: RECOMMENDED_FAST,
    system_prompt: 'You are a friendly, patient support agent. Ask one clarifying question at a time, restate the problem, and drive to a resolution.',
    temperature: 0.4,
  },
] as const;

export const CHANNEL_OPTIONS = [
  { type: 'webchat',  label: 'Webchat',  desc: 'Built in. Enabled by default.',    disabled: true,  builtin: true },
  { type: 'telegram', label: 'Telegram', desc: 'Bot token from @BotFather.',       disabled: false, builtin: false },
  { type: 'discord',  label: 'Discord',  desc: 'Bot token from Developer Portal.', disabled: false, builtin: false },
  { type: 'email',    label: 'Email',    desc: 'IMAP/SMTP mailbox.',               disabled: false, builtin: false },
  { type: 'slack',    label: 'Slack',    desc: 'Coming soon.',                     disabled: true,  builtin: false },
] as const;

export const STEP_TITLES = [
  'Create admin account',
  'Name your workspace',
  'Configure your assistant',
  'Connect an LLM provider',
  'Connect channels',
  'Voice (optional)',
  'Security & access',
  'Test chat',
  "You're all set",
];

export type Provider = {
  id: string; name: string; display_name: string;
  provider_type: string; api_base: string; enabled: boolean;
};

export type AgentSummary = {
  id: string; agent_key: string; display_name: string; role: string; model: string;
};

export type ProviderManifest = {
  id: string; name: string; icon?: string; category?: string;
  auth_type: string; default_api_base?: string; default_model?: string;
  models?: string[];
  fields?: Array<{ name: string; label?: string; placeholder?: string; type?: string; required?: boolean }>;
};

export type AddedProvider = { id: string; name: string; providerDbId: string };
