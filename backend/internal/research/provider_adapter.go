// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package research

import (
	"context"

	"github.com/qorvenai/qorven/internal/llm"
	"github.com/qorvenai/qorven/internal/providers"
)

// providerRegistryAdapter bridges the gateway's providers.Registry to the
// narrower llm.Provider interface that the research engine expects. It
// lets research use the same Bedrock/OpenAI-compat stack that the rest
// of the app uses, without research taking a direct dependency on the
// fuller providers package.
//
// The adapter is intentionally minimal: Chat() resolves a provider based
// on req.Model (so Bedrock inference profiles dispatch to the Bedrock
// provider, OpenAI-style ids go wherever the registry routes them) and
// falls back to the default provider when the model name doesn't tell
// us enough.
type providerRegistryAdapter struct {
	reg *providers.Registry
}

// NewLLMAdapter returns an llm.Provider backed by a providers.Registry.
// Safe to pass nil: research.NewEngine already nil-guards the Chat path.
func NewLLMAdapter(reg *providers.Registry) llm.Provider {
	if reg == nil {
		return nil
	}
	return &providerRegistryAdapter{reg: reg}
}

func (p *providerRegistryAdapter) Name() string { return "registry" }

func (p *providerRegistryAdapter) Chat(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	model := req.Model
	// The engine passes "balanced" or empty for the synthesis step —
	// treat those as "use the default provider's default model" rather
	// than a model ID to resolve.
	var prov providers.Provider
	switch model {
	case "", "balanced", "quick", "deep":
		// Prefer Bedrock if configured — it's the only provider on this
		// machine that has valid credentials. Otherwise fall back to
		// the first enabled provider. This prevents research from
		// synthesising against a misconfigured OpenAI key.
		for _, cfg := range p.reg.List() {
			if cfg.ProviderType == providers.TypeBedrock && cfg.Enabled {
				if x, ok := p.reg.Get(cfg.ID); ok { prov = x; break }
			}
		}
		if prov == nil { prov = p.reg.Default() }
		model = "" // let the provider pick its default model
	default:
		prov = p.reg.ProviderForModel(model)
		if prov == nil {
			prov = p.reg.Default()
		}
	}
	if prov == nil {
		return nil, errNoProvider
	}
	outerMsgs := make([]providers.Message, 0, len(req.Messages))
	for _, m := range req.Messages {
		outerMsgs = append(outerMsgs, providers.Message{Role: m.Role, Content: m.Content})
	}
	opts := map[string]any{}
	if req.Temperature != 0 { opts["temperature"] = req.Temperature }
	if req.MaxTokens != 0   { opts["max_tokens"]   = req.MaxTokens }

	resp, err := prov.Chat(ctx, providers.ChatRequest{
		Model:    model,
		Messages: outerMsgs,
		Options:  opts,
	})
	if err != nil {
		return nil, err
	}
	out := &llm.ChatResponse{
		Content:      resp.Content,
		Model:        model,
		FinishReason: resp.FinishReason,
	}
	if resp.Usage != nil {
		out.InputTokens = resp.Usage.PromptTokens
		out.OutputTokens = resp.Usage.CompletionTokens
	}
	return out, nil
}

// ChatStream is not used by the research engine — research only calls
// Chat — so we implement it as a delegation to Chat + one final chunk
// rather than pulling streaming through the adapter. Matches what the
// tests expect.
func (p *providerRegistryAdapter) ChatStream(ctx context.Context, req llm.ChatRequest, onChunk func(llm.StreamChunk)) (*llm.ChatResponse, error) {
	resp, err := p.Chat(ctx, req)
	if err != nil { return nil, err }
	if onChunk != nil {
		onChunk(llm.StreamChunk{Delta: resp.Content, FinishReason: resp.FinishReason})
	}
	return resp, nil
}

func (p *providerRegistryAdapter) ListModels(ctx context.Context) ([]string, error) {
	// Research doesn't call this — return an empty list so callers that
	// do probe it don't error. The actual model catalog lives in the
	// main /v1/models endpoint.
	return nil, nil
}

// errNoProvider is returned when the adapter can't find any enabled
// provider to route a request through. Distinct sentinel so the
// research engine's error path is easy to recognise in logs.
var errNoProvider = &noProviderError{}

type noProviderError struct{}

func (e *noProviderError) Error() string { return "research: no LLM provider available" }
