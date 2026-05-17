package inbound

import (
	"testing"

	"github.com/qorvenai/qorven/internal/channels"
)

func TestClassifyHeuristic_Automated_XMailer(t *testing.T) {
	c := &Classifier{}
	got := c.heuristicClassify(channels.InboundMessage{
		Metadata: map[string]string{"x-mailer": "mailchimp"},
	})
	if got != LabelAutomated {
		t.Errorf("expected automated, got %q", got)
	}
}

func TestClassifyHeuristic_Automated_ListUnsubscribe(t *testing.T) {
	c := &Classifier{}
	got := c.heuristicClassify(channels.InboundMessage{
		Metadata: map[string]string{"list-unsubscribe": "<mailto:unsub@example.com>"},
	})
	if got != LabelAutomated {
		t.Errorf("expected automated, got %q", got)
	}
}

func TestClassifyHeuristic_Unknown_Normal(t *testing.T) {
	c := &Classifier{}
	got := c.heuristicClassify(channels.InboundMessage{
		SenderID: "alice@example.com",
		Metadata: map[string]string{},
	})
	if got != LabelUnknown {
		t.Errorf("expected unknown for normal sender, got %q", got)
	}
}

func TestRulesEngine_ContactWins(t *testing.T) {
	rules := []Rule{
		{Priority: 1, MatchType: MatchContact, MatchValue: "buyer@acme.com", Mode: ModeFullyAutonomous},
		{Priority: 999, MatchType: MatchDefault, MatchValue: "*", Mode: ModeContextOnly},
	}
	re := &RulesEngine{rules: rules}
	got := re.Match("buyer@acme.com", "email", "")
	if got != ModeFullyAutonomous {
		t.Errorf("contact rule should win, got %q", got)
	}
}

func TestRulesEngine_DomainMatch(t *testing.T) {
	rules := []Rule{
		{Priority: 1, MatchType: MatchDomain, MatchValue: "@acme.com", Mode: ModeDraftAndApprove},
		{Priority: 999, MatchType: MatchDefault, MatchValue: "*", Mode: ModeContextOnly},
	}
	re := &RulesEngine{rules: rules}
	got := re.Match("other@acme.com", "email", "")
	if got != ModeDraftAndApprove {
		t.Errorf("domain rule should match, got %q", got)
	}
}

func TestRulesEngine_ChannelMatch(t *testing.T) {
	rules := []Rule{
		{Priority: 1, MatchType: MatchChannel, MatchValue: "wechat", Mode: ModeDraftAndApprove},
		{Priority: 999, MatchType: MatchDefault, MatchValue: "*", Mode: ModeContextOnly},
	}
	re := &RulesEngine{rules: rules}
	got := re.Match("anyone@anywhere.com", "wechat", "")
	if got != ModeDraftAndApprove {
		t.Errorf("channel rule should match, got %q", got)
	}
}

func TestRulesEngine_DefaultFallback(t *testing.T) {
	rules := []Rule{
		{Priority: 999, MatchType: MatchDefault, MatchValue: "*", Mode: ModeContextOnly},
	}
	re := &RulesEngine{rules: rules}
	got := re.Match("unknown@stranger.com", "email", "")
	if got != ModeContextOnly {
		t.Errorf("default should apply, got %q", got)
	}
}

func TestRulesEngine_NoMatch(t *testing.T) {
	rules := []Rule{
		{Priority: 1, MatchType: MatchContact, MatchValue: "specific@contact.com", Mode: ModeFullyAutonomous},
	}
	re := &RulesEngine{rules: rules}
	got := re.Match("other@contact.com", "email", "")
	if got != "" {
		t.Errorf("no match should return empty string, got %q", got)
	}
}
