// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package system

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// PackageInfo describes a Go package discovered by scanning.
type PackageInfo struct {
	Name        string   `json:"name"`
	Path        string   `json:"path"`
	Files       int      `json:"files"`
	Functions   int      `json:"functions"`
	Lines       int      `json:"lines"`
	Description string   `json:"description"` // from first doc comment
	KeyFiles    []string `json:"key_files"`    // most important files
}

// PageInfo describes a frontend page.
type PageInfo struct {
	Name       string   `json:"name"`
	Path       string   `json:"path"`
	Lines      int      `json:"lines"`
	Imports    []string `json:"imports"`    // imported components
	APICalls   []string `json:"api_calls"` // API endpoints used
}

// ConfigInfo describes the parsed config.
type ConfigInfo struct {
	ServerListen string         `json:"server_listen"`
	DatabaseDSN  string         `json:"database_dsn"` // redacted
	Providers    []ProviderInfo `json:"providers"`
}

// ProviderInfo describes a configured LLM provider.
type ProviderInfo struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	APIBase string `json:"api_base"`
	HasKey  bool   `json:"has_key"` // true if api_key is set (don't expose the key)
}

// ServiceHealth describes a running service.
type ServiceHealth struct {
	Name    string        `json:"name"`
	Port    int           `json:"port"`
	Status  string        `json:"status"` // "up", "down", "timeout"
	Latency time.Duration `json:"latency"`
}

// ToolInfo describes a registered tool.
type ToolInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// RouteInfo describes an API route.
type RouteInfo struct {
	Method string `json:"method"`
	Path   string `json:"path"`
}

// --- Scanners ---

// ScanGoPackages walks the internal/ directory and extracts package metadata.
func ScanGoPackages(rootDir string) []PackageInfo {
	internalDir := filepath.Join(rootDir, "internal")
	entries, err := os.ReadDir(internalDir)
	if err != nil {
		return nil
	}

	packages := []PackageInfo{}
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		pkgDir := filepath.Join(internalDir, entry.Name())
		info := scanPackage(entry.Name(), pkgDir)
		if info.Files > 0 {
			packages = append(packages, info)
		}
	}

	sort.Slice(packages, func(i, j int) bool {
		return packages[i].Lines > packages[j].Lines // sort by size
	})
	return packages
}

func scanPackage(name, dir string) PackageInfo {
	info := PackageInfo{Name: name, Path: dir}

	files, _ := filepath.Glob(filepath.Join(dir, "*.go"))
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		info.Files++

		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		content := string(data)
		lines := strings.Count(content, "\n")
		info.Lines += lines

		// Count exported functions
		funcRe := regexp.MustCompile(`(?m)^func [A-Z]`)
		info.Functions += len(funcRe.FindAllString(content, -1))

		// Extract first doc comment as description
		if info.Description == "" {
			scanner := bufio.NewScanner(strings.NewReader(content))
			for scanner.Scan() {
				line := scanner.Text()
				if strings.HasPrefix(line, "// ") && !strings.HasPrefix(line, "// +build") && !strings.HasPrefix(line, "//go:") {
					info.Description = strings.TrimPrefix(line, "// ")
					break
				}
				if strings.HasPrefix(line, "package ") {
					break
				}
			}
		}

		// Track key files (largest non-test files)
		base := filepath.Base(f)
		if lines > 50 {
			info.KeyFiles = append(info.KeyFiles, base)
		}
	}

	// Limit key files to top 5
	if len(info.KeyFiles) > 5 {
		info.KeyFiles = info.KeyFiles[:5]
	}

	return info
}

// ScanFrontendPages walks the app/ directory and extracts page metadata.
func ScanFrontendPages(webDir string) []PageInfo {
	appDir := filepath.Join(webDir, "app", "(app)")
	if _, err := os.Stat(appDir); err != nil {
		return nil
	}

	pages := []PageInfo{}
	filepath.Walk(appDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || info.Name() != "page.tsx" {
			return nil
		}

		data, _ := os.ReadFile(path)
		content := string(data)

		// Extract page name from directory
		rel, _ := filepath.Rel(appDir, filepath.Dir(path))
		if rel == "." {
			rel = "home"
		}

		page := PageInfo{
			Name:  rel,
			Path:  path,
			Lines: strings.Count(content, "\n"),
		}

		// Extract imports
		importRe := regexp.MustCompile(`import\s+.*from\s+['"](@/[^'"]+)['"]`)
		for _, m := range importRe.FindAllStringSubmatch(content, -1) {
			page.Imports = append(page.Imports, m[1])
		}

		// Extract API calls
		apiRe := regexp.MustCompile(`['"](/api/v1/[^'"]+)['"]`)
		for _, m := range apiRe.FindAllStringSubmatch(content, -1) {
			page.APICalls = append(page.APICalls, m[1])
		}
		fetchRe := regexp.MustCompile(`api\(['"]([^'"]+)['"]\)`)
		for _, m := range fetchRe.FindAllStringSubmatch(content, -1) {
			page.APICalls = append(page.APICalls, m[1])
		}

		pages = append(pages, page)
		return nil
	})

	return pages
}

// ScanConfig parses config.toml and extracts provider/service info.
func ScanConfig(configPath string) *ConfigInfo {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil
	}
	content := string(data)

	info := &ConfigInfo{}

	// Extract server listen
	if m := regexp.MustCompile(`listen\s*=\s*"([^"]+)"`).FindStringSubmatch(content); len(m) > 1 {
		info.ServerListen = m[1]
	}

	// Extract database DSN (redact password)
	if m := regexp.MustCompile(`dsn\s*=\s*"([^"]+)"`).FindStringSubmatch(content); len(m) > 1 {
		dsn := m[1]
		// Redact password
		dsn = regexp.MustCompile(`://[^:]+:[^@]+@`).ReplaceAllString(dsn, "://***:***@")
		info.DatabaseDSN = dsn
	}

	// Extract providers
	providerBlocks := regexp.MustCompile(`(?s)\[\[providers\]\](.*?)(?:\[\[|\z)`).FindAllStringSubmatch(content, -1)
	for _, block := range providerBlocks {
		p := ProviderInfo{}
		if m := regexp.MustCompile(`name\s*=\s*"([^"]+)"`).FindStringSubmatch(block[1]); len(m) > 1 {
			p.Name = m[1]
		}
		if m := regexp.MustCompile(`type\s*=\s*"([^"]+)"`).FindStringSubmatch(block[1]); len(m) > 1 {
			p.Type = m[1]
		}
		if m := regexp.MustCompile(`api_base\s*=\s*"([^"]+)"`).FindStringSubmatch(block[1]); len(m) > 1 {
			p.APIBase = m[1]
		}
		p.HasKey = strings.Contains(block[1], "api_key")
		if p.Name != "" {
			info.Providers = append(info.Providers, p)
		}
	}

	return info
}

// ScanServiceHealth checks if services are responding on their ports.
func ScanServiceHealth() []ServiceHealth {
	services := []struct {
		name string
		port int
		path string
	}{
		{"Backend", 4200, "/health"},
		{"Frontend", 3000, "/"},
		{"LiteLLM", 4100, "/health"},
		{"STT (Whisper)", 8881, "/health"},
		{"PostgreSQL", 5432, ""},
	}

	results := []ServiceHealth{}
	for _, svc := range services {
		health := ServiceHealth{Name: svc.name, Port: svc.port}
		start := time.Now()

		if svc.path != "" {
			// HTTP check
			client := &http.Client{Timeout: 2 * time.Second}
			resp, err := client.Get(fmt.Sprintf("http://localhost:%d%s", svc.port, svc.path))
			if err != nil {
				health.Status = "down"
			} else {
				resp.Body.Close()
				health.Status = "up"
			}
		} else {
			// TCP check (for PostgreSQL)
			conn, err := net.DialTimeout("tcp", fmt.Sprintf("localhost:%d", svc.port), 2*time.Second)
			if err != nil {
				health.Status = "down"
			} else {
				conn.Close()
				health.Status = "up"
			}
		}

		health.Latency = time.Since(start)
		results = append(results, health)
	}

	return results
}

// ScanTools extracts registered tool names from the tool registry.
func ScanTools(gatewayPath string) []ToolInfo {
	data, err := os.ReadFile(gatewayPath)
	if err != nil {
		return nil
	}
	content := string(data)

	tools := []ToolInfo{}
	// Match: reg.Register(tools.NewXxxTool(...))
	re := regexp.MustCompile(`reg\.Register\(tools\.New(\w+Tool)\(`)
	for _, m := range re.FindAllStringSubmatch(content, -1) {
		name := camelToSnake(strings.TrimSuffix(m[1], "Tool"))
		tools = append(tools, ToolInfo{Name: name})
	}
	return tools
}

// ScanRoutes extracts API routes from gateway.go.
func ScanRoutes(gatewayPath string) []RouteInfo {
	data, err := os.ReadFile(gatewayPath)
	if err != nil {
		return nil
	}

	routes := []RouteInfo{}
	re := regexp.MustCompile(`r\.(Get|Post|Put|Delete|Patch)\("([^"]+)"`)
	for _, m := range re.FindAllStringSubmatch(string(data), -1) {
		routes = append(routes, RouteInfo{Method: strings.ToUpper(m[1]), Path: m[2]})
	}
	return routes
}

func camelToSnake(s string) string {
	re := regexp.MustCompile(`([a-z])([A-Z])`)
	snake := re.ReplaceAllString(s, "${1}_${2}")
	return strings.ToLower(snake)
}

// --- Generator ---

// GenerateSystemKnowledge runs all scanners and produces the QORVEN.md content.
func GenerateSystemKnowledge(ctx context.Context, backendDir, frontendDir, configPath string) string {
	var sb strings.Builder

	sb.WriteString("# QORVEN.md — Auto-Generated System Knowledge\n\n")
	sb.WriteString("> Generated at: " + time.Now().Format(time.RFC3339) + "\n\n")

	// Architecture
	sb.WriteString("## Architecture\n\n")
	services := ScanServiceHealth()
	sb.WriteString("| Service | Port | Status |\n|---------|------|--------|\n")
	for _, s := range services {
		sb.WriteString(fmt.Sprintf("| %s | %d | %s (%s) |\n", s.Name, s.Port, s.Status, s.Latency.Round(time.Millisecond)))
	}
	sb.WriteString("\n")

	// Config
	if cfg := ScanConfig(configPath); cfg != nil {
		sb.WriteString("## Configuration\n\n")
		sb.WriteString(fmt.Sprintf("- Server: `%s`\n", cfg.ServerListen))
		sb.WriteString(fmt.Sprintf("- Database: `%s`\n", cfg.DatabaseDSN))
		sb.WriteString(fmt.Sprintf("- Providers: %d configured\n\n", len(cfg.Providers)))
		for _, p := range cfg.Providers {
			sb.WriteString(fmt.Sprintf("  - **%s** (%s) → %s\n", p.Name, p.Type, p.APIBase))
		}
		sb.WriteString("\n")
	}

	// Packages
	packages := ScanGoPackages(backendDir)
	sb.WriteString(fmt.Sprintf("## Backend Packages (%d)\n\n", len(packages)))
	sb.WriteString("| Package | Files | Functions | Lines | Description |\n|---------|-------|-----------|-------|-------------|\n")
	for _, p := range packages {
		desc := p.Description
		if len(desc) > 60 {
			desc = desc[:60] + "..."
		}
		sb.WriteString(fmt.Sprintf("| %s | %d | %d | %d | %s |\n", p.Name, p.Files, p.Functions, p.Lines, desc))
	}
	sb.WriteString("\n")

	// Frontend pages
	pages := ScanFrontendPages(frontendDir)
	if len(pages) > 0 {
		sb.WriteString(fmt.Sprintf("## Frontend Pages (%d)\n\n", len(pages)))
		sb.WriteString("| Page | Lines | API Calls |\n|------|-------|-----------|\n")
		for _, p := range pages {
			apis := strings.Join(p.APICalls, ", ")
			if len(apis) > 60 {
				apis = apis[:60] + "..."
			}
			sb.WriteString(fmt.Sprintf("| %s | %d | %s |\n", p.Name, p.Lines, apis))
		}
		sb.WriteString("\n")
	}

	// Routes
	routes := ScanRoutes(filepath.Join(backendDir, "internal", "gateway", "gateway.go"))
	sb.WriteString(fmt.Sprintf("## API Routes (%d)\n\n", len(routes)))
	// Group by prefix
	groups := map[string][]RouteInfo{}
	for _, r := range routes {
		parts := strings.SplitN(r.Path, "/", 3)
		group := "/"
		if len(parts) > 1 {
			group = "/" + parts[1]
		}
		groups[group] = append(groups[group], r)
	}
	for group, rs := range groups {
		sb.WriteString(fmt.Sprintf("### %s (%d routes)\n", group, len(rs)))
		for _, r := range rs {
			sb.WriteString(fmt.Sprintf("- `%s %s`\n", r.Method, r.Path))
		}
		sb.WriteString("\n")
	}

	// Tools
	tools := ScanTools(filepath.Join(backendDir, "internal", "gateway", "gateway.go"))
	if len(tools) > 0 {
		sb.WriteString(fmt.Sprintf("## Registered Tools (%d)\n\n", len(tools)))
		for _, t := range tools {
			sb.WriteString(fmt.Sprintf("- `%s`\n", t.Name))
		}
		sb.WriteString("\n")
	}

	// Safe modification rules (always included)
	sb.WriteString(`## Safe Modification Rules

1. **Build before restart**: ` + "`CGO_ENABLED=0 go build ./...`" + `
2. **Test before deploy**: ` + "`CGO_ENABLED=0 go test ./... -count=1`" + `
3. **Config changes**: edit config.toml, restart backend
4. **Frontend changes**: edit in qorven-web/, build with ` + "`npx next build`" + `
5. **Database changes**: create new migration, apply with psql
6. **Never modify**: migrations/ (append only)
7. **Restart backend**: ` + "`sudo systemctl restart qorven`" + `
8. **Restart frontend**: ` + "`sudo systemctl restart qorven-web`" + `
`)

	return sb.String()
}
