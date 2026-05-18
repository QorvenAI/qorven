// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package store

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/adhocore/gronx"
	"github.com/google/uuid"
)

var (
	ErrCronJobNotFound    = errors.New("cron job not found")
	ErrCronJobNoFutureRun = errors.New("cron job has no future run")
)

// CronJob represents a scheduled job.
type CronJob struct {
	ID             string       `json:"id"`
	TenantID       uuid.UUID    `json:"tenantId,omitempty"`
	Name           string       `json:"name"`
	AgentID        string       `json:"agentId,omitempty"`
	UserID         string       `json:"userId,omitempty"`
	Enabled        bool         `json:"enabled"`
	Schedule       CronSchedule `json:"schedule"`
	Payload        CronPayload  `json:"payload"`
	State          CronJobState `json:"state"`
	CreatedAtMS    int64        `json:"createdAtMs"`
	UpdatedAtMS    int64        `json:"updatedAtMs"`
	DeleteAfterRun bool         `json:"deleteAfterRun,omitempty"`
	Stateless      bool         `json:"stateless"`
	Deliver        bool         `json:"deliver"`
	DeliverChannel string       `json:"deliverChannel"`
	DeliverTo      string       `json:"deliverTo"`
	WakeHeartbeat  bool         `json:"wakeHeartbeat"`
}

type CronSchedule struct {
	Kind    string `json:"kind"`
	AtMS    *int64 `json:"atMs,omitempty"`
	EveryMS *int64 `json:"everyMs,omitempty"`
	Expr    string `json:"expr,omitempty"`
	TZ      string `json:"tz,omitempty"`
}

type CronPayload struct {
	Kind    string `json:"kind"`
	Message string `json:"message"`
	Command string `json:"command,omitempty"`
}

type CronJobState struct {
	NextRunAtMS *int64 `json:"nextRunAtMs,omitempty"`
	LastRunAtMS *int64 `json:"lastRunAtMs,omitempty"`
	LastStatus  string `json:"lastStatus,omitempty"`
	LastError   string `json:"lastError,omitempty"`
}

type CronRunLogEntry struct {
	Ts           int64  `json:"ts"`
	JobID        string `json:"jobId"`
	Status       string `json:"status,omitempty"`
	Error        string `json:"error,omitempty"`
	Summary      string `json:"summary,omitempty"`
	DurationMS   int64  `json:"durationMs,omitempty"`
	InputTokens  int    `json:"inputTokens,omitempty"`
	OutputTokens int    `json:"outputTokens,omitempty"`
}

type CronJobResult struct {
	Content      string `json:"content"`
	InputTokens  int    `json:"inputTokens,omitempty"`
	OutputTokens int    `json:"outputTokens,omitempty"`
	DurationMS   int64  `json:"durationMs,omitempty"`
}

type CronJobPatch struct {
	Name           string        `json:"name,omitempty"`
	AgentID        *string       `json:"agentId,omitempty"`
	Enabled        *bool         `json:"enabled,omitempty"`
	Schedule       *CronSchedule `json:"schedule,omitempty"`
	Message        string        `json:"message,omitempty"`
	DeleteAfterRun *bool         `json:"deleteAfterRun,omitempty"`
	Stateless      *bool         `json:"stateless,omitempty"`
	Deliver        *bool         `json:"deliver,omitempty"`
	DeliverChannel *string       `json:"deliverChannel,omitempty"`
	DeliverTo      *string       `json:"deliverTo,omitempty"`
	WakeHeartbeat  *bool         `json:"wakeHeartbeat,omitempty"`
}

type CronEvent struct {
	Action  string `json:"action"`
	JobID   string `json:"jobId"`
	JobName string `json:"jobName,omitempty"`
	UserID  string `json:"userId,omitempty"`
	Status  string `json:"status,omitempty"`
	Error   string `json:"error,omitempty"`
}

type CronJobMutableState struct {
	Enabled   bool
	Schedule  CronSchedule
	NextRunAt *time.Time
	Payload   CronPayload
}

// CronStore manages scheduled jobs.
type CronStore interface {
	AddJob(ctx context.Context, name string, schedule CronSchedule, message string, deliver bool, channel, to, agentID, userID string) (*CronJob, error)
	GetJob(ctx context.Context, jobID string) (*CronJob, bool)
	ListJobs(ctx context.Context, includeDisabled bool, agentID, userID string) []CronJob
	RemoveJob(ctx context.Context, jobID string) error
	UpdateJob(ctx context.Context, jobID string, patch CronJobPatch) (*CronJob, error)
	EnableJob(ctx context.Context, jobID string, enabled bool) error
	GetRunLog(ctx context.Context, jobID string, limit, offset int) ([]CronRunLogEntry, int)
	Status() map[string]any
	Start() error
	Stop()
	SetOnJob(handler func(job *CronJob) (*CronJobResult, error))
	SetOnEvent(handler func(event CronEvent))
	RunJob(ctx context.Context, jobID string, force bool) (ran bool, reason string, err error)
	SetDefaultTimezone(tz string)
	GetDueJobs(now time.Time) []CronJob
}

// CacheInvalidatable is an optional interface for stores that support cache invalidation.
type CacheInvalidatable interface {
	InvalidateCache()
}

// ComputeNextRun calculates the next run time for a cron schedule.
func ComputeNextRun(schedule *CronSchedule, now time.Time, defaultTZ string) *time.Time {
	switch schedule.Kind {
	case "at":
		if schedule.AtMS != nil {
			t := time.UnixMilli(*schedule.AtMS)
			if t.After(now) {
				return &t
			}
		}
		return nil
	case "every":
		if schedule.EveryMS != nil && *schedule.EveryMS > 0 {
			t := now.Add(time.Duration(*schedule.EveryMS) * time.Millisecond)
			return &t
		}
		return nil
	case "cron":
		if schedule.Expr == "" {
			return nil
		}
		tz := schedule.TZ
		if tz == "" {
			tz = defaultTZ
		}
		evalTime := now
		if tz != "" {
			if loc, err := time.LoadLocation(tz); err == nil {
				evalTime = now.In(loc)
			}
		}
		nextTime, err := gronx.NextTickAfter(schedule.Expr, evalTime, false)
		if err != nil {
			return nil
		}
		utcNext := nextTime.UTC()
		return &utcNext
	default:
		return nil
	}
}

func NextRunForSchedule(schedule *CronSchedule, enabled bool, now time.Time, defaultTZ string) (*time.Time, error) {
	if !enabled {
		return nil, nil
	}
	next := ComputeNextRun(schedule, now, defaultTZ)
	if next != nil {
		return next, nil
	}
	switch schedule.Kind {
	case "at":
		return nil, fmt.Errorf("%w: at schedule is already in the past", ErrCronJobNoFutureRun)
	case "cron":
		return nil, fmt.Errorf("%w: cron schedule has no valid next execution", ErrCronJobNoFutureRun)
	case "every":
		return nil, fmt.Errorf("%w: every schedule has no valid interval", ErrCronJobNoFutureRun)
	default:
		return nil, fmt.Errorf("%w: unsupported schedule kind %q", ErrCronJobNoFutureRun, schedule.Kind)
	}
}

func NextRunForToggle(schedule *CronSchedule, enabled, currentlyEnabled bool, currentNextRunAt *time.Time, now time.Time, defaultTZ string) (*time.Time, error) {
	if !enabled {
		return nil, nil
	}
	if currentlyEnabled && currentNextRunAt != nil {
		next := *currentNextRunAt
		return &next, nil
	}
	return NextRunForSchedule(schedule, true, now, defaultTZ)
}

func MergeCronSchedule(current CronSchedule, patch *CronSchedule) CronSchedule {
	if patch == nil {
		return current
	}
	newKind := patch.Kind
	if newKind == "" {
		newKind = current.Kind
	}
	merged := CronSchedule{Kind: newKind, TZ: patch.TZ}
	switch newKind {
	case "cron":
		if patch.Expr != "" {
			merged.Expr = patch.Expr
		} else if current.Kind == newKind {
			merged.Expr = current.Expr
		}
	case "every":
		if patch.EveryMS != nil {
			merged.EveryMS = patch.EveryMS
		} else if current.Kind == newKind {
			merged.EveryMS = current.EveryMS
		}
	case "at":
		if patch.AtMS != nil {
			merged.AtMS = patch.AtMS
		} else if current.Kind == newKind {
			merged.AtMS = current.AtMS
		}
	}
	return merged
}

func ValidateCronSchedule(schedule *CronSchedule) error {
	switch schedule.Kind {
	case "cron":
		if schedule.Expr == "" {
			return fmt.Errorf("cron schedule requires expr")
		}
		if !gronx.New().IsValid(schedule.Expr) {
			return fmt.Errorf("invalid cron expression: %s", schedule.Expr)
		}
		if schedule.TZ != "" {
			if _, err := time.LoadLocation(schedule.TZ); err != nil {
				return fmt.Errorf("invalid timezone: %s", schedule.TZ)
			}
		}
	case "every":
		if schedule.EveryMS == nil || *schedule.EveryMS <= 0 {
			return fmt.Errorf("every schedule requires positive everyMs")
		}
	case "at":
		if schedule.AtMS == nil {
			return fmt.Errorf("at schedule requires atMs")
		}
	default:
		return fmt.Errorf("invalid schedule kind: %s", schedule.Kind)
	}
	return nil
}

func ApplyCronScheduleUpdates(updates map[string]any, schedule CronSchedule) {
	updates["schedule_kind"] = schedule.Kind
	if schedule.TZ != "" {
		updates["timezone"] = schedule.TZ
	} else {
		updates["timezone"] = nil
	}
	switch schedule.Kind {
	case "cron":
		updates["cron_expression"] = schedule.Expr
		updates["interval_ms"] = nil
		updates["run_at"] = nil
	case "every":
		updates["cron_expression"] = nil
		updates["interval_ms"] = *schedule.EveryMS
		updates["run_at"] = nil
	case "at":
		runAt := time.UnixMilli(*schedule.AtMS)
		updates["cron_expression"] = nil
		updates["interval_ms"] = nil
		updates["run_at"] = runAt
	}
}

func SortedUpdateColumns(updates map[string]any) []string {
	cols := make([]string, 0, len(updates))
	for col := range updates {
		cols = append(cols, col)
	}
	sort.Strings(cols)
	return cols
}
