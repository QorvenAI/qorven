// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

package llm

import (
	"context"
	"log/slog"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ModelPricing holds per-token USD pricing for one model.
type ModelPricing struct {
	InputPer1M  float64 // USD per 1M input tokens
	OutputPer1M float64 // USD per 1M output tokens
	CacheWrite  float64 // USD per 1M cache-write tokens (Anthropic prompt caching)
	CacheRead   float64 // USD per 1M cache-read tokens
}

// pricingTable is loaded from the DB at CostLedger creation time and
// refreshed every 24h via a background goroutine (future work).
// Keyed by litellm model id (e.g. "claude-sonnet-4-5").
var (
	pricingMu    sync.RWMutex
	pricingTable = map[string]ModelPricing{
		// Seed with common models so the ledger works before the DB price
		// fetch has run. Prices are USD per 1M tokens (input / output).
		"claude-opus-4-7":           {InputPer1M: 15.00, OutputPer1M: 75.00},
		"claude-sonnet-4-6":         {InputPer1M: 3.00,  OutputPer1M: 15.00},
		"claude-haiku-4-5":          {InputPer1M: 0.80,  OutputPer1M: 4.00},
		"claude-3-5-sonnet-20241022":{InputPer1M: 3.00,  OutputPer1M: 15.00},
		"claude-3-5-haiku-20241022": {InputPer1M: 0.80,  OutputPer1M: 4.00},
		"gpt-4o":                    {InputPer1M: 2.50,  OutputPer1M: 10.00},
		"gpt-4o-mini":               {InputPer1M: 0.15,  OutputPer1M: 0.60},
		"o1":                        {InputPer1M: 15.00, OutputPer1M: 60.00},
		"o3-mini":                   {InputPer1M: 1.10,  OutputPer1M: 4.40},
		"gemini-2.0-flash":          {InputPer1M: 0.075, OutputPer1M: 0.30},
		"gemini-2.5-pro":            {InputPer1M: 1.25,  OutputPer1M: 10.00},
		"gemini-2.5-flash":          {InputPer1M: 0.15,  OutputPer1M: 0.60},
		"deepseek-chat":             {InputPer1M: 0.27,  OutputPer1M: 1.10},
		"deepseek-reasoner":         {InputPer1M: 0.55,  OutputPer1M: 2.19},
		"grok-3":                    {InputPer1M: 3.00,  OutputPer1M: 15.00},
		"llama-3.3-70b-versatile":   {InputPer1M: 0.59,  OutputPer1M: 0.79},
		"mistral-large-latest":      {InputPer1M: 2.00,  OutputPer1M: 6.00},
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

// EstimateCost returns the estimated USD cost for a model call.
// Falls back to zero if no pricing data is available for the model.
func EstimateCost(model string, tokensIn, tokensOut int) float64 {
	pricingMu.RLock()
	p, ok := pricingTable[model]
	pricingMu.RUnlock()
	if !ok {
		return 0
	}
	return (float64(tokensIn)*p.InputPer1M + float64(tokensOut)*p.OutputPer1M) / 1_000_000
}

// writeChan is a buffered channel that the ledger worker drains.
// Non-blocking writes ensure the hot path is never delayed by DB latency.
const writeBufSize = 4096

type spendEntry struct {
	tenantID   string
	agentID    string
	providerID string
	modelID    string
	tokensIn   int
	tokensOut  int
	costUSD    float64
}

// CostLedger records per-request cost to gateway_spend asynchronously.
// It also notifies the BudgetEngine so the next budget check sees fresh data.
type CostLedger struct {
	db      *pgxpool.Pool
	budget  *BudgetEngine
	writes  chan spendEntry
	stopCh  chan struct{}
	once    sync.Once
}

// NewCostLedger creates a CostLedger and starts its background writer goroutine.
func NewCostLedger(db *pgxpool.Pool, budget *BudgetEngine) *CostLedger {
	l := &CostLedger{
		db:     db,
		budget: budget,
		writes: make(chan spendEntry, writeBufSize),
		stopCh: make(chan struct{}),
	}
	go l.worker()
	return l
}

// Record is called after every successful LLM call. It is non-blocking —
// it enqueues the entry and returns immediately. If the write buffer is
// full the entry is dropped (cost under-counting is better than latency).
func (l *CostLedger) Record(ctx context.Context, req GatewayRequest, resp *GatewayResponse) {
	if l.db == nil || resp == nil || resp.ChatResponse == nil {
		return
	}
	tokensIn, tokensOut := 0, 0
	if resp.Usage != nil {
		tokensIn = resp.Usage.PromptTokens
		tokensOut = resp.Usage.CompletionTokens
	}
	cost := EstimateCost(resp.ModelResolved, tokensIn, tokensOut)
	if cost == 0 && tokensIn == 0 && tokensOut == 0 {
		return
	}
	entry := spendEntry{
		tenantID:   req.TenantID,
		agentID:    req.AgentID,
		providerID: resp.ProviderID,
		modelID:    resp.ModelResolved,
		tokensIn:   tokensIn,
		tokensOut:  tokensOut,
		costUSD:    cost,
	}
	select {
	case l.writes <- entry:
	default:
		slog.Warn("gateway.cost_ledger: write buffer full, dropping spend entry", "agent", req.AgentID)
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

func (l *CostLedger) flush(e spendEntry) {
	if l.db == nil {
		return
	}
	ctx := context.Background()
	_, err := l.db.Exec(ctx, `
		INSERT INTO gateway_spend
		    (tenant_id, agent_id, provider_id, model_id, tokens_in, tokens_out, cost_usd, period)
		VALUES ($1, $2, $3, $4, $5, $6, $7, CURRENT_DATE)
		ON CONFLICT (tenant_id, agent_id, period)
		DO UPDATE SET
		    tokens_in  = gateway_spend.tokens_in  + $5,
		    tokens_out = gateway_spend.tokens_out + $6,
		    cost_usd   = gateway_spend.cost_usd   + $7
	`, e.tenantID, e.agentID, e.providerID, e.modelID, e.tokensIn, e.tokensOut, e.costUSD)
	if err != nil {
		slog.Warn("gateway.cost_ledger: failed to write spend", "error", err)
		return
	}
	if l.budget != nil && e.costUSD > 0 {
		l.budget.AddSpend(ctx, e.tenantID, e.agentID, e.costUSD)
	}
}

// Stop halts the background writer. Called on gateway shutdown.
func (l *CostLedger) Stop() {
	l.once.Do(func() { close(l.stopCh) })
}
