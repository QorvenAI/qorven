// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	discordch "github.com/qorvenai/qorven/internal/channels/discord"
	dingtalkhch "github.com/qorvenai/qorven/internal/channels/dingtalk"
	emailch "github.com/qorvenai/qorven/internal/channels/email"
	facebookch "github.com/qorvenai/qorven/internal/channels/facebook"
	feishuch "github.com/qorvenai/qorven/internal/channels/feishu"
	githubch "github.com/qorvenai/qorven/internal/channels/github_ch"
	imessagech "github.com/qorvenai/qorven/internal/channels/imessage"
	linech "github.com/qorvenai/qorven/internal/channels/line"
	mattermostch "github.com/qorvenai/qorven/internal/channels/mattermost"
	matrixch "github.com/qorvenai/qorven/internal/channels/matrix"
	signalch "github.com/qorvenai/qorven/internal/channels/signal"
	slackch "github.com/qorvenai/qorven/internal/channels/slack"
	smsch "github.com/qorvenai/qorven/internal/channels/sms"
	teamsch "github.com/qorvenai/qorven/internal/channels/teams"
	telegramch "github.com/qorvenai/qorven/internal/channels/telegram"
	webchat "github.com/qorvenai/qorven/internal/channels/webchat"
	wecomch "github.com/qorvenai/qorven/internal/channels/wecom"
	webhookch "github.com/qorvenai/qorven/internal/channels/webhook"
	whatsappch "github.com/qorvenai/qorven/internal/channels/whatsapp"
	"github.com/qorvenai/qorven/internal/channels/zalo"
)

func (gw *Gateway) loadChannels() {
	rows, err := gw.db.Pool.Query(context.Background(),
		`SELECT id, agent_id, channel_type, config FROM channel_instances WHERE tenant_id = $1 AND enabled = true`, defaultTenant)
	if err != nil {
		slog.Warn("loadChannels: query failed", "error", err)
		return
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var id, chType string
		var agentID *string
		var configJSON []byte
		if err := rows.Scan(&id, &agentID, &chType, &configJSON); err != nil {
			slog.Warn("loadChannels: scan failed", "error", err)
			continue
		}
		if agentID == nil {
			continue
		}

		var cfg map[string]any
		if err := json.Unmarshal(configJSON, &cfg); err != nil {
			slog.Warn("loadChannels: invalid config JSON", "channel", id, "error", err)
			continue
		}

		switch chType {
		case "email":
			emailCfg := emailch.Config{
				AgentID:     *agentID,
				Email:       strVal(cfg, "email"),
				Password:    strVal(cfg, "password"),
				IMAPHost:    strVal(cfg, "imap_host"),
				IMAPPort:    intVal(cfg, "imap_port"),
				SMTPHost:    strVal(cfg, "smtp_host"),
				SMTPPort:    intVal(cfg, "smtp_port"),
				PollSeconds: intVal(cfg, "poll_seconds"),
			}
			if emailCfg.Email != "" && emailCfg.Password != "" {
				ch := emailch.New(emailCfg, gw.chanMgr.Handler())
				// Wire mail saver for GUI visibility
				if gw.mailStore != nil {
					ch.SetMailSaver(&mailSaverAdapter{store: gw.mailStore}, defaultTenant)
					// Wire thread loader — enables Outlook-style verified thread history
					// Agent reads prior conversation from its own DB records, not email body
					ch.SetThreadLoader(gw.mailStore)
				}
				// Wire alias routing for shared mailbox
				if sharedMailbox := strVal(cfg, "shared_mailbox"); sharedMailbox != "" {
					aliases := make(map[string]string)
					if aliasJSON := strVal(cfg, "aliases"); aliasJSON != "" {
						json.Unmarshal([]byte(aliasJSON), &aliases)
					}
					ch.SetRouter(&emailch.AliasRouter{SharedMailbox: sharedMailbox, Aliases: aliases, DefaultAgent: *agentID})
				}
				gw.chanMgr.Register(id, ch)
				count++
			}
		case "telegram":
			tgCfg := telegramch.Config{
				AgentID:        *agentID,
				BotToken:       strVal(cfg, "bot_token"),
				BotName:        strVal(cfg, "bot_name"),
				GroupPolicy:    strVal(cfg, "group_policy"),
				RequireMention: cfg["require_mention"] == true || strVal(cfg, "require_mention") == "true",
			}
			if tgCfg.BotToken != "" {
				ch := telegramch.New(tgCfg, gw.chanMgr.Handler())
				if gw.voicePipeline != nil && gw.voicePipeline.CanTranscribe() {
					ch.Transcribe = gw.voicePipeline.TranscribeAudio
				}
				gw.chanMgr.Register(id, ch)
				count++
			}
		case "discord":
			dcCfg := discordch.Config{
				AgentID:  *agentID,
				BotToken: strVal(cfg, "bot_token"),
			}
			if dcCfg.BotToken != "" {
				ch := discordch.New(dcCfg, gw.chanMgr.Handler())
				gw.chanMgr.Register(id, ch)
				count++
			}
		case "slack":
			slCfg := slackch.Config{
				AgentID:  *agentID,
				BotToken: strVal(cfg, "bot_token"),
				AppToken: strVal(cfg, "app_token"),
			}
			if slCfg.BotToken != "" && slCfg.AppToken != "" {
				ch := slackch.New(slCfg, gw.chanMgr.Handler())
				gw.chanMgr.Register(id, ch)
				count++
			}
		case "whatsapp":
			waCfg := whatsappch.Config{
				AgentID:       *agentID,
				PhoneNumberID: strVal(cfg, "phone_number_id"),
				AccessToken:   strVal(cfg, "access_token"),
				VerifyToken:   strVal(cfg, "verify_token"),
			}
			if waCfg.PhoneNumberID != "" && waCfg.AccessToken != "" {
				ch := whatsappch.New(waCfg, gw.chanMgr.Handler())
				if gw.voicePipeline != nil && gw.voicePipeline.CanTranscribe() {
					ch.Transcribe = gw.voicePipeline.TranscribeAudio
				}
				gw.chanMgr.Register(id, ch)
				count++
			}
		case "zalo":
			zaloCfg := zalo.ZaloConfig{
				AgentID:      *agentID,
				AppID:        strVal(cfg, "app_id"),
				AppSecret:    strVal(cfg, "app_secret"),
				RefreshToken: strVal(cfg, "refresh_token"),
				AccessToken:  strVal(cfg, "access_token"),
				PersonalMode: cfg["personal_mode"] == "true",
				SecretKey:    strVal(cfg, "secret_key"),
				IMEI:         strVal(cfg, "imei"),
			}
			if zaloCfg.AppID != "" || zaloCfg.AccessToken != "" || zaloCfg.SecretKey != "" {
				ch := zalo.New(zaloCfg, gw.chanMgr.Handler())
				webhookPath := fmt.Sprintf("/v1/webhooks/zalo/%s", id)
				gw.router.Post(webhookPath, ch.HandleWebhook)
				gw.chanMgr.Register(id, ch)
				count++
			}
		case "sms":
			smsCfg := smsch.Config{
				AgentID:             *agentID,
				AccountSID:          strVal(cfg, "account_sid"),
				AuthToken:           strVal(cfg, "auth_token"),
				ApiKeySid:           strVal(cfg, "api_key_sid"),
				ApiKeySecret:        strVal(cfg, "api_key_secret"),
				FromNumber:          strVal(cfg, "from_number"),
				MessagingServiceSid: strVal(cfg, "messaging_service_sid"),
			}
			if smsCfg.AccountSID != "" {
				ch := smsch.New(smsCfg, gw.chanMgr.Handler())
				webhookPath := fmt.Sprintf("/v1/webhooks/sms/%s", id)
				statusPath := fmt.Sprintf("/v1/webhooks/sms/%s/status", id)
				gw.router.Post(webhookPath, ch.HandleWebhook)
				gw.router.Post(statusPath, ch.HandleStatusWebhook)
				gw.chanMgr.Register(id, ch)
				count++
			}
		case "teams":
			tmCfg := teamsch.Config{
				AgentID:   *agentID,
				AppID:     strVal(cfg, "app_id"),
				AppSecret: strVal(cfg, "app_secret"),
			}
			if tmCfg.AppID != "" && tmCfg.AppSecret != "" {
				ch := teamsch.New(tmCfg, gw.chanMgr.Handler())
				gw.chanMgr.Register(id, ch)
				count++
			}
		case "github":
			ghCfg := githubch.Config{
				AgentID:       *agentID,
				AccessToken:   strVal(cfg, "access_token"),
				WebhookSecret: strVal(cfg, "webhook_secret"),
				Owner:         strVal(cfg, "owner"),
				Repo:          strVal(cfg, "repo"),
			}
			if ghCfg.AccessToken != "" {
				ch := githubch.New(ghCfg, gw.chanMgr.Handler())
				gw.chanMgr.Register(id, ch)
				// Register webhook route so GitHub can POST events
				webhookPath := fmt.Sprintf("/v1/webhooks/github/%s", id)
				gw.router.Post(webhookPath, ch.HandleWebhook)
				slog.Info("github.webhook_route", "path", webhookPath)
				count++
			}
		case "webchat":
			wcCfg := webchat.Config{
				AgentID: *agentID,
			}
			ch := webchat.New(wcCfg, gw.chanMgr.Handler())
			webchat.ValidateToken = func(token string) bool {
				if gw.authSvc == nil {
					return true
				}
				_, err := gw.authSvc.ValidateToken(token)
				return err == nil
			}
			gw.chanMgr.Register(id, ch)
			count++
		case "webhook":
			whCfg := webhookch.Config{
				AgentID:     *agentID,
				InboundPath: strVal(cfg, "inbound_path"),
				OutboundURL: strVal(cfg, "outbound_url"),
			}
			if whCfg.OutboundURL != "" {
				ch := webhookch.New(whCfg, gw.chanMgr.Handler())
				gw.chanMgr.Register(id, ch)
				count++
			}

		// --- Channels previously implemented but not registered ---

		case "line":
			lineCfg := linech.Config{
				AgentID:       *agentID,
				ChannelSecret: strVal(cfg, "channel_secret"),
				ChannelToken:  strVal(cfg, "channel_token"),
				WebhookPath:   strVal(cfg, "webhook_path"),
			}
			if lineCfg.ChannelSecret != "" && lineCfg.ChannelToken != "" {
				ch := linech.New(lineCfg, gw.chanMgr.Handler())
				gw.chanMgr.Register(id, ch)
				// Register webhook route
				path := lineCfg.WebhookPath
				if path == "" {
					path = fmt.Sprintf("/v1/webhooks/line/%s", id)
				}
				gw.router.Post(path, ch.HandleWebhook)
				count++
			}

		case "feishu", "lark":
			feishuCfg := feishuch.Config{
				AgentID:     *agentID,
				AppID:       strVal(cfg, "app_id"),
				AppSecret:   strVal(cfg, "app_secret"),
				BotName:     strVal(cfg, "bot_name"),
				IsLark:      cfg["is_lark"] == true || strVal(cfg, "is_lark") == "true",
				EncryptKey:  strVal(cfg, "encrypt_key"),
				VerifyToken: strVal(cfg, "verify_token"),
			}
			if feishuCfg.AppID != "" && feishuCfg.AppSecret != "" {
				ch := feishuch.New(feishuCfg, gw.chanMgr.Handler())
				gw.chanMgr.Register(id, ch)
				count++
			}

		case "dingtalk":
			dtCfg := dingtalkhch.Config{
				AgentID:       *agentID,
				AppKey:        strVal(cfg, "app_key"),
				AppSecret:     strVal(cfg, "app_secret"),
				RobotCode:     strVal(cfg, "robot_code"),
				WebhookURL:    strVal(cfg, "webhook_url"),
				WebhookSecret: strVal(cfg, "webhook_secret"),
			}
			if dtCfg.AppKey != "" && dtCfg.AppSecret != "" {
				ch := dingtalkhch.New(dtCfg, gw.chanMgr.Handler())
				gw.chanMgr.Register(id, ch)
				count++
			}

		case "wecom":
			wcCfg := wecomch.Config{
				AgentID:      *agentID,
				CorpID:       strVal(cfg, "corp_id"),
				AgentSecret:  strVal(cfg, "agent_secret"),
				WecomAgentID: intVal(cfg, "wecom_agent_id"),
				Token:        strVal(cfg, "token"),
				EncodingKey:  strVal(cfg, "encoding_key"),
			}
			if wcCfg.CorpID != "" && wcCfg.AgentSecret != "" {
				if ch, err := wecomch.New(wcCfg, gw.chanMgr.Handler()); err == nil {
					gw.chanMgr.Register(id, ch)
					count++
				} else {
					slog.Warn("wecom.init.failed", "error", err)
				}
			}

		case "mattermost":
			mmCfg := mattermostch.Config{
				AgentID:        *agentID,
				ServerURL:      strVal(cfg, "server_url"),
				BotToken:       strVal(cfg, "bot_token"),
				TeamID:         strVal(cfg, "team_id"),
				RequireMention: cfg["require_mention"] == true || strVal(cfg, "require_mention") == "true",
			}
			if mmCfg.ServerURL != "" && mmCfg.BotToken != "" {
				ch := mattermostch.New(mmCfg, gw.chanMgr.Handler())
				gw.chanMgr.Register(id, ch)
				count++
			}

		case "signal":
			sigCfg := signalch.Config{
				AgentID:      *agentID,
				APIURL:       strVal(cfg, "api_url"),
				PhoneNumber:  strVal(cfg, "phone_number"),
				UseWebSocket: cfg["use_websocket"] == true || strVal(cfg, "use_websocket") == "true",
			}
			if sigCfg.APIURL != "" && sigCfg.PhoneNumber != "" {
				ch := signalch.New(sigCfg, gw.chanMgr.Handler())
				gw.chanMgr.Register(id, ch)
				count++
			}

		case "imessage":
			imCfg := imessagech.Config{
				AgentID:       *agentID,
				ServerURL:     strVal(cfg, "server_url"),
				Password:      strVal(cfg, "password"),
				WebhookSecret: strVal(cfg, "webhook_secret"),
				UseWebhook:    cfg["use_webhook"] == true || strVal(cfg, "use_webhook") == "true",
			}
			if imCfg.ServerURL != "" && imCfg.Password != "" {
				ch := imessagech.New(imCfg, gw.chanMgr.Handler())
				if imCfg.UseWebhook {
					webhookPath := fmt.Sprintf("/v1/webhooks/imessage/%s", id)
					gw.router.Post(webhookPath, ch.HandleWebhook)
				}
				gw.chanMgr.Register(id, ch)
				count++
			}

		case "matrix":
			mxCfg := matrixch.Config{
				AgentID:       *agentID,
				HomeserverURL: strVal(cfg, "homeserver_url"),
				AccessToken:   strVal(cfg, "access_token"),
				UserID:        strVal(cfg, "user_id"),
			}
			if mxCfg.HomeserverURL != "" && mxCfg.AccessToken != "" {
				ch := matrixch.New(mxCfg, gw.chanMgr.Handler())
				gw.chanMgr.Register(id, ch)
				count++
			}

		case "facebook", "messenger":
			fbCfg := facebookch.Config{
				AgentID:         *agentID,
				PageAccessToken: strVal(cfg, "page_access_token"),
				VerifyToken:     strVal(cfg, "verify_token"),
				AppSecret:       strVal(cfg, "app_secret"),
			}
			if fbCfg.PageAccessToken != "" && fbCfg.VerifyToken != "" {
				ch := facebookch.New(fbCfg, gw.chanMgr.Handler())
				gw.chanMgr.Register(id, ch)
				// Register webhook route for Facebook verification + inbound events
				webhookPath := fmt.Sprintf("/v1/webhooks/facebook/%s", id)
				gw.router.Get(webhookPath, ch.HandleWebhook)  // for GET verification
				gw.router.Post(webhookPath, ch.HandleWebhook) // for POST events
				slog.Info("facebook.webhook_route", "path", webhookPath)
				count++
			}
		}
	}
	if count > 0 {
		gw.chanMgr.StartAll(context.Background())
		slog.Info("channels loaded", "count", count)
	}
}

func (gw *Gateway) loadSingleChannel(ctx context.Context, id string) {
	var chType string
	var agentID *string
	var configJSON []byte
	gw.db.Pool.QueryRow(ctx,
		`SELECT agent_id, channel_type, config FROM channel_instances WHERE id = $1`, id,
	).Scan(&agentID, &chType, &configJSON)
	if agentID == nil {
		return
	}

	var cfg map[string]any
	json.Unmarshal(configJSON, &cfg)

	switch chType {
	case "telegram":
		tgCfg := telegramch.Config{
			AgentID: *agentID, BotToken: strVal(cfg, "bot_token"), BotName: strVal(cfg, "bot_name"),
			GroupPolicy: strVal(cfg, "group_policy"),
			RequireMention: cfg["require_mention"] == true || strVal(cfg, "require_mention") == "true",
		}
		if tgCfg.BotToken != "" {
			ch := telegramch.New(tgCfg, gw.chanMgr.Handler())
			if gw.voicePipeline != nil && gw.voicePipeline.CanTranscribe() {
				ch.Transcribe = gw.voicePipeline.TranscribeAudio
			}
			gw.chanMgr.Register(id, ch)
		}
	case "discord":
		dcCfg := discordch.Config{AgentID: *agentID, BotToken: strVal(cfg, "bot_token")}
		if dcCfg.BotToken != "" {
			ch := discordch.New(dcCfg, gw.chanMgr.Handler())
			gw.chanMgr.Register(id, ch)
		}
	case "slack":
		slCfg := slackch.Config{AgentID: *agentID, BotToken: strVal(cfg, "bot_token"), AppToken: strVal(cfg, "app_token")}
		if slCfg.BotToken != "" {
			ch := slackch.New(slCfg, gw.chanMgr.Handler())
			gw.chanMgr.Register(id, ch)
		}
	case "whatsapp":
		waCfg := whatsappch.Config{AgentID: *agentID, PhoneNumberID: strVal(cfg, "phone_number_id"), AccessToken: strVal(cfg, "access_token")}
		if waCfg.AccessToken != "" {
			ch := whatsappch.New(waCfg, gw.chanMgr.Handler())
			gw.chanMgr.Register(id, ch)
		}
	}
	slog.Info("channel.loaded_single", "id", id, "type", chType)
}

func (gw *Gateway) findTelegramChannel(agentID string) (*telegramch.TelegramChannel, bool) {
	for _, ch := range gw.chanMgr.List() {
		if fmt.Sprintf("%v", ch["agent_id"]) == agentID && ch["type"] == "telegram" && ch["running"] == true {
			if tgCh, ok := gw.chanMgr.GetChannel(ch["id"].(string)).(*telegramch.TelegramChannel); ok {
				return tgCh, true
			}
		}
	}
	return nil, false
}

// sendOTPViaTelegram sends the OTP to all paired Telegram chat IDs across any
// running Telegram channel. Returns true if at least one message was delivered.
func (gw *Gateway) sendOTPViaTelegram(ctx context.Context, tenantID, otp string) bool {
	if gw.db == nil { return false }

	// Find all paired Telegram chat IDs for this tenant
	rows, err := gw.db.Pool.Query(ctx,
		`SELECT sender_id FROM paired_devices WHERE tenant_id = $1 AND channel = 'telegram'`,
		tenantID)
	if err != nil { return false }
	defer rows.Close()

	var chatIDs []int64
	for rows.Next() {
		var senderID string
		if rows.Scan(&senderID) == nil {
			var id int64
			fmt.Sscanf(senderID, "%d", &id)
			if id != 0 { chatIDs = append(chatIDs, id) }
		}
	}

	if len(chatIDs) == 0 { return false }

	// Find any running Telegram channel
	var tgCh *telegramch.TelegramChannel
	for _, ch := range gw.chanMgr.List() {
		if ch["type"] == "telegram" && ch["running"] == true {
			if c, ok := gw.chanMgr.GetChannel(ch["id"].(string)).(*telegramch.TelegramChannel); ok {
				tgCh = c
				break
			}
		}
	}
	if tgCh == nil { return false }

	msg := fmt.Sprintf("🔐 Your Qorven password reset code:\n\n*%s*\n\nValid for 15 minutes. Do not share this code.", otp)
	sent := false
	for _, chatID := range chatIDs {
		if err := tgCh.SendText(ctx, chatID, msg); err == nil {
			sent = true
		}
	}
	return sent
}

func strVal(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func intVal(m map[string]any, key string) int {
	if v, ok := m[key].(float64); ok {
		return int(v)
	}
	return 0
}
