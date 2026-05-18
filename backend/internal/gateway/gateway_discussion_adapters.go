// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"context"
	"strings"

	"github.com/qorvenai/qorven/internal/providers"
)

// gatewayLabeller implements discussion.LabelGenerator using the default provider.
type gatewayLabeller struct{ gw *Gateway }

func (l *gatewayLabeller) GenerateLabel(ctx context.Context, excerpt string) (string, error) {
	prov := l.gw.providerReg.Default()
	if prov == nil {
		return "", nil
	}
	prompt := "In 4-6 words, name the topic of this conversation. Be specific, use nouns. Examples: 'Election results NTK scraping', 'DB migration planning'. Excerpt:\n\n" + excerpt
	resp, err := prov.Chat(ctx, providers.ChatRequest{
		Messages: []providers.Message{
			{Role: "user", Content: prompt},
		},
		Options: map[string]any{"max_tokens": 30},
	})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(resp.Content), nil
}
