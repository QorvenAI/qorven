// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package social

import "context"

// RelayRouter routes publishing to the correct relay client based on integration config.
type RelayRouter struct {
	relayStore *RelayStore
	publisher  *Publisher
	decrypt    func(string) (string, error)
}

func NewRelayRouter(relayStore *RelayStore, decrypt func(string) (string, error)) *RelayRouter {
	return &RelayRouter{relayStore: relayStore, publisher: NewPublisher(), decrypt: decrypt}
}

// PublishToIntegration publishes content through the integration's configured relay.
func (r *RelayRouter) PublishToIntegration(ctx context.Context, integration *Integration, content string, mediaURLs []string) *PostResult {
	opts := PublishOpts{MediaURLs: mediaURLs}

	switch integration.RelayProvider {
	case "outstand", "postforme", "buffer":
		apiKey, err := r.relayStore.GetKeyByID(ctx, integration.RelayProviderKeyID)
		if err != nil {
			return &PostResult{Platform: integration.Platform, Success: false, Error: "relay key not found: " + err.Error()}
		}
		client := r.newClient(integration.RelayProvider, apiKey)
		if client == nil {
			return &PostResult{Platform: integration.Platform, Success: false, Error: "unknown relay provider: " + integration.RelayProvider}
		}
		result, err := client.Publish(ctx, integration.RelayAccountID, content, opts)
		if err != nil {
			return &PostResult{Platform: integration.Platform, Success: false, Error: err.Error()}
		}
		r.relayStore.IncrementPostCount(ctx, integration.RelayProviderKeyID)
		result.Platform = integration.Platform
		return result

	default: // "direct"
		token := integration.AccessToken
		if r.decrypt != nil {
			if plain, err := r.decrypt(token); err == nil && plain != "" {
				token = plain
			}
		}
		result, err := r.publisher.PublishWithMeta(ctx, integration.Platform, token, content, mediaURLs, integration.RelayMetadata)
		if err != nil {
			return &PostResult{Platform: integration.Platform, Success: false, Error: err.Error()}
		}
		return result
	}
}

func (r *RelayRouter) newClient(provider, apiKey string) RelayPublisher {
	switch provider {
	case "outstand":
		return NewOutstandClient(apiKey)
	case "postforme":
		return NewPostForMeClient(apiKey)
	case "buffer":
		return NewBufferClient(apiKey)
	default:
		return nil
	}
}
