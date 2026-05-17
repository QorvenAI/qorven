// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package workflow

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/qorvenai/qorven/internal/providers"
	"github.com/qorvenai/qorven/internal/tools"
)

// Executor runs a workflow step by step.
type Executor struct {
	store       *Store
	providerReg interface{ Default() providers.Provider }
	toolReg     *tools.Registry
	tenantID    string
	// OnDelegate is called when a step delegates to a Soul
	OnDelegate func(ctx context.Context, soulKey, task string)
}

func NewExecutor(store *Store, provReg interface{ Default() providers.Provider }, toolReg *tools.Registry, tenantID string) *Executor {
	return &Executor{store: store, providerReg: provReg, toolReg: toolReg, tenantID: tenantID}
}

// Run executes a workflow from start to finish.
func (e *Executor) Run(ctx context.Context, wf *Workflow, input map[string]any) (*Run, error) {
	// Parse steps
	var steps []Step
	if err := json.Unmarshal(wf.Steps, &steps); err != nil {
		return nil, fmt.Errorf("invalid steps: %w", err)
	}
	if len(steps) == 0 {
		return nil, fmt.Errorf("workflow has no steps")
	}

	// Init variables from workflow defaults + input
	vars := make(map[string]any)
	if wf.Variables != nil {
		json.Unmarshal(wf.Variables, &vars)
	}
	for k, v := range input {
		vars[k] = v
	}

	// Create run record
	runID, err := e.store.CreateRun(ctx, e.tenantID, wf.ID, wf.AgentID)
	if err != nil {
		return nil, err
	}

	slog.Info("workflow.run.start", "workflow", wf.Name, "run", runID, "steps", len(steps))

	// Build step index for branching
	stepIdx := map[string]int{}
	for i, s := range steps {
		stepIdx[s.ID] = i
	}

	// Execute steps
	currentIdx := 0
	var lastResult string

	for currentIdx < len(steps) {
		step := steps[currentIdx]
		varsJSON, _ := json.Marshal(vars)
		e.store.UpdateRun(ctx, runID, "running", currentIdx, varsJSON, lastResult, "")

		slog.Info("workflow.step", "run", runID, "step", step.ID, "type", step.Type, "idx", currentIdx)

		// Per-step timeout: 5 minutes max
		stepCtx, stepCancel := context.WithTimeout(ctx, 5*time.Minute)
		result, nextStepID, err := e.executeStep(stepCtx, step, vars)
		stepCancel()
		if err != nil {
			slog.Error("workflow.step.error", "step", step.ID, "error", err)
			varsJSON, _ := json.Marshal(vars)
			e.store.UpdateRun(ctx, runID, "failed", currentIdx, varsJSON, lastResult, err.Error())
			run, _ := e.store.GetRun(ctx, runID)
			return run, err
		}

		// Save result to variable if specified
		if step.SaveAs != "" && result != "" {
			vars[step.SaveAs] = result
		}
		lastResult = result

		// Determine next step
		if nextStepID != "" {
			if idx, ok := stepIdx[nextStepID]; ok {
				currentIdx = idx
			} else {
				break // unknown step = end
			}
		} else if step.Next != "" {
			if idx, ok := stepIdx[step.Next]; ok {
				currentIdx = idx
			} else {
				break
			}
		} else {
			currentIdx++ // sequential
		}
	}

	// Complete
	varsJSON, _ := json.Marshal(vars)
	e.store.UpdateRun(ctx, runID, "completed", len(steps)-1, varsJSON, lastResult, "")
	slog.Info("workflow.run.complete", "workflow", wf.Name, "run", runID)

	return e.store.GetRun(ctx, runID)
}

// executeStep runs a single step and returns (result, nextStepID, error).
func (e *Executor) executeStep(ctx context.Context, step Step, vars map[string]any) (string, string, error) {
	switch step.Type {
	case StepPrompt:
		return e.execPrompt(ctx, step, vars)
	case StepTool:
		return e.execTool(ctx, step, vars)
	case StepCondition:
		return e.execCondition(ctx, step, vars)
	case StepAPI:
		return e.execAPI(ctx, step, vars)
	case StepDelegate:
		return e.execDelegate(ctx, step, vars)
	case StepNotify:
		return e.execPrompt(ctx, step, vars) // notify = prompt for now
	case "parallel":
		return e.execParallel(ctx, step, vars)
	default:
		return "", "", fmt.Errorf("unknown step type: %s", step.Type)
	}
}

func (e *Executor) execPrompt(ctx context.Context, step Step, vars map[string]any) (string, string, error) {
	prompt := interpolate(step.Prompt, vars)
	p := e.providerReg.Default()
	if p == nil {
		return "", "", fmt.Errorf("no LLM provider")
	}
	resp, err := p.Chat(ctx, providers.ChatRequest{
		Messages: []providers.Message{{Role: "user", Content: prompt}},
		Options:  map[string]any{"temperature": 0.5, "max_tokens": 1000},
	})
	if err != nil {
		return "", "", err
	}
	return resp.Content, "", nil
}

func (e *Executor) execTool(ctx context.Context, step Step, vars map[string]any) (string, string, error) {
	args := make(map[string]any)
	for k, v := range step.Args {
		if s, ok := v.(string); ok {
			args[k] = interpolate(s, vars)
		} else {
			args[k] = v
		}
	}
	toolCtx := tools.WithWorkspace(ctx, "/tmp/qorven-workspace")
	result := e.toolReg.Execute(toolCtx, step.Tool, args)
	return result.ForLLM, "", nil
}

func (e *Executor) execCondition(ctx context.Context, step Step, vars map[string]any) (string, string, error) {
	prompt := interpolate(step.Prompt, vars)
	// Ask LLM to classify — respond with just the branch key
	options := make([]string, 0, len(step.Branches))
	for k := range step.Branches {
		options = append(options, k)
	}
	classifyPrompt := fmt.Sprintf("%s\n\nRespond with ONLY one of these exact words: %s", prompt, strings.Join(options, ", "))

	p := e.providerReg.Default()
	if p == nil {
		return "", "", fmt.Errorf("no LLM provider")
	}
	resp, err := p.Chat(ctx, providers.ChatRequest{
		Messages: []providers.Message{{Role: "user", Content: classifyPrompt}},
		Options:  map[string]any{"temperature": 0.1, "max_tokens": 20},
	})
	if err != nil {
		return "", "", err
	}

	classification := strings.TrimSpace(strings.ToLower(resp.Content))
	if nextStep, ok := step.Branches[classification]; ok {
		return classification, nextStep, nil
	}
	// Fuzzy match
	for k, v := range step.Branches {
		if strings.Contains(classification, k) {
			return k, v, nil
		}
	}
	return classification, "", nil // no match = continue sequential
}

func (e *Executor) execAPI(ctx context.Context, step Step, vars map[string]any) (string, string, error) {
	url := interpolate(step.URL, vars)
	method := step.Method
	if method == "" {
		method = "GET"
	}

	var bodyReader io.Reader
	if step.Body != nil && (method == "POST" || method == "PUT") {
		interpolated := make(map[string]any)
		for k, v := range step.Body {
			if s, ok := v.(string); ok {
				interpolated[k] = interpolate(s, vars)
			} else {
				interpolated[k] = v
			}
		}
		b, _ := json.Marshal(interpolated)
		bodyReader = bytes.NewReader(b)
	}

	apiCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(apiCtx, method, url, bodyReader)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if len(body) > 5000 {
		body = body[:5000]
	}
	return string(body), "", nil
}

func (e *Executor) execDelegate(ctx context.Context, step Step, vars map[string]any) (string, string, error) {
	task := interpolate(step.Task, vars)
	if e.OnDelegate != nil {
		e.OnDelegate(ctx, step.SoulKey, task)
	}
	return fmt.Sprintf("Delegated to @%s: %s", step.SoulKey, task), "", nil
}

func (e *Executor) execParallel(ctx context.Context, step Step, vars map[string]any) (string, string, error) {
	if len(step.Parallel) == 0 {
		return "", "", fmt.Errorf("parallel step has no sub-steps")
	}
	type result struct {
		idx    int
		output string
		err    error
	}
	results := make([]result, len(step.Parallel))
	var wg sync.WaitGroup
	for i, sub := range step.Parallel {
		wg.Add(1)
		go func(idx int, s Step) {
			defer wg.Done()
			out, _, err := e.executeStep(ctx, s, vars)
			results[idx] = result{idx: idx, output: out, err: err}
		}(i, sub)
	}
	wg.Wait()
	var combined []string
	for _, r := range results {
		if r.err != nil {
			slog.Warn("parallel.step.error", "idx", r.idx, "error", r.err)
			continue
		}
		if r.output != "" {
			combined = append(combined, r.output)
		}
		// Save sub-step results to vars
		sub := step.Parallel[r.idx]
		if sub.SaveAs != "" && r.output != "" {
			vars[sub.SaveAs] = r.output
		}
	}
	return strings.Join(combined, "\n"), "", nil
}

// interpolate replaces {{var}} with values from vars map.
func interpolate(template string, vars map[string]any) string {
	result := template
	for k, v := range vars {
		result = strings.ReplaceAll(result, "{{"+k+"}}", fmt.Sprintf("%v", v))
	}
	return result
}
