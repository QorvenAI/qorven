// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package channels

// RegisterRun associates a run ID with a channel context for event forwarding.
func (m *Manager) RegisterRunContext(runID, channelName, chatID, messageID string, metadata map[string]string, streaming, blockReply, toolStatus bool) {
	m.runs.Store(runID, &RunContext{
		ChannelName:       channelName,
		ChatID:            chatID,
		MessageID:         messageID,
		Metadata:          metadata,
		Streaming:         streaming,
		BlockReplyEnabled: blockReply,
		ToolStatusEnabled: toolStatus,
	})
}

// UnregisterRun removes a run tracking entry.
func (m *Manager) UnregisterRun(runID string) {
	m.runs.Delete(runID)
}

// IsStreamingChannel checks if a named channel implements StreamingChannel
// AND has streaming currently enabled for the given chat type.
func (m *Manager) IsStreamingChannel(channelName string, isGroup bool) bool {
	m.mu.RLock()
	ch := m.channels[channelName]
	m.mu.RUnlock()
	if ch == nil {
		return false
	}
	sc, ok := ch.(StreamingChannel)
	if !ok {
		return false
	}
	return sc.StreamEnabled(isGroup)
}

// BlockReplyChannel is optionally implemented by channels that override block_reply.
type BlockReplyChannel interface {
	BlockReplyEnabled() *bool
}

// ResolveBlockReply checks per-channel override, falls back to gateway default.
func (m *Manager) ResolveBlockReply(channelName string, globalDefault *bool) bool {
	m.mu.RLock()
	ch := m.channels[channelName]
	m.mu.RUnlock()
	if ch != nil {
		if bc, ok := ch.(BlockReplyChannel); ok {
			if v := bc.BlockReplyEnabled(); v != nil {
				return *v
			}
		}
	}
	return globalDefault != nil && *globalDefault
}
