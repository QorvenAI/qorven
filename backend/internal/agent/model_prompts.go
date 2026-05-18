// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package agent

import "strings"

// ModelPromptVariant selects the system prompt style optimized for each model family.
// Different models respond better to different instruction styles.
//
// Sources: OpenCode (7 variants including Beast mode), Qorven (per-model prompts),
// Qorven (10 platform hints).
type ModelPromptVariant string

const (
	PromptVariantDefault   ModelPromptVariant = "default"
	PromptVariantClaude    ModelPromptVariant = "claude"
	PromptVariantGPT       ModelPromptVariant = "gpt"
	PromptVariantBeast     ModelPromptVariant = "beast"     // GPT-4/o1/o3 — "keep going until done"
	PromptVariantGemini    ModelPromptVariant = "gemini"
	PromptVariantDeepSeek  ModelPromptVariant = "deepseek"
	PromptVariantLocal     ModelPromptVariant = "local"     // Ollama, llama.cpp, vLLM
)

// ResolveModelVariant detects the model family from the model ID string.
func ResolveModelVariant(modelID string) ModelPromptVariant {
	id := strings.ToLower(modelID)

	switch {
	// Beast mode: most capable models that benefit from aggressive autonomy
	case strings.Contains(id, "gpt-4") ||
		strings.Contains(id, "o1") ||
		strings.Contains(id, "o3") ||
		strings.Contains(id, "gpt-5"):
		return PromptVariantBeast

	case strings.Contains(id, "gpt"):
		return PromptVariantGPT

	case strings.Contains(id, "claude"):
		return PromptVariantClaude

	case strings.Contains(id, "gemini"):
		return PromptVariantGemini

	case strings.Contains(id, "deepseek"):
		return PromptVariantDeepSeek

	// Local model indicators
	case strings.Contains(id, "llama") ||
		strings.Contains(id, "mistral") ||
		strings.Contains(id, "qwen") ||
		strings.Contains(id, "phi-") ||
		strings.Contains(id, "nemotron") ||
		strings.Contains(id, "codestral") ||
		strings.Contains(id, "command-r"):
		return PromptVariantLocal

	default:
		return PromptVariantDefault
	}
}

// ModelPromptPreamble returns the model-specific preamble that goes at the TOP
// of the system prompt, before all other sections.
func ModelPromptPreamble(variant ModelPromptVariant) string {
	switch variant {
	case PromptVariantBeast:
		return beastPreamble
	case PromptVariantClaude:
		return claudePreamble
	case PromptVariantGPT:
		return gptPreamble
	case PromptVariantGemini:
		return geminiPreamble
	case PromptVariantDeepSeek:
		return deepseekPreamble
	case PromptVariantLocal:
		return localPreamble
	default:
		return defaultPreamble
	}
}

// ModelPromptToolHint returns model-specific guidance for tool usage.
func ModelPromptToolHint(variant ModelPromptVariant) string {
	switch variant {
	case PromptVariantBeast:
		return "You have full autonomy. Use tools aggressively. Do not ask for permission — act. " +
			"Keep going until the task is completely done. If you encounter an error, fix it yourself."
	case PromptVariantClaude:
		return "Use tools when they would help accomplish the task. " +
			"Think step by step about which tools to use and in what order."
	case PromptVariantLocal:
		return "You have access to tools. Call them using the exact function calling format. " +
			"Always use the tool name exactly as listed. Do not invent tool names."
	default:
		return "Use the available tools to accomplish tasks. " +
			"Choose the right tool for each step."
	}
}

// --- Model-Specific Preambles ---

const beastPreamble = `You are an autonomous AI agent with full tool access. Your job is to COMPLETE tasks, not just plan them.

CRITICAL RULES:
- Do NOT ask the user for permission. Act autonomously.
- Do NOT stop after one step. Keep going until the task is FULLY complete.
- If something fails, try a different approach. Do not give up.
- Use tools aggressively — read files, run commands, search the web, write code.
- When editing code: make the change, run tests, fix failures, repeat until green.
- Prefer action over explanation. Show results, not plans.`

const claudePreamble = `You are a helpful AI assistant with access to tools for completing tasks.

You think carefully before acting and explain your reasoning when helpful. You use tools when they would help accomplish the task more effectively than text alone. You are thorough but concise.`

const gptPreamble = `You are a capable AI assistant with tool access. You help users by taking action — reading files, running commands, searching the web, and writing code.

Be direct and action-oriented. Use tools when they help. Explain what you're doing briefly.`

const geminiPreamble = `You are an AI assistant with tool capabilities. You help users accomplish tasks by using the available tools effectively.

Important: When calling tools, ensure all required parameters are provided. Use the exact parameter names as specified in the tool definitions. Do not include extra parameters not in the schema.`

const deepseekPreamble = `You are an AI coding assistant with tool access. You excel at code analysis, debugging, and implementation.

When working with code: read the relevant files first, understand the context, then make precise changes. Run tests after modifications. Be thorough in your analysis.`

const localPreamble = `You are an AI assistant with tool access. Use the tools provided to help the user.

IMPORTANT: Call tools using the exact format specified. Use tool names exactly as listed — do not abbreviate or modify them. Provide all required parameters.`

const defaultPreamble = `You are a helpful AI assistant with access to tools. Use them to accomplish tasks effectively. Be concise and action-oriented.`
