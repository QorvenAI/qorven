package inbound

import (
	"context"
	"strings"

	"github.com/qorvenai/qorven/internal/channels"
)

// ClassifyLabel is the result of classification.
type ClassifyLabel string

const (
	LabelRealPerson ClassifyLabel = "real_person"
	LabelAutomated  ClassifyLabel = "automated"
	LabelSpam       ClassifyLabel = "spam"
	LabelUnknown    ClassifyLabel = "unknown"
)

// Classifier performs intent classification on inbound messages.
type Classifier struct{}

// Classify returns a label. Fast heuristics run first; LLM only for unknowns.
func (c *Classifier) Classify(ctx context.Context, msg channels.InboundMessage) ClassifyLabel {
	label := c.heuristicClassify(msg)
	if label != LabelUnknown {
		return label
	}
	// LLM fallback is a future improvement — treat unknown as real_person
	return LabelRealPerson
}

func (c *Classifier) heuristicClassify(msg channels.InboundMessage) ClassifyLabel {
	sender := strings.ToLower(msg.SenderID)

	for _, pat := range []string{"noreply@spam", "bounce@", "mailer-daemon@", "postmaster@"} {
		if strings.Contains(sender, pat) {
			return LabelSpam
		}
	}

	meta := msg.Metadata
	if meta != nil {
		xMailer := strings.ToLower(meta["x-mailer"])
		if strings.Contains(xMailer, "mailchimp") || strings.Contains(xMailer, "sendgrid") ||
			strings.Contains(xMailer, "mailgun") || strings.Contains(xMailer, "marketo") {
			return LabelAutomated
		}
		if meta["list-unsubscribe"] != "" || meta["list-id"] != "" {
			return LabelAutomated
		}
	}

	return LabelUnknown
}
