// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package providers

import (
	"sync"
	"sync/atomic"
	"log/slog"
)

// CredentialPool manages multiple API keys for the same provider
// with automatic least_used rotation and 401 failover.
type CredentialPool struct {
	mu       sync.RWMutex
	keys     []*PoolKey
	strategy string // "least_used", "round_robin"
}

type PoolKey struct {
	Key      string
	Label    string
	UseCount atomic.Int64
	Failed   bool
}

func NewCredentialPool(strategy string) *CredentialPool {
	if strategy == "" { strategy = "least_used" }
	return &CredentialPool{strategy: strategy}
}

func (p *CredentialPool) Add(key, label string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.keys = append(p.keys, &PoolKey{Key: key, Label: label})
}

// Next returns the next key to use based on strategy.
func (p *CredentialPool) Next() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if len(p.keys) == 0 { return "" }

	var best *PoolKey
	var bestCount int64 = 1<<62
	for _, k := range p.keys {
		if k.Failed { continue }
		count := k.UseCount.Load()
		if count < bestCount {
			bestCount = count
			best = k
		}
	}
	if best == nil { return "" }
	best.UseCount.Add(1)
	return best.Key
}

// RotateOn401 marks the current key as failed and returns the next one.
func (p *CredentialPool) RotateOn401(failedKey string) string {
	p.mu.Lock()
	for _, k := range p.keys {
		if k.Key == failedKey {
			k.Failed = true
			slog.Warn("credential_pool.rotated", "label", k.Label, "reason", "401")
			break
		}
	}
	p.mu.Unlock()
	return p.Next()
}

func (p *CredentialPool) Size() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.keys)
}

func (p *CredentialPool) ActiveCount() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	count := 0
	for _, k := range p.keys {
		if !k.Failed {
			count++
		}
	}
	return count
}
