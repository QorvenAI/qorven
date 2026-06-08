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

// GetPricingSnapshot returns a copy of the current pricing table for reconciliation.
func GetPricingSnapshot() map[string]ModelPricing {
	pricingMu.RLock()
	defer pricingMu.RUnlock()
	out := make(map[string]ModelPricing, len(pricingTable))
	for k, v := range pricingTable {
		out[k] = v
	}
	return out
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

	// PricingMissing is true when no rate was found for this model.
	// Tokens are still recorded; cost is 0. Callers should log/alert on this.
	PricingMissing bool
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
// providerID is optional. When set, the lookup tries "providerID/model"
// first so that OpenRouter's rates (which include their markup) are used
// for calls routed through OpenRouter, while direct Anthropic/OpenAI calls
// use provider-native rates. Falls back to bare model ID if no
// provider-specific price exists.
//
// For unknown models the cost is 0 (safe: we undercharge rather than
// overcharge). Token counts are still recorded for audit purposes.
func ComputeCost(model string, usage *providers.Usage, providerID ...string) CallCost {
	if usage == nil {
		return CallCost{}
	}

	pricingMu.RLock()
	var p ModelPricing
	var ok bool
	if len(providerID) > 0 && providerID[0] != "" {
		p, ok = pricingTable[providerID[0]+"/"+model]
	}
	if !ok {
		p, ok = pricingTable[model]
	}
	pricingMu.RUnlock()

	c := CallCost{
		TokensIn:         usage.PromptTokens,
		TokensOut:        usage.CompletionTokens,
		TokensThinking:   usage.ThinkingTokens,
		TokensCacheWrite: usage.CacheCreationTokens,
		TokensCacheRead:  usage.CacheReadTokens,
	}
	if !ok {
		// Unknown model — tokens recorded, cost is 0.
		// PricingMissing lets callers emit warnings and update the feed.
		c.PricingMissing = true
		slog.Warn("gateway.cost: pricing not found, tokens recorded but cost is zero",
			"model", model)
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
	keyID      string // UUID of the specific provider_key used; empty if unknown
	cost       CallCost
	cacheHit   bool
	// pricingMissing mirrors cost.PricingMissing — duplicated here so
	// flush() can write it to DB without deriving it from cost again.
	pricingMissing bool
	// origin classifies what triggered the call (agent, memory, council, …)
	// for cost attribution. Empty for pipeline (agent-path) calls.
	origin string
}

// writeBufSize is the primary channel buffer depth.
const writeBufSize = 8192

// GapReporter is implemented by gateway.CapabilityGapReporter.
// The interface lives here to keep the llm package free of gateway imports.
type GapReporter interface {
	ReportPricingGap(ctx context.Context, modelID, providerID string, tokensIn, tokensOut int)
}

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
	gaps    GapReporter // optional — notifies Prime of unknown model prices
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

// WithGapReporter attaches a CapabilityGapReporter so Prime is notified
// whenever a model with unknown pricing is encountered.
func (l *CostLedger) WithGapReporter(r GapReporter) {
	l.gaps = r
}

// Record is called after every LLM call. Non-blocking — enqueues for async write.
func (l *CostLedger) Record(ctx context.Context, req GatewayRequest, resp *GatewayResponse) {
	if l.db == nil || resp == nil || resp.ChatResponse == nil {
		return
	}
	cost := ComputeCost(resp.ModelResolved, resp.Usage, resp.ProviderID)
	// Skip only calls with zero tokens and no cost — nothing to record.
	// Calls with tokens but missing pricing still go through so the raw
	// audit log captures the token consumption even when we can't price it.
	if cost.TotalUUSD == 0 && cost.TokensIn == 0 && cost.TokensOut == 0 &&
		cost.TokensThinking == 0 && !resp.CacheHit && !cost.PricingMissing {
		return
	}
	if cost.PricingMissing {
		slog.Warn("gateway.cost_ledger: unknown model price, tokens recorded with zero cost",
			"model", resp.ModelResolved, "provider", resp.ProviderID,
			"tokens_in", cost.TokensIn, "tokens_out", cost.TokensOut)
		if l.gaps != nil {
			go l.gaps.ReportPricingGap(ctx, resp.ModelResolved, resp.ProviderID, cost.TokensIn, cost.TokensOut)
		}
	}
	entry := spendEntry{
		tenantID:       req.TenantID,
		agentID:        req.AgentID,
		sessionID:      req.SessionID,
		providerID:     resp.ProviderID,
		modelID:        resp.ModelResolved,
		keyID:          resp.KeyID,
		cost:           cost,
		cacheHit:       resp.CacheHit,
		pricingMissing: cost.PricingMissing,
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

// RecordScoped records a completed LLM call attributed by MeterScope. Entry
// point for calls that do NOT go through the gateway pipeline (memory,
// background, council, etc.). Reuses the same async ledger as Record.
// Implements providers.Recorder.
func (l *CostLedger) RecordScoped(ctx context.Context, scope providers.MeterScope, model, providerID, keyID string, usage providers.Usage) {
	if l.db == nil {
		return
	}
	cost := ComputeCost(model, &usage, providerID)
	if cost.TotalUUSD == 0 && cost.TokensIn == 0 && cost.TokensOut == 0 &&
		cost.TokensThinking == 0 && !cost.PricingMissing {
		return
	}
	if cost.PricingMissing {
		slog.Warn("gateway.cost_ledger: unknown model price (scoped)",
			"model", model, "provider", providerID, "origin", scope.Origin)
		if l.gaps != nil {
			go l.gaps.ReportPricingGap(ctx, model, providerID, cost.TokensIn, cost.TokensOut)
		}
	}
	entry := spendEntry{
		tenantID:       scope.TenantID,
		agentID:        scope.AgentID,
		sessionID:      scope.SessionID,
		providerID:     providerID,
		modelID:        model,
		keyID:          keyID,
		cost:           cost,
		pricingMissing: cost.PricingMissing,
		origin:         scope.Origin,
	}
	select {
	case l.writes <- entry:
	default:
		select {
		case l.retries <- entry:
		default:
			slog.Error("gateway.cost_ledger: buffers full, SCOPED SPEND LOST",
				"agent", scope.AgentID, "origin", scope.Origin, "model", model,
				"total_uusd", cost.TotalUUSD)
		}
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

	// key_id is optional — NULL if the call didn't go through the key pool
	var keyIDPtr *string
	if e.keyID != "" {
		keyIDPtr = &e.keyID
	}

	// 1. Append to immutable raw log — never update or delete this table.
	_, err := l.db.Exec(ctx, `
		INSERT INTO gateway_spend_raw (
			tenant_id, agent_id, session_id, provider_id, model_id, key_id,
			tokens_in, tokens_out, tokens_thinking, tokens_cache_write, tokens_cache_read,
			cost_input_uusd, cost_output_uusd, cost_thinking_uusd, cost_cache_w_uusd, cost_cache_r_uusd,
			cost_total_uusd, cache_hit, pricing_missing, origin
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20)
	`,
		e.tenantID, agentIDPtr, sessionIDPtr, e.providerID, e.modelID, keyIDPtr,
		e.cost.TokensIn, e.cost.TokensOut, e.cost.TokensThinking,
		e.cost.TokensCacheWrite, e.cost.TokensCacheRead,
		e.cost.CostInputUUSD, e.cost.CostOutputUUSD, e.cost.CostThinkingUUSD,
		e.cost.CostCacheWUUSD, e.cost.CostCacheRUUSD,
		e.cost.TotalUUSD, e.cacheHit, e.pricingMissing, e.origin,
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
			tenant_id, agent_id, provider_id, model_id, key_id,
			tokens_in, tokens_out, tokens_thinking, tokens_cache_write, tokens_cache_read,
			cost_usd, cost_total_uusd, period
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, CURRENT_DATE)
		ON CONFLICT (tenant_id, agent_id, period)
		DO UPDATE SET
			tokens_in          = gateway_spend.tokens_in          + $6,
			tokens_out         = gateway_spend.tokens_out         + $7,
			tokens_thinking    = gateway_spend.tokens_thinking    + $8,
			tokens_cache_write = gateway_spend.tokens_cache_write + $9,
			tokens_cache_read  = gateway_spend.tokens_cache_read  + $10,
			cost_usd           = gateway_spend.cost_usd           + $11,
			cost_total_uusd    = gateway_spend.cost_total_uusd    + $12
	`,
		e.tenantID, e.agentID, e.providerID, e.modelID, keyIDPtr,
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
		l.budget.AddSpend(ctx, e.tenantID, e.agentID, e.cost.TotalUSD(), e.cost.TokensIn, e.cost.TokensOut)
	}

	// 3a. Update provider_keys spend counters so per-key budget enforcement
	// always sees fresh data (spent_usd_month, spent_tokens_month).
	if keyIDPtr != nil && e.cost.TotalUUSD > 0 {
		totalTokens := int64(e.cost.TokensIn + e.cost.TokensOut + e.cost.TokensThinking)
		_, kerr := l.db.Exec(ctx, `
			UPDATE provider_keys
			SET spent_usd_month    = spent_usd_month    + $1,
			    spent_tokens_month = spent_tokens_month + $2,
			    total_requests     = total_requests     + 1,
			    total_tokens_in    = total_tokens_in    + $3,
			    total_tokens_out   = total_tokens_out   + $4,
			    last_used_at       = now()
			WHERE id = $5
		`, e.cost.TotalUSD(), totalTokens, e.cost.TokensIn, e.cost.TokensOut, keyIDPtr)
		if kerr != nil {
			slog.Warn("gateway.cost_ledger: provider_keys spend update failed",
				"key_id", e.keyID, "error", kerr)
		}
	}

	// 4. Upsert a trace row (one per session) and append a span (one per LLM call).
	// Traces aggregate token + cost totals; spans are the individual call records.
	if e.sessionID != "" && e.agentID != "" {
		costCents := int(e.cost.TotalUSD() * 100)
		// Upsert trace: create on first call in a session, accumulate on subsequent ones.
		_, err = l.db.Exec(ctx, `
			INSERT INTO traces (tenant_id, agent_id, session_key, total_input_tokens, total_output_tokens, total_cost_cents, status)
			VALUES ($1, $2::uuid, $3, $4, $5, $6, 'ok')
			ON CONFLICT (tenant_id, session_key)
			DO UPDATE SET
				total_input_tokens  = traces.total_input_tokens  + $4,
				total_output_tokens = traces.total_output_tokens + $5,
				total_cost_cents    = traces.total_cost_cents    + $6,
				end_time            = now(),
				duration_ms         = EXTRACT(EPOCH FROM (now() - traces.start_time)) * 1000
		`,
			e.tenantID, e.agentID, e.sessionID,
			e.cost.TokensIn, e.cost.TokensOut, costCents,
		)
		if err != nil {
			slog.Warn("gateway.cost_ledger: trace upsert failed", "error", err, "session", e.sessionID)
		}

		// Insert span — look up the trace id we just upserted.
		var traceID string
		qerr := l.db.QueryRow(ctx,
			`SELECT id FROM traces WHERE tenant_id = $1 AND session_key = $2 LIMIT 1`,
			e.tenantID, e.sessionID,
		).Scan(&traceID)
		if qerr == nil && traceID != "" {
			_, _ = l.db.Exec(ctx, `
				INSERT INTO spans (tenant_id, trace_id, span_type, model, provider, input_tokens, output_tokens, cost_cents, status)
				VALUES ($1, $2::uuid, 'llm', $3, $4, $5, $6, $7, 'ok')
			`,
				e.tenantID, traceID, e.modelID, e.providerID,
				e.cost.TokensIn, e.cost.TokensOut, costCents,
			)
		}
	}
}

// Stop halts the background writers. Called on gateway shutdown.
func (l *CostLedger) Stop() {
	l.once.Do(func() { close(l.stopCh) })
}

// PricingGap is one model whose calls were recorded without a known price.
type PricingGap struct {
	ModelID    string `json:"model_id"`
	ProviderID string `json:"provider_id"`
	CallCount  int    `json:"call_count"`
	TokensIn   int64  `json:"tokens_in"`
	TokensOut  int64  `json:"tokens_out"`
	OldestAt   string `json:"oldest_at"`
}

// QueryPricingGaps returns a list of models that have gateway_spend_raw rows
// with pricing_missing=true, ordered by token volume descending.
func QueryPricingGaps(ctx context.Context, db *pgxpool.Pool) ([]PricingGap, error) {
	rows, err := db.Query(ctx, `
		SELECT
			model_id,
			COALESCE(provider_id, '') AS provider_id,
			COUNT(*)                  AS call_count,
			SUM(tokens_in)            AS tokens_in,
			SUM(tokens_out)           AS tokens_out,
			MIN(created_at)::text     AS oldest_at
		FROM gateway_spend_raw
		WHERE pricing_missing = true
		GROUP BY model_id, provider_id
		ORDER BY (SUM(tokens_in) + SUM(tokens_out)) DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var gaps []PricingGap
	for rows.Next() {
		var g PricingGap
		if err := rows.Scan(&g.ModelID, &g.ProviderID, &g.CallCount, &g.TokensIn, &g.TokensOut, &g.OldestAt); err != nil {
			return nil, err
		}
		gaps = append(gaps, g)
	}
	return gaps, rows.Err()
}

// BackfillResult summarises what BackfillMissingPrices did.
type BackfillResult struct {
	RowsUpdated int `json:"rows_updated"`
	ModelsFixed int `json:"models_fixed"`
}

// BackfillMissingPrices reprices every gateway_spend_raw row that has
// pricing_missing=true, using the current in-process pricingTable.
//
// For each such row it recalculates all five cost components using the
// same integer µUSD arithmetic as ComputeCost, then:
//  1. Updates gateway_spend_raw in place (flips pricing_missing=false, sets costs).
//  2. Adds the delta cost to the matching gateway_spend aggregate row.
//
// Safe to call at any time — rows that still have no rate are left untouched.
// Called automatically by the pricing aggregator after every successful Refresh.
func BackfillMissingPrices(ctx context.Context, db *pgxpool.Pool) (BackfillResult, error) {
	if db == nil {
		return BackfillResult{}, nil
	}

	// Fetch all distinct (model_id, provider_id) combos that need backfill.
	type rawRow struct {
		id             string
		modelID        string
		providerID     string
		tokensIn       int
		tokensOut      int
		tokensThinking int
		tokensCacheW   int
		tokensCacheR   int
		agentID        *string
		tenantID       string
		period         string
	}

	rows, err := db.Query(ctx, `
		SELECT id, model_id, COALESCE(provider_id,''),
		       tokens_in, tokens_out, tokens_thinking, tokens_cache_write, tokens_cache_read,
		       agent_id, tenant_id::text,
		       created_at::date::text
		FROM gateway_spend_raw
		WHERE pricing_missing = true
		ORDER BY created_at
	`)
	if err != nil {
		return BackfillResult{}, err
	}
	defer rows.Close()

	var pending []rawRow
	for rows.Next() {
		var r rawRow
		if err := rows.Scan(
			&r.id, &r.modelID, &r.providerID,
			&r.tokensIn, &r.tokensOut, &r.tokensThinking, &r.tokensCacheW, &r.tokensCacheR,
			&r.agentID, &r.tenantID, &r.period,
		); err != nil {
			return BackfillResult{}, err
		}
		pending = append(pending, r)
	}
	if err := rows.Err(); err != nil {
		return BackfillResult{}, err
	}

	fixedModels := map[string]struct{}{}
	updated := 0

	for _, r := range pending {
		usage := &providers.Usage{
			PromptTokens:        r.tokensIn,
			CompletionTokens:    r.tokensOut,
			ThinkingTokens:      r.tokensThinking,
			CacheCreationTokens: r.tokensCacheW,
			CacheReadTokens:     r.tokensCacheR,
		}
		cost := ComputeCost(r.modelID, usage, r.providerID)
		if cost.PricingMissing {
			// Rate still not known — skip.
			continue
		}

		// 1. Update raw row.
		_, err := db.Exec(ctx, `
			UPDATE gateway_spend_raw SET
				cost_input_uusd    = $2,
				cost_output_uusd   = $3,
				cost_thinking_uusd = $4,
				cost_cache_w_uusd  = $5,
				cost_cache_r_uusd  = $6,
				cost_total_uusd    = $7,
				pricing_missing    = false
			WHERE id = $1
		`,
			r.id,
			cost.CostInputUUSD, cost.CostOutputUUSD, cost.CostThinkingUUSD,
			cost.CostCacheWUUSD, cost.CostCacheRUUSD, cost.TotalUUSD,
		)
		if err != nil {
			slog.Error("gateway.backfill: raw update failed", "id", r.id, "model", r.modelID, "err", err)
			continue
		}

		// 2. Add cost delta to daily aggregate (if the row has an agent).
		if r.agentID != nil && *r.agentID != "" && cost.TotalUUSD > 0 {
			_, err = db.Exec(ctx, `
				INSERT INTO gateway_spend
					(tenant_id, agent_id, provider_id, model_id,
					 tokens_in, tokens_out, tokens_thinking, tokens_cache_write, tokens_cache_read,
					 cost_usd, cost_total_uusd, period)
				VALUES ($1,$2,$3,$4, 0,0,0,0,0, $5,$6, $7::date)
				ON CONFLICT (tenant_id, agent_id, period)
				DO UPDATE SET
					cost_usd        = gateway_spend.cost_usd        + $5,
					cost_total_uusd = gateway_spend.cost_total_uusd + $6
			`,
				r.tenantID, *r.agentID, r.providerID, r.modelID,
				cost.TotalUSD(), cost.TotalUUSD, r.period,
			)
			if err != nil {
				slog.Error("gateway.backfill: aggregate update failed", "agent", *r.agentID, "model", r.modelID, "err", err)
			}
		}

		fixedModels[r.modelID] = struct{}{}
		updated++
	}

	result := BackfillResult{RowsUpdated: updated, ModelsFixed: len(fixedModels)}
	if updated > 0 {
		slog.Info("gateway.backfill: repriced rows", "rows", updated, "models", len(fixedModels))
	}
	return result, nil
}
