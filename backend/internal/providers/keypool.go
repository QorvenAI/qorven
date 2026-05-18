// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package providers

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qorvenai/qorven/internal/crypto"
)

// KeyRecord represents a single API key in the pool.
type KeyRecord struct {
	ID               string     `json:"id"`
	ProviderID       string     `json:"provider_id"`
	Label            string     `json:"label"`
	KeyHash          string     `json:"key_hash"` // last 4 chars for display
	Status           string     `json:"status"`   // unverified, verified, failed, retired
	VerifiedAt       *time.Time `json:"verified_at"`
	LastUsedAt       *time.Time `json:"last_used_at"`
	RateLimitedUntil *time.Time `json:"rate_limited_until"`
	RotationOrder    int        `json:"rotation_order"`
	TotalRequests    int64      `json:"total_requests"`
	TotalTokensIn    int64      `json:"total_tokens_in"`
	TotalTokensOut   int64      `json:"total_tokens_out"`
	// Budget fields (loaded from DB col; nil = unlimited)
	BudgetUSDMonthly    *float64   `json:"budget_usd_monthly,omitempty"`
	BudgetTokensMonthly *int64     `json:"budget_tokens_monthly,omitempty"`
	SpentUSDMonth       float64    `json:"spent_usd_month"`
	SpentTokensMonth    int64      `json:"spent_tokens_month"`
	BudgetResetAt       *time.Time `json:"budget_reset_at,omitempty"`
	encryptedKey        []byte
}

// PoolConfig holds provider-level rotation settings.
type PoolConfig struct {
	Strategy     RotationStrategy `json:"strategy"`
	FailoverMode string           `json:"failover_mode"` // on_exhaust | on_error | always
}

// RotationStrategy controls how keys are selected.
type RotationStrategy string

const (
	StrategyRoundRobin RotationStrategy = "round_robin"
	StrategyPriority   RotationStrategy = "priority"
	StrategyLeastUsed  RotationStrategy = "least_used"
	StrategyRandom     RotationStrategy = "random"
)

// KeyPool manages multiple API keys for a provider with rotation.
type KeyPool struct {
	mu            sync.Mutex
	keys          []*KeyRecord
	cursor        int
	strategy      RotationStrategy
	encryptionKey string
}

func NewKeyPool(keys []*KeyRecord, strategy RotationStrategy, encKey string) *KeyPool {
	return &KeyPool{keys: keys, strategy: strategy, encryptionKey: encKey}
}

// Next returns the next available key. Skips rate-limited, over-budget, and non-verified keys.
func (p *KeyPool) Next() (*KeyRecord, string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now()
	total := len(p.keys)
	if total == 0 {
		return nil, "", fmt.Errorf("no keys configured")
	}

	for i := 0; i < total; i++ {
		var k *KeyRecord
		switch p.strategy {
		case StrategyLeastUsed:
			k = p.leastUsed(now)
		case StrategyPriority:
			k = p.priority(now)
		case StrategyRandom:
			k = p.randomAvailable(now)
		default: // round_robin
			idx := (p.cursor + i) % total
			candidate := p.keys[idx]
			if p.isAvailable(candidate, now) {
				p.cursor = (idx + 1) % total
				k = candidate
			}
		}
		if k != nil {
			decrypted, err := crypto.Decrypt(k.encryptedKey, p.encryptionKey)
			if err != nil {
				continue
			}
			return k, string(decrypted), nil
		}
		// For non-iterating strategies, break after one attempt
		if p.strategy != StrategyRoundRobin {
			break
		}
	}
	return nil, "", fmt.Errorf("all keys exhausted, rate-limited, or over budget")
}

// isAvailable returns true if the key can be used right now.
func (p *KeyPool) isAvailable(k *KeyRecord, now time.Time) bool {
	if k.Status != "verified" {
		return false
	}
	if k.RateLimitedUntil != nil && now.Before(*k.RateLimitedUntil) {
		return false
	}
	// Reset monthly counters if past reset time
	if k.BudgetResetAt != nil && now.After(*k.BudgetResetAt) {
		k.SpentUSDMonth = 0
		k.SpentTokensMonth = 0
		next := k.BudgetResetAt.AddDate(0, 1, 0)
		k.BudgetResetAt = &next
	}
	if k.BudgetUSDMonthly != nil && k.SpentUSDMonth >= *k.BudgetUSDMonthly {
		return false
	}
	if k.BudgetTokensMonthly != nil && k.SpentTokensMonth >= *k.BudgetTokensMonthly {
		return false
	}
	return true
}

func (p *KeyPool) randomAvailable(now time.Time) *KeyRecord {
	var avail []*KeyRecord
	for _, k := range p.keys {
		if p.isAvailable(k, now) {
			avail = append(avail, k)
		}
	}
	if len(avail) == 0 {
		return nil
	}
	return avail[rand.Intn(len(avail))]
}

func (p *KeyPool) leastUsed(now time.Time) *KeyRecord {
	var best *KeyRecord
	for _, k := range p.keys {
		if !p.isAvailable(k, now) { continue }
		if best == nil || k.TotalRequests < best.TotalRequests { best = k }
	}
	return best
}

func (p *KeyPool) priority(now time.Time) *KeyRecord {
	for _, k := range p.keys {
		if p.isAvailable(k, now) { return k }
	}
	return nil
}

// MarkRateLimited sets a cooldown on a key.
func (p *KeyPool) MarkRateLimited(keyID string, until time.Time) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, k := range p.keys {
		if k.ID == keyID {
			k.RateLimitedUntil = &until
			slog.Info("key.rate_limited", "key", k.KeyHash, "until", until)
			return
		}
	}
}

// RecordUsage updates counters on a key.
func (p *KeyPool) RecordUsage(keyID string, tokensIn, tokensOut int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	now := time.Now()
	for _, k := range p.keys {
		if k.ID == keyID {
			k.TotalRequests++
			k.TotalTokensIn += int64(tokensIn)
			k.TotalTokensOut += int64(tokensOut)
			k.LastUsedAt = &now
			return
		}
	}
}

// KeyPoolStore manages provider keys in the database.
type KeyPoolStore struct {
	pool          *pgxpool.Pool
	encryptionKey string
}

func NewKeyPoolStore(pool *pgxpool.Pool, encKey string) *KeyPoolStore {
	return &KeyPoolStore{pool: pool, encryptionKey: encKey}
}

// AddKey encrypts and stores a new API key.
func (s *KeyPoolStore) AddKey(ctx context.Context, tenantID, providerID, label, rawKey string) (*KeyRecord, error) {
	encrypted, err := crypto.Encrypt([]byte(rawKey), s.encryptionKey)
	if err != nil {
		return nil, err
	}
	hash := hashKey(rawKey)
	kr := &KeyRecord{}
	err = s.pool.QueryRow(ctx,
		`INSERT INTO provider_keys (tenant_id, provider_id, label, key_hash, key_enc, status)
		 VALUES ($1, $2, $3, $4, $5, 'unverified') RETURNING id, created_at`,
		tenantID, providerID, label, hash, encrypted).Scan(&kr.ID, &kr.LastUsedAt)
	kr.ProviderID = providerID
	kr.Label = label
	kr.KeyHash = hash
	kr.Status = "unverified"
	return kr, err
}

// ListKeys returns all keys for a provider, including budget fields.
func (s *KeyPoolStore) ListKeys(ctx context.Context, tenantID, providerID string) ([]*KeyRecord, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, provider_id, COALESCE(label,''), key_hash, key_enc, status,
		        verified_at, last_used_at, rate_limited_until, rotation_order,
		        total_requests, total_tokens_in, total_tokens_out,
		        budget_usd_monthly, budget_tokens_monthly,
		        spent_usd_month, spent_tokens_month, budget_reset_at
		 FROM provider_keys
		 WHERE tenant_id = $1 AND provider_id = $2 AND status != 'retired'
		 ORDER BY rotation_order, created_at`,
		tenantID, providerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	keys := []*KeyRecord{}
	for rows.Next() {
		k := &KeyRecord{}
		rows.Scan(
			&k.ID, &k.ProviderID, &k.Label, &k.KeyHash, &k.encryptedKey,
			&k.Status, &k.VerifiedAt, &k.LastUsedAt, &k.RateLimitedUntil,
			&k.RotationOrder, &k.TotalRequests, &k.TotalTokensIn, &k.TotalTokensOut,
			&k.BudgetUSDMonthly, &k.BudgetTokensMonthly,
			&k.SpentUSDMonth, &k.SpentTokensMonth, &k.BudgetResetAt,
		)
		keys = append(keys, k)
	}
	return keys, nil
}

// VerifyKey marks a key as verified.
func (s *KeyPoolStore) VerifyKey(ctx context.Context, keyID string) error {
	_, err := s.pool.Exec(ctx, `UPDATE provider_keys SET status = 'verified', verified_at = now() WHERE id = $1`, keyID)
	return err
}

// RetireKey soft-deletes a key (preserves logs).
func (s *KeyPoolStore) RetireKey(ctx context.Context, keyID string) error {
	_, err := s.pool.Exec(ctx, `UPDATE provider_keys SET status = 'retired' WHERE id = $1`, keyID)
	return err
}

// MarkKeyFailed marks a key as failed after a live verification error.
func (s *KeyPoolStore) MarkKeyFailed(ctx context.Context, keyID string) error {
	_, err := s.pool.Exec(ctx, `UPDATE provider_keys SET status = 'failed' WHERE id = $1`, keyID)
	return err
}

// LogUsage writes a usage record.
func (s *KeyPoolStore) LogUsage(ctx context.Context, keyID string, agentID, model string, tokensIn, tokensOut, latencyMs int, status, errMsg string) {
	s.pool.Exec(ctx,
		`INSERT INTO key_usage_log (key_id, agent_id, model, tokens_in, tokens_out, latency_ms, status, error_msg)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		keyID, nilIfEmpty(agentID), model, tokensIn, tokensOut, latencyMs, status, errMsg)
	// Update denormalized counters
	s.pool.Exec(ctx,
		`UPDATE provider_keys SET total_requests = total_requests + 1, total_tokens_in = total_tokens_in + $1, total_tokens_out = total_tokens_out + $2, last_used_at = now() WHERE id = $3`,
		tokensIn, tokensOut, keyID)
}

func nilIfEmpty(s string) *string {
	if s == "" { return nil }
	return &s
}

func hashKey(key string) string {
	if len(key) < 8 { return "••••" }
	return fmt.Sprintf("••••%s", key[len(key)-4:])
}

// BuildPool creates a KeyPool from stored keys.
func (s *KeyPoolStore) BuildPool(ctx context.Context, tenantID, providerID string, strategy RotationStrategy) (*KeyPool, error) {
	keys, err := s.ListKeys(ctx, tenantID, providerID)
	if err != nil {
		return nil, err
	}
	return NewKeyPool(keys, strategy, s.encryptionKey), nil
}

func (k *KeyRecord) EncryptedKey() []byte { return k.encryptedKey }


// DecryptKeyBytes decrypts an encrypted key using the given encryption key.
func DecryptKeyBytes(enc []byte, encKey string) (string, error) {
	if len(enc) == 0 { return "", nil }
	if encKey == "" { return string(enc), nil }
	b, err := crypto.Decrypt(enc, encKey)
	if err != nil { return string(enc), nil }
	return string(b), nil
}

// SelectLeastUsed picks the key with fewest total requests (not rate-limited, not over-budget).
func (p *KeyPool) SelectLeastUsed() *KeyRecord {
	p.mu.Lock()
	defer p.mu.Unlock()
	now := time.Now()
	var best *KeyRecord
	for _, k := range p.keys {
		if !p.isAvailable(k, now) { continue }
		if best == nil || k.TotalRequests < best.TotalRequests { best = k }
	}
	return best
}

// RetryWith429 attempts the call, rotating keys on 429/401 errors.
func (p *KeyPool) RetryWith429(maxRetries int, fn func(apiKey string) (int, error)) error {
	for i := 0; i < maxRetries; i++ {
		key := p.SelectLeastUsed()
		if key == nil { return fmt.Errorf("all keys exhausted or rate-limited") }
		decrypted, err := crypto.Decrypt(key.encryptedKey, p.encryptionKey)
		if err != nil { return err }
		status, err := fn(string(decrypted))
		if err == nil { return nil }
		switch status {
		case 429:
			t := time.Now().Add(60 * time.Second)
			p.MarkRateLimited(key.ID, t)
			slog.Info("keypool.429_retry", "key", key.KeyHash, "attempt", i+1)
		case 401, 403:
			t := time.Now().Add(24 * time.Hour)
			p.MarkRateLimited(key.ID, t)
		default:
			return err
		}
	}
	return fmt.Errorf("all %d retries exhausted", maxRetries)
}

// ─── Pool config (strategy / failover) ────────────────────────────────────────

// LoadPoolConfig returns the rotation config for a provider, or defaults.
func (s *KeyPoolStore) LoadPoolConfig(ctx context.Context, tenantID, providerID string) (PoolConfig, error) {
	cfg := PoolConfig{Strategy: StrategyPriority, FailoverMode: "on_exhaust"}
	err := s.pool.QueryRow(ctx,
		`SELECT strategy, failover_mode FROM provider_pool_config WHERE tenant_id = $1 AND provider_id = $2`,
		tenantID, providerID).Scan(&cfg.Strategy, &cfg.FailoverMode)
	if err != nil { // no row = use defaults
		return cfg, nil
	}
	return cfg, nil
}

// SavePoolConfig upserts rotation strategy and failover mode.
func (s *KeyPoolStore) SavePoolConfig(ctx context.Context, tenantID, providerID string, cfg PoolConfig) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO provider_pool_config (tenant_id, provider_id, strategy, failover_mode, updated_at)
		 VALUES ($1, $2, $3, $4, now())
		 ON CONFLICT (tenant_id, provider_id)
		 DO UPDATE SET strategy = $3, failover_mode = $4, updated_at = now()`,
		tenantID, providerID, string(cfg.Strategy), cfg.FailoverMode)
	return err
}

// ─── Budget management ─────────────────────────────────────────────────────────

// SetKeyBudget updates the monthly budget caps for a key.
func (s *KeyPoolStore) SetKeyBudget(ctx context.Context, keyID string, budgetUSD *float64, budgetTokens *int64) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE provider_keys SET budget_usd_monthly = $1, budget_tokens_monthly = $2 WHERE id = $3`,
		budgetUSD, budgetTokens, keyID)
	return err
}

// RecordSpend adds cost to the key's monthly spend counters.
func (s *KeyPoolStore) RecordSpend(ctx context.Context, keyID string, usdCost float64, tokens int64) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE provider_keys
		 SET spent_usd_month = spent_usd_month + $1,
		     spent_tokens_month = spent_tokens_month + $2
		 WHERE id = $3`,
		usdCost, tokens, keyID)
	return err
}

// ResetMonthlySpend zeroes spend counters and advances reset_at by one month.
// Called by a cron job at the start of each billing period.
func (s *KeyPoolStore) ResetMonthlySpend(ctx context.Context, tenantID string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE provider_keys
		 SET spent_usd_month = 0, spent_tokens_month = 0,
		     budget_reset_at = budget_reset_at + INTERVAL '1 month'
		 WHERE tenant_id = $1 AND budget_reset_at <= now()`,
		tenantID)
	return err
}

// BuildPoolWithConfig loads config + keys and builds a KeyPool.
func (s *KeyPoolStore) BuildPoolWithConfig(ctx context.Context, tenantID, providerID string) (*KeyPool, PoolConfig, error) {
	cfg, err := s.LoadPoolConfig(ctx, tenantID, providerID)
	if err != nil {
		return nil, cfg, err
	}
	pool, err := s.BuildPool(ctx, tenantID, providerID, cfg.Strategy)
	return pool, cfg, err
}
