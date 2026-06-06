// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).
package gateway

import (
	"context"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

// cpuStat reads a single /proc/stat sample.
func cpuStat() (idle, total uint64) {
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(line, "cpu ") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 5 {
			break
		}
		var vals [8]uint64
		for i := 1; i < len(fields) && i <= 8; i++ {
			vals[i-1], _ = strconv.ParseUint(fields[i], 10, 64)
		}
		idle = vals[3] + vals[4]
		total = vals[0] + vals[1] + vals[2] + vals[3] + vals[4] + vals[5] + vals[6] + vals[7]
		return
	}
	return
}

// startCPUSampler runs a background goroutine that samples CPU every 10s and
// stores the result in cpuPct (atomic uint32, scaled ×100 to avoid floats).
// Returns the pointer so handleStatsBar can read it without blocking.
func startCPUSampler(ctx context.Context) *atomic.Uint32 {
	v := &atomic.Uint32{}
	go func() {
		idle1, total1 := cpuStat()
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(10 * time.Second):
			}
			idle2, total2 := cpuStat()
			totalDiff := total2 - total1
			idleDiff := idle2 - idle1
			if totalDiff > 0 {
				pct := (1.0 - float64(idleDiff)/float64(totalDiff)) * 100.0
				v.Store(uint32(pct * 100)) // store as integer ×100
			}
			idle1, total1 = idle2, total2
		}
	}()
	return v
}

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

	// --- disk (platform-specific helper) ---
	diskUsedGB, diskTotalGB := readDiskGB()

	// --- uptime ---
	uptimeSec := int64(time.Since(gw.startTime).Seconds())

	// --- db health ---
	dbOK := false
	if gw.db != nil {
		dbOK = gw.db.Pool.Ping(r.Context()) == nil
	}

	// --- cost + tokens ---
	// Source: gateway_spend (modern, per-call precision) with fallback to
	// agents.credit_used_cents (legacy) if gateway_spend is empty.
	var costMonthUSD float64
	var tokensInToday, tokensOutToday int64
	type agentSpend struct {
		ID       string  `json:"id"`
		Name     string  `json:"name"`
		CostUSD  float64 `json:"cost_usd"`
		TokensIn int64   `json:"tokens_in"`
		TokensOut int64  `json:"tokens_out"`
	}
	var topAgents []agentSpend
	if gw.db != nil {
		// Total cost this calendar month from gateway_spend (accurate µUSD precision)
		gw.db.Pool.QueryRow(r.Context(),
			`SELECT COALESCE(SUM(cost_usd), 0)
			 FROM gateway_spend
			 WHERE tenant_id = $1
			   AND period >= date_trunc('month', CURRENT_DATE)`,
			defaultTenant,
		).Scan(&costMonthUSD)

		// Fall back to legacy agents.credit_used_cents if gateway_spend has no data yet
		if costMonthUSD == 0 {
			var legacyCents int64
			gw.db.Pool.QueryRow(r.Context(),
				`SELECT COALESCE(SUM(credit_used_cents), 0) FROM agents
				 WHERE tenant_id = $1 AND deleted_at IS NULL`,
				defaultTenant,
			).Scan(&legacyCents)
			costMonthUSD = float64(legacyCents) / 100.0
		}

		// Tokens from gateway_spend_raw today (exact per-call counts)
		gw.db.Pool.QueryRow(r.Context(),
			`SELECT COALESCE(SUM(tokens_in), 0), COALESCE(SUM(tokens_out), 0)
			 FROM gateway_spend_raw
			 WHERE tenant_id = $1
			   AND created_at >= date_trunc('day', now())`,
			defaultTenant,
		).Scan(&tokensInToday, &tokensOutToday)

		// Fallback: tokens from sessions if gateway_spend_raw empty
		if tokensInToday == 0 && tokensOutToday == 0 {
			gw.db.Pool.QueryRow(r.Context(),
				`SELECT COALESCE(SUM(input_tokens),0), COALESCE(SUM(output_tokens),0)
				 FROM sessions
				 WHERE tenant_id = $1 AND updated_at >= date_trunc('day', now())`,
				defaultTenant,
			).Scan(&tokensInToday, &tokensOutToday)
		}

		// Top 5 agents by cost this month from gateway_spend
		rows, err := gw.db.Pool.Query(r.Context(),
			`SELECT gs.agent_id,
			        COALESCE(a.display_name, a.agent_key, 'Unknown'),
			        SUM(gs.cost_usd),
			        SUM(gs.tokens_in),
			        SUM(gs.tokens_out)
			 FROM gateway_spend gs
			 JOIN agents a ON a.id = gs.agent_id::uuid
			 WHERE gs.tenant_id = $1
			   AND gs.period >= date_trunc('month', CURRENT_DATE)
			 GROUP BY gs.agent_id, a.display_name, a.agent_key
			 ORDER BY SUM(gs.cost_usd) DESC
			 LIMIT 5`,
			defaultTenant,
		)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var a agentSpend
				rows.Scan(&a.ID, &a.Name, &a.CostUSD, &a.TokensIn, &a.TokensOut)
				topAgents = append(topAgents, a)
			}
		}
		if topAgents == nil {
			topAgents = []agentSpend{}
		}
	}

	// --- active Qors (agents with status='active') ---
	var activeQors int
	if gw.db != nil {
		gw.db.Pool.QueryRow(r.Context(),
			`SELECT COUNT(*) FROM agents WHERE tenant_id = $1 AND status = 'active'`,
			defaultTenant,
		).Scan(&activeQors)
	}

	// --- active tasks (in_progress) ---
	var activeTasks int
	if gw.db != nil {
		gw.db.Pool.QueryRow(r.Context(),
			`SELECT COUNT(*) FROM tasks WHERE tenant_id = $1 AND status = 'in_progress'`,
			defaultTenant,
		).Scan(&activeTasks)
	}

	// --- active sessions (updated in last 30 minutes) ---
	var activeSessions int
	if gw.db != nil {
		gw.db.Pool.QueryRow(r.Context(),
			`SELECT COUNT(*) FROM sessions WHERE tenant_id = $1 AND updated_at >= now() - interval '30 minutes'`,
			defaultTenant,
		).Scan(&activeSessions)
	}

	// --- pending approvals ---
	var pendingApprovals int
	if gw.db != nil {
		gw.db.Pool.QueryRow(r.Context(),
			`SELECT COUNT(*) FROM outbound_queue WHERE tenant_id = $1 AND status = 'pending'`,
			defaultTenant,
		).Scan(&pendingApprovals)
	}

	// --- CPU percent (read cached value from background sampler) ---
	var cpuPct float64
	if gw.cpuSampler != nil {
		cpuPct = float64(gw.cpuSampler.Load()) / 100.0
	}

	writeJSON(w, 200, map[string]any{
		"mem_used_gb":       memUsedGB,
		"mem_total_gb":      memTotalGB,
		"disk_used_gb":      diskUsedGB,
		"disk_total_gb":     diskTotalGB,
		"cpu_percent":       cpuPct,
		"uptime_sec":        uptimeSec,
		"db_ok":             dbOK,
		"cost_month_usd":    costMonthUSD,
		"tokens_in_today":   tokensInToday,
		"tokens_out_today":  tokensOutToday,
		"active_qors":       activeQors,
		"active_tasks":      activeTasks,
		"active_sessions":   activeSessions,
		"pending_approvals": pendingApprovals,
		"goroutines":        runtime.NumGoroutine(),
		"top_agents":        topAgents,
	})
}
