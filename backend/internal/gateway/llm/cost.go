// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

package llm

import (
	"context"
	"log/slog"
	"math"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qorvenai/qorven/internal/providers"
)

// uusdPerUSD is the conversion factor: 1 USD = 1,000,000 µUSD (micro-dollars).
// All internal cost arithmetic uses integer µUSD to eliminate float64 drift.
const uusdPerUSD = int64(1_000_000)

// ModelPricing holds per-token USD pricing for one model.
// All rates are USD per 1,000,000 tokens, which equals µUSD per token.
type ModelPricing struct {
	InputPer1M  float64 // USD per 1M input tokens
	OutputPer1M float64 // USD per 1M output tokens
	CacheWrite  float64 // USD per 1M cache-write tokens (Anthropic prompt caching; 1.25× input)
	CacheRead   float64 // USD per 1M cache-read tokens  (Anthropic prompt caching; 0.10× input)
}

// pricingTable is the in-process model pricing seed. Keyed by litellm model ID.
// UpdatePricingTable merges live DB data in at runtime.
var (
	pricingMu    sync.RWMutex
	pricingTable = map[string]ModelPricing{
		// Anthropic — cache rates: write = 1.25× input, read = 0.10× input
		"claude-opus-4-7":            {InputPer1M: 15.00, OutputPer1M: 75.00, CacheWrite: 18.75, CacheRead: 1.50},
		"claude-sonnet-4-6":          {InputPer1M: 3.00, OutputPer1M: 15.00, CacheWrite: 3.75, CacheRead: 0.30},
		"claude-haiku-4-5":           {InputPer1M: 0.80, OutputPer1M: 4.00, CacheWrite: 1.00, CacheRead: 0.08},
		"claude-3-5-sonnet-20241022": {InputPer1M: 3.00, OutputPer1M: 15.00, CacheWrite: 3.75, CacheRead: 0.30},
		"claude-3-5-haiku-20241022":  {InputPer1M: 0.80, OutputPer1M: 4.00, CacheWrite: 1.00, CacheRead: 0.08},
		// OpenAI
		"gpt-4o":      {InputPer1M: 2.50, OutputPer1M: 10.00},
		"gpt-4o-mini": {InputPer1M: 0.15, OutputPer1M: 0.60},
		"o1":          {InputPer1M: 15.00, OutputPer1M: 60.00},
		"o3":          {InputPer1M: 10.00, OutputPer1M: 40.00},
		"o3-mini":     {InputPer1M: 1.10, OutputPer1M: 4.40},
		"o4-mini":     {InputPer1M: 1.10, OutputPer1M: 4.40},
		// Google Gemini
		"gemini-2.0-flash":      {InputPer1M: 0.075, OutputPer1M: 0.30},
		"gemini-2.0-flash-lite": {InputPer1M: 0.075, OutputPer1M: 0.30},
		"gemini-2.5-pro":        {InputPer1M: 1.25, OutputPer1M: 10.00},
		"gemini-2.5-flash":      {InputPer1M: 0.15, OutputPer1M: 0.60},
		// DeepSeek
		"deepseek-chat":     {InputPer1M: 0.27, OutputPer1M: 1.10},
		"deepseek-reasoner": {InputPer1M: 0.55, OutputPer1M: 2.19},
		// xAI Grok
		"grok-3":      {InputPer1M: 3.00, OutputPer1M: 15.00},
		"grok-3-mini": {InputPer1M: 0.30, OutputPer1M: 0.50},
		// Meta Llama (via Groq / Together)
		"llama-3.3-70b-versatile": {InputPer1M: 0.59, OutputPer1M: 0.79},
		"llama-4-scout":           {InputPer1M: 0.11, OutputPer1M: 0.34},
		"llama-4-maverick":        {InputPer1M: 0.50, OutputPer1M: 0.77},
		// Mistral
		"mistral-large-latest": {InputPer1M: 2.00, OutputPer1M: 6.00},
		"mistral-small-latest": {InputPer1M: 0.10, OutputPer1M: 0.30},
	}
)

// UpdatePricingTable merges new pricing data into the global table.
// Called by the existing PricingStore after it fetches fresh data.
func UpdatePricingTable(entries map[string]ModelPricing) {
	pricingMu.Lock()
	defer pricingMu.Unlock()
	for k, v := range entries {
		pricingTable[k] = v
	}
}

// toUUSD converts a rate (USD per 1M tokens) and a token count to integer µUSD.
//
// Math: ratePerMillion (USD/1M tokens) = ratePerMillion µUSD/token
// because 1 USD = 1,000,000 µUSD and rate is per 1,000,000 tokens.
// So: cost_µUSD = tokens × ratePerMillion  (no division needed).
//
// math.Round eliminates systematic truncation bias; sub-µUSD amounts
// are < $0.000001 — negligible for any single call.
func toUUSD(ratePerMillion float64, tokens int) int64 {
	if tokens == 0 || ratePerMillion == 0 {
		return 0
	}
	return int64(math.Round(float64(tokens) * ratePerMillion))
}

// CallCost is the precise per-call cost breakdown in integer µUSD.
// All arithmetic uses int64 — never float64 accumulation.
type CallCost struct {
	TokensIn         int
	TokensOut        int
	TokensThinking   int
	TokensCacheWrite int
	TokensCacheRead  int

	CostInputUUSD    int64
	CostOutputUUSD   int64
	CostThinkingUUSD int64
	CostCacheWUUSD   int64
	CostCacheRUUSD   int64
	TotalUUSD        int64 // exact integer sum of all components
}

// TotalUSD converts the integer µUSD total to float64 for display only.
// Never use this value for arithmetic — use TotalUUSD.
func (c CallCost) TotalUSD() float64 {
	return float64(c.TotalUUSD) / float64(uusdPerUSD)
}

// ComputeCost calculates the exact µUSD cost for a model call.
// All five token types are accounted for: input, output, thinking,
// cache-write, and cache-read.
//
// For unknown models the cost is 0 (safe: we undercharge rather than
// overcharge). Token counts are still recorded for audit purposes.
func ComputeCost(model string, usage *providers.Usage) CallCost {
	if usage == nil {
		return CallCost{}
	}
	pricingMu.RLock()
	p, ok := pricingTable[model]
	pricingMu.RUnlock()

	c := CallCost{
		TokensIn:         usage.PromptTokens,
		TokensOut:        usage.CompletionTokens,
		TokensThinking:   usage.ThinkingTokens,
		TokensCacheWrite: usage.CacheCreationTokens,
		TokensCacheRead:  usage.CacheReadTokens,
	}
	if !ok {
		// Unknown model — tokens recorded, cost is 0. Never overcharge.
		return c
	}

	c.CostInputUUSD = toUUSD(p.InputPer1M, c.TokensIn)
	c.CostOutputUUSD = toUUSD(p.OutputPer1M, c.TokensOut)
	// Thinking tokens are billed at the same rate as output tokens.
	c.CostThinkingUUSD = toUUSD(p.OutputPer1M, c.TokensThinking)
	c.CostCacheWUUSD = toUUSD(p.CacheWrite, c.TokensCacheWrite)
	c.CostCacheRUUSD = toUUSD(p.CacheRead, c.TokensCacheRead)
	c.TotalUUSD = c.CostInputUUSD + c.CostOutputUUSD + c.CostThinkingUUSD + c.CostCacheWUUSD + c.CostCacheRUUSD
	return c
}

// EstimateCost returns estimated USD cost. Kept for backward compatibility.
// Use ComputeCost for precise accounting.
func EstimateCost(model string, tokensIn, tokensOut int) float64 {
	usage := &providers.Usage{PromptTokens: tokensIn, CompletionTokens: tokensOut}
	return ComputeCost(model, usage).TotalUSD()
}

// spendEntry is one unit of work queued for the background DB writer.
type spendEntry struct {
	tenantID   string
	agentID    string
	sessionID  string
	providerID string
	modelID    string
	cost       CallCost
	cacheHit   bool
}

// writeBufSize is the primary channel buffer depth.
const writeBufSize = 8192

// CostLedger records per-request cost to gateway_spend_raw (immutable) and
// gateway_spend (daily aggregate) asynchronously. It also notifies the
// BudgetEngine after each write so the next budget check sees fresh data.
//
// Buffer overflow policy:
//  1. Try the primary writes channel (8192 slots).
//  2. On overflow, try the retry channel (512 slots) — logged at Warn.
//  3. If both are full, log at Error with full token/cost detail so the
//     spend is never silently lost from the audit trail, then discard.
type CostLedger struct {
	db      *pgxpool.Pool
	budget  *BudgetEngine
	writes  chan spendEntry
	retries chan spendEntry
	stopCh  chan struct{}
	once    sync.Once
}

// NewCostLedger creates a CostLedger and starts its background writer goroutines.
func NewCostLedger(db *pgxpool.Pool, budget *BudgetEngine) *CostLedger {
	l := &CostLedger{
		db:      db,
		budget:  budget,
		writes:  make(chan spendEntry, writeBufSize),
		retries: make(chan spendEntry, 512),
		stopCh:  make(chan struct{}),
	}
	go l.worker()
	go l.retryWorker()
	return l
}

// Record is called after every LLM call. Non-blocking — enqueues for async write.
func (l *CostLedger) Record(ctx context.Context, req GatewayRequest, resp *GatewayResponse) {
	if l.db == nil || resp == nil || resp.ChatResponse == nil {
		return
	}
	cost := ComputeCost(resp.ModelResolved, resp.Usage)
	// Skip zero-cost calls with no tokens (e.g. gateway cache hits with no
	// provider charge). Still record if cacheHit is true so the raw table
	// has a complete picture, but skip the aggregate write path.
	if cost.TotalUUSD == 0 && cost.TokensIn == 0 && cost.TokensOut == 0 && !resp.CacheHit {
		return
	}
	entry := spendEntry{
		tenantID:   req.TenantID,
		agentID:    req.AgentID,
		sessionID:  req.SessionID,
		providerID: resp.ProviderID,
		modelID:    resp.ModelResolved,
		cost:       cost,
		cacheHit:   resp.CacheHit,
	}
	select {
	case l.writes <- entry:
		return
	default:
	}
	// Primary buffer full — try retry queue.
	select {
	case l.retries <- entry:
		slog.Warn("gateway.cost_ledger: primary buffer full, queued to retry",
			"agent", req.AgentID, "model", resp.ModelResolved)
	default:
		// Both buffers full — log everything so the spend is traceable in logs.
		slog.Error("gateway.cost_ledger: all buffers full, SPEND LOST",
			"agent", req.AgentID, "model", resp.ModelResolved,
			"tokens_in", cost.TokensIn, "tokens_out", cost.TokensOut,
			"tokens_thinking", cost.TokensThinking,
			"tokens_cache_write", cost.TokensCacheWrite,
			"tokens_cache_read", cost.TokensCacheRead,
			"total_uusd", cost.TotalUUSD)
	}
}

func (l *CostLedger) worker() {
	for {
		select {
		case <-l.stopCh:
			return
		case e := <-l.writes:
			l.flush(e)
		}
	}
}

// retryWorker drains the retry channel and attempts to re-queue entries
// into the primary writes channel. Falls back to a direct synchronous flush
// if the primary channel is still full.
func (l *CostLedger) retryWorker() {
	for {
		select {
		case <-l.stopCh:
			return
		case e := <-l.retries:
			select {
			case l.writes <- e:
			default:
				// Primary still full — flush synchronously as last resort.
				l.flush(e)
			}
		}
	}
}

// flush performs the actual dual-write:
//  1. INSERT into gateway_spend_raw (immutable audit log, always)
//  2. UPSERT into gateway_spend (daily aggregate, skipped for cache hits
//     and entries with no agentID)
func (l *CostLedger) flush(e spendEntry) {
	if l.db == nil {
		return
	}
	ctx := context.Background()

	// Nullable FK pointers: empty string → NULL in Postgres.
	var agentIDPtr *string
	if e.agentID != "" {
		agentIDPtr = &e.agentID
	}
	var sessionIDPtr *string
	if e.sessionID != "" {
		sessionIDPtr = &e.sessionID
	}

	// 1. Append to immutable raw log — never update or delete this table.
	_, err := l.db.Exec(ctx, `
		INSERT INTO gateway_spend_raw (
			tenant_id, agent_id, session_id, provider_id, model_id,
			tokens_in, tokens_out, tokens_thinking, tokens_cache_write, tokens_cache_read,
			cost_input_uusd, cost_output_uusd, cost_thinking_uusd, cost_cache_w_uusd, cost_cache_r_uusd,
			cost_total_uusd, cache_hit
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
	`,
		e.tenantID, agentIDPtr, sessionIDPtr, e.providerID, e.modelID,
		e.cost.TokensIn, e.cost.TokensOut, e.cost.TokensThinking,
		e.cost.TokensCacheWrite, e.cost.TokensCacheRead,
		e.cost.CostInputUUSD, e.cost.CostOutputUUSD, e.cost.CostThinkingUUSD,
		e.cost.CostCacheWUUSD, e.cost.CostCacheRUUSD,
		e.cost.TotalUUSD, e.cacheHit,
	)
	if err != nil {
		slog.Error("gateway.cost_ledger: raw insert failed",
			"error", err, "agent", e.agentID, "model", e.modelID)
		// Do not return — still attempt the aggregate upsert.
	}

	// Cache hits are free — skip aggregate and budget accounting.
	if e.cacheHit {
		return
	}
	// Aggregate upsert requires an agentID.
	if e.agentID == "" {
		return
	}

	// 2. Upsert into daily aggregate (fast budget queries).
	_, err = l.db.Exec(ctx, `
		INSERT INTO gateway_spend (
			tenant_id, agent_id, provider_id, model_id,
			tokens_in, tokens_out, tokens_thinking, tokens_cache_write, tokens_cache_read,
			cost_usd, cost_total_uusd, period
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, CURRENT_DATE)
		ON CONFLICT (tenant_id, agent_id, period)
		DO UPDATE SET
			tokens_in          = gateway_spend.tokens_in          + $5,
			tokens_out         = gateway_spend.tokens_out         + $6,
			tokens_thinking    = gateway_spend.tokens_thinking    + $7,
			tokens_cache_write = gateway_spend.tokens_cache_write + $8,
			tokens_cache_read  = gateway_spend.tokens_cache_read  + $9,
			cost_usd           = gateway_spend.cost_usd           + $10,
			cost_total_uusd    = gateway_spend.cost_total_uusd    + $11
	`,
		e.tenantID, e.agentID, e.providerID, e.modelID,
		e.cost.TokensIn, e.cost.TokensOut, e.cost.TokensThinking,
		e.cost.TokensCacheWrite, e.cost.TokensCacheRead,
		e.cost.TotalUSD(), e.cost.TotalUUSD,
	)
	if err != nil {
		slog.Error("gateway.cost_ledger: aggregate upsert failed",
			"error", err, "agent", e.agentID, "model", e.modelID)
		return
	}

	if l.budget != nil && e.cost.TotalUUSD > 0 {
		l.budget.AddSpend(ctx, e.tenantID, e.agentID, e.cost.TotalUSD())
	}
}

// Stop halts the background writers. Called on gateway shutdown.
func (l *CostLedger) Stop() {
	l.once.Do(func() { close(l.stopCh) })
}
