// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).
package gateway

import (
	"net/http"
	"os"
	"runtime"
	"syscall"
	"time"

	"github.com/qorvenai/qorven/internal/providers"
)

func (gw *Gateway) handleStatsBar(w http.ResponseWriter, r *http.Request) {
	// --- system: Go runtime memory ---
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	memUsedMB := ms.Sys / 1024 / 1024
	memHeapMB := ms.HeapInuse / 1024 / 1024

	// --- system: disk (statvfs on the binary's directory) ---
	var diskUsedGB, diskTotalGB float64
	var stat syscall.Statfs_t
	dir := "/"
	if exe, err := os.Executable(); err == nil {
		dir = exe
	}
	if err := syscall.Statfs(dir, &stat); err == nil {
		total := stat.Blocks * uint64(stat.Bsize)
		free := stat.Bavail * uint64(stat.Bsize)
		diskTotalGB = float64(total) / 1e9
		diskUsedGB = float64(total-free) / 1e9
	}

	// --- uptime ---
	uptime := time.Since(gw.startTime).Round(time.Second).String()

	// --- db health ---
	dbOK := false
	if gw.db != nil {
		dbOK = gw.db.Pool.Ping(r.Context()) == nil
	}

	// --- cost + tokens today ---
	var costMonthUSD float64
	var tokensInToday, tokensOutToday int64
	if gw.db != nil {
		ps := providers.NewPricingStore(gw.db.Pool)
		costMonthUSD = ps.GetAccountSpend(r.Context(), defaultTenant)

		// tokens today from soul_usage
		gw.db.Pool.QueryRow(r.Context(),
			`SELECT COALESCE(SUM(input_tokens),0), COALESCE(SUM(output_tokens),0)
             FROM soul_usage
             WHERE tenant_id = $1 AND called_at >= date_trunc('day', now())`,
			defaultTenant,
		).Scan(&tokensInToday, &tokensOutToday)
	}

	// --- active sessions (last 5 min) ---
	var activeSessions int
	if gw.db != nil {
		gw.db.Pool.QueryRow(r.Context(),
			`SELECT COUNT(*) FROM sessions WHERE tenant_id = $1 AND updated_at >= now() - interval '5 minutes'`,
			defaultTenant,
		).Scan(&activeSessions)
	}

	writeJSON(w, 200, map[string]any{
		"mem_sys_mb":       memUsedMB,
		"mem_heap_mb":      memHeapMB,
		"disk_used_gb":     diskUsedGB,
		"disk_total_gb":    diskTotalGB,
		"uptime":           uptime,
		"db_ok":            dbOK,
		"cost_month_usd":   costMonthUSD,
		"tokens_in_today":  tokensInToday,
		"tokens_out_today": tokensOutToday,
		"active_sessions":  activeSessions,
		"goroutines":       runtime.NumGoroutine(),
	})
}
