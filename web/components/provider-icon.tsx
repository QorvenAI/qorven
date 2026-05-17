'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { cn } from '@/lib/utils';

// Values are either a local filename (served from /icons/providers/) or a full https:// URL.
const ICON_MAP: Record<string, string> = {
  openai: 'openai.webp',
  anthropic: 'anthropic.webp',
  // Google / Gemini variants
  gemini: 'googlecloud.webp',
  google: 'googlecloud.webp',
  'google gemini': 'googlecloud.webp',
  'google-gemini': 'googlecloud.webp',
  googlegemini: 'googlecloud.webp',
  // AWS (all variants share the same bedrock icon)
  bedrock: 'bedrock.webp',
  'bedrock-local': 'bedrock.webp',
  'aws bedrock': 'bedrock.webp',
  'aws-bedrock': 'bedrock.webp',
  awsbedrock: 'bedrock.webp',
  aws: 'bedrock.webp',
  bedrock_converse: 'bedrock.webp',
  bedrock_mantle: 'bedrock.webp',
  sagemaker: 'bedrock.webp',
  // Meta Llama / LlamaSwap
  llama_swap: 'https://cdn.simpleicons.org/meta',
  llama: 'https://cdn.simpleicons.org/meta',
  meta: 'https://cdn.simpleicons.org/meta',
  // Alibaba DashScope
  dashscope: 'https://cdn.simpleicons.org/alibabadotcom',
  alibaba: 'https://cdn.simpleicons.org/alibabadotcom',
  // Azure
  azure: 'azureai.webp',
  azureai: 'azureai.webp',
  'azure openai': 'azureai.webp',
  'azure-openai': 'azureai.webp',
  // OpenAI compat providers
  groq: 'groq.webp',
  deepseek: 'deepseek.webp',
  mistral: 'mistral.webp',
  'mistral ai': 'mistral.webp',
  'mistral-ai': 'mistral.webp',
  cohere: 'cohere.webp',
  perplexity: 'perplexity.webp',
  'perplexity-search': 'perplexity.webp',
  'perplexity-embeddings': 'perplexity.webp',
  together: 'together.webp',
  'together ai': 'together.webp',
  'together-ai': 'together.webp',
  togetherai: 'together.webp',
  fireworks: 'fireworks.webp',
  'fireworks ai': 'fireworks.webp',
  'fireworks-ai': 'fireworks.webp',
  openrouter: 'openrouter.webp',
  replicate: 'replicate.webp',
  anyscale: 'anyscale.webp',
  deepinfra: 'deepinfra.webp',
  'deep infra': 'deepinfra.webp',
  lepton: 'leptonai.webp',
  leptonai: 'leptonai.webp',
  moonshot: 'moonshot.webp',
  minimax: 'minimax.webp',
  zhipu: 'zhipu.webp',
  baichuan: 'baichuan.webp',
  yi: 'yi.webp',
  qwen: 'qwen.webp',
  // Local / self-hosted
  ollama: 'ollama.webp',
  lmstudio: 'lmstudio.webp',
  'lm studio': 'lmstudio.webp',
  'lm-studio': 'lmstudio.webp',
  vllm: 'vllm.webp',
  nous: 'nousresearch.webp',
  nousresearch: 'nousresearch.webp',
  huggingface: 'huggingface.webp',
  'hugging face': 'huggingface.webp',
  'hugging-face': 'huggingface.webp',
  claude: 'claude.webp',
  // Search / tools
  tavily: 'tavily.webp',
  exa: 'exa.webp',
  elevenlabs: 'elevenlabs.webp',
  deepgram: 'deepgram.jpeg',
  'brave-search': 'brave.svg',
  brave: 'brave.svg',
  github: 'github.webp',
  'github-mcp': 'github.webp',
  notion: 'notion.webp',
  'notion-mcp': 'notion.webp',
  stability: 'stability.webp',
  assemblyai: 'assemblyai.webp',
  serper: 'googlecloud.webp',
  searxng: 'openrouter.webp',
  litellm: 'openrouter.webp',
};

interface ProviderIconProps {
  provider: string;
  size?: number;
  className?: string;
}

export function ProviderIcon({ provider, size = 20, className }: ProviderIconProps) {
  const icon = ICON_MAP[provider.toLowerCase()];

  if (!icon) {
    // Fallback: first letter in a circle
    return (
      <div className={cn("flex items-center justify-center rounded-md bg-muted text-xs font-semibold text-muted-foreground", className)}
        style={{ width: size, height: size }}>
        {provider.charAt(0).toUpperCase()}
      </div>
    );
  }

  const src = icon.startsWith('https://') ? icon : `/icons/providers/${icon}`;

  return (
    <img
      src={src}
      alt={provider}
      width={size}
      height={size}
      className={cn("rounded-sm object-contain", className)}
    />
  );
}
