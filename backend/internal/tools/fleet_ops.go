// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// OnFleetStatus is wired in gateway_tools.go to read live fleet data from DB.
var OnFleetStatus func(ctx context.Context) (FleetStatusData, error)

// OnOrgFinance is wired in gateway_tools.go to read org finance data from DB.
var OnOrgFinance func(ctx context.Context, days int) (OrgFinanceData, error)

type FleetStatusData struct {
	TotalAgents   int             `json:"total_agents"`
	ActiveAgents  int             `json:"active_agents"`
	IdleAgents    int             `json:"idle_agents"`
	ErrorAgents   int             `json:"error_agents"`
	TierBreakdown map[string]int  `json:"tier_breakdown"`
	RecentErrors  []AgentError    `json:"recent_errors,omitempty"`
	SessionsToday int             `json:"sessions_today"`
	Agents        []AgentSummary  `json:"agents"`
}

type AgentSummary struct {
	ID          string  `json:"id"`
	DisplayName string  `json:"display_name"`
	OrgRole     string  `json:"org_role,omitempty"`
	OrgLevel    string  `json:"org_level"`
	Status      string  `json:"status"`
	LastActive  string  `json:"last_active,omitempty"`
	SpendToday  float64 `json:"spend_today_usd,omitempty"`
}

type AgentError struct {
	AgentName string `json:"agent"`
	Error     string `json:"error"`
	At        string `json:"at"`
}

type OrgFinanceData struct {
	TotalMonthUSD   float64         `json:"total_month_usd"`
	TotalTodayUSD   float64         `json:"total_today_usd"`
	DayOverDay      string          `json:"day_over_day"`
	BudgetUsage     []BudgetAgent   `json:"budget_usage"`
	TopSpenders     []SpenderAgent  `json:"top_spenders"`
	DailyTrend      []DailySpend    `json:"daily_trend"`
}

type BudgetAgent struct {
	DisplayName   string  `json:"display_name"`
	OrgRole       string  `json:"org_role"`
	MonthlyBudget float64 `json:"monthly_budget_usd"`
	SpentUSD      float64 `json:"spent_usd"`
	PercentUsed   float64 `json:"percent_used"`
}

type SpenderAgent struct {
	DisplayName  string `json:"display_name"`
	OrgRole      string `json:"org_role"`
	MonthCostUSD float64 `json:"month_cost_usd"`
	TokensIn     int64  `json:"tokens_in"`
	TokensOut    int64  `json:"tokens_out"`
}

type DailySpend struct {
	Date    string  `json:"date"`
	CostUSD float64 `json:"cost_usd"`
}

// FleetStatusTool provides live fleet health data to executive agents.
type FleetStatusTool struct{}

func NewFleetStatusTool() *FleetStatusTool { return &FleetStatusTool{} }
func (t *FleetStatusTool) Name() string    { return "fleet_status" }
func (t *FleetStatusTool) Description() string {
	return "Get current fleet health: active/idle/error agents by tier, recent errors, session counts. Use this to monitor the org's operational state."
}
func (t *FleetStatusTool) Parameters() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
}
func (t *FleetStatusTool) Execute(ctx context.Context, args map[string]any) *Result {
	if OnFleetStatus == nil {
		return ErrorResult("fleet_status not available (no DB connection)")
	}
	data, err := OnFleetStatus(ctx)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to get fleet status: %v", err))
	}
	out, _ := json.MarshalIndent(data, "", "  ")
	return TextResult(string(out))
}

// OrgFinanceTool provides org-level finance data to CFO/CAIO agents.
type OrgFinanceTool struct{}

func NewOrgFinanceTool() *OrgFinanceTool { return &OrgFinanceTool{} }
func (t *OrgFinanceTool) Name() string   { return "org_finance" }
func (t *OrgFinanceTool) Description() string {
	return "Get org finance data: month-to-date spend, per-agent cost breakdown, budget utilization, daily spend trend. Use this for finance reports and budget alerts."
}
func (t *OrgFinanceTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"days": map[string]any{
				"type":        "integer",
				"description": "Number of days of trend data to include (default 7, max 90)",
			},
		},
	}
}
func (t *OrgFinanceTool) Execute(ctx context.Context, args map[string]any) *Result {
	if OnOrgFinance == nil {
		return ErrorResult("org_finance not available (no DB connection)")
	}
	days := 7
	if d, ok := args["days"].(float64); ok && d > 0 && d <= 90 {
		days = int(d)
	}
	data, err := OnOrgFinance(ctx, days)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to get org finance: %v", err))
	}

	var sb strings.Builder
	sb.WriteString("## Org Finance Summary\n\n")
	sb.WriteString(fmt.Sprintf("- Month-to-date spend: $%.4f\n", data.TotalMonthUSD))
	sb.WriteString(fmt.Sprintf("- Today's spend: $%.4f\n", data.TotalTodayUSD))
	sb.WriteString(fmt.Sprintf("- Day-over-day: %s\n\n", data.DayOverDay))

	if len(data.TopSpenders) > 0 {
		sb.WriteString("### Top Spenders\n")
		for _, s := range data.TopSpenders {
			role := s.OrgRole
			if role == "" {
				role = "specialist"
			}
			sb.WriteString(fmt.Sprintf("- %s (%s): $%.4f (%dK tokens in, %dK out)\n",
				s.DisplayName, role, s.MonthCostUSD, s.TokensIn/1000, s.TokensOut/1000))
		}
		sb.WriteString("\n")
	}

	if len(data.BudgetUsage) > 0 {
		sb.WriteString("### Budget Utilization\n")
		for _, b := range data.BudgetUsage {
			flag := ""
			if b.PercentUsed > 80 {
				flag = " ⚠️ OVER 80%"
			}
			sb.WriteString(fmt.Sprintf("- %s: $%.2f / $%.2f (%.0f%%)%s\n",
				b.DisplayName, b.SpentUSD, b.MonthlyBudget, b.PercentUsed, flag))
		}
		sb.WriteString("\n")
	}

	if len(data.DailyTrend) > 0 {
		sb.WriteString("### Daily Trend\n")
		for _, d := range data.DailyTrend {
			sb.WriteString(fmt.Sprintf("- %s: $%.4f\n", d.Date, d.CostUSD))
		}
	}

	return TextResult(sb.String())
}
