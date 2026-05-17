// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package agent

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// EnvironmentPayload is the structured context injected into every agent call.
// Parsed deterministically BEFORE the LLM sees the message.
type EnvironmentPayload struct {
	SourcePlatform  string   `json:"source_platform"`
	SourceChannelID string   `json:"source_channel_id"`
	SourceUserID    string   `json:"source_user_id"`
	SourceUserName  string   `json:"source_user_name"`
	MentionedAgent  string   `json:"mentioned_agent"`
	DeliveryIntent  string   `json:"delivery_intent"`
	DeliveryChannel string   `json:"delivery_channel"`
	AvailChannels   []string `json:"available_channels"`
	Language        string   `json:"language,omitempty"`
	Location        string   `json:"location,omitempty"`
}

// Channel keyword map — deterministic, no LLM guessing
var channelKeywords = []struct {
	keyword string
	channel string
}{
	{"dm me", "internal_dm"},
	{"message me", "internal_dm"},
	{"send me a dm", "internal_dm"},
	{"privately", "internal_dm"},
	{"in private", "internal_dm"},
	{"directly dm", "internal_dm"},
	{"direct dm", "internal_dm"},
	{"send dm", "internal_dm"},
	{"via dm", "internal_dm"},
	{"to my dm", "internal_dm"},
	{"in dm", "internal_dm"},
	{"send to telegram", "telegram"},
	{"telegram me", "telegram"},
	{"on telegram", "telegram"},
	{"via telegram", "telegram"},
	{"send to whatsapp", "whatsapp"},
	{"whatsapp me", "whatsapp"},
	{"on whatsapp", "whatsapp"},
	{"via whatsapp", "whatsapp"},
	{"email me", "email"},
	{"send email", "email"},
	{"send an email", "email"},
	{"via email", "email"},
	{"on email", "email"},
	{"send to slack", "slack"},
	{"on slack", "slack"},
	{"via slack", "slack"},
	{"reply here", "group_chat"},
	{"post here", "group_chat"},
	{"in this thread", "group_chat"},
	{"in this room", "group_chat"},
}

// DetectDeliveryChannel parses the message text to find delivery intent.
// Returns the channel name or "group_chat" as default.
func DetectDeliveryChannel(message string) string {
	lower := strings.ToLower(message)
	for _, kw := range channelKeywords {
		if strings.Contains(lower, kw.keyword) {
			return kw.channel
		}
	}
	return "group_chat"
}

// BuildEnvironmentSection renders the environment payload as a system prompt section.
func (ep *EnvironmentPayload) BuildSection() string {
	var b strings.Builder

	b.WriteString("## Your Current Environment\n")
	b.WriteString(fmt.Sprintf("- Invoked from: %s", ep.SourcePlatform))
	if ep.SourceChannelID != "" {
		b.WriteString(fmt.Sprintf(" (channel: %s)", ep.SourceChannelID))
	}
	b.WriteString("\n")

	if ep.SourceUserName != "" {
		b.WriteString(fmt.Sprintf("- Mentioned by: %s", ep.SourceUserName))
		if ep.SourceUserID != "" {
			b.WriteString(fmt.Sprintf(" (id: %s)", ep.SourceUserID))
		}
		b.WriteString("\n")
	}

	if ep.DeliveryIntent != "" && ep.DeliveryIntent != "group_chat" {
		b.WriteString(fmt.Sprintf("- User wants delivery via: %s\n", ep.DeliveryIntent))
	}

	b.WriteString(fmt.Sprintf("- Deliver response to: %s\n", ep.DeliveryChannel))

	b.WriteString("\n## Delivery Rules\n")
	switch ep.DeliveryChannel {
	case "internal_dm":
		b.WriteString("- Complete the task, then use `soul_message` to send the result as a DM to the user.\n")
		b.WriteString("- Post a SHORT confirmation in this chat: \"Done! Sent via DM.\"\n")
	case "telegram":
		b.WriteString("- Complete the task, then use the telegram channel tool to send the result.\n")
		b.WriteString("- Post a SHORT confirmation in this chat: \"Done! Sent via Telegram.\"\n")
	case "whatsapp":
		b.WriteString("- Complete the task, then use the whatsapp channel tool to send the result.\n")
		b.WriteString("- Post a SHORT confirmation in this chat: \"Done! Sent via WhatsApp.\"\n")
	case "email":
		b.WriteString("- Complete the task, then use `email_send` to send the result.\n")
		b.WriteString("- Post a SHORT confirmation in this chat: \"Done! Sent via email.\"\n")
	default:
		b.WriteString("- Respond directly in this chat with the full content.\n")
	}

	return b.String()
}

// IsSchedulingRequest detects if a message is asking to set up a recurring task.
func IsSchedulingRequest(message string) bool {
	lower := strings.ToLower(message)
	keywords := []string{"every day", "every morning", "every evening", "daily", "weekly",
		"every hour", "every monday", "every week", "schedule", "remind me",
		"set up a task", "recurring", "at 5pm", "at 9am", "every night"}
	for _, kw := range keywords {
		if strings.Contains(lower, kw) { return true }
	}
	return false
}

// ParseSchedule extracts a cron expression from natural language.
// Returns (expression, task, ok). Simple patterns only — daily at HH:MM.
func ParseSchedule(message string) (expr string, task string, ok bool) {
	lower := strings.ToLower(message)

	// Extract time: "at 5pm", "at 5:30pm", "at 17:00", "at 5:35 pm"
	timePatterns := []struct {
		re   string
		conv func([]string) (int, int)
	}{
		{`at (\d{1,2}):(\d{2})\s*(am|pm)`, func(m []string) (int, int) {
			h, _ := strconv.Atoi(m[1]); min, _ := strconv.Atoi(m[2])
			if m[3] == "pm" && h != 12 { h += 12 }
			if m[3] == "am" && h == 12 { h = 0 }
			return h, min
		}},
		{`at (\d{1,2})\s*(am|pm)`, func(m []string) (int, int) {
			h, _ := strconv.Atoi(m[1])
			if m[2] == "pm" && h != 12 { h += 12 }
			if m[2] == "am" && h == 12 { h = 0 }
			return h, 0
		}},
		{`at (\d{1,2}):(\d{2})`, func(m []string) (int, int) {
			h, _ := strconv.Atoi(m[1]); min, _ := strconv.Atoi(m[2])
			return h, min
		}},
		{`(\d{1,2})\.(\d{2})\s*(am|pm)`, func(m []string) (int, int) {
			h, _ := strconv.Atoi(m[1]); min, _ := strconv.Atoi(m[2])
			if m[3] == "pm" && h != 12 { h += 12 }
			return h, min
		}},
		{`(\d{1,2})\.(\d{2})\s*pm`, func(m []string) (int, int) {
			h, _ := strconv.Atoi(m[1]); min, _ := strconv.Atoi(m[2])
			if h != 12 { h += 12 }
			return h, min
		}},
		{`(\d{1,2})\.(\d{2})(?:\s|$|[^ap])`, func(m []string) (int, int) {
			h, _ := strconv.Atoi(m[1]); min, _ := strconv.Atoi(m[2])
			return h, min
		}},
	}

	hour, minute := -1, -1
	for _, p := range timePatterns {
		re := regexp.MustCompile(p.re)
		if m := re.FindStringSubmatch(lower); m != nil {
			hour, minute = p.conv(m)
			break
		}
	}
	if hour < 0 { return "", "", false }
	if hour > 23 || minute > 59 || minute < 0 { return "", "", false }

	// Build cron expression (daily by default)
	expr = fmt.Sprintf("%d %d * * *", minute, hour)

	// Extract the task: strip scheduling words, keep the action
	task = message
	// Remove common scheduling phrases
	for _, phrase := range []string{
		"every day", "every morning", "every evening", "every night", "daily",
		"at \\d{1,2}[:.:]\\d{2}\\s*(am|pm|ist)?", "at \\d{1,2}\\s*(am|pm|ist)?",
		"\\d{1,2}\\.\\d{2}\\s*(am|pm|ist)?",
		"schedule", "set up", "remind me", "send me", "give me",
	} {
		re := regexp.MustCompile("(?i)" + phrase)
		task = re.ReplaceAllString(task, "")
	}
	task = strings.TrimSpace(task)
	if task == "" { task = message }

	return expr, task, true
}
