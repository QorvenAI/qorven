// Copyright 2026 Tekky AI Academy LLP. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package github_ch

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/qorvenai/qorven/internal/channels"
)

// GitHub channel — webhook for issues, PRs, comments

const (
	ghAPIVersion = "2022-11-28"
	ghUserAgent  = "Qorven.ai/1.0"
)

type Config struct {
	AgentID       string `json:"agent_id"`
	AccessToken   string `json:"access_token"` // PAT or GitHub App installation token
	WebhookSecret string `json:"webhook_secret"`
	Owner         string `json:"owner"`
	Repo          string `json:"repo"`
}

type GitHubChannel struct {
	cfg     Config
	handler channels.InboundHandler
	running bool
	mu      sync.Mutex
	dedup   sync.Map // X-GitHub-Delivery GUID → time.Time
}

func New(cfg Config, handler channels.InboundHandler) *GitHubChannel {
	return &GitHubChannel{cfg: cfg, handler: handler}
}

func (g *GitHubChannel) Name() string    { return fmt.Sprintf("github:%s/%s", g.cfg.Owner, g.cfg.Repo) }
func (g *GitHubChannel) Type() string    { return "github" }
func (g *GitHubChannel) AgentID() string { return g.cfg.AgentID }
func (g *GitHubChannel) IsRunning() bool { g.mu.Lock(); defer g.mu.Unlock(); return g.running }
func (g *GitHubChannel) Start(_ context.Context) error {
	g.mu.Lock(); g.running = true; g.mu.Unlock(); return nil
}
func (g *GitHubChannel) Stop(_ context.Context) error {
	g.mu.Lock(); g.running = false; g.mu.Unlock(); return nil
}

func (g *GitHubChannel) HandleWebhook(rw http.ResponseWriter, r *http.Request) {
	event := r.Header.Get("X-GitHub-Event")
	delivery := r.Header.Get("X-GitHub-Delivery")
	body, _ := io.ReadAll(r.Body)

	// Verify webhook signature (X-Hub-Signature-256, not deprecated SHA-1)
	if g.cfg.WebhookSecret != "" {
		sig := r.Header.Get("X-Hub-Signature-256")
		mac := hmac.New(sha256.New, []byte(g.cfg.WebhookSecret))
		mac.Write(body)
		expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
		if !hmac.Equal([]byte(expected), []byte(sig)) {
			http.Error(rw, "invalid signature", http.StatusForbidden)
			return
		}
	}

	// Dedup using X-GitHub-Delivery GUID (GitHub retries share the same GUID)
	if delivery != "" {
		if _, already := g.dedup.LoadOrStore(delivery, time.Now()); already {
			rw.WriteHeader(http.StatusOK)
			return
		}
		go func() { time.Sleep(2 * time.Hour); g.dedup.Delete(delivery) }()
	}

	var payload struct {
		Action string `json:"action"`
		Sender *struct {
			Login string `json:"login"`
			Type  string `json:"type"` // "User" or "Bot"
		} `json:"sender"`
		Installation *struct {
			ID int64 `json:"id"`
		} `json:"installation"`
		Issue *struct {
			Number int    `json:"number"`
			Title  string `json:"title"`
			Body   string `json:"body"`
			User   struct {
				Login string `json:"login"`
			} `json:"user"`
		} `json:"issue"`
		Comment *struct {
			Body string `json:"body"`
			User struct {
				Login string `json:"login"`
			} `json:"user"`
		} `json:"comment"`
		PullRequest *struct {
			Number int    `json:"number"`
			Title  string `json:"title"`
			Body   string `json:"body"`
			Draft  bool   `json:"draft"`
			User   struct {
				Login string `json:"login"`
			} `json:"user"`
		} `json:"pull_request"`
		Review *struct {
			Body string `json:"body"`
			User struct {
				Login string `json:"login"`
			} `json:"user"`
		} `json:"review"`
	}
	json.Unmarshal(body, &payload)

	// Bot loop prevention: skip events triggered by our own bot comments
	if payload.Sender != nil && payload.Sender.Type == "Bot" {
		rw.WriteHeader(http.StatusOK)
		return
	}

	var content, senderID, chatID string
	switch event {
	case "issues":
		if payload.Issue != nil && (payload.Action == "opened" || payload.Action == "reopened") {
			content = fmt.Sprintf("[Issue #%d %s] %s\n\n%s",
				payload.Issue.Number, payload.Action,
				payload.Issue.Title, payload.Issue.Body)
			senderID = payload.Issue.User.Login
			chatID = strconv.Itoa(payload.Issue.Number)
		}
	case "issue_comment":
		if payload.Comment != nil && payload.Issue != nil && payload.Action == "created" {
			content = fmt.Sprintf("[Comment on issue #%d]\n%s", payload.Issue.Number, payload.Comment.Body)
			senderID = payload.Comment.User.Login
			chatID = strconv.Itoa(payload.Issue.Number)
		}
	case "pull_request":
		if payload.PullRequest != nil && !payload.PullRequest.Draft &&
			(payload.Action == "opened" || payload.Action == "reopened") {
			content = fmt.Sprintf("[PR #%d %s] %s\n\n%s",
				payload.PullRequest.Number, payload.Action,
				payload.PullRequest.Title, payload.PullRequest.Body)
			senderID = payload.PullRequest.User.Login
			chatID = strconv.Itoa(payload.PullRequest.Number)
		}
	case "pull_request_review_comment":
		if payload.Comment != nil && payload.PullRequest != nil && payload.Action == "created" {
			content = fmt.Sprintf("[Review comment on PR #%d]\n%s",
				payload.PullRequest.Number, payload.Comment.Body)
			senderID = payload.Comment.User.Login
			chatID = strconv.Itoa(payload.PullRequest.Number)
		}
	}

	if content == "" || senderID == "" {
		rw.WriteHeader(http.StatusOK)
		return
	}

	meta := map[string]string{
		"event":    event,
		"chat_id":  chatID,
		"delivery": delivery,
	}
	if payload.Installation != nil {
		meta["installation_id"] = strconv.FormatInt(payload.Installation.ID, 10)
	}

	slog.Info("github.inbound", "event", event, "from", senderID, "issue", chatID)
	g.handler(r.Context(), channels.InboundMessage{
		ChannelName: g.Name(),
		ChannelType: "github",
		AgentID:     g.cfg.AgentID,
		SenderID:    senderID,
		SenderName:  senderID,
		ChatID:      chatID,
		Content:     content,
		Metadata:    meta,
	})
	rw.WriteHeader(http.StatusOK)
}

func (g *GitHubChannel) Send(_ context.Context, msg channels.OutboundMessage) error {
	issueNum := msg.RecipientID
	if issueNum == "" {
		issueNum = msg.ChatID
	}
	if issueNum == "" {
		return fmt.Errorf("github send: no issue/PR number in RecipientID or ChatID")
	}

	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/issues/%s/comments",
		g.cfg.Owner, g.cfg.Repo, issueNum)
	payload, _ := json.Marshal(map[string]string{"body": msg.Content})

	req, _ := http.NewRequest("POST", url, bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+g.cfg.AccessToken)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", ghAPIVersion)
	req.Header.Set("User-Agent", ghUserAgent)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("github API %d: %s", resp.StatusCode, string(body))
	}
	return nil
}
