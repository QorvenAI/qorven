// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"
)

// ─── Callbacks (wired in gateway_tools.go) ───────────────────────────────────

var OnReconcile func(ctx context.Context) (ReconciliationReport, error)
var OnForecastSpend func(ctx context.Context, days int) (SpendForecast, error)
var OnSetBudget func(ctx context.Context, agentID string, monthlyUSD, dailyUSD float64) error
var OnRequestBudgetRaise func(ctx context.Context, agentID, reason string, requestedUSD float64) (string, error)
var OnListBudgetRequests func(ctx context.Context) ([]BudgetRequest, error)
var OnDecideBudgetRequest func(ctx context.Context, requestID, decision, note string) error
var OnCFOReport func(ctx context.Context, days int) (CFOReportData, error)

// OnEffectiveBudget reconciles the declared overall budget against provider-key
// allowances. Read-only — allocation tools are a later subsystem.
var OnEffectiveBudget func(ctx context.Context) (declaredRemainingUUSD, providerRemainingUUSD, effectiveUUSD int64, binding string, warnings []string, err error)

// ─── Data types ──────────────────────────────────────────────────────────────

type ReconciliationReport struct {
	RunAt             string                `json:"run_at"`
	Status            string                `json:"status"` // "balanced", "drift_detected"
	TotalRawUUSD      int64                 `json:"total_raw_uusd"`
	TotalAggregateUSD float64               `json:"total_aggregate_usd"`
	DriftUUSD         int64                 `json:"drift_uusd"`
	DriftPercent      float64               `json:"drift_percent"`
	PricingCoverage   float64               `json:"pricing_coverage_percent"`
	MissingModels     []string              `json:"missing_models,omitempty"`
	PerModelCheck     []ModelReconciliation  `json:"per_model_check,omitempty"`
	Explanation       string                `json:"explanation"`
}

type ModelReconciliation struct {
	Model            string  `json:"model"`
	TotalCalls       int     `json:"total_calls"`
	TokensIn         int64   `json:"tokens_in"`
	TokensOut        int64   `json:"tokens_out"`
	OurCostUUSD      int64   `json:"our_cost_uusd"`
	ExpectedCostUUSD int64   `json:"expected_cost_uusd"`
	DriftUUSD        int64   `json:"drift_uusd"`
	InputRate        float64 `json:"input_rate_per_1m"`
	OutputRate       float64 `json:"output_rate_per_1m"`
	Match            bool    `json:"match"`
}

type SpendForecast struct {
	AsOf                string           `json:"as_of"`
	DaysInMonth         int              `json:"days_in_month"`
	DaysElapsed         int              `json:"days_elapsed"`
	DaysRemaining       int              `json:"days_remaining"`
	SpentSoFarUSD       float64          `json:"spent_so_far_usd"`
	ProjectedMonthUSD   float64          `json:"projected_month_usd"`
	DailyAvgUSD         float64          `json:"daily_avg_usd"`
	TrendDirection      string           `json:"trend"` // "increasing", "decreasing", "stable"
	AgentForecasts      []AgentForecast  `json:"agent_forecasts"`
	Anomalies           []SpendAnomaly   `json:"anomalies,omitempty"`
}

type AgentForecast struct {
	AgentID          string  `json:"agent_id"`
	DisplayName      string  `json:"display_name"`
	OrgRole          string  `json:"org_role"`
	SpentThisMonth   float64 `json:"spent_this_month_usd"`
	ProjectedMonth   float64 `json:"projected_month_usd"`
	MonthlyBudget    float64 `json:"monthly_budget_usd"`
	BudgetUtilPct    float64 `json:"budget_util_percent"`
	WillExceedBudget bool    `json:"will_exceed_budget"`
	ExceedByUSD      float64 `json:"exceed_by_usd,omitempty"`
}

type SpendAnomaly struct {
	AgentName   string  `json:"agent_name"`
	Date        string  `json:"date"`
	SpendUSD    float64 `json:"spend_usd"`
	AvgUSD      float64 `json:"avg_usd"`
	StdDev      float64 `json:"std_dev"`
	Sigma       float64 `json:"sigma"`
	Description string  `json:"description"`
}

type BudgetRequest struct {
	ID           string  `json:"id"`
	AgentID      string  `json:"agent_id"`
	AgentName    string  `json:"agent_name"`
	OrgRole      string  `json:"org_role"`
	CurrentUSD   float64 `json:"current_budget_usd"`
	RequestedUSD float64 `json:"requested_usd"`
	Reason       string  `json:"reason"`
	Status       string  `json:"status"` // pending, approved, denied
	DecidedBy    string  `json:"decided_by,omitempty"`
	DecisionNote string  `json:"decision_note,omitempty"`
	CreatedAt    string  `json:"created_at"`
	DecidedAt    string  `json:"decided_at,omitempty"`
}

type CFOReportData struct {
	GeneratedAt       string                `json:"generated_at"`
	Period            string                `json:"period"`
	Reconciliation    ReconciliationReport  `json:"reconciliation"`
	Forecast          SpendForecast         `json:"forecast"`
	BudgetStatus      []AgentBudgetStatus   `json:"budget_status"`
	PendingRequests   []BudgetRequest       `json:"pending_requests"`
	Recommendations   []string              `json:"recommendations"`
	TotalMonthUSD     float64               `json:"total_month_usd"`
	TotalTodayUSD     float64               `json:"total_today_usd"`
}

type AgentBudgetStatus struct {
	DisplayName   string  `json:"display_name"`
	OrgRole       string  `json:"org_role"`
	MonthlyBudget float64 `json:"monthly_budget_usd"`
	SpentUSD      float64 `json:"spent_usd"`
	PercentUsed   float64 `json:"percent_used"`
	Status        string  `json:"status"` // "healthy", "warning", "critical", "exceeded"
}

// ─── Reconciliation Tool ─────────────────────────────────────────────────────

type ReconcileTool struct{}

func NewReconcileTool() *ReconcileTool { return &ReconcileTool{} }
func (t *ReconcileTool) Name() string  { return "reconcile_costs" }
func (t *ReconcileTool) Description() string {
	return "Run a full cost reconciliation: verify that our internal µUSD calculations match " +
		"expected provider pricing for every model. Returns drift analysis, pricing coverage, " +
		"and per-model verification. Use this to prove our numbers are bank-grade accurate."
}
func (t *ReconcileTool) Parameters() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{}}
}
func (t *ReconcileTool) Execute(ctx context.Context, args map[string]any) *Result {
	if OnReconcile == nil {
		return ErrorResult("reconcile_costs not available (no DB)")
	}
	report, err := OnReconcile(ctx)
	if err != nil {
		return ErrorResult(fmt.Sprintf("reconciliation failed: %v", err))
	}

	var sb strings.Builder
	sb.WriteString("## Cost Reconciliation Report\n\n")
	sb.WriteString(fmt.Sprintf("**Status:** %s\n", strings.ToUpper(report.Status)))
	sb.WriteString(fmt.Sprintf("**Run at:** %s\n\n", report.RunAt))
	sb.WriteString(fmt.Sprintf("- Raw ledger total: %d µUSD ($%.6f)\n", report.TotalRawUUSD, float64(report.TotalRawUUSD)/1_000_000))
	sb.WriteString(fmt.Sprintf("- Aggregate total: $%.6f\n", report.TotalAggregateUSD))
	sb.WriteString(fmt.Sprintf("- Drift: %d µUSD (%.4f%%)\n", report.DriftUUSD, report.DriftPercent))
	sb.WriteString(fmt.Sprintf("- Pricing coverage: %.1f%% of calls have known pricing\n\n", report.PricingCoverage))

	if len(report.MissingModels) > 0 {
		sb.WriteString("### Models Missing Pricing\n")
		for _, m := range report.MissingModels {
			sb.WriteString(fmt.Sprintf("- %s\n", m))
		}
		sb.WriteString("\n")
	}

	if len(report.PerModelCheck) > 0 {
		sb.WriteString("### Per-Model Verification\n")
		for _, m := range report.PerModelCheck {
			status := "✓"
			if !m.Match {
				status = "✗ DRIFT"
			}
			sb.WriteString(fmt.Sprintf("- %s %s: %d calls, in=%d out=%d, our=$%.6f expected=$%.6f\n",
				status, m.Model, m.TotalCalls, m.TokensIn, m.TokensOut,
				float64(m.OurCostUUSD)/1_000_000, float64(m.ExpectedCostUUSD)/1_000_000))
		}
	}

	sb.WriteString(fmt.Sprintf("\n**Explanation:** %s\n", report.Explanation))
	return TextResult(sb.String())
}

// ─── Forecast Tool ───────────────────────────────────────────────────────────

type ForecastSpendTool struct{}

func NewForecastSpendTool() *ForecastSpendTool { return &ForecastSpendTool{} }
func (t *ForecastSpendTool) Name() string      { return "forecast_spend" }
func (t *ForecastSpendTool) Description() string {
	return "Project month-end spend for the org and per-agent. Detects anomalies (3σ spikes), " +
		"flags agents on track to exceed budget, and provides trend analysis. " +
		"Use this for budget planning and proactive alerts."
}
func (t *ForecastSpendTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"lookback_days": map[string]any{
				"type":        "integer",
				"description": "Days of history for trend calculation (default 7, max 30)",
			},
		},
	}
}
func (t *ForecastSpendTool) Execute(ctx context.Context, args map[string]any) *Result {
	if OnForecastSpend == nil {
		return ErrorResult("forecast_spend not available (no DB)")
	}
	days := 7
	if d, ok := args["lookback_days"].(float64); ok && d > 0 && d <= 30 {
		days = int(d)
	}
	forecast, err := OnForecastSpend(ctx, days)
	if err != nil {
		return ErrorResult(fmt.Sprintf("forecast failed: %v", err))
	}

	var sb strings.Builder
	sb.WriteString("## Spend Forecast\n\n")
	sb.WriteString(fmt.Sprintf("**Period:** %s (day %d of %d, %d remaining)\n",
		forecast.AsOf, forecast.DaysElapsed, forecast.DaysInMonth, forecast.DaysRemaining))
	sb.WriteString(fmt.Sprintf("- Spent so far: $%.4f\n", forecast.SpentSoFarUSD))
	sb.WriteString(fmt.Sprintf("- Projected month-end: $%.4f\n", forecast.ProjectedMonthUSD))
	sb.WriteString(fmt.Sprintf("- Daily average: $%.4f\n", forecast.DailyAvgUSD))
	sb.WriteString(fmt.Sprintf("- Trend: %s\n\n", forecast.TrendDirection))

	if len(forecast.AgentForecasts) > 0 {
		sb.WriteString("### Per-Agent Forecast\n")
		for _, af := range forecast.AgentForecasts {
			flag := ""
			if af.WillExceedBudget {
				flag = fmt.Sprintf(" ⚠️ WILL EXCEED by $%.4f", af.ExceedByUSD)
			}
			budget := "unlimited"
			if af.MonthlyBudget > 0 {
				budget = fmt.Sprintf("$%.2f (%.0f%% used)", af.MonthlyBudget, af.BudgetUtilPct)
			}
			sb.WriteString(fmt.Sprintf("- **%s** (%s): spent $%.4f → projected $%.4f | budget: %s%s\n",
				af.DisplayName, af.OrgRole, af.SpentThisMonth, af.ProjectedMonth, budget, flag))
		}
		sb.WriteString("\n")
	}

	if len(forecast.Anomalies) > 0 {
		sb.WriteString("### Anomalies Detected\n")
		for _, a := range forecast.Anomalies {
			sb.WriteString(fmt.Sprintf("- **%s** on %s: $%.4f (%.1fσ above avg $%.4f) — %s\n",
				a.AgentName, a.Date, a.SpendUSD, a.Sigma, a.AvgUSD, a.Description))
		}
	}

	return TextResult(sb.String())
}

// ─── Budget Management Tools ─────────────────────────────────────────────────

type SetBudgetTool struct{}

func NewSetBudgetTool() *SetBudgetTool { return &SetBudgetTool{} }
func (t *SetBudgetTool) Name() string  { return "set_budget" }
func (t *SetBudgetTool) Description() string {
	return "Set or update the monthly/daily spend budget for an agent. Only CFO-level agents should use this. " +
		"Setting to 0 removes the cap (unlimited)."
}
func (t *SetBudgetTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"agent_id":    map[string]any{"type": "string", "description": "Agent UUID to set budget for"},
			"monthly_usd": map[string]any{"type": "number", "description": "Monthly budget in USD (0 = unlimited)"},
			"daily_usd":   map[string]any{"type": "number", "description": "Daily budget in USD (0 = unlimited)"},
		},
		"required": []string{"agent_id", "monthly_usd"},
	}
}
func (t *SetBudgetTool) Execute(ctx context.Context, args map[string]any) *Result {
	if OnSetBudget == nil {
		return ErrorResult("set_budget not available")
	}
	agentID, _ := args["agent_id"].(string)
	monthlyUSD, _ := args["monthly_usd"].(float64)
	dailyUSD, _ := args["daily_usd"].(float64)
	if agentID == "" {
		return ErrorResult("agent_id is required")
	}
	if err := OnSetBudget(ctx, agentID, monthlyUSD, dailyUSD); err != nil {
		return ErrorResult(fmt.Sprintf("set_budget failed: %v", err))
	}
	return SuccessResult(fmt.Sprintf("Budget set: agent=%s monthly=$%.2f daily=$%.4f", agentID, monthlyUSD, dailyUSD))
}

type RequestBudgetRaiseTool struct{}

func NewRequestBudgetRaiseTool() *RequestBudgetRaiseTool { return &RequestBudgetRaiseTool{} }
func (t *RequestBudgetRaiseTool) Name() string           { return "request_budget_raise" }
func (t *RequestBudgetRaiseTool) Description() string {
	return "Request a budget increase from the CFO. Provide justification for why you need more budget. " +
		"The CFO will review and approve or deny the request."
}
func (t *RequestBudgetRaiseTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"requested_monthly_usd": map[string]any{"type": "number", "description": "Desired new monthly budget in USD"},
			"reason":                map[string]any{"type": "string", "description": "Business justification for the increase"},
		},
		"required": []string{"requested_monthly_usd", "reason"},
	}
}
func (t *RequestBudgetRaiseTool) Execute(ctx context.Context, args map[string]any) *Result {
	if OnRequestBudgetRaise == nil {
		return ErrorResult("request_budget_raise not available")
	}
	agentID := AgentIDFromCtx(ctx)
	if agentID == "" {
		return ErrorResult("cannot determine requesting agent")
	}
	requestedUSD, _ := args["requested_monthly_usd"].(float64)
	reason, _ := args["reason"].(string)
	if requestedUSD <= 0 || reason == "" {
		return ErrorResult("requested_monthly_usd and reason are required")
	}
	reqID, err := OnRequestBudgetRaise(ctx, agentID, reason, requestedUSD)
	if err != nil {
		return ErrorResult(fmt.Sprintf("request failed: %v", err))
	}
	return SuccessResult(fmt.Sprintf("Budget raise request submitted (id=%s). The CFO will review it.", reqID))
}

type ListBudgetRequestsTool struct{}

func NewListBudgetRequestsTool() *ListBudgetRequestsTool { return &ListBudgetRequestsTool{} }
func (t *ListBudgetRequestsTool) Name() string           { return "list_budget_requests" }
func (t *ListBudgetRequestsTool) Description() string {
	return "List all pending and recent budget raise requests. CFO uses this to review and decide on requests."
}
func (t *ListBudgetRequestsTool) Parameters() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{}}
}
func (t *ListBudgetRequestsTool) Execute(ctx context.Context, args map[string]any) *Result {
	if OnListBudgetRequests == nil {
		return ErrorResult("list_budget_requests not available")
	}
	requests, err := OnListBudgetRequests(ctx)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed: %v", err))
	}
	if len(requests) == 0 {
		return TextResult("No budget requests found.")
	}
	out, _ := json.MarshalIndent(requests, "", "  ")
	return TextResult(string(out))
}

type DecideBudgetRequestTool struct{}

func NewDecideBudgetRequestTool() *DecideBudgetRequestTool { return &DecideBudgetRequestTool{} }
func (t *DecideBudgetRequestTool) Name() string            { return "decide_budget_request" }
func (t *DecideBudgetRequestTool) Description() string {
	return "Approve or deny a budget raise request. Only CFO-level agents should use this. " +
		"On approval, the agent's budget is automatically updated."
}
func (t *DecideBudgetRequestTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"request_id": map[string]any{"type": "string", "description": "ID of the budget request"},
			"decision":   map[string]any{"type": "string", "enum": []string{"approved", "denied"}, "description": "approve or deny"},
			"note":       map[string]any{"type": "string", "description": "Optional note explaining the decision"},
		},
		"required": []string{"request_id", "decision"},
	}
}
func (t *DecideBudgetRequestTool) Execute(ctx context.Context, args map[string]any) *Result {
	if OnDecideBudgetRequest == nil {
		return ErrorResult("decide_budget_request not available")
	}
	requestID, _ := args["request_id"].(string)
	decision, _ := args["decision"].(string)
	note, _ := args["note"].(string)
	if requestID == "" || (decision != "approved" && decision != "denied") {
		return ErrorResult("request_id and decision (approved/denied) are required")
	}
	if err := OnDecideBudgetRequest(ctx, requestID, decision, note); err != nil {
		return ErrorResult(fmt.Sprintf("decision failed: %v", err))
	}
	return SuccessResult(fmt.Sprintf("Budget request %s: %s", requestID, decision))
}

// ─── CFO Report Tool ─────────────────────────────────────────────────────────

type CFOReportTool struct{}

func NewCFOReportTool() *CFOReportTool { return &CFOReportTool{} }
func (t *CFOReportTool) Name() string  { return "cfo_report" }
func (t *CFOReportTool) Description() string {
	return "Generate a comprehensive CFO financial report: reconciliation proof, spend forecast, " +
		"budget utilization, pending requests, and actionable recommendations. " +
		"This is the bank-grade report that proves our cost accounting is 100% accurate."
}
func (t *CFOReportTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"lookback_days": map[string]any{
				"type":        "integer",
				"description": "Days of trend data (default 7)",
			},
		},
	}
}
func (t *CFOReportTool) Execute(ctx context.Context, args map[string]any) *Result {
	if OnCFOReport == nil {
		return ErrorResult("cfo_report not available (no DB)")
	}
	days := 7
	if d, ok := args["lookback_days"].(float64); ok && d > 0 {
		days = int(d)
	}
	report, err := OnCFOReport(ctx, days)
	if err != nil {
		return ErrorResult(fmt.Sprintf("CFO report failed: %v", err))
	}

	var sb strings.Builder
	sb.WriteString("# CFO Financial Report\n\n")
	sb.WriteString(fmt.Sprintf("**Generated:** %s | **Period:** %s\n\n", report.GeneratedAt, report.Period))

	// Summary
	sb.WriteString("## Executive Summary\n\n")
	sb.WriteString(fmt.Sprintf("- Total month-to-date: **$%.4f**\n", report.TotalMonthUSD))
	sb.WriteString(fmt.Sprintf("- Today's spend: **$%.4f**\n", report.TotalTodayUSD))
	sb.WriteString(fmt.Sprintf("- Projected month-end: **$%.4f**\n", report.Forecast.ProjectedMonthUSD))
	sb.WriteString(fmt.Sprintf("- Trend: %s\n\n", report.Forecast.TrendDirection))

	// Reconciliation
	sb.WriteString("## Cost Reconciliation (Proof of Accuracy)\n\n")
	sb.WriteString(fmt.Sprintf("- Status: **%s**\n", strings.ToUpper(report.Reconciliation.Status)))
	sb.WriteString(fmt.Sprintf("- Drift: %d µUSD (%.6f%%)\n", report.Reconciliation.DriftUUSD, report.Reconciliation.DriftPercent))
	sb.WriteString(fmt.Sprintf("- Pricing coverage: %.1f%%\n", report.Reconciliation.PricingCoverage))
	sb.WriteString(fmt.Sprintf("- %s\n\n", report.Reconciliation.Explanation))

	// Budget Status
	if len(report.BudgetStatus) > 0 {
		sb.WriteString("## Budget Utilization\n\n")
		sb.WriteString("| Agent | Role | Budget | Spent | Used | Status |\n")
		sb.WriteString("|-------|------|--------|-------|------|--------|\n")
		for _, b := range report.BudgetStatus {
			sb.WriteString(fmt.Sprintf("| %s | %s | $%.2f | $%.4f | %.0f%% | %s |\n",
				b.DisplayName, b.OrgRole, b.MonthlyBudget, b.SpentUSD, b.PercentUsed, b.Status))
		}
		sb.WriteString("\n")
	}

	// Pending Requests
	if len(report.PendingRequests) > 0 {
		sb.WriteString("## Pending Budget Requests\n\n")
		for _, r := range report.PendingRequests {
			sb.WriteString(fmt.Sprintf("- **%s** (%s): requesting $%.2f (current $%.2f) — %s\n",
				r.AgentName, r.OrgRole, r.RequestedUSD, r.CurrentUSD, r.Reason))
		}
		sb.WriteString("\n")
	}

	// Anomalies
	if len(report.Forecast.Anomalies) > 0 {
		sb.WriteString("## Anomalies\n\n")
		for _, a := range report.Forecast.Anomalies {
			sb.WriteString(fmt.Sprintf("- **%s** (%s): $%.4f — %.1fσ above normal\n",
				a.AgentName, a.Date, a.SpendUSD, a.Sigma))
		}
		sb.WriteString("\n")
	}

	// Recommendations
	if len(report.Recommendations) > 0 {
		sb.WriteString("## Recommendations\n\n")
		for _, r := range report.Recommendations {
			sb.WriteString(fmt.Sprintf("- %s\n", r))
		}
	}

	return TextResult(sb.String())
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

// stdDev calculates standard deviation for a slice of float64.
func StdDev(data []float64) float64 {
	if len(data) < 2 {
		return 0
	}
	var sum float64
	for _, v := range data {
		sum += v
	}
	mean := sum / float64(len(data))
	var variance float64
	for _, v := range data {
		diff := v - mean
		variance += diff * diff
	}
	variance /= float64(len(data) - 1)
	return math.Sqrt(variance)
}

// Mean calculates the arithmetic mean.
func Mean(data []float64) float64 {
	if len(data) == 0 {
		return 0
	}
	var sum float64
	for _, v := range data {
		sum += v
	}
	return sum / float64(len(data))
}

// keep time import used
var _ = time.Now
