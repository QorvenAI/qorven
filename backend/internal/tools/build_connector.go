// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package tools

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// connectorToolDef mirrors apps.ToolDef for YAML generation inside build_connector.
// It is intentionally local to avoid an import cycle (apps imports tools).
type connectorToolDef struct {
	Name        string         `yaml:"name"`
	Description string         `yaml:"description,omitempty"`
	Command     string         `yaml:"command"`
	Parameters  map[string]any `yaml:"parameters,omitempty"`
	Timeout     int            `yaml:"timeout,omitempty"`
}

// connectorManifest mirrors apps.Manifest for YAML generation inside build_connector.
type connectorManifest struct {
	Slug        string             `yaml:"slug"`
	DisplayName string             `yaml:"display_name"`
	Version     string             `yaml:"version"`
	Description string             `yaml:"description"`
	Permissions []string           `yaml:"permissions,omitempty"`
	RequiresEnv []string           `yaml:"requires_env,omitempty"`
	Tools       []connectorToolDef `yaml:"tools,omitempty"`
}

// AppRunToolFunc invokes a tool from an already-installed app.
type AppRunToolFunc func(ctx context.Context, slug, toolName string, args map[string]any) (*Result, error)

// BuildConnectorTool compiles a Go connector binary, generates app.yaml,
// installs the connector, and optionally runs a test invocation.
type BuildConnectorTool struct {
	install AppInstallFunc
	runTool AppRunToolFunc
}

// NewBuildConnectorTool creates a BuildConnectorTool with the given callbacks.
func NewBuildConnectorTool(install AppInstallFunc, runTool AppRunToolFunc) *BuildConnectorTool {
	return &BuildConnectorTool{install: install, runTool: runTool}
}

func (t *BuildConnectorTool) Name() string { return "build_connector" }
func (t *BuildConnectorTool) Description() string {
	return "Build and install a connector from Go source code. " +
		"Compiles the Go source in the given directory, generates app.yaml, installs the connector, " +
		"and optionally runs a test invocation to verify it works. " +
		"The source directory must contain main.go and go.mod (use get_connector_template to get a starting point). " +
		"Returns the installed connector slug and available tool names."
}

func (t *BuildConnectorTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"dir": map[string]any{
				"type":        "string",
				"description": "Absolute path to the directory containing main.go and go.mod.",
			},
			"slug": map[string]any{
				"type":        "string",
				"description": "Connector identifier: lowercase letters, digits, hyphens (e.g. 'binance', 'aramex-tracking').",
			},
			"display_name": map[string]any{
				"type":        "string",
				"description": "Human-readable connector name (e.g. 'Binance', 'Aramex Tracking').",
			},
			"description": map[string]any{
				"type":        "string",
				"description": "One-sentence description of what this connector does.",
			},
			"tools_schema": map[string]any{
				"type":        "object",
				"description": "Tool definitions for app.yaml. Each key is a tool name, value is {description, command, parameters, timeout}. Example: {\"get_price\": {\"description\": \"Get current price\", \"parameters\": {\"type\": \"object\", \"properties\": {\"symbol\": {\"type\": \"string\"}}, \"required\": [\"symbol\"]}}}",
			},
			"credential_env": map[string]any{
				"type":        "string",
				"description": "The env var name the connector reads for its API key (e.g. CONNECTOR_BINANCE_KEY). Defaults to CONNECTOR_<SLUG>_KEY.",
			},
			"test_args": map[string]any{
				"type":        "object",
				"description": "Optional args to test-invoke the first tool after install. If the invocation fails, build_connector returns an error.",
			},
			"version": map[string]any{
				"type":        "string",
				"description": "Connector version string (default: 1.0.0).",
			},
		},
		"required": []string{"dir", "slug", "display_name", "description", "tools_schema"},
	}
}

var slugRegexp = regexp.MustCompile(`^[a-z][a-z0-9\-]{0,61}$`)
var envVarRegexp = regexp.MustCompile(`^CONNECTOR_[A-Z][A-Z0-9_]*$`)

func (t *BuildConnectorTool) Execute(ctx context.Context, args map[string]any) *Result {
	// Step 1: Validate inputs
	dir, _ := args["dir"].(string)
	slug, _ := args["slug"].(string)
	displayName, _ := args["display_name"].(string)
	description, _ := args["description"].(string)
	toolsSchema, _ := args["tools_schema"].(map[string]any)
	credentialEnv, _ := args["credential_env"].(string)
	testArgs, _ := args["test_args"].(map[string]any)
	version, _ := args["version"].(string)
	if version == "" {
		version = "1.0.0"
	}

	if dir == "" {
		return ErrorResult("dir is required")
	}
	if !filepath.IsAbs(dir) {
		return ErrorResult("dir must be an absolute path")
	}
	dir = filepath.Clean(dir)
	if slug == "" {
		return ErrorResult("slug is required")
	}
	if displayName == "" {
		return ErrorResult("display_name is required")
	}
	if description == "" {
		return ErrorResult("description is required")
	}
	if len(toolsSchema) == 0 {
		return ErrorResult("tools_schema is required and must not be empty")
	}

	// Normalize slug
	slug = strings.ToLower(strings.TrimSpace(slug))
	if !slugRegexp.MatchString(slug) {
		return ErrorResult("slug must be lowercase letters, digits, hyphens, starting with a letter (max 62 chars)")
	}

	if credentialEnv == "" {
		credentialEnv = "CONNECTOR_" + strings.ToUpper(strings.ReplaceAll(slug, "-", "_")) + "_KEY"
	} else if !envVarRegexp.MatchString(credentialEnv) {
		return ErrorResult("credential_env must be a valid env var name (uppercase letters, digits, underscores; must start with CONNECTOR_)")
	}

	// Step 2: Validate directory
	if _, err := os.Stat(filepath.Join(dir, "main.go")); err != nil {
		return ErrorResult(fmt.Sprintf("main.go not found in %s — write your connector source first", dir))
	}
	if _, err := os.Stat(filepath.Join(dir, "go.mod")); err != nil {
		return ErrorResult(fmt.Sprintf("go.mod not found in %s — write a go.mod file first (use ConnectorGoModTemplate)", dir))
	}

	// Step 3: Compile
	binaryPath := filepath.Join(dir, "connector")
	buildCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	buildCmd := exec.CommandContext(buildCtx, "go", "build", "-o", binaryPath, "./...")
	buildCmd.Dir = dir
	buildCmd.Env = append(os.Environ(), "GOFLAGS=-mod=mod")
	var buildStderr bytes.Buffer
	buildCmd.Stderr = &buildStderr

	if err := buildCmd.Run(); err != nil {
		msg := buildStderr.String()
		if len(msg) > 65536 {
			msg = msg[:65536] + "\n... (truncated)"
		}
		return ErrorResult(fmt.Sprintf("compilation failed:\n%s", msg))
	}

	// Step 4: Generate app.yaml
	var toolDefs []connectorToolDef
	for name, v := range toolsSchema {
		defMap, _ := v.(map[string]any)
		td := connectorToolDef{
			Name:    name,
			Command: "./connector",
			Timeout: 30,
		}
		if defMap != nil {
			if d, ok := defMap["description"].(string); ok {
				td.Description = d
			}
			if c, ok := defMap["command"].(string); ok && c != "" {
				td.Command = c
			}
			if tmt, ok := defMap["timeout"].(float64); ok && tmt > 0 {
				td.Timeout = int(tmt)
			}
			if p, ok := defMap["parameters"].(map[string]any); ok {
				td.Parameters = p
			}
		}
		toolDefs = append(toolDefs, td)
	}

	sort.Slice(toolDefs, func(i, j int) bool {
		return toolDefs[i].Name < toolDefs[j].Name
	})

	// Do NOT set RequiresEnv: credentials for connectors are injected by credLookup
	// at subprocess execution time (not via the main process environment), so adding
	// the env var name here would cause LoadAll to skip the connector on every restart.
	manifest := connectorManifest{
		Slug:        slug,
		DisplayName: displayName,
		Version:     version,
		Description: description,
		Permissions: []string{"tool_register"},
		Tools:       toolDefs,
	}

	yamlBytes, err := yaml.Marshal(manifest)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to generate app.yaml: %v", err))
	}
	if err := os.WriteFile(filepath.Join(dir, "app.yaml"), yamlBytes, 0644); err != nil {
		return ErrorResult(fmt.Sprintf("failed to write app.yaml: %v", err))
	}

	// Step 5: Install
	_, installedSlug, _, err := t.install(ctx, dir)
	if err != nil {
		os.Remove(binaryPath) // best-effort cleanup of compiled binary
		return ErrorResult(fmt.Sprintf("install failed: %v", err))
	}

	// Step 6: Optional test invocation
	toolNames := make([]string, 0, len(toolDefs))
	for _, td := range toolDefs {
		toolNames = append(toolNames, td.Name)
	}

	if testArgs != nil && t.runTool != nil && len(toolDefs) > 0 {
		firstTool := toolDefs[0].Name
		testResult, err := t.runTool(ctx, installedSlug, firstTool, testArgs)
		if err != nil || (testResult != nil && testResult.IsError) {
			errMsg := "test invocation failed"
			if err != nil {
				errMsg = err.Error()
			} else if testResult != nil {
				errMsg = testResult.ForLLM
			}
			return ErrorResult(fmt.Sprintf("connector installed but test invocation of %q failed: %s", firstTool, errMsg))
		}
	}

	// Step 7: Return success
	toolList := strings.Join(toolNames, ", ")
	return &Result{
		ForLLM: fmt.Sprintf(
			"Connector %q installed successfully. Available tools: %s. Invoke as: %s.%s(args). "+
				"Credential env var: %s (store via store_credential tool).",
			installedSlug, toolList, installedSlug, toolDefs[0].Name, credentialEnv),
		ForUser: fmt.Sprintf("Connector `%s` installed — tools: `%s`", installedSlug, toolList),
	}
}
