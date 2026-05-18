// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package slack

import (
	"context"
	"fmt"
	"sync"
	"time"

	slackapi "github.com/slack-go/slack"
	"github.com/qorvenai/qorven/internal/channels"
)

// SlackApproval implements native Slack Block Kit buttons for command approval.

type pendingApproval struct {
	requestID string
	chatID    string
	messageTS string
	command   string
	result    *channels.ApprovalResult
	expiresAt time.Time
}

var (
	approvalsMu sync.Mutex
	approvals   = map[string]*pendingApproval{}
)

// SendApprovalRequest sends a Slack message with Block Kit approve/deny buttons.
func (s *SlackChannel) SendApprovalRequest(ctx context.Context, chatID, message string, opts channels.ApprovalOptions) (string, error) {
	requestID := fmt.Sprintf("approve_%d", time.Now().UnixMilli())

	blocks := []slackapi.Block{
		slackapi.NewSectionBlock(
			slackapi.NewTextBlockObject("mrkdwn", fmt.Sprintf("⚠️ *Approval Required*\n\n%s\n\n```%s```", message, opts.Command), false, false),
			nil, nil,
		),
		slackapi.NewContextBlock("", slackapi.NewTextBlockObject("mrkdwn", fmt.Sprintf("Risk: *%s* | Timeout: %ds", opts.Risk, opts.Timeout), false, false)),
		slackapi.NewActionBlock("approval_"+requestID,
			slackapi.NewButtonBlockElement("approve_yes", requestID, slackapi.NewTextBlockObject("plain_text", opts.ApproveText, false, false)).WithStyle(slackapi.StylePrimary),
			slackapi.NewButtonBlockElement("approve_no", requestID, slackapi.NewTextBlockObject("plain_text", opts.DenyText, false, false)).WithStyle(slackapi.StyleDanger),
		),
	}

	_, ts, err := s.api.PostMessage(chatID, slackapi.MsgOptionBlocks(blocks...))
	if err != nil { return "", err }

	approvalsMu.Lock()
	approvals[requestID] = &pendingApproval{
		requestID: requestID, chatID: chatID, messageTS: ts,
		command: opts.Command,
		expiresAt: time.Now().Add(time.Duration(opts.Timeout) * time.Second),
	}
	approvalsMu.Unlock()

	return requestID, nil
}

// GetApprovalResult checks if an approval request has been answered.
func (s *SlackChannel) GetApprovalResult(_ context.Context, requestID string) (*channels.ApprovalResult, error) {
	approvalsMu.Lock()
	defer approvalsMu.Unlock()

	pa, ok := approvals[requestID]
	if !ok { return nil, fmt.Errorf("approval %s not found", requestID) }

	if pa.result == nil && time.Now().After(pa.expiresAt) {
		pa.result = &channels.ApprovalResult{RequestID: requestID, Approved: false, Reason: "timeout"}
	}

	return pa.result, nil
}

// HandleApprovalAction processes a Slack interactive button click.
func HandleApprovalAction(requestID string, approved bool, userName string) {
	approvalsMu.Lock()
	defer approvalsMu.Unlock()

	pa, ok := approvals[requestID]
	if !ok { return }

	pa.result = &channels.ApprovalResult{RequestID: requestID, Approved: approved}
	if approved {
		pa.result.ApprovedBy = userName
	} else {
		pa.result.DeniedBy = userName
		pa.result.Reason = "denied by " + userName
	}
}
