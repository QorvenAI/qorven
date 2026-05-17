// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package coworker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// integrations.go — Gmail + Calendar integration + system instructions.

// ── Gmail Integration ──

type GmailClient struct {
	accessToken string
	client      *http.Client
}

func NewGmailClient(accessToken string) *GmailClient {
	return &GmailClient{accessToken: accessToken, client: &http.Client{Timeout: 15 * time.Second}}
}

func (g *GmailClient) ListMessages(ctx context.Context, query string, maxResults int) ([]EmailMessage, error) {
	if maxResults <= 0 { maxResults = 10 }
	u := fmt.Sprintf("https://gmail.googleapis.com/gmail/v1/users/me/messages?q=%s&maxResults=%d",
		url.QueryEscape(query), maxResults)
	data, err := g.get(ctx, u)
	if err != nil { return nil, err }

	var resp struct {
		Messages []struct{ ID, ThreadID string } `json:"messages"`
	}
	json.Unmarshal(data, &resp)

	var emails []EmailMessage
	for _, m := range resp.Messages {
		email, err := g.GetMessage(ctx, m.ID)
		if err != nil { continue }
		emails = append(emails, *email)
	}
	return emails, nil
}

func (g *GmailClient) GetMessage(ctx context.Context, messageID string) (*EmailMessage, error) {
	u := fmt.Sprintf("https://gmail.googleapis.com/gmail/v1/users/me/messages/%s?format=full", messageID)
	data, err := g.get(ctx, u)
	if err != nil { return nil, err }

	var resp struct {
		ID      string `json:"id"`
		Snippet string `json:"snippet"`
		Payload struct {
			Headers []struct{ Name, Value string } `json:"headers"`
			Body    struct{ Data string }           `json:"body"`
			Parts   []struct {
				MimeType string `json:"mimeType"`
				Body     struct{ Data string } `json:"body"`
			} `json:"parts"`
		} `json:"payload"`
		InternalDate string `json:"internalDate"`
	}
	json.Unmarshal(data, &resp)

	email := &EmailMessage{ID: resp.ID, Snippet: resp.Snippet}
	for _, h := range resp.Payload.Headers {
		switch h.Name {
		case "From": email.From = h.Value
		case "To": email.To = h.Value
		case "Subject": email.Subject = h.Value
		case "Date": email.Date = h.Value
		}
	}

	// Extract body
	if resp.Payload.Body.Data != "" {
		email.Body = resp.Payload.Body.Data
	}
	for _, part := range resp.Payload.Parts {
		if part.MimeType == "text/plain" && part.Body.Data != "" {
			email.Body = part.Body.Data
		}
	}

	return email, nil
}

func (g *GmailClient) SendDraft(ctx context.Context, to, subject, body string) error {
	raw := fmt.Sprintf("To: %s\r\nSubject: %s\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n%s", to, subject, body)
	payload := fmt.Sprintf(`{"raw":"%s"}`, raw) // should be base64 encoded
	u := "https://gmail.googleapis.com/gmail/v1/users/me/messages/send"

	req, _ := http.NewRequestWithContext(ctx, "POST", u, strings.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+g.accessToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := g.client.Do(req)
	if err != nil { return err }
	resp.Body.Close()
	if resp.StatusCode >= 400 { return fmt.Errorf("gmail send: HTTP %d", resp.StatusCode) }
	return nil
}

func (g *GmailClient) get(ctx context.Context, u string) ([]byte, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", u, nil)
	req.Header.Set("Authorization", "Bearer "+g.accessToken)
	resp, err := g.client.Do(req)
	if err != nil { return nil, err }
	defer resp.Body.Close()
	if resp.StatusCode >= 400 { return nil, fmt.Errorf("gmail: HTTP %d", resp.StatusCode) }
	return io.ReadAll(io.LimitReader(resp.Body, 2<<20))
}

type EmailMessage struct {
	ID      string `json:"id"`
	From    string `json:"from"`
	To      string `json:"to"`
	Subject string `json:"subject"`
	Date    string `json:"date"`
	Body    string `json:"body"`
	Snippet string `json:"snippet"`
}

// ── Google Calendar Integration ──

type CalendarClient struct {
	accessToken string
	client      *http.Client
}

func NewCalendarClient(accessToken string) *CalendarClient {
	return &CalendarClient{accessToken: accessToken, client: &http.Client{Timeout: 15 * time.Second}}
}

func (c *CalendarClient) ListUpcoming(ctx context.Context, maxResults int) ([]CalendarEvent, error) {
	if maxResults <= 0 { maxResults = 10 }
	now := time.Now().Format(time.RFC3339)
	u := fmt.Sprintf("https://www.googleapis.com/calendar/v3/calendars/primary/events?timeMin=%s&maxResults=%d&singleEvents=true&orderBy=startTime",
		url.QueryEscape(now), maxResults)

	req, _ := http.NewRequestWithContext(ctx, "GET", u, nil)
	req.Header.Set("Authorization", "Bearer "+c.accessToken)
	resp, err := c.client.Do(req)
	if err != nil { return nil, err }
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)

	var calResp struct {
		Items []struct {
			ID          string `json:"id"`
			Summary     string `json:"summary"`
			Description string `json:"description"`
			Location    string `json:"location"`
			Start       struct{ DateTime, Date string } `json:"start"`
			End         struct{ DateTime, Date string } `json:"end"`
			Attendees   []struct{ Email, DisplayName string } `json:"attendees"`
			Organizer   struct{ Email, DisplayName string } `json:"organizer"`
		} `json:"items"`
	}
	json.Unmarshal(data, &calResp)

	var events []CalendarEvent
	for _, item := range calResp.Items {
		startStr := item.Start.DateTime
		if startStr == "" { startStr = item.Start.Date }
		start, _ := time.Parse(time.RFC3339, startStr)

		var attendees []string
		for _, a := range item.Attendees {
			name := a.DisplayName
			if name == "" { name = a.Email }
			attendees = append(attendees, name)
		}

		events = append(events, CalendarEvent{
			ID: item.ID, Title: item.Summary, Description: item.Description,
			Location: item.Location, Start: start,
			Attendees: attendees, Organizer: item.Organizer.DisplayName,
		})
	}
	return events, nil
}

type CalendarEvent struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Location    string    `json:"location"`
	Start       time.Time `json:"start"`
	Attendees   []string  `json:"attendees"`
	Organizer   string    `json:"organizer"`
}

// ── System Instructions ──

// BuildInstructions generates the system prompt for the coworker agent.
func BuildInstructions(agent *Agent) string {
	var b strings.Builder
	b.WriteString("You are a personal AI coworker with persistent memory.\n\n")
	b.WriteString("## Capabilities\n")
	b.WriteString("- Remember important context (people, projects, decisions, commitments)\n")
	b.WriteString("- Search your memory vault for relevant notes\n")
	b.WriteString("- Create and update notes with [[backlinks]]\n")
	b.WriteString("- Prepare for meetings using prior context\n")
	b.WriteString("- Draft emails grounded in history\n")
	b.WriteString("- Track topics with live auto-updating notes\n\n")

	b.WriteString("## Memory Vault\n")
	notes := agent.vault.ListRecent(10)
	if len(notes) > 0 {
		b.WriteString(fmt.Sprintf("You have %d notes in your vault. Recent:\n", len(agent.vault.Notes)))
		for _, n := range notes {
			b.WriteString(fmt.Sprintf("- [[%s]]: %s\n", n.ID, truncStr(n.Title, 60)))
		}
	} else {
		b.WriteString("Your vault is empty. Start remembering things!\n")
	}

	b.WriteString("\n## Integrations\n")
	for name, integ := range agent.integrations {
		status := "inactive"
		if integ.Active { status = "active" }
		b.WriteString(fmt.Sprintf("- %s: %s\n", name, status))
	}

	b.WriteString("\n## Rules\n")
	b.WriteString("- Always use [[backlinks]] when referencing notes\n")
	b.WriteString("- Save important information as notes proactively\n")
	b.WriteString("- Before meetings, search vault for relevant context\n")
	b.WriteString("- When drafting emails, ground in vault context\n")
	b.WriteString("- All notes are plain Markdown, Obsidian-compatible\n")

	return b.String()
}

func truncStr(s string, n int) string { if len(s) <= n { return s }; return s[:n] + "..." }
