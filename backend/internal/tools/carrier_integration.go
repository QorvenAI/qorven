// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package tools

import (
	"context"
	"fmt"
	"strings"
)

// CarrierTemplate holds the shipment-tracking connector source template.
// The agent receives this, adapts the URL/parsing for the specific carrier's API,
// then passes the result to build_connector.
const CarrierTemplate = `package main

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

// CARRIER_NAME is replaced by the agent with the real carrier name.
const CARRIER_NAME = "REPLACE_CARRIER"

// TRACKING_URL is the carrier's tracking API endpoint.
// The agent replaces this with the real URL (use {{TRACKING_NUMBER}} as placeholder).
const TRACKING_URL = "https://api.REPLACE_CARRIER.com/v1/track?tracking_number={{TRACKING_NUMBER}}"

// AUTH_HEADER is the header name for authentication (usually "Authorization" or a custom header).
const AUTH_HEADER = "Authorization"

// AUTH_PREFIX is prepended to the API key (e.g. "Bearer ", "Api-Key ", or "").
const AUTH_PREFIX = "Bearer "

func main() {
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

	trackingNumber, _ := args["tracking_number"].(string)
	if trackingNumber == "" {
		writeError("tracking_number is required")
		return
	}

	apiKey := os.Getenv("CONNECTOR_REPLACE_SLUG_KEY")
	if apiKey == "" {
		writeError("CONNECTOR_REPLACE_SLUG_KEY not set — store credentials via store_credential")
		return
	}

	url := strings.ReplaceAll(TRACKING_URL, "{{TRACKING_NUMBER}}", trackingNumber)

	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		writeError(fmt.Sprintf("failed to build request: %v", err))
		return
	}
	req.Header.Set(AUTH_HEADER, AUTH_PREFIX+apiKey)
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
		writeError(fmt.Sprintf("authentication failed (HTTP %d) — check API key", resp.StatusCode))
		return
	}
	if resp.StatusCode != http.StatusOK {
		writeError(fmt.Sprintf("unexpected HTTP %d: %s", resp.StatusCode, truncate(string(body), 300)))
		return
	}

	var pretty strings.Builder
	var data any
	if err := json.Unmarshal(body, &data); err != nil {
		writeSuccess(string(body), string(body))
		return
	}
	enc := json.NewEncoder(&pretty)
	enc.SetIndent("", "  ")
	_ = enc.Encode(data)

	text := strings.TrimSpace(pretty.String())
	writeSuccess(text, "**Tracking Result ("+CARRIER_NAME+"):**\n\n    "+strings.ReplaceAll(text, "\n", "\n    "))
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

// KnownCarriers holds API specs for major carriers the system can auto-scaffold.
var KnownCarriers = map[string]CarrierSpec{
	"fedex": {
		Name:        "FedEx",
		Slug:        "fedex-tracking",
		TrackingURL: "https://apis.fedex.com/track/v1/trackingnumbers",
		Method:      "POST",
		AuthHeader:  "Authorization",
		AuthPrefix:  "Bearer ",
		DocsURL:     "https://developer.fedex.com/api/en-us/catalog/track/v1/docs.html",
		Notes:       "FedEx uses OAuth2 — requires client_id+secret to get Bearer token. POST body: {\"trackingInfo\":[{\"trackingNumberInfo\":{\"trackingNumber\":\"...\"}}]}",
	},
	"ups": {
		Name:        "UPS",
		Slug:        "ups-tracking",
		TrackingURL: "https://onlinetools.ups.com/api/track/v1/details/{{TRACKING_NUMBER}}",
		Method:      "GET",
		AuthHeader:  "Authorization",
		AuthPrefix:  "Bearer ",
		DocsURL:     "https://developer.ups.com/api/reference/tracking",
		Notes:       "UPS uses OAuth2 with client credentials grant.",
	},
	"dhl": {
		Name:        "DHL Express",
		Slug:        "dhl-tracking",
		TrackingURL: "https://api-eu.dhl.com/track/shipments?trackingNumber={{TRACKING_NUMBER}}",
		Method:      "GET",
		AuthHeader:  "DHL-API-Key",
		AuthPrefix:  "",
		DocsURL:     "https://developer.dhl.com/api-reference/shipment-tracking",
		Notes:       "DHL uses a direct API key in DHL-API-Key header. Free tier: 250 requests/day.",
	},
	"usps": {
		Name:        "USPS",
		Slug:        "usps-tracking",
		TrackingURL: "https://api.usps.com/tracking/v3/tracking/{{TRACKING_NUMBER}}?expand=DETAIL",
		Method:      "GET",
		AuthHeader:  "Authorization",
		AuthPrefix:  "Bearer ",
		DocsURL:     "https://developer.usps.com/api/85",
		Notes:       "USPS uses OAuth2 client credentials. Token endpoint: https://api.usps.com/oauth2/v3/token",
	},
	"aramex": {
		Name:        "Aramex",
		Slug:        "aramex-tracking",
		TrackingURL: "https://ws.aramex.net/ShippingAPI.V2/Tracking/Service_1_0.svc/json/TrackShipments",
		Method:      "POST",
		AuthHeader:  "Content-Type",
		AuthPrefix:  "",
		DocsURL:     "https://www.aramex.com/developers/apis/tracking",
		Notes:       "Aramex uses account credentials in the POST body (ClientInfo object), not a header.",
	},
	"maersk": {
		Name:        "Maersk",
		Slug:        "maersk-tracking",
		TrackingURL: "https://api.maersk.com/track/{{TRACKING_NUMBER}}?operator=MAEU",
		Method:      "GET",
		AuthHeader:  "Consumer-Key",
		AuthPrefix:  "",
		DocsURL:     "https://developer.maersk.com/api-catalogue/Tracking",
		Notes:       "Maersk uses Consumer-Key header. Register at developer.maersk.com.",
	},
}

type CarrierSpec struct {
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	TrackingURL string `json:"tracking_url"`
	Method      string `json:"method"`
	AuthHeader  string `json:"auth_header"`
	AuthPrefix  string `json:"auth_prefix"`
	DocsURL     string `json:"docs_url"`
	Notes       string `json:"notes"`
}

// BuildCarrierTool provides a high-level tool for agents to scaffold carrier integrations.
// It returns the carrier template + known spec (if available) + instructions for the agent
// to adapt and build_connector the result.
type BuildCarrierTool struct{}

func NewBuildCarrierTool() *BuildCarrierTool { return &BuildCarrierTool{} }
func (t *BuildCarrierTool) Name() string     { return "build_carrier" }
func (t *BuildCarrierTool) Description() string {
	return "Scaffold a shipment-tracking carrier connector. Provide the carrier name (e.g. 'fedex', 'dhl', 'aramex') " +
		"and optionally their API docs URL. Returns a ready-to-adapt Go source template with the carrier's known " +
		"API spec pre-filled, plus step-by-step instructions to compile and install it as a track_shipment tool. " +
		"For unknown carriers, returns the generic template with adaptation instructions."
}

func (t *BuildCarrierTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"carrier": map[string]any{
				"type":        "string",
				"description": "Carrier identifier (e.g. 'fedex', 'dhl', 'ups', 'aramex', 'maersk', 'usps', or any custom name).",
			},
			"api_key": map[string]any{
				"type":        "string",
				"description": "Optional API key/credentials to store immediately. If provided, stored via store_credential after install.",
			},
			"docs_url": map[string]any{
				"type":        "string",
				"description": "Optional URL to the carrier's API documentation. The agent can web_fetch this to learn the exact API shape.",
			},
			"tracking_url": map[string]any{
				"type":        "string",
				"description": "Optional override for the tracking endpoint URL. Use {{TRACKING_NUMBER}} as placeholder.",
			},
		},
		"required": []string{"carrier"},
	}
}

func (t *BuildCarrierTool) Execute(_ context.Context, args map[string]any) *Result {
	carrier, _ := args["carrier"].(string)
	docsURL, _ := args["docs_url"].(string)
	trackingURL, _ := args["tracking_url"].(string)

	if carrier == "" {
		return ErrorResult("carrier name is required")
	}

	carrierKey := strings.ToLower(strings.TrimSpace(carrier))
	carrierKey = strings.ReplaceAll(carrierKey, " ", "-")

	spec, known := KnownCarriers[carrierKey]
	if !known {
		spec = CarrierSpec{
			Name:        carrier,
			Slug:        carrierKey + "-tracking",
			TrackingURL: trackingURL,
			Method:      "GET",
			AuthHeader:  "Authorization",
			AuthPrefix:  "Bearer ",
		}
		if docsURL != "" {
			spec.DocsURL = docsURL
		}
	}

	if trackingURL != "" {
		spec.TrackingURL = trackingURL
	}

	envVar := "CONNECTOR_" + strings.ToUpper(strings.ReplaceAll(spec.Slug, "-", "_")) + "_KEY"

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## Carrier Integration: %s\n\n", spec.Name))

	if known {
		sb.WriteString("**Known carrier** — pre-filled API spec:\n")
		sb.WriteString(fmt.Sprintf("- Endpoint: `%s`\n", spec.TrackingURL))
		sb.WriteString(fmt.Sprintf("- Method: %s\n", spec.Method))
		sb.WriteString(fmt.Sprintf("- Auth header: `%s`\n", spec.AuthHeader))
		sb.WriteString(fmt.Sprintf("- Auth prefix: `%s`\n", spec.AuthPrefix))
		if spec.DocsURL != "" {
			sb.WriteString(fmt.Sprintf("- Docs: %s\n", spec.DocsURL))
		}
		if spec.Notes != "" {
			sb.WriteString(fmt.Sprintf("- Notes: %s\n", spec.Notes))
		}
	} else {
		sb.WriteString("**Unknown carrier** — you'll need to adapt the template:\n")
		if docsURL != "" {
			sb.WriteString(fmt.Sprintf("- Use `web_fetch` on %s to learn the API shape.\n", docsURL))
		} else {
			sb.WriteString("- Ask the user for API documentation or search for it.\n")
		}
	}

	sb.WriteString("\n### Build Steps\n\n")
	sb.WriteString(fmt.Sprintf("1. Create directory: `/tmp/%s/`\n", spec.Slug))
	sb.WriteString("2. Write `go.mod` (use ConnectorGoModTemplate):\n   ```\n   module connector\n\n   go 1.22\n   ```\n")
	sb.WriteString("3. Write `main.go` — adapt the template below:\n")
	sb.WriteString(fmt.Sprintf("   - Replace `REPLACE_CARRIER` with `%s`\n", spec.Name))
	sb.WriteString(fmt.Sprintf("   - Replace `REPLACE_SLUG` with `%s`\n", strings.ToUpper(strings.ReplaceAll(spec.Slug, "-", "_"))))
	sb.WriteString(fmt.Sprintf("   - Set TRACKING_URL to `%s`\n", spec.TrackingURL))
	sb.WriteString(fmt.Sprintf("   - Set AUTH_HEADER to `%s`\n", spec.AuthHeader))
	sb.WriteString(fmt.Sprintf("   - Set AUTH_PREFIX to `%s`\n", spec.AuthPrefix))
	if spec.Method == "POST" {
		sb.WriteString("   - Change HTTP method to POST and build JSON body per the API docs\n")
	}
	sb.WriteString("4. Call `build_connector` with:\n")
	sb.WriteString("   ```json\n")
	sb.WriteString("   {\n")
	sb.WriteString(fmt.Sprintf("     \"dir\": \"/tmp/%s\",\n", spec.Slug))
	sb.WriteString(fmt.Sprintf("     \"slug\": \"%s\",\n", spec.Slug))
	sb.WriteString(fmt.Sprintf("     \"display_name\": \"%s Tracking\",\n", spec.Name))
	sb.WriteString(fmt.Sprintf("     \"description\": \"Track shipments via %s API\",\n", spec.Name))
	sb.WriteString("     \"tools_schema\": {\n")
	sb.WriteString("       \"track_shipment\": {\n")
	sb.WriteString("         \"description\": \"Track a shipment by tracking number\",\n")
	sb.WriteString("         \"parameters\": {\n")
	sb.WriteString("           \"type\": \"object\",\n")
	sb.WriteString("           \"properties\": {\n")
	sb.WriteString("             \"tracking_number\": {\"type\": \"string\", \"description\": \"The shipment tracking number\"}\n")
	sb.WriteString("           },\n")
	sb.WriteString("           \"required\": [\"tracking_number\"]\n")
	sb.WriteString("         }\n")
	sb.WriteString("       }\n")
	sb.WriteString("     },\n")
	sb.WriteString(fmt.Sprintf("     \"credential_env\": \"%s\"\n", envVar))
	sb.WriteString("   }\n")
	sb.WriteString("   ```\n")
	sb.WriteString(fmt.Sprintf("5. Store the API key: `store_credential(\"%s\", \"<api_key>\")`\n", envVar))
	sb.WriteString(fmt.Sprintf("\n### Source Template\n\n```go\n%s\n```\n", CarrierTemplate))

	return &Result{
		ForLLM:  sb.String(),
		ForUser: fmt.Sprintf("Carrier integration scaffold ready for **%s** — follow the build steps above.", spec.Name),
	}
}

// ListCarriersTool lists all known carrier specs that can be auto-scaffolded.
type ListCarriersTool struct{}

func NewListCarriersTool() *ListCarriersTool { return &ListCarriersTool{} }
func (t *ListCarriersTool) Name() string     { return "list_carriers" }
func (t *ListCarriersTool) Description() string {
	return "List all known shipping carriers with pre-configured API specs for auto-scaffolding. " +
		"Use this to show the user which carriers have ready-made integrations."
}
func (t *ListCarriersTool) Parameters() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{}}
}

func (t *ListCarriersTool) Execute(_ context.Context, _ map[string]any) *Result {
	var sb strings.Builder
	sb.WriteString("## Known Carriers (auto-scaffold ready)\n\n")
	sb.WriteString("| Carrier | Slug | Method | Auth | Docs |\n")
	sb.WriteString("|---------|------|--------|------|------|\n")
	for _, spec := range KnownCarriers {
		docs := "—"
		if spec.DocsURL != "" {
			docs = spec.DocsURL
		}
		sb.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s |\n",
			spec.Name, spec.Slug, spec.Method, spec.AuthHeader, docs))
	}
	sb.WriteString("\nFor any carrier not listed, provide the carrier name + API docs URL and I'll scaffold a custom connector.")
	return TextResult(sb.String())
}
