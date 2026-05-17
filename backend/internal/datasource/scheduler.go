// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

package datasource

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/adhocore/gronx"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qorvenai/qorven/internal/apps"
	"github.com/qorvenai/qorven/internal/tools"
)

// Scheduler reads installed connector apps that have data_source config and
// registers recurring cron jobs that snapshot tool output to PostgreSQL.
type Scheduler struct {
	pool      *pgxpool.Pool
	snapshots *SnapshotStore
	runTool   func(ctx context.Context, slug, tool string, args map[string]any) (*tools.Result, error)
	tenantID  string

	mu      sync.Mutex
	jobs    map[string]*dsJob // keyed by slug
	stop    chan struct{}
	running atomic.Bool
}

// dsJob holds a registered data source job.
type dsJob struct {
	slug       string
	cfg        *apps.DataSourceConfig
	nextRun    time.Time
	inProgress bool
}

// NewScheduler creates a Scheduler.
func NewScheduler(
	pool *pgxpool.Pool,
	snapshots *SnapshotStore,
	runTool func(ctx context.Context, slug, tool string, args map[string]any) (*tools.Result, error),
	tenantID string,
) *Scheduler {
	return &Scheduler{
		pool:      pool,
		snapshots: snapshots,
		runTool:   runTool,
		tenantID:  tenantID,
		jobs:      make(map[string]*dsJob),
		stop:      make(chan struct{}),
	}
}

// SyncAll reads all enabled apps for the tenant, parses their app.yaml for
// DataSource config, and registers cron jobs for any that have DataSource.Enabled=true.
func (s *Scheduler) SyncAll(ctx context.Context) error {
	rows, err := s.pool.Query(ctx,
		`SELECT slug, install_path FROM apps WHERE enabled = true AND tenant_id = $1`,
		s.tenantID,
	)
	if err != nil {
		return err
	}
	defer rows.Close()

	gx := gronx.New()
	registered := 0

	for rows.Next() {
		var slug, installPath string
		if scanErr := rows.Scan(&slug, &installPath); scanErr != nil {
			slog.Warn("datasource.sync.scan_failed", "err", scanErr)
			continue
		}

		manifest, loadErr := apps.LoadManifest(installPath)
		if loadErr != nil {
			slog.Debug("datasource.sync.no_manifest", "slug", slug, "err", loadErr)
			continue
		}
		cfg := manifest.DataSource
		if cfg == nil || !cfg.Enabled || cfg.Schedule == "" || cfg.Tool == "" {
			continue
		}

		// Validate the cron expression.
		if !gx.IsValid(cfg.Schedule) {
			slog.Warn("datasource.sync.invalid_schedule", "slug", slug, "schedule", cfg.Schedule)
			continue
		}

		nextRun, err := gronx.NextTickAfter(cfg.Schedule, time.Now(), false)
		if err != nil {
			slog.Warn("datasource.sync.next_tick_failed", "slug", slug, "err", err)
			continue
		}

		s.mu.Lock()
		s.jobs[slug] = &dsJob{slug: slug, cfg: cfg, nextRun: nextRun}
		s.mu.Unlock()

		registered++
		slog.Info("datasource.registered", "slug", slug, "schedule", cfg.Schedule, "next", nextRun)
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return rowsErr
	}

	slog.Info("datasource.sync_all_done", "registered", registered)

	// Start background tick loop if we have at least one job and not already running.
	if registered > 0 && s.running.CompareAndSwap(false, true) {
		go s.tickLoop()
	}
	return nil
}

// tickLoop polls every 30 seconds and fires any due data source jobs.
func (s *Scheduler) tickLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-s.stop:
			return
		case now := <-ticker.C:
			s.mu.Lock()
			var due []*dsJob
			for _, job := range s.jobs {
				if now.After(job.nextRun) {
					due = append(due, job)
				}
			}
			s.mu.Unlock()

			for _, job := range due {
				// Skip if a previous run is still in flight.
				s.mu.Lock()
				if job.inProgress {
					s.mu.Unlock()
					continue
				}
				job.inProgress = true
				// Advance nextRun while we hold the lock.
				next, err := gronx.NextTickAfter(job.cfg.Schedule, now, false)
				if err == nil {
					job.nextRun = next
				}
				s.mu.Unlock()

				go func(j *dsJob) {
					defer func() {
						s.mu.Lock()
						j.inProgress = false
						s.mu.Unlock()
					}()
					s.run(j.slug, j.cfg)
				}(job)
			}
		}
	}
}

// Stop halts the background tick loop.
func (s *Scheduler) Stop() {
	select {
	case <-s.stop: // already closed
	default:
		close(s.stop)
	}
}

// run executes a single data source tick: calls the connector tool and snapshots the result.
func (s *Scheduler) run(slug string, cfg *apps.DataSourceConfig) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	result, err := s.runTool(ctx, slug, cfg.Tool, cfg.Args)
	if err != nil {
		slog.Warn("datasource.run.tool_error", "slug", slug, "tool", cfg.Tool, "err", err)
		return
	}
	if result == nil || result.IsError {
		slog.Warn("datasource.run.tool_is_error", "slug", slug, "tool", cfg.Tool)
		return
	}

	// Wrap the tool's text output in a JSON object keyed by resultKey.
	resultKey := cfg.ResultKey
	if resultKey == "" {
		resultKey = "data"
	}

	// Build a JSON object: {"<resultKey>": "<result.ForLLM>"}
	payload := map[string]any{resultKey: result.ForLLM}
	jsonBytes, marshalErr := json.Marshal(payload)
	if marshalErr != nil {
		slog.Warn("datasource.run.marshal_failed", "slug", slug, "err", marshalErr)
		return
	}

	if insertErr := s.snapshots.Insert(ctx, s.tenantID, slug, resultKey, string(jsonBytes)); insertErr != nil {
		slog.Warn("datasource.run.insert_failed", "slug", slug, "err", insertErr)
		return
	}
	slog.Info("datasource.run.snapshot_saved", "slug", slug, "result_key", resultKey)
}
