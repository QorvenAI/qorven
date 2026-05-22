// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

package llm

import (
	"context"
	"fmt"

	"github.com/qorvenai/qorven/internal/providers"
)

// Pipeline is the AI Gateway middleware chain. Every agent LLM call goes
// through here instead of calling a provider directly.
//
// Current implementation: passthrough — routes to the existing provider
// registry using the same key-pool selection as the pre-gateway code.
// Middleware stubs (budget, priority queue, cache, aliases, circuit breaker,
// cost ledger) will be activated in Phases 2 and 3.
type Pipeline struct {
	reg      *providers.Registry
	keyStore *providers.KeyPoolStore
	tenantID string

	// Phase 2+: filled in by NewPipeline options
	budget   BudgetChecker
	queue    QueueAcquirer
	cache    CacheLookup
	aliases  AliasResolver
	circuit  CircuitExecutor
	cost     CostRecorder
}

// Middleware hook interfaces — each becomes a no-op by default, replaced
// by real implementations as each Phase lands.

type BudgetChecker interface {
	Check(ctx context.Context, req GatewayRequest) error
}

type QueueAcquirer interface {
	Acquire(ctx context.Context, p Priority) error
	Release(p Priority)
}

type CacheLookup interface {
	Lookup(ctx context.Context, req GatewayRequest) (*GatewayResponse, bool)
	Store(ctx context.Context, req GatewayRequest, resp *GatewayResponse)
}

type AliasResolver interface {
	Resolve(ctx context.Context, req *GatewayRequest) error
}

type CircuitExecutor interface {
	Execute(keyID string, fn func() error) error
}

type CostRecorder interface {
	Record(ctx context.Context, req GatewayRequest, resp *GatewayResponse)
}

// nop implementations used until real middleware is wired in.

type nopBudget struct{}
func (nopBudget) Check(_ context.Context, _ GatewayRequest) error { return nil }

type nopQueue struct{}
func (nopQueue) Acquire(_ context.Context, _ Priority) error { return nil }
func (nopQueue) Release(_ Priority)                          {}

type nopCache struct{}
func (nopCache) Lookup(_ context.Context, _ GatewayRequest) (*GatewayResponse, bool) { return nil, false }
func (nopCache) Store(_ context.Context, _ GatewayRequest, _ *GatewayResponse)       {}

type nopAlias struct{}
func (nopAlias) Resolve(_ context.Context, _ *GatewayRequest) error { return nil }

type nopCircuit struct{}
func (nopCircuit) Execute(_ string, fn func() error) error { return fn() }

type nopCost struct{}
func (nopCost) Record(_ context.Context, _ GatewayRequest, _ *GatewayResponse) {}

// Option configures a Pipeline component.
type Option func(*Pipeline)

func WithBudget(b BudgetChecker) Option     { return func(p *Pipeline) { p.budget = b } }
func WithQueue(q QueueAcquirer) Option      { return func(p *Pipeline) { p.queue = q } }
func WithCache(c CacheLookup) Option        { return func(p *Pipeline) { p.cache = c } }
func WithAliases(a AliasResolver) Option    { return func(p *Pipeline) { p.aliases = a } }
func WithCircuit(c CircuitExecutor) Option  { return func(p *Pipeline) { p.circuit = c } }
func WithCost(r CostRecorder) Option        { return func(p *Pipeline) { p.cost = r } }

// NewPipeline creates a Pipeline wired to the existing provider registry and
// key pool. Options layer in middleware components as they become available.
func NewPipeline(reg *providers.Registry, keyStore *providers.KeyPoolStore, tenantID string, opts ...Option) *Pipeline {
	p := &Pipeline{
		reg:      reg,
		keyStore: keyStore,
		tenantID: tenantID,
		budget:   nopBudget{},
		queue:    nopQueue{},
		cache:    nopCache{},
		aliases:  nopAlias{},
		circuit:  nopCircuit{},
		cost:     nopCost{},
	}
	for _, o := range opts {
		o(p)
	}
	return p
}

// Chat dispatches a non-streaming request through the pipeline.
func (p *Pipeline) Chat(ctx context.Context, req GatewayRequest) (*GatewayResponse, error) {
	var resp *GatewayResponse
	err := p.ChatStream(ctx, req, nil, func(r *GatewayResponse) {
		resp = r
	})
	return resp, err
}

// ChatStream dispatches a streaming request through the full pipeline:
//
//  1. Budget check
//  2. Priority queue slot
//  3. Cache lookup
//  4. Alias resolution
//  5. Provider + key selection
//  6. Dispatch via circuit breaker
//  7. Cost recording (async)
//  8. Cache store (async)
//
// onChunk receives streaming tokens; onDone receives the final GatewayResponse.
// Either callback may be nil.
func (p *Pipeline) ChatStream(
	ctx context.Context,
	req GatewayRequest,
	onChunk func(StreamChunk),
	onDone func(*GatewayResponse),
) error {
	// 1. Budget check
	if err := p.budget.Check(ctx, req); err != nil {
		return err
	}

	// 2. Priority queue
	if err := p.queue.Acquire(ctx, req.Priority); err != nil {
		return err
	}
	defer p.queue.Release(req.Priority)

	// 3. Cache lookup
	if !req.SkipCache {
		if hit, ok := p.cache.Lookup(ctx, req); ok {
			if onChunk != nil && hit.Content != "" {
				onChunk(StreamChunk{StreamChunk: providers.StreamChunk{Content: hit.Content, Done: true}})
			}
			if onDone != nil {
				onDone(hit)
			}
			return nil
		}
	}

	// 4. Alias resolution
	if err := p.aliases.Resolve(ctx, &req); err != nil {
		return err
	}

	// 5. Provider + key selection — use existing registry + key pool logic.
	provider, keyID, err := p.selectProvider(ctx, req)
	if err != nil {
		return err
	}

	// 6. Dispatch
	chatReq := providers.ChatRequest{
		Messages:   req.Messages,
		Tools:      req.Tools,
		ToolChoice: req.ToolChoice,
		Model:      req.Model,
		Options:    req.Options,
	}

	var provChunkWrapper func(providers.StreamChunk)
	if onChunk != nil {
		provChunkWrapper = func(chunk providers.StreamChunk) {
			onChunk(StreamChunk{StreamChunk: chunk, ProviderID: keyID})
		}
	}

	var llmResp *providers.ChatResponse
	dispatchErr := p.circuit.Execute(keyID, func() error {
		var e error
		llmResp, e = provider.ChatStream(ctx, chatReq, provChunkWrapper)
		return e
	})
	if dispatchErr != nil {
		return dispatchErr
	}

	var callCost CallCost
	if llmResp != nil {
		callCost = ComputeCost(req.Model, llmResp.Usage, req.Provider)
	}
	gResp := &GatewayResponse{
		ChatResponse:  llmResp,
		KeyID:         keyID,
		ModelResolved: req.Model,
		Cost:          callCost,
	}

	// 7. Cost recording (non-blocking)
	go p.cost.Record(ctx, req, gResp)

	// 8. Cache store (non-blocking)
	if !req.SkipCache {
		go p.cache.Store(ctx, req, gResp)
	}

	if onDone != nil {
		onDone(gResp)
	}
	return nil
}

// CircuitBreaker returns the circuit breaker bank if one was configured, nil otherwise.
func (p *Pipeline) CircuitBreaker() *CircuitBreakerBank {
	if cb, ok := p.circuit.(*CircuitBreakerBank); ok {
		return cb
	}
	return nil
}

// Queue returns the priority queue if one was configured, nil otherwise.
func (p *Pipeline) Queue() *PriorityQueue {
	if q, ok := p.queue.(*PriorityQueue); ok {
		return q
	}
	return nil
}

// Cache returns the LRU cache layer if one was configured, nil otherwise.
func (p *Pipeline) Cache() *CacheLayer {
	if c, ok := p.cache.(*CacheLayer); ok {
		return c
	}
	return nil
}

// selectProvider picks a provider + key using the existing registry and key pool.
// Returns the provider, a key identifier (for circuit breaker and cost attribution),
// and any error.
func (p *Pipeline) selectProvider(ctx context.Context, req GatewayRequest) (providers.Provider, string, error) {
	// If a specific provider is pinned, use it.
	if req.Provider != "" {
		prov, ok := p.reg.Get(req.Provider)
		if !ok {
			return nil, "", fmt.Errorf("gateway: pinned provider %q not found", req.Provider)
		}
		return prov, req.Provider, nil
	}

	// Look up a provider that can serve the requested model.
	prov := p.reg.ProviderForModel(req.Model)
	if prov == nil {
		// Fall back to the registry default.
		prov = p.reg.Default()
	}
	if prov == nil {
		return nil, "", fmt.Errorf("gateway: no provider available for model %q", req.Model)
	}
	return prov, prov.Name(), nil
}
