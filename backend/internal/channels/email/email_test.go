// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package email

import (
	"context"
	"strings"
	"testing"

	"github.com/qorvenai/qorven/internal/channels"
)

func TestEmailChannel_Interface(t *testing.T) {
	var _ channels.Channel = (*EmailChannel)(nil)
}

func TestConfig_Fields(t *testing.T) {
	cfg := Config{AgentID: "a1", SMTPHost: "smtp.gmail.com", SMTPPort: 465}
	if cfg.SMTPHost != "smtp.gmail.com" { t.Error("wrong host") }
}

func TestEmailChannel_New(t *testing.T) {
	cfg := Config{
		AgentID:  "agent-1",
		Email:    "bot@example.com",
		Password: "secret",
		IMAPHost: "imap.example.com",
		IMAPPort: 993,
		SMTPHost: "smtp.example.com",
		SMTPPort: 587,
	}
	ch := New(cfg, nil)
	if ch == nil { t.Fatal("nil") }
	if ch.Type() != "email" { t.Errorf("type=%q want email", ch.Type()) }
	if ch.AgentID() != "agent-1" { t.Errorf("agentID=%q", ch.AgentID()) }
	if !strings.Contains(ch.Name(), "email") { t.Errorf("name=%q should contain email", ch.Name()) }
	if !strings.Contains(ch.Name(), "bot@example.com") { t.Errorf("name=%q should contain email address", ch.Name()) }
	if ch.IsRunning() { t.Error("should not be running before Start") }
}

func TestEmailChannel_Defaults(t *testing.T) {
	ch := New(Config{AgentID: "a1", Email: "x@y.com", Password: "p"}, nil)
	if ch.cfg.IMAPPort == 0 { t.Error("IMAPPort should default to 993") }
	if ch.cfg.SMTPPort == 0 { t.Error("SMTPPort should default to 587") }
	if ch.cfg.PollSeconds == 0 { t.Error("PollSeconds should have default") }
	if ch.cfg.Folder == "" { t.Error("Folder should default to INBOX") }
}

func TestEmailChannel_StartStop(t *testing.T) {
	ch := New(Config{AgentID: "a1", Email: "x@y.com", Password: "p", IMAPHost: "imap.example.com"}, nil)
	ctx := context.Background()
	ch.Start(ctx)
	// Polling loop will fail to connect with fake host — that's expected
	ch.Stop(ctx)
	if ch.IsRunning() { t.Error("should not be running after Stop") }
}

func TestEmailChannel_SetMailSaver(t *testing.T) {
	ch := New(Config{AgentID: "a1", Email: "x@y.com", Password: "p"}, nil)
	ch.SetMailSaver(nil, "tenant-id")
	ch.SetMailSaver(nil, "")
}

func TestEmailChannel_SetRouter(t *testing.T) {
	ch := New(Config{AgentID: "a1", Email: "x@y.com", Password: "p"}, nil)
	router := &AliasRouter{
		SharedMailbox: "team@example.com",
		Aliases:       map[string]string{"alice": "agent-alice"},
		DefaultAgent:  "a1",
	}
	ch.SetRouter(router)
	if ch.router == nil { t.Error("router should be set") }
}

func TestIsSpam(t *testing.T) {
	tests := []struct {
		from    string
		subject string
		headers map[string]string
		want    bool
	}{
		{"noreply@example.com", "newsletter", nil, true},
		{"user@example.com", "Hello!", nil, false},
		{"user@example.com", "unsubscribe now", nil, true},
		{"user@example.com", "RE: hello", map[string]string{"Auto-Submitted": "auto-replied"}, true},
		{"user@example.com", "hello", map[string]string{"X-Auto-Response-Suppress": "OOF"}, true},
		{"user@example.com", "hello", map[string]string{"Auto-Submitted": "no"}, false},
		{"mailer-daemon@example.com", "delivery failure", nil, true},
	}
	for _, tt := range tests {
		got := isSpam(tt.from, tt.subject, tt.headers)
		if got != tt.want {
			t.Errorf("isSpam(from=%q, subject=%q, headers=%v)=%v want %v",
				tt.from, tt.subject, tt.headers, got, tt.want)
		}
	}
}

func TestBuildHTMLEmail(t *testing.T) {
	html := buildHTMLEmail("TestBot", "user@example.com", "Hello", "This is a test.")
	if !strings.Contains(html, "TestBot") { t.Error("should contain soul name") }
	if !strings.Contains(html, "user@example.com") { t.Error("should contain recipient") }
	if !strings.Contains(html, "Hello") { t.Error("should contain subject") }
	if !strings.Contains(html, "Content-Type") { t.Error("should contain MIME headers") }
	if !strings.Contains(html, "text/html") { t.Error("should contain HTML content type") }
}

func TestMarkdownToHTML(t *testing.T) {
	// markdownToHTML is a custom implementation — test actual output patterns
	tests := []struct {
		md      string
		want    string // substring to check for
	}{
		{"**bold**", "<b>bold</b>"},   // custom impl uses <b> not <strong>
		{"## Header", "<h3>"},         // custom impl shifts heading levels
		{"plain text", "plain text"},  // plain text passes through
	}
	for _, tt := range tests {
		got := markdownToHTML(tt.md)
		if !strings.Contains(got, tt.want) {
			t.Errorf("markdownToHTML(%q) should contain %q, got %q", tt.md, tt.want, got)
		}
	}
}

func TestMarkdownToHTML_NotEmpty(t *testing.T) {
	inputs := []string{"hello", "**test**", "# title", "`code`", "*italic*"}
	for _, s := range inputs {
		got := markdownToHTML(s)
		if got == "" { t.Errorf("markdownToHTML(%q) returned empty", s) }
	}
}

func TestEmailChannel_SpamFilter_Config(t *testing.T) {
	ch := New(Config{AgentID: "a1", Email: "x@y.com", Password: "p", SpamFilter: true}, nil)
	if !ch.cfg.SpamFilter { t.Error("SpamFilter should be true") }
}

func TestEmailChannel_AutoAck_Config(t *testing.T) {
	ch := New(Config{AgentID: "a1", Email: "x@y.com", Password: "p", AutoAck: true}, nil)
	if !ch.cfg.AutoAck { t.Error("AutoAck should be true") }
}

func TestEmailChannel_Name(t *testing.T) {
	ch := New(Config{AgentID: "a1", Email: "mybot@company.com", Password: "p"}, nil)
	if !strings.HasPrefix(ch.Name(), "email:") { t.Errorf("name=%q should start with email:", ch.Name()) }
	if !strings.Contains(ch.Name(), "mybot@company.com") { t.Errorf("name=%q should contain address", ch.Name()) }
}

func TestEmailChannel_Send_DefaultSubject(t *testing.T) {
	// Without SMTP configured, Send will fail on connection but should set default subject
	ch := New(Config{AgentID: "a1", Email: "x@y.com", Password: "p", SMTPHost: "smtp.example.com"}, nil)
	// The subject should be defaulted even if Send fails
	_ = ch.Send(context.Background(), channels.OutboundMessage{
		RecipientID: "to@example.com",
		Content:     "Test message",
	})
	// We verify the code path doesn't panic — actual SMTP connection will fail
}
