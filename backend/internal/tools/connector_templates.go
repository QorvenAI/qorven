// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package tools

import (
	"context"
	"fmt"
)

// ConnectorGoModTemplate is the minimal go.mod for a standalone connector binary.
const ConnectorGoModTemplate = `module connector

go 1.22
`

// ConnectorTemplateRESTGET is a complete main.go template for REST APIs that use
// GET requests with Bearer token authentication. Agents write this to disk, adapt
// BASE_URL and field extraction, then pass the directory to build_connector.
const ConnectorTemplateRESTGET = `// Qorven Connector Template: REST GET
// Adapt BASE_URL, field extraction, and auth for your specific API.
// See: https://docs.qorven.ai/connectors

package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// BASE_URL is the endpoint this connector calls.
// Replace with the real API endpoint before building.
const BASE_URL = "https://api.example.com/v1/endpoint"

func main() {
	// Read args from stdin (JSON object)
	args := map[string]any{}
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 1<<20), 1<<20)
	var inputLines []string
	for scanner.Scan() {
		inputLines = append(inputLines, scanner.Text())
	}
	if len(inputLines) > 0 {
		if err := json.Unmarshal([]byte(strings.Join(inputLines, "\n")), &args); err != nil {
			writeError(fmt.Sprintf("failed to parse input: %v", err))
			return
		}
	}

	// Read API key from environment (injected by Qorven connector manager)
	// TODO: Replace SLUG with your connector's slug in UPPER_SNAKE_CASE (e.g. CONNECTOR_BINANCE_KEY)
	apiKey := os.Getenv("CONNECTOR_SLUG_KEY")
	if apiKey == "" {
		writeError("CONNECTOR_SLUG_KEY environment variable is not set")
		return
	}

	// Build query string from all string/numeric args
	params := url.Values{}
	for k, v := range args {
		switch val := v.(type) {
		case string:
			params.Set(k, val)
		case float64:
			params.Set(k, fmt.Sprintf("%g", val))
		case bool:
			if val {
				params.Set(k, "true")
			} else {
				params.Set(k, "false")
			}
		}
	}

	targetURL := BASE_URL
	if len(params) > 0 {
		targetURL = BASE_URL + "?" + params.Encode()
	}

	// Make HTTP GET request
	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest(http.MethodGet, targetURL, nil)
	if err != nil {
		writeError(fmt.Sprintf("failed to build request: %v", err))
		return
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		writeError(fmt.Sprintf("request failed: %v", err))
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		writeError(fmt.Sprintf("failed to read response: %v", err))
		return
	}

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		writeError(fmt.Sprintf("authentication failed (HTTP %d) — check CONNECTOR_SLUG_KEY", resp.StatusCode))
		return
	}
	if resp.StatusCode != http.StatusOK {
		writeError(fmt.Sprintf("unexpected HTTP %d: %s", resp.StatusCode, truncate(string(body), 300)))
		return
	}

	// Pretty-print JSON response
	var pretty strings.Builder
	var data any
	if err := json.Unmarshal(body, &data); err != nil {
		// Not JSON — return raw body
		writeSuccess(string(body), string(body))
		return
	}
	enc := json.NewEncoder(&pretty)
	enc.SetIndent("", "  ")
	_ = enc.Encode(data)

	text := strings.TrimSpace(pretty.String())
	// TODO: adapt the user-facing summary to extract meaningful fields
	writeSuccess(text, "**API Response:**\n\n    "+strings.ReplaceAll(text, "\n", "\n    "))
}

func writeSuccess(text, userMD string) {
	out := map[string]string{"text": text, "user": userMD}
	b, _ := json.Marshal(out)
	fmt.Print("#!qorven:json\n")
	fmt.Println(string(b))
}

func writeError(msg string) {
	fmt.Fprintln(os.Stderr, "error: "+msg)
	os.Exit(1)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
`

// ConnectorTemplateRESTPOST is a complete main.go template for REST APIs that use
// POST requests with a JSON body and Bearer token authentication.
const ConnectorTemplateRESTPOST = `// Qorven Connector Template: REST POST
// Adapt BASE_URL, field extraction, and auth for your specific API.
// See: https://docs.qorven.ai/connectors

package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// BASE_URL is the endpoint this connector calls.
// Replace with the real API endpoint before building.
const BASE_URL = "https://api.example.com/v1/endpoint"

func main() {
	// Read args from stdin (JSON object)
	args := map[string]any{}
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 1<<20), 1<<20)
	var inputLines []string
	for scanner.Scan() {
		inputLines = append(inputLines, scanner.Text())
	}
	if len(inputLines) > 0 {
		if err := json.Unmarshal([]byte(strings.Join(inputLines, "\n")), &args); err != nil {
			writeError(fmt.Sprintf("failed to parse input: %v", err))
			return
		}
	}

	// Read API key from environment (injected by Qorven connector manager)
	// TODO: Replace SLUG with your connector's slug in UPPER_SNAKE_CASE (e.g. CONNECTOR_BINANCE_KEY)
	apiKey := os.Getenv("CONNECTOR_SLUG_KEY")
	if apiKey == "" {
		writeError("CONNECTOR_SLUG_KEY environment variable is not set")
		return
	}

	// Serialize args as JSON body
	bodyBytes, err := json.Marshal(args)
	if err != nil {
		writeError(fmt.Sprintf("failed to serialize request body: %v", err))
		return
	}

	// Make HTTP POST request
	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest(http.MethodPost, BASE_URL, bytes.NewReader(bodyBytes))
	if err != nil {
		writeError(fmt.Sprintf("failed to build request: %v", err))
		return
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		writeError(fmt.Sprintf("request failed: %v", err))
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		writeError(fmt.Sprintf("failed to read response: %v", err))
		return
	}

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		writeError(fmt.Sprintf("authentication failed (HTTP %d) — check CONNECTOR_SLUG_KEY", resp.StatusCode))
		return
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		writeError(fmt.Sprintf("unexpected HTTP %d: %s", resp.StatusCode, truncate(string(body), 300)))
		return
	}

	// Pretty-print JSON response
	var pretty strings.Builder
	var data any
	if err := json.Unmarshal(body, &data); err != nil {
		// Not JSON — return raw body
		writeSuccess(string(body), string(body))
		return
	}
	enc := json.NewEncoder(&pretty)
	enc.SetIndent("", "  ")
	_ = enc.Encode(data)

	text := strings.TrimSpace(pretty.String())
	// TODO: adapt the user-facing summary to extract meaningful fields
	writeSuccess(text, "**API Response:**\n\n    "+strings.ReplaceAll(text, "\n", "\n    "))
}

func writeSuccess(text, userMD string) {
	out := map[string]string{"text": text, "user": userMD}
	b, _ := json.Marshal(out)
	fmt.Print("#!qorven:json\n")
	fmt.Println(string(b))
}

func writeError(msg string) {
	fmt.Fprintln(os.Stderr, "error: "+msg)
	os.Exit(1)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
`

// ConnectorTemplateRSS is a complete main.go template for RSS/Atom feed ingestion.
// No authentication is required. Accepts feed_url and optional limit from stdin.
const ConnectorTemplateRSS = `// Qorven Connector Template: RSS/Atom Feed
// Adapt feed parsing and field extraction for your specific feed structure.
// See: https://docs.qorven.ai/connectors

package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

func main() {
	// Read args from stdin (JSON object)
	args := map[string]any{}
	scanner := bufio.NewScanner(os.Stdin)
	var inputLines []string
	for scanner.Scan() {
		inputLines = append(inputLines, scanner.Text())
	}
	if len(inputLines) > 0 {
		if err := json.Unmarshal([]byte(strings.Join(inputLines, "\n")), &args); err != nil {
			writeError(fmt.Sprintf("failed to parse input: %v", err))
			return
		}
	}

	// Required: feed_url
	feedURL, _ := args["feed_url"].(string)
	if feedURL == "" {
		writeError("feed_url is required")
		return
	}

	// Optional: limit (default 10)
	limit := 10
	if v, ok := args["limit"].(float64); ok && v > 0 {
		limit = int(v)
	}

	// Fetch the feed
	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest(http.MethodGet, feedURL, nil)
	if err != nil {
		writeError(fmt.Sprintf("failed to build request: %v", err))
		return
	}
	req.Header.Set("Accept", "application/rss+xml, application/atom+xml, text/xml, */*")
	req.Header.Set("User-Agent", "Qorven-Connector/1.0")

	resp, err := client.Do(req)
	if err != nil {
		writeError(fmt.Sprintf("request failed: %v", err))
		return
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		writeError(fmt.Sprintf("failed to read response: %v", err))
		return
	}
	if resp.StatusCode != http.StatusOK {
		writeError(fmt.Sprintf("unexpected HTTP %d", resp.StatusCode))
		return
	}

	content := string(bodyBytes)

	// Try RSS 2.0 first (<item> elements)
	items := extractItems(content, "item")
	isAtom := false
	if len(items) == 0 {
		// Fall back to Atom (<entry> elements)
		items = extractItems(content, "entry")
		isAtom = true
	}

	if len(items) == 0 {
		writeError("no items found in feed — the URL may not be a valid RSS/Atom feed")
		return
	}

	if len(items) > limit {
		items = items[:limit]
	}

	var sb strings.Builder
	var userSB strings.Builder

	feedTitle := extractTagContent(content, "title")
	header := fmt.Sprintf("Feed: %s (%d items)\n\n", feedTitle, len(items))
	sb.WriteString(header)
	userSB.WriteString("**" + feedTitle + "**\n\n")

	for _, item := range items {
		title := extractTagContent(item, "title")
		link := ""
		if isAtom {
			// Atom <link href="..."/>
			link = extractAtomLink(item)
		} else {
			link = extractTagContent(item, "link")
		}
		desc := extractTagContent(item, "description")
		if desc == "" {
			desc = extractTagContent(item, "summary")
		}
		pubDate := extractTagContent(item, "pubDate")
		if pubDate == "" {
			pubDate = extractTagContent(item, "published")
			if pubDate == "" {
				pubDate = extractTagContent(item, "updated")
			}
		}

		// Strip CDATA wrapper if present
		title = stripCDATA(title)
		desc = stripCDATA(desc)

		sb.WriteString(fmt.Sprintf("• %s\n  %s\n  %s\n\n", title, link, pubDate))
		userSB.WriteString(fmt.Sprintf("- **%s**\n  %s  \n  *%s*\n\n", title, link, pubDate))
		_ = desc // desc available for richer output — adapt as needed
	}

	writeSuccess(strings.TrimSpace(sb.String()), strings.TrimSpace(userSB.String()))
}

// extractItems returns all occurrences of <tag>...</tag> or self-closing <tag .../> blocks.
func extractItems(s, tag string) []string {
	var results []string
	openTag := "<" + tag
	closeTag := "</" + tag + ">"
	pos := 0
	for {
		start := strings.Index(s[pos:], openTag)
		if start < 0 {
			break
		}
		start += pos
		end := strings.Index(s[start:], closeTag)
		if end < 0 {
			break
		}
		end = start + end + len(closeTag)
		results = append(results, s[start:end])
		pos = end
	}
	return results
}

// extractTagContent returns the text content of the first occurrence of <tag>...</tag>.
func extractTagContent(s, tag string) string {
	open := "<" + tag
	close := "</" + tag + ">"
	start := strings.Index(s, open)
	if start < 0 {
		return ""
	}
	// Find the end of the opening tag (skip attributes)
	tagEnd := strings.Index(s[start:], ">")
	if tagEnd < 0 {
		return ""
	}
	contentStart := start + tagEnd + 1
	end := strings.Index(s[contentStart:], close)
	if end < 0 {
		return ""
	}
	return strings.TrimSpace(s[contentStart : contentStart+end])
}

// extractAtomLink extracts the href attribute from an Atom <link href="..."/> element.
func extractAtomLink(s string) string {
	linkIdx := strings.Index(s, "<link")
	if linkIdx < 0 {
		return ""
	}
	hrefIdx := strings.Index(s[linkIdx:], "href=")
	if hrefIdx < 0 {
		return ""
	}
	rest := s[linkIdx+hrefIdx+5:]
	if len(rest) == 0 {
		return ""
	}
	quote := rest[0]
	if quote != '"' && quote != '\'' {
		return ""
	}
	rest = rest[1:]
	end := strings.IndexByte(rest, quote)
	if end < 0 {
		return ""
	}
	return rest[:end]
}

// stripCDATA removes <![CDATA[ ... ]]> wrappers.
func stripCDATA(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "<![CDATA[") && strings.HasSuffix(s, "]]>") {
		return strings.TrimSpace(s[9 : len(s)-3])
	}
	return s
}

func writeSuccess(text, userMD string) {
	out := map[string]string{"text": text, "user": userMD}
	b, _ := json.Marshal(out)
	fmt.Print("#!qorven:json\n")
	fmt.Println(string(b))
}

func writeError(msg string) {
	fmt.Fprintln(os.Stderr, "error: "+msg)
	os.Exit(1)
}
`

// GetConnectorTemplateTool returns a ready-to-adapt Go connector template.
// Agents write the template to disk, customize it for a specific API, and
// then call build_connector to compile it into a runnable binary.
type GetConnectorTemplateTool struct{}

func (t *GetConnectorTemplateTool) Name() string { return "get_connector_template" }

func (t *GetConnectorTemplateTool) Description() string {
	return "Get a ready-to-adapt Go connector template. " +
		"Returns a complete main.go you can write to disk, adapt for a specific API, and pass to build_connector. " +
		"Supported types: rest_get (REST GET with Bearer auth), rest_post (REST POST JSON body), rss (RSS/Atom feed ingestion)."
}

func (t *GetConnectorTemplateTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"type": map[string]any{
				"type":        "string",
				"description": "Template type to retrieve.",
				"enum":        []string{"rest_get", "rest_post", "rss"},
			},
		},
		"required": []string{"type"},
	}
}

func (t *GetConnectorTemplateTool) Execute(_ context.Context, args map[string]any) *Result {
	templateType, _ := args["type"].(string)

	var template string
	switch templateType {
	case "rest_get":
		template = ConnectorTemplateRESTGET
	case "rest_post":
		template = ConnectorTemplateRESTPOST
	case "rss":
		template = ConnectorTemplateRSS
	default:
		return ErrorResult(fmt.Sprintf("unknown template type %q — supported: rest_get, rest_post, rss", templateType))
	}

	instructions := fmt.Sprintf(
		"Template type: %s\n\n"+
			"Next steps:\n"+
			"1. Write this template to <dir>/main.go (e.g. /tmp/my-connector/main.go)\n"+
			"2. Write the go.mod template (ConnectorGoModTemplate) to <dir>/go.mod:\n"+
			"   module connector\n\n   go 1.22\n\n"+
			"3. Adapt BASE_URL and field extraction for the specific API\n"+
			"4. Rename CONNECTOR_SLUG_KEY in the template to match your connector's slug\n"+
			"   (e.g. if slug is 'binance', the env var is CONNECTOR_BINANCE_KEY)\n"+
			"5. Call build_connector with the directory path to compile the binary\n\n"+
			"--- main.go ---\n%s",
		templateType, template,
	)

	return &Result{
		ForLLM:  instructions,
		ForUser: fmt.Sprintf("Connector template `%s` retrieved. Write to disk and adapt for your API.", templateType),
	}
}
