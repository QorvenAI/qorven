// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).
package gateway

import (
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// readCPUPercent reads a 100ms CPU sample from /proc/stat (Linux).
func readCPUPercent() float64 {
	sample := func() (idle, total uint64) {
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
			// user, nice, system, idle, iowait, irq, softirq, steal
			idle = vals[3] + vals[4]
			total = vals[0] + vals[1] + vals[2] + vals[3] + vals[4] + vals[5] + vals[6] + vals[7]
			return
		}
		return
	}
	idle1, total1 := sample()
	time.Sleep(200 * time.Millisecond)
	idle2, total2 := sample()
	totalDiff := total2 - total1
	idleDiff := idle2 - idle1
	if totalDiff == 0 {
		return 0
	}
	return (1.0 - float64(idleDiff)/float64(totalDiff)) * 100.0
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

	// --- cost + tokens today ---
	// Cost: sum credit_used_cents from agents (where actual spend is tracked).
	// Tokens: sum from sessions updated today (where token counts are stored).
	var costMonthUSD float64
	var tokensInToday, tokensOutToday int64
	type agentSpend struct {
		ID          string  `json:"id"`
		Name        string  `json:"name"`
		CostUSD     float64 `json:"cost_usd"`
		TokensIn    int64   `json:"tokens_in"`
		TokensOut   int64   `json:"tokens_out"`
	}
	var topAgents []agentSpend
	if gw.db != nil {
		// Total monthly cost from agents.credit_used_cents (100 cents = $1)
		gw.db.Pool.QueryRow(r.Context(),
			`SELECT COALESCE(SUM(credit_used_cents), 0) FROM agents
			 WHERE tenant_id = $1 AND deleted_at IS NULL`,
			defaultTenant,
		).Scan(&costMonthUSD)
		costMonthUSD /= 100.0

		// Tokens from sessions updated today
		gw.db.Pool.QueryRow(r.Context(),
			`SELECT COALESCE(SUM(input_tokens),0), COALESCE(SUM(output_tokens),0)
			 FROM sessions
			 WHERE tenant_id = $1 AND updated_at >= date_trunc('day', now())`,
			defaultTenant,
		).Scan(&tokensInToday, &tokensOutToday)

		// Per-agent spend for hover breakdown (top 5 by spend)
		rows, err := gw.db.Pool.Query(r.Context(),
			`SELECT id, COALESCE(display_name, agent_key, 'Unknown'),
			        credit_used_cents,
			        COALESCE((SELECT SUM(input_tokens) FROM sessions WHERE agent_id = agents.id), 0),
			        COALESCE((SELECT SUM(output_tokens) FROM sessions WHERE agent_id = agents.id), 0)
			 FROM agents
			 WHERE tenant_id = $1 AND deleted_at IS NULL AND credit_used_cents > 0
			 ORDER BY credit_used_cents DESC LIMIT 5`,
			defaultTenant,
		)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var a agentSpend
				var cents int64
				rows.Scan(&a.ID, &a.Name, &cents, &a.TokensIn, &a.TokensOut)
				a.CostUSD = float64(cents) / 100.0
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

	// --- CPU percent (non-blocking: sample already taken above) ---
	cpuPct := readCPUPercent()

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
