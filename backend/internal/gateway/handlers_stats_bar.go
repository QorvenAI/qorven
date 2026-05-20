// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).
package gateway

import (
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/qorvenai/qorven/internal/providers"
)

// readMemInfoGB reads /proc/meminfo for MemTotal and MemAvailable (Linux).
func readMemInfoGB() (usedGB, totalGB float64) {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return
	}
	var total, available uint64
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		val, _ := strconv.ParseUint(fields[1], 10, 64)
		switch fields[0] {
		case "MemTotal:":
			total = val
		case "MemAvailable:":
			available = val
		}
	}
	totalGB = float64(total) / 1e6
	usedGB = float64(total-available) / 1e6
	return
}

func (gw *Gateway) handleStatsBar(w http.ResponseWriter, r *http.Request) {
	// --- system RAM (from /proc/meminfo) ---
	memUsedGB, memTotalGB := readMemInfoGB()

	// --- disk (statvfs /) ---
	var diskUsedGB, diskTotalGB float64
	var stat syscall.Statfs_t
	if err := syscall.Statfs("/", &stat); err == nil {
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

	// --- active Qors (agents with status='active') ---
	var activeQors int
	if gw.db != nil {
		gw.db.Pool.QueryRow(r.Context(),
			`SELECT COUNT(*) FROM agents WHERE tenant_id = $1 AND status = 'active'`,
			defaultTenant,
		).Scan(&activeQors)
	}

	writeJSON(w, 200, map[string]any{
		"mem_used_gb":      memUsedGB,
		"mem_total_gb":     memTotalGB,
		"disk_used_gb":     diskUsedGB,
		"disk_total_gb":    diskTotalGB,
		"uptime":           uptime,
		"db_ok":            dbOK,
		"cost_month_usd":   costMonthUSD,
		"tokens_in_today":  tokensInToday,
		"tokens_out_today": tokensOutToday,
		"active_qors":      activeQors,
		"goroutines":       runtime.NumGoroutine(),
	})
}
