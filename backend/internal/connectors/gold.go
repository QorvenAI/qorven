// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package connectors

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// --- Helper ---

// Shared HTTP client — connection pooling, no per-request overhead
var sharedClient = &http.Client{Timeout: 30 * time.Second}

func apiCall(ctx context.Context, method, url string, headers map[string]string, body any) ([]byte, error) {
	var bodyReader io.Reader
	var bodyBytes []byte
	if body != nil {
		bodyBytes, _ = json.Marshal(body)
		bodyReader = bytes.NewReader(bodyBytes)
	}
	req, _ := http.NewRequestWithContext(ctx, method, url, bodyReader)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := sharedClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Retry once on 429 (rate limited)
	if resp.StatusCode == 429 {
		time.Sleep(2 * time.Second)
		if bodyBytes != nil {
			bodyReader = bytes.NewReader(bodyBytes)
		}
		req2, _ := http.NewRequestWithContext(ctx, method, url, bodyReader)
		for k, v := range headers { req2.Header.Set(k, v) }
		if body != nil { req2.Header.Set("Content-Type", "application/json") }
		resp, err = sharedClient.Do(req2)
		if err != nil { return nil, err }
		defer resp.Body.Close()
	}

	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("API %d: %s", resp.StatusCode, string(data[:min(len(data), 200)]))
	}
	return data, nil
}

func authHeader(creds map[string]string) map[string]string {
	if t := creds["access_token"]; t != "" {
		return map[string]string{"Authorization": "Bearer " + t}
	}
	if t := creds["api_key"]; t != "" {
		return map[string]string{"Authorization": "Bearer " + t}
	}
	return nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ============================================================
// 1. GitHub
// ============================================================

type GitHubConnector struct{}

func (c *GitHubConnector) Manifest() Manifest {
	return Manifest{
		ID: "github", Status: "active", Name: "GitHub", Description: "Issues, PRs, repos, code search",
		Icon: "github", Category: "developer",
		AuthSchema: AuthSchema{Type: "api_key", Fields: []AuthField{
			{Name: "access_token", Label: "Personal Access Token", Type: "password", Required: true},
		}},
		Actions: []Action{
			{ID: "create_issue", Name: "Create Issue", Description: "Create a GitHub issue", Parameters: []ActionParam{
				{Name: "owner", Type: "string", Required: true, Description: "Repo owner"},
				{Name: "repo", Type: "string", Required: true, Description: "Repo name"},
				{Name: "title", Type: "string", Required: true, Description: "Issue title"},
				{Name: "body", Type: "string", Required: false, Description: "Issue body"},
			}},
			{ID: "list_issues", Name: "List Issues", Description: "List repo issues"},
			{ID: "create_pr_comment", Name: "Comment on PR", Description: "Add a comment to a pull request"},
			{ID: "search_code", Name: "Search Code", Description: "Search code across repos"},
		},
	}
}

func (c *GitHubConnector) TestConnection(ctx context.Context, creds map[string]string) error {
	_, err := apiCall(ctx, "GET", "https://api.github.com/user", authHeader(creds), nil)
	return err
}

func (c *GitHubConnector) Execute(ctx context.Context, action string, creds map[string]string, params map[string]any) (any, error) {
	h := authHeader(creds)
	switch action {
	case "create_issue":
		url := fmt.Sprintf("https://api.github.com/repos/%s/%s/issues", params["owner"], params["repo"])
		return apiCall(ctx, "POST", url, h, map[string]any{"title": params["title"], "body": params["body"]})
	case "list_issues":
		url := fmt.Sprintf("https://api.github.com/repos/%s/%s/issues", params["owner"], params["repo"])
		return apiCall(ctx, "GET", url, h, nil)
	default:
		return nil, fmt.Errorf("unknown action: %s", action)
	}
}

// ============================================================
// 2. Gmail
// ============================================================

type GmailConnector struct{}

func (c *GmailConnector) Manifest() Manifest {
	return Manifest{
		ID: "gmail", Status: "active", Name: "Gmail", Description: "Send and read emails via Gmail API",
		Icon: "mail", Category: "workplace",
		AuthSchema: AuthSchema{Type: "oauth2", Fields: []AuthField{
			{Name: "access_token", Label: "OAuth2 Access Token", Type: "password", Required: true},
		}},
		Actions: []Action{
			{ID: "send_email", Name: "Send Email", Description: "Send an email", Parameters: []ActionParam{
				{Name: "to", Type: "string", Required: true, Description: "Recipient email"},
				{Name: "subject", Type: "string", Required: true, Description: "Subject"},
				{Name: "body", Type: "string", Required: true, Description: "Email body"},
			}},
			{ID: "list_messages", Name: "List Messages", Description: "List recent emails"},
			{ID: "search_messages", Name: "Search Messages", Description: "Search emails by query"},
		},
	}
}

func (c *GmailConnector) TestConnection(ctx context.Context, creds map[string]string) error {
	_, err := apiCall(ctx, "GET", "https://gmail.googleapis.com/gmail/v1/users/me/profile", authHeader(creds), nil)
	return err
}

func (c *GmailConnector) Execute(ctx context.Context, action string, creds map[string]string, params map[string]any) (any, error) {
	h := authHeader(creds)
	switch action {
	case "list_messages":
		return apiCall(ctx, "GET", "https://gmail.googleapis.com/gmail/v1/users/me/messages?maxResults=10", h, nil)
	case "send_email":
		// Gmail API requires base64url-encoded RFC 2822 message
		to, _ := params["to"].(string)
		subject, _ := params["subject"].(string)
		body, _ := params["body"].(string)
		raw := fmt.Sprintf("To: %s\r\nSubject: %s\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n%s", to, subject, body)
		encoded := base64.URLEncoding.EncodeToString([]byte(raw))
		return apiCall(ctx, "POST", "https://gmail.googleapis.com/gmail/v1/users/me/messages/send", h, map[string]string{"raw": encoded})
	case "search_messages":
		q, _ := params["query"].(string)
		return apiCall(ctx, "GET", "https://gmail.googleapis.com/gmail/v1/users/me/messages?q="+q+"&maxResults=10", h, nil)
	default:
		return nil, fmt.Errorf("unknown action: %s", action)
	}
}

// ============================================================
// 3. Google Calendar
// ============================================================

type CalendarConnector struct{}

func (c *CalendarConnector) Manifest() Manifest {
	return Manifest{
		ID: "google-calendar", Status: "active", Name: "Google Calendar", Description: "Create, list, update calendar events",
		Icon: "calendar", Category: "workplace",
		AuthSchema: AuthSchema{Type: "oauth2", Fields: []AuthField{
			{Name: "access_token", Label: "OAuth2 Access Token", Type: "password", Required: true},
		}},
		Actions: []Action{
			{ID: "list_events", Name: "List Events", Description: "List upcoming events"},
			{ID: "create_event", Name: "Create Event", Description: "Create a calendar event", Parameters: []ActionParam{
				{Name: "summary", Type: "string", Required: true, Description: "Event title"},
				{Name: "start", Type: "string", Required: true, Description: "Start time (ISO 8601)"},
				{Name: "end", Type: "string", Required: true, Description: "End time (ISO 8601)"},
			}},
		},
	}
}

func (c *CalendarConnector) TestConnection(ctx context.Context, creds map[string]string) error {
	_, err := apiCall(ctx, "GET", "https://www.googleapis.com/calendar/v3/calendars/primary", authHeader(creds), nil)
	return err
}

func (c *CalendarConnector) Execute(ctx context.Context, action string, creds map[string]string, params map[string]any) (any, error) {
	h := authHeader(creds)
	switch action {
	case "list_events":
		return apiCall(ctx, "GET", "https://www.googleapis.com/calendar/v3/calendars/primary/events?maxResults=10&orderBy=startTime&singleEvents=true", h, nil)
	default:
		return nil, fmt.Errorf("unknown action: %s", action)
	}
}

// ============================================================
// 4-12: Remaining connectors (Slack, Telegram, WhatsApp, Notion, Jira, Stripe, Postgres, Google Sheets, Weather)
// ============================================================

type SlackConnector struct{}

func (c *SlackConnector) Manifest() Manifest {
	return Manifest{
		ID: "slack", Status: "active", Name: "Slack", Description: "Send messages, manage channels",
		Icon: "slack", Category: "workplace",
		AuthSchema: AuthSchema{Type: "api_key", Fields: []AuthField{
			{Name: "api_key", Label: "Bot Token", Type: "password", Required: true},
		}},
		Actions: []Action{
			{ID: "send_message", Name: "Send Message", Description: "Post a message to a channel"},
			{ID: "list_channels", Name: "List Channels", Description: "List workspace channels"},
		},
	}
}
func (c *SlackConnector) TestConnection(ctx context.Context, creds map[string]string) error {
	_, err := apiCall(ctx, "GET", "https://slack.com/api/auth.test", authHeader(creds), nil)
	return err
}
func (c *SlackConnector) Execute(ctx context.Context, action string, creds map[string]string, params map[string]any) (any, error) {
	h := authHeader(creds)
	switch action {
	case "send_message":
		return apiCall(ctx, "POST", "https://slack.com/api/chat.postMessage", h, params)
	case "list_channels":
		return apiCall(ctx, "GET", "https://slack.com/api/conversations.list", h, nil)
	default:
		return nil, fmt.Errorf("unknown action: %s", action)
	}
}

type NotionConnector struct{}

func (c *NotionConnector) Manifest() Manifest {
	return Manifest{
		ID: "notion", Status: "beta", Name: "Notion", Description: "Pages, databases, search",
		Icon: "notion", Category: "productivity",
		AuthSchema: AuthSchema{Type: "api_key", Fields: []AuthField{
			{Name: "api_key", Label: "Integration Token", Type: "password", Required: true},
		}},
		Actions: []Action{
			{ID: "search", Name: "Search", Description: "Search Notion pages and databases"},
			{ID: "create_page", Name: "Create Page", Description: "Create a new page"},
			{ID: "query_database", Name: "Query Database", Description: "Query a Notion database"},
		},
	}
}
func (c *NotionConnector) TestConnection(ctx context.Context, creds map[string]string) error {
	h := map[string]string{"Authorization": "Bearer " + creds["api_key"], "Notion-Version": "2022-06-28"}
	_, err := apiCall(ctx, "GET", "https://api.notion.com/v1/users/me", h, nil)
	return err
}
func (c *NotionConnector) Execute(ctx context.Context, action string, creds map[string]string, params map[string]any) (any, error) {
	h := map[string]string{"Authorization": "Bearer " + creds["api_key"], "Notion-Version": "2022-06-28"}
	switch action {
	case "search":
		return apiCall(ctx, "POST", "https://api.notion.com/v1/search", h, params)
	default:
		return nil, fmt.Errorf("unknown action: %s", action)
	}
}

type JiraConnector struct{}

func (c *JiraConnector) Manifest() Manifest {
	return Manifest{
		ID: "jira", Status: "coming_soon", Name: "Jira", Description: "Issues, projects, sprints",
		Icon: "jira", Category: "developer",
		AuthSchema: AuthSchema{Type: "basic", Fields: []AuthField{
			{Name: "domain", Label: "Jira Domain", Type: "string", Required: true, Placeholder: "yourcompany.atlassian.net"},
			{Name: "email", Label: "Email", Type: "string", Required: true},
			{Name: "api_key", Label: "API Token", Type: "password", Required: true},
		}},
		Actions: []Action{
			{ID: "create_issue", Name: "Create Issue", Description: "Create a Jira issue"},
			{ID: "search_issues", Name: "Search Issues", Description: "Search with JQL"},
			{ID: "get_issue", Name: "Get Issue", Description: "Get issue details"},
		},
	}
}
func (c *JiraConnector) TestConnection(ctx context.Context, creds map[string]string) error {
	url := creds["url"]
	if url == "" { return fmt.Errorf("jira url required") }
	req, _ := http.NewRequestWithContext(ctx, "GET", url+"/rest/api/2/myself", nil)
	req.SetBasicAuth(creds["email"], creds["api_token"])
	resp, err := http.DefaultClient.Do(req)
	if err != nil { return fmt.Errorf("jira connection failed: %w", err) }
	defer resp.Body.Close()
	if resp.StatusCode != 200 { return fmt.Errorf("jira auth failed: %d", resp.StatusCode) }
	return nil
}
func (c *JiraConnector) Execute(ctx context.Context, action string, creds map[string]string, params map[string]any) (any, error) {
	return nil, fmt.Errorf("action %s: not yet implemented", action)
}

type StripeConnector struct{}

func (c *StripeConnector) Manifest() Manifest {
	return Manifest{
		ID: "stripe", Status: "coming_soon", Name: "Stripe", Description: "Payments, customers, invoices",
		Icon: "stripe", Category: "commerce",
		AuthSchema: AuthSchema{Type: "api_key", Fields: []AuthField{
			{Name: "api_key", Label: "Secret Key", Type: "password", Required: true},
		}},
		Actions: []Action{
			{ID: "list_customers", Name: "List Customers", Description: "List Stripe customers"},
			{ID: "create_invoice", Name: "Create Invoice", Description: "Create a new invoice"},
			{ID: "list_payments", Name: "List Payments", Description: "List recent payments"},
		},
	}
}
func (c *StripeConnector) TestConnection(ctx context.Context, creds map[string]string) error {
	h := map[string]string{"Authorization": "Bearer " + creds["api_key"]}
	_, err := apiCall(ctx, "GET", "https://api.stripe.com/v1/balance", h, nil)
	return err
}
func (c *StripeConnector) Execute(ctx context.Context, action string, creds map[string]string, params map[string]any) (any, error) {
	h := map[string]string{"Authorization": "Bearer " + creds["api_key"]}
	switch action {
	case "list_customers":
		return apiCall(ctx, "GET", "https://api.stripe.com/v1/customers?limit=10", h, nil)
	default:
		return nil, fmt.Errorf("unknown action: %s", action)
	}
}

type GoogleSheetsConnector struct{}

func (c *GoogleSheetsConnector) Manifest() Manifest {
	return Manifest{
		ID: "google-sheets", Status: "coming_soon", Name: "Google Sheets", Description: "Read and write spreadsheet data",
		Icon: "sheets", Category: "productivity",
		AuthSchema: AuthSchema{Type: "oauth2", Fields: []AuthField{
			{Name: "access_token", Label: "OAuth2 Access Token", Type: "password", Required: true},
		}},
		Actions: []Action{
			{ID: "read_range", Name: "Read Range", Description: "Read cells from a range"},
			{ID: "append_row", Name: "Append Row", Description: "Append a row to a sheet"},
			{ID: "update_range", Name: "Update Range", Description: "Update cells in a range"},
		},
	}
}
func (c *GoogleSheetsConnector) TestConnection(ctx context.Context, creds map[string]string) error {
	token := creds["access_token"]
	if token == "" { return fmt.Errorf("google access_token required") }
	req, _ := http.NewRequestWithContext(ctx, "GET", "https://www.googleapis.com/oauth2/v1/tokeninfo?access_token="+token, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil { return fmt.Errorf("google auth failed: %w", err) }
	defer resp.Body.Close()
	if resp.StatusCode != 200 { return fmt.Errorf("google token invalid: %d", resp.StatusCode) }
	return nil
}
func (c *GoogleSheetsConnector) Execute(ctx context.Context, action string, creds map[string]string, params map[string]any) (any, error) {
	return nil, fmt.Errorf("action %s: not yet implemented", action)
}

type WeatherConnector struct{}

func (c *WeatherConnector) Manifest() Manifest {
	return Manifest{
		ID: "weather", Status: "active", Name: "Weather", Description: "Current weather and forecasts via OpenWeatherMap",
		Icon: "cloud-sun", Category: "data",
		AuthSchema: AuthSchema{Type: "api_key", Fields: []AuthField{
			{Name: "api_key", Label: "OpenWeatherMap API Key", Type: "password", Required: true},
		}},
		Actions: []Action{
			{ID: "current", Name: "Current Weather", Description: "Get current weather for a city", Parameters: []ActionParam{
				{Name: "city", Type: "string", Required: true, Description: "City name"},
			}},
			{ID: "forecast", Name: "5-Day Forecast", Description: "Get 5-day forecast"},
		},
	}
}
func (c *WeatherConnector) TestConnection(ctx context.Context, creds map[string]string) error {
	url := "https://api.openweathermap.org/data/2.5/weather?q=London&appid=" + creds["api_key"]
	_, err := apiCall(ctx, "GET", url, nil, nil)
	return err
}
func (c *WeatherConnector) Execute(ctx context.Context, action string, creds map[string]string, params map[string]any) (any, error) {
	switch action {
	case "current":
		url := fmt.Sprintf("https://api.openweathermap.org/data/2.5/weather?q=%s&appid=%s&units=metric", params["city"], creds["api_key"])
		return apiCall(ctx, "GET", url, nil, nil)
	case "forecast":
		url := fmt.Sprintf("https://api.openweathermap.org/data/2.5/forecast?q=%s&appid=%s&units=metric", params["city"], creds["api_key"])
		return apiCall(ctx, "GET", url, nil, nil)
	default:
		return nil, fmt.Errorf("unknown action: %s", action)
	}
}

// RegisterAll registers all 12 gold connectors.
func RegisterAll(reg *Registry) {
	reg.Register(&GitHubConnector{})
	reg.Register(&GmailConnector{})
	reg.Register(&CalendarConnector{})
	reg.Register(&SlackConnector{})
	reg.Register(&NotionConnector{})
	reg.Register(&JiraConnector{})
	reg.Register(&StripeConnector{})
	reg.Register(&GoogleSheetsConnector{})
	reg.Register(&WeatherConnector{})
	// Telegram, WhatsApp, Postgres are handled via channels/tools already
}
