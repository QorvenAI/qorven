// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package social

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/qorvenai/qorven/internal/tools"
)

// intelligence_tool.go — Agent tool for Qorven Social Intelligence.

type IntelligenceTool struct {
	engine  *IntelligenceEngine
	monitor *TopicMonitor
}

func NewIntelligenceTool(engine *IntelligenceEngine, monitor *TopicMonitor) *IntelligenceTool {
	return &IntelligenceTool{engine: engine, monitor: monitor}
}

func (t *IntelligenceTool) Name() string { return "qorven_intelligence" }
func (t *IntelligenceTool) Description() string {
	return `Search and analyze trends across social platforms (Reddit, HN, GitHub, RSS). Actions: search_trends (parallel multi-platform search), analyze_topic (deep analysis with synthesis), read_item (read a specific post/thread), monitor_topic (auto-track a topic), list_monitors, stop_monitor.`
}

func (t *IntelligenceTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type": "string",
				"enum": []string{"search_trends", "analyze_topic", "read_item", "monitor_topic", "list_monitors", "stop_monitor"},
			},
			"query":     map[string]any{"type": "string", "description": "Search query or topic"},
			"platforms": map[string]any{"type": "string", "description": "Comma-separated: reddit,hackernews,github"},
			"url":       map[string]any{"type": "string", "description": "URL to read (for read_item)"},
			"platform":  map[string]any{"type": "string", "description": "Platform for read_item"},
			"monitor_id": map[string]any{"type": "string", "description": "Monitor ID for stop_monitor"},
			"interval":  map[string]any{"type": "string", "description": "Monitor interval: 1h, 6h, 24h"},
			"max_results": map[string]any{"type": "integer", "description": "Max results (default 10)"},
			"time_range": map[string]any{"type": "string", "description": "Time range: 1d, 7d, 30d"},
		},
		"required": []string{"action"},
	}
}

func (t *IntelligenceTool) Execute(ctx context.Context, args map[string]any) *tools.Result {
	action, _ := args["action"].(string)
	query, _ := args["query"].(string)
	agentID := "default"

	switch action {
	case "search_trends":
		if query == "" { return tools.ErrorResult("query required") }
		opts := t.parseOpts(args)
		platforms := t.parsePlatforms(args)

		var results []ScoredResult
		var err error
		if len(platforms) > 0 {
			results, err = t.engine.SearchPlatforms(ctx, query, platforms, opts)
		} else {
			results, err = t.engine.SearchAll(ctx, query, opts)
		}
		if err != nil { return tools.ErrorResult("search failed: " + err.Error()) }

		return tools.SuccessResult(formatResults(query, results))

	case "analyze_topic":
		if query == "" { return tools.ErrorResult("query required") }
		opts := t.parseOpts(args)
		brief, results, err := t.engine.Synthesize(ctx, query, opts)
		if err != nil { return tools.ErrorResult("analysis failed: " + err.Error()) }

		output := brief
		if len(results) > 0 {
			output += fmt.Sprintf("\n\n---\n%d total results scored by engagement.", len(results))
		}
		return tools.SuccessResult(output)

	case "read_item":
		urlStr, _ := args["url"].(string)
		platformStr, _ := args["platform"].(string)
		if urlStr == "" { return tools.ErrorResult("url required") }

		platform := Platform(platformStr)
		if platform == "" { platform = detectPlatform(urlStr) }

		result, err := t.engine.ReadItem(ctx, platform, urlStr)
		if err != nil { return tools.ErrorResult("read failed: " + err.Error()) }

		data, _ := json.MarshalIndent(result, "", "  ")
		return tools.SuccessResult(string(data))

	case "monitor_topic":
		if query == "" { return tools.ErrorResult("query required") }
		if t.monitor == nil { return tools.ErrorResult("monitor not configured") }

		platforms := t.parsePlatforms(args)
		interval := t.parseInterval(args)
		id := t.monitor.Add(agentID, query, platforms, interval)
		return tools.SuccessResult(fmt.Sprintf("📡 Monitor created: %s\nTopic: %q\nInterval: %s\nPlatforms: %v", id, query, interval, platforms))

	case "list_monitors":
		if t.monitor == nil { return tools.ErrorResult("monitor not configured") }
		monitors := t.monitor.List(agentID)
		if len(monitors) == 0 { return tools.SuccessResult("No active monitors.") }

		var b strings.Builder
		b.WriteString(fmt.Sprintf("📡 %d active monitors:\n\n", len(monitors)))
		for _, m := range monitors {
			b.WriteString(fmt.Sprintf("- **%s**: %q every %s (since %s)\n",
				m.ID, m.Topic, m.Interval, m.CreatedAt.Format("Jan 2")))
		}
		return tools.SuccessResult(b.String())

	case "stop_monitor":
		monID, _ := args["monitor_id"].(string)
		if monID == "" { return tools.ErrorResult("monitor_id required") }
		if t.monitor == nil { return tools.ErrorResult("monitor not configured") }
		t.monitor.Remove(monID)
		return tools.SuccessResult(fmt.Sprintf("🛑 Monitor %s stopped.", monID))

	default:
		return tools.ErrorResult("unknown action: " + action)
	}
}

func (t *IntelligenceTool) parseOpts(args map[string]any) SearchOpts {
	opts := SearchOpts{MaxResults: 10, TimeRange: 30 * 24 * time.Hour}
	if n, ok := args["max_results"].(float64); ok { opts.MaxResults = int(n) }
	if tr, ok := args["time_range"].(string); ok {
		switch tr {
		case "1d": opts.TimeRange = 24 * time.Hour
		case "7d": opts.TimeRange = 7 * 24 * time.Hour
		case "30d": opts.TimeRange = 30 * 24 * time.Hour
		}
	}
	return opts
}

func (t *IntelligenceTool) parsePlatforms(args map[string]any) []Platform {
	s, _ := args["platforms"].(string)
	if s == "" { return nil }
	var out []Platform
	for _, p := range strings.Split(s, ",") {
		out = append(out, Platform(strings.TrimSpace(p)))
	}
	return out
}

func (t *IntelligenceTool) parseInterval(args map[string]any) time.Duration {
	s, _ := args["interval"].(string)
	switch s {
	case "1h": return time.Hour
	case "6h": return 6 * time.Hour
	case "24h": return 24 * time.Hour
	default: return time.Hour
	}
}

func detectPlatform(u string) Platform {
	switch {
	case strings.Contains(u, "reddit.com"): return PlatformReddit
	case strings.Contains(u, "news.ycombinator.com"): return PlatformHN
	case strings.Contains(u, "github.com"): return PlatformGitHubRead
	default: return ""
	}
}
