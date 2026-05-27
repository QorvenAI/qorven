// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package social

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qorvenai/qorven/internal/tools"
)

// SocialRelayTool allows agents (e.g. COO) to manage social relay providers
// and connected accounts conversationally. Supports key management, account
// connection/disconnection, per-account rules, and system status overview.
type SocialRelayTool struct {
	relayStore  *RelayStore
	socialStore *Store
	pool        *pgxpool.Pool
}

// NewSocialRelayTool creates a manage_social_relay tool instance.
func NewSocialRelayTool(relayStore *RelayStore, socialStore *Store, pool *pgxpool.Pool) *SocialRelayTool {
	return &SocialRelayTool{
		relayStore:  relayStore,
		socialStore: socialStore,
		pool:        pool,
	}
}

func (t *SocialRelayTool) Name() string { return "manage_social_relay" }

func (t *SocialRelayTool) Description() string {
	return "Manage social relay providers and connected social accounts. Supports adding/removing API keys, connecting/disconnecting accounts, setting per-account content rules, testing keys, and viewing relay system status."
}

func (t *SocialRelayTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"enum":        []string{"list_keys", "add_key", "remove_key", "list_accounts", "connect_account", "disconnect_account", "set_rules", "get_rules", "test_key", "status"},
				"description": "The operation to perform on social relay providers and accounts.",
			},
			"provider": map[string]any{
				"type":        "string",
				"enum":        []string{"outstand", "postforme", "buffer"},
				"description": "Relay provider name (for add_key, connect_account).",
			},
			"key_id": map[string]any{
				"type":        "string",
				"description": "Relay provider key UUID (for remove_key, connect_account, test_key).",
			},
			"api_key": map[string]any{
				"type":        "string",
				"description": "API key value (for add_key).",
			},
			"label": map[string]any{
				"type":        "string",
				"description": "Human-readable label (for add_key).",
			},
			"platform": map[string]any{
				"type":        "string",
				"description": "Social platform (twitter, instagram, linkedin, etc.) for connect_account or list_accounts filter.",
			},
			"integration_id": map[string]any{
				"type":        "string",
				"description": "Integration UUID (for disconnect_account, set_rules, get_rules).",
			},
			"agent_id": map[string]any{
				"type":        "string",
				"description": "Agent UUID (for set_rules, get_rules, connect_account, list_accounts). Defaults to calling agent.",
			},
			"voice_style": map[string]any{
				"type":        "string",
				"description": "Voice/tone for this account (for set_rules).",
			},
			"content_rules": map[string]any{
				"type":        "string",
				"description": "Content rules and restrictions (for set_rules).",
			},
			"knowledge_context": map[string]any{
				"type":        "string",
				"description": "Knowledge context for content generation (for set_rules).",
			},
			"posting_guidelines": map[string]any{
				"type":        "string",
				"description": "Posting schedule and frequency guidelines (for set_rules).",
			},
		},
		"required": []string{"action"},
	}
}

func (t *SocialRelayTool) Execute(ctx context.Context, args map[string]any) *tools.Result {
	action, _ := args["action"].(string)
	if action == "" {
		return tools.ErrorResult("action is required")
	}

	tenantID := tools.TenantIDFromCtx(ctx)
	if tenantID == "" {
		return tools.ErrorResult("tenant context not available")
	}

	switch action {
	case "list_keys":
		return t.listKeys(ctx, tenantID)
	case "add_key":
		return t.addKey(ctx, tenantID, args)
	case "remove_key":
		return t.removeKey(ctx, args)
	case "list_accounts":
		return t.listAccounts(ctx, args)
	case "connect_account":
		return t.connectAccount(ctx, tenantID, args)
	case "disconnect_account":
		return t.disconnectAccount(ctx, args)
	case "set_rules":
		return t.setRules(ctx, tenantID, args)
	case "get_rules":
		return t.getRules(ctx, args)
	case "test_key":
		return t.testKey(ctx, args)
	case "status":
		return t.status(ctx, tenantID)
	default:
		return tools.ErrorResult(fmt.Sprintf("unknown action: %s", action))
	}
}

func (t *SocialRelayTool) listKeys(ctx context.Context, tenantID string) *tools.Result {
	keys, err := t.relayStore.ListKeys(ctx, tenantID)
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("failed to list relay keys: %v", err))
	}

	if len(keys) == 0 {
		return tools.TextResult("No relay provider keys configured. Use add_key to add one (supported providers: outstand, postforme, buffer).")
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Relay Provider Keys (%d total):\n\n", len(keys)))
	for _, k := range keys {
		lastUsed := "never"
		if k.LastUsedAt != nil {
			lastUsed = k.LastUsedAt.Format("2006-01-02 15:04")
		}
		sb.WriteString(fmt.Sprintf("- [%s] %s | Label: %s | Status: %s | Posts: %d | Last used: %s | Accounts: %d\n",
			k.ID, k.Provider, k.Label, k.Status, k.TotalPosts, lastUsed, k.AccountsCount))
	}
	return tools.TextResult(sb.String())
}

func (t *SocialRelayTool) addKey(ctx context.Context, tenantID string, args map[string]any) *tools.Result {
	provider, _ := args["provider"].(string)
	label, _ := args["label"].(string)
	apiKey, _ := args["api_key"].(string)

	if provider == "" {
		return tools.ErrorResult("provider is required for add_key (outstand, postforme, or buffer)")
	}
	if apiKey == "" {
		return tools.ErrorResult("api_key is required for add_key")
	}
	if label == "" {
		label = provider + " key"
	}

	id, err := t.relayStore.AddKey(ctx, tenantID, provider, label, apiKey)
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("failed to add relay key: %v", err))
	}

	return tools.TextResult(fmt.Sprintf("Relay key added successfully.\n  ID: %s\n  Provider: %s\n  Label: %s\n\nYou can now use connect_account with key_id=%s to connect social accounts through this provider.", id, provider, label, id))
}

func (t *SocialRelayTool) removeKey(ctx context.Context, args map[string]any) *tools.Result {
	keyID, _ := args["key_id"].(string)
	if keyID == "" {
		return tools.ErrorResult("key_id is required for remove_key")
	}

	err := t.relayStore.DeleteKey(ctx, keyID)
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("failed to remove relay key: %v", err))
	}

	return tools.TextResult(fmt.Sprintf("Relay key %s removed successfully. Any accounts connected via this key will no longer be able to publish.", keyID))
}

func (t *SocialRelayTool) listAccounts(ctx context.Context, args map[string]any) *tools.Result {
	agentID, _ := args["agent_id"].(string)
	if agentID == "" {
		agentID = tools.AgentIDFromCtx(ctx)
	}
	if agentID == "" {
		return tools.ErrorResult("agent_id is required (or must be called from an agent context)")
	}

	platform, _ := args["platform"].(string)
	provider, _ := args["provider"].(string)

	integrations, err := t.socialStore.ListIntegrations(ctx, agentID)
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("failed to list accounts: %v", err))
	}

	// Apply filters
	var filtered []Integration
	for _, i := range integrations {
		if platform != "" && string(i.Platform) != platform {
			continue
		}
		if provider != "" && i.RelayProvider != provider {
			continue
		}
		filtered = append(filtered, i)
	}

	if len(filtered) == 0 {
		msg := "No connected social accounts found"
		if platform != "" || provider != "" {
			msg += " matching the filter"
		}
		msg += ". Use connect_account to add one."
		return tools.TextResult(msg)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Connected Social Accounts (%d):\n\n", len(filtered)))
	for _, i := range filtered {
		status := "active"
		if !i.Active {
			status = "inactive"
		}
		if i.Paused {
			status = "paused"
		}
		relay := i.RelayProvider
		if relay == "" || relay == "direct" {
			relay = "direct (OAuth)"
		}
		sb.WriteString(fmt.Sprintf("- [%s] %s @%s | Platform: %s | Via: %s | Status: %s\n",
			i.ID, i.AccountName, i.AccountID, string(i.Platform), relay, status))
	}
	return tools.TextResult(sb.String())
}

func (t *SocialRelayTool) connectAccount(ctx context.Context, tenantID string, args map[string]any) *tools.Result {
	keyID, _ := args["key_id"].(string)
	platform, _ := args["platform"].(string)
	provider, _ := args["provider"].(string)

	if platform == "" {
		return tools.ErrorResult("platform is required for connect_account (twitter, instagram, linkedin, etc.)")
	}

	// Determine which key to use
	if keyID == "" && provider == "" {
		return tools.ErrorResult("either key_id or provider is required for connect_account")
	}

	if keyID == "" {
		// Find first active key for the provider
		keys, err := t.relayStore.ListKeys(ctx, tenantID)
		if err != nil {
			return tools.ErrorResult(fmt.Sprintf("failed to find relay keys: %v", err))
		}
		for _, k := range keys {
			if k.Provider == provider && k.Status == "active" {
				keyID = k.ID
				break
			}
		}
		if keyID == "" {
			return tools.ErrorResult(fmt.Sprintf("no active relay key found for provider %s. Use add_key first.", provider))
		}
	}

	// Get the provider and API key for this relay key
	providerName, apiKey, err := t.relayStore.GetKeyWithProvider(ctx, keyID)
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("failed to retrieve relay key: %v", err))
	}

	// Create a relay client and get the auth URL
	client := t.newRelayClient(providerName, apiKey)
	if client == nil {
		return tools.ErrorResult(fmt.Sprintf("unsupported relay provider: %s", providerName))
	}

	redirectURL := fmt.Sprintf("/api/v1/social/relay/callback?key_id=%s&platform=%s", keyID, platform)
	authURL, err := client.GetAuthURL(ctx, platform, tenantID, redirectURL)
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("failed to get authorization URL: %v", err))
	}

	return tools.TextResult(fmt.Sprintf("To connect your %s account via %s, the user needs to visit this authorization URL:\n\n%s\n\nAfter authorization, the account will appear in list_accounts.", platform, providerName, authURL))
}

func (t *SocialRelayTool) disconnectAccount(ctx context.Context, args map[string]any) *tools.Result {
	integrationID, _ := args["integration_id"].(string)
	if integrationID == "" {
		return tools.ErrorResult("integration_id is required for disconnect_account")
	}

	// Get integration details first for confirmation
	integration, err := t.socialStore.GetIntegrationByID(ctx, integrationID)
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("integration not found: %v", err))
	}

	err = t.socialStore.DeleteIntegration(ctx, integrationID)
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("failed to disconnect account: %v", err))
	}

	return tools.TextResult(fmt.Sprintf("Disconnected %s account @%s (%s) successfully. It will no longer receive published posts.",
		string(integration.Platform), integration.AccountID, integration.AccountName))
}

func (t *SocialRelayTool) setRules(ctx context.Context, tenantID string, args map[string]any) *tools.Result {
	integrationID, _ := args["integration_id"].(string)
	if integrationID == "" {
		return tools.ErrorResult("integration_id is required for set_rules")
	}

	agentID, _ := args["agent_id"].(string)
	if agentID == "" {
		agentID = tools.AgentIDFromCtx(ctx)
	}
	if agentID == "" {
		return tools.ErrorResult("agent_id is required for set_rules")
	}

	voiceStyle, _ := args["voice_style"].(string)
	contentRules, _ := args["content_rules"].(string)
	knowledgeContext, _ := args["knowledge_context"].(string)
	postingGuidelines, _ := args["posting_guidelines"].(string)

	// Fetch existing rules to merge (upsert semantics — only overwrite provided fields)
	existing, _ := t.socialStore.GetAccountRules(ctx, integrationID)

	rules := &AccountRules{
		TenantID:      tenantID,
		AgentID:       agentID,
		IntegrationID: integrationID,
	}

	if existing != nil {
		rules.VoiceStyle = existing.VoiceStyle
		rules.ContentRules = existing.ContentRules
		rules.KnowledgeContext = existing.KnowledgeContext
		rules.PostingGuidelines = existing.PostingGuidelines
		rules.HashtagSets = existing.HashtagSets
	}

	// Override only provided fields
	if voiceStyle != "" {
		rules.VoiceStyle = voiceStyle
	}
	if contentRules != "" {
		rules.ContentRules = contentRules
	}
	if knowledgeContext != "" {
		rules.KnowledgeContext = knowledgeContext
	}
	if postingGuidelines != "" {
		rules.PostingGuidelines = postingGuidelines
	}

	err := t.socialStore.UpsertAccountRules(ctx, rules)
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("failed to set rules: %v", err))
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Rules saved for integration %s (agent %s):\n", integrationID, agentID))
	if rules.VoiceStyle != "" {
		sb.WriteString(fmt.Sprintf("  Voice style: %s\n", rules.VoiceStyle))
	}
	if rules.ContentRules != "" {
		sb.WriteString(fmt.Sprintf("  Content rules: %s\n", rules.ContentRules))
	}
	if rules.KnowledgeContext != "" {
		sb.WriteString(fmt.Sprintf("  Knowledge context: %s\n", socialRelayTruncate(rules.KnowledgeContext, 100)))
	}
	if rules.PostingGuidelines != "" {
		sb.WriteString(fmt.Sprintf("  Posting guidelines: %s\n", rules.PostingGuidelines))
	}
	return tools.TextResult(sb.String())
}

func (t *SocialRelayTool) getRules(ctx context.Context, args map[string]any) *tools.Result {
	integrationID, _ := args["integration_id"].(string)
	if integrationID == "" {
		return tools.ErrorResult("integration_id is required for get_rules")
	}

	rules, err := t.socialStore.GetAccountRules(ctx, integrationID)
	if err != nil {
		return tools.TextResult(fmt.Sprintf("No rules configured for integration %s. Use set_rules to configure voice style, content rules, and posting guidelines.", integrationID))
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Account Rules for integration %s:\n\n", integrationID))
	sb.WriteString(fmt.Sprintf("  Agent: %s\n", rules.AgentID))
	sb.WriteString(fmt.Sprintf("  Voice style: %s\n", socialRelayDefault(rules.VoiceStyle, "(not set)")))
	sb.WriteString(fmt.Sprintf("  Content rules: %s\n", socialRelayDefault(rules.ContentRules, "(not set)")))
	sb.WriteString(fmt.Sprintf("  Knowledge context: %s\n", socialRelayDefault(rules.KnowledgeContext, "(not set)")))
	sb.WriteString(fmt.Sprintf("  Posting guidelines: %s\n", socialRelayDefault(rules.PostingGuidelines, "(not set)")))
	if len(rules.HashtagSets) > 0 {
		sb.WriteString("  Hashtag sets:\n")
		for name, tags := range rules.HashtagSets {
			sb.WriteString(fmt.Sprintf("    %s: %s\n", name, strings.Join(tags, ", ")))
		}
	}
	sb.WriteString(fmt.Sprintf("  Last updated: %s\n", rules.UpdatedAt.Format("2006-01-02 15:04")))
	return tools.TextResult(sb.String())
}

func (t *SocialRelayTool) testKey(ctx context.Context, args map[string]any) *tools.Result {
	keyID, _ := args["key_id"].(string)
	if keyID == "" {
		return tools.ErrorResult("key_id is required for test_key")
	}

	providerName, apiKey, err := t.relayStore.GetKeyWithProvider(ctx, keyID)
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("failed to retrieve relay key: %v", err))
	}

	client := t.newRelayClient(providerName, apiKey)
	if client == nil {
		return tools.ErrorResult(fmt.Sprintf("unsupported relay provider: %s", providerName))
	}

	err = client.TestConnection(ctx)
	if err != nil {
		return tools.TextResult(fmt.Sprintf("Key test FAILED for %s key %s:\n  Error: %v\n\nThe API key may be invalid or expired. Check with the provider.", providerName, keyID, err))
	}

	return tools.TextResult(fmt.Sprintf("Key test PASSED for %s key %s. Connection is healthy and ready to publish.", providerName, keyID))
}

func (t *SocialRelayTool) status(ctx context.Context, tenantID string) *tools.Result {
	// Get relay keys summary
	keys, err := t.relayStore.ListKeys(ctx, tenantID)
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("failed to fetch relay status: %v", err))
	}

	// Count keys by provider
	providerCounts := map[string]int{}
	activeKeys := 0
	totalPosts := int64(0)
	totalAccounts := 0
	for _, k := range keys {
		providerCounts[k.Provider]++
		if k.Status == "active" {
			activeKeys++
		}
		totalPosts += k.TotalPosts
		totalAccounts += k.AccountsCount
	}

	// Get tenant-wide integration counts by platform
	platformCounts := map[string]int{}
	if t.pool != nil {
		rows, qErr := t.pool.Query(ctx,
			`SELECT platform, COUNT(*) FROM social_integrations
			 WHERE agent_id IN (SELECT id FROM agents WHERE tenant_id = $1::uuid AND deleted_at IS NULL)
			 GROUP BY platform ORDER BY COUNT(*) DESC`, tenantID)
		if qErr == nil {
			defer rows.Close()
			for rows.Next() {
				var plat string
				var count int
				rows.Scan(&plat, &count)
				platformCounts[plat] = count
			}
		}
	}

	var sb strings.Builder
	sb.WriteString("Social Relay System Status\n")
	sb.WriteString("==========================\n\n")

	sb.WriteString(fmt.Sprintf("Relay Keys: %d total (%d active)\n", len(keys), activeKeys))
	if len(providerCounts) > 0 {
		sb.WriteString("  By provider:\n")
		for prov, count := range providerCounts {
			sb.WriteString(fmt.Sprintf("    %s: %d key(s)\n", prov, count))
		}
	}

	sb.WriteString(fmt.Sprintf("\nConnected Accounts: %d total (via relay keys)\n", totalAccounts))
	if len(platformCounts) > 0 {
		sb.WriteString("  By platform:\n")
		for plat, count := range platformCounts {
			sb.WriteString(fmt.Sprintf("    %s: %d account(s)\n", plat, count))
		}
	}

	sb.WriteString(fmt.Sprintf("\nTotal Posts Published: %d\n", totalPosts))

	if len(keys) == 0 {
		sb.WriteString("\nNo relay providers configured yet. Use add_key to get started.")
	}

	return tools.TextResult(sb.String())
}

// newRelayClient creates a relay client for the given provider and API key.
func (t *SocialRelayTool) newRelayClient(provider, apiKey string) RelayConnector {
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

func socialRelayTruncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

func socialRelayDefault(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}
