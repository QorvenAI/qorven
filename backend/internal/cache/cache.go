// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package cache

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"
)

// Cache is a generic key-value cache interface.
type Cache[V any] interface {
	Get(ctx context.Context, key string) (V, bool)
	Set(ctx context.Context, key string, value V, ttl time.Duration)
	Delete(ctx context.Context, key string)
	DeleteByPrefix(ctx context.Context, prefix string)
	Clear(ctx context.Context)
}

// NewFromEnv creates a Cache using the backend configured by QORVEN_CACHE_BACKEND.
//
//   - "redis" (or "redis:<url>") — Redis-backed cache; requires REDIS_URL if no
//     inline URL is provided. Falls back to in-memory and logs a warning on
//     connection failure so the process keeps running.
//   - anything else (default) — in-memory cache.
//
// The prefix is used to namespace Redis keys: "qorven:<prefix>:*".
func NewFromEnv[V any](prefix string) Cache[V] {
	backend := os.Getenv("QORVEN_CACHE_BACKEND")
	if backend == "" || backend == "inmemory" || backend == "memory" {
		return NewInMemoryCache[V]()
	}

	// Accept "redis" (uses REDIS_URL) or "redis:<url>"
	redisURL := os.Getenv("REDIS_URL")
	if len(backend) > 6 && backend[:6] == "redis:" && backend != "redis://" {
		// inline URL: QORVEN_CACHE_BACKEND=redis:redis://localhost:6379
		redisURL = backend[6:]
	}
	if redisURL == "" {
		slog.Warn("cache: QORVEN_CACHE_BACKEND=redis but REDIS_URL not set; falling back to inmemory")
		return NewInMemoryCache[V]()
	}

	client, err := NewRedisClient(redisURL)
	if err != nil {
		slog.Warn("cache: redis connection failed; falling back to inmemory", "error", err)
		return NewInMemoryCache[V]()
	}
	slog.Info("cache: using redis backend", "prefix", fmt.Sprintf("qorven:%s", prefix))
	return NewRedisCache[V](client, prefix)
}
