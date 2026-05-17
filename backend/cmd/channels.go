// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/qorvenai/qorven/cmd/output"
	"github.com/qorvenai/qorven/cmd/tui"
)

// channelSetupGuides holds plain-text onboarding guides for each channel type.
var channelSetupGuides = map[string]string{
	"telegram":   "1. Open Telegram and search for @BotFather\n2. Send /newbot — choose a name and username\n3. Copy the bot token and use: qorven channels create --type telegram --config '{\"bot_token\":\"<token>\"}'",
	"discord":    "1. discord.com/developers/applications → New Application → Bot tab → Reset Token\n2. Enable Message Content Intent under Privileged Gateway Intents\n3. Use: qorven channels create --type discord --config '{\"bot_token\":\"<token>\"}'",
	"slack":      "1. api.slack.com/apps → Create New App → Socket Mode → enable\n2. OAuth & Permissions → install; copy Bot Token (xoxb-) and App Token (xapp-)\n3. Use: qorven channels create --type slack --config '{\"bot_token\":\"xoxb-...\",\"app_token\":\"xapp-...\"}'",
	"whatsapp":   "Cloud API: Meta Business → WhatsApp → API Setup → copy Phone Number ID + access token.\nBridge: run Baileys sidecar, use bridge_url config key.\nqorven channels create --type whatsapp --config @config.json",
	"email":      "1. Enable IMAP in Gmail settings\n2. Create App Password at myaccount.google.com/apppasswords\n3. Use: qorven channels create --type email --config '{\"email\":\"you@gmail.com\",\"password\":\"<app-pw>\",\"imap_host\":\"imap.gmail.com\",\"smtp_host\":\"smtp.gmail.com\"}'",
	"sms":        "1. Twilio console → buy a number → copy Account SID + Auth Token\n2. Use: qorven channels create --type sms --config '{\"from_number\":\"+1...\",\"api_key\":\"<SID>\",\"api_secret\":\"<token>\"}'",
	"teams":      "1. Azure Portal → App registrations → New registration; copy App ID + Tenant ID\n2. Certificates & secrets → New client secret\n3. Register bot at dev.botframework.com\n4. Use: qorven channels create --type teams --config @teams-config.json",
	"github":     "1. github.com/settings/apps/new → set webhook URL → download private key\n2. Install on repo/org; note Installation ID from URL\n3. Use: qorven channels create --type github --config @github-config.json",
	"webchat":    "1. Save the channel\n2. Copy the embed snippet from the UI and paste into your site's <head>\n3. qorven channels create --type webchat --config '{\"allowed_domains\":\"example.com\"}'",
	"webhook":    "1. Save the channel — note the inbound URL\n2. POST JSON with {\"text\":\"...\",\"userId\":\"...\"} to that URL\n3. qorven channels create --type webhook --config '{\"secret\":\"<optional-hmac-secret>\"}'",
	"signal":     "1. Install signal-cli and register: signal-cli -u +15551234567 register\n2. Start daemon: signal-cli daemon --socket /run/user/1000/signal-cli/socket\n3. Use: qorven channels create --type signal --config '{\"phone_number\":\"+1...\",\"socket_path\":\"/run/.../socket\"}'",
	"imessage":   "1. Install BlueBubbles server on a Mac and enable Cloud Sync\n2. Note the ngrok/CF Tunnel URL and server password\n3. Use: qorven channels create --type imessage --config '{\"server_url\":\"https://...\",\"password\":\"...\"}'",
	"facebook":   "1. developers.facebook.com/apps → add Messenger product → Generate Page Access Token\n2. Set webhook verify token in Meta config\n3. Use: qorven channels create --type facebook --config '{\"page_access_token\":\"...\",\"verify_token\":\"...\"}'",
	"line":       "1. developers.line.biz → Messaging API channel → Channel access token → Issue\n2. Copy Channel Secret from Basic settings\n3. Use: qorven channels create --type line --config '{\"channel_access_token\":\"...\",\"channel_secret\":\"...\"}'",
	"zalo":       "1. oa.zalo.me → API → create app → copy App ID + Secret\n2. Complete OA authorization flow to get Refresh Token; note OA ID\n3. Use: qorven channels create --type zalo --config @zalo-config.json",
	"feishu":     "1. open.feishu.cn → create app → copy App ID + Secret\n2. Permissions: add im:message, im:message:receive_v1\n3. Use: qorven channels create --type feishu --config '{\"app_id\":\"...\",\"app_secret\":\"...\"}'",
	"dingtalk":   "1. open.dingtalk.com → create app → copy Client ID + Secret\n2. Permissions: add message receive and send scopes\n3. Use: qorven channels create --type dingtalk --config '{\"client_id\":\"...\",\"client_secret\":\"...\"}'",
	"wecom":      "1. WeCom admin → Apps → Create custom app → copy Corp ID, AgentId, App Secret\n2. Set callback URL, Token, and EncodingAESKey under Receive Messages\n3. Use: qorven channels create --type wecom --config @wecom-config.json",
	"matrix":     "1. Create bot account on your homeserver; log in with Element\n2. Settings → Help & About → Advanced → Access Token\n3. Use: qorven channels create --type matrix --config '{\"homeserver_url\":\"https://matrix.org\",\"user_id\":\"@bot:matrix.org\",\"access_token\":\"...\"}'",
	"mattermost": "1. System Console → Integrations → Bot Accounts → Enable\n2. Integrations → Bot Accounts → Add Bot Account → copy token\n3. Use: qorven channels create --type mattermost --config '{\"server_url\":\"https://...\",\"bot_token\":\"...\"}'",
}

// channelOfficialLinks holds the canonical URL for creating a bot/token for each channel type.
var channelOfficialLinks = map[string]string{
	"telegram":   "https://t.me/BotFather",
	"discord":    "https://discord.com/developers/applications",
	"slack":      "https://api.slack.com/apps",
	"whatsapp":   "https://business.facebook.com",
	"email":      "https://myaccount.google.com/apppasswords",
	"sms":        "https://console.twilio.com",
	"teams":      "https://portal.azure.com/#view/Microsoft_AAD_RegisteredApps",
	"github":     "https://github.com/settings/apps/new",
	"webchat":    "",
	"webhook":    "",
	"signal":     "https://github.com/AsamK/signal-cli",
	"imessage":   "https://bluebubbles.app",
	"facebook":   "https://developers.facebook.com/apps",
	"line":       "https://developers.line.biz/console/",
	"zalo":       "https://oa.zalo.me/home",
	"feishu":     "https://open.feishu.cn/app",
	"dingtalk":   "https://open.dingtalk.com",
	"wecom":      "https://work.weixin.qq.com/wework_admin",
	"matrix":     "https://app.element.io",
	"mattermost": "https://mattermost.com",
}

var channelsCmd = &cobra.Command{
	Use:   "channels",
	Short: "Manage messaging channels",
}

var channelsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List channel instances",
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := newHTTP()
		if err != nil {
			return err
		}
		agentID, _ := cmd.Flags().GetString("agent")
		status, _ := cmd.Flags().GetString("status")

		path := "/v1/channels"
		var params []string
		if agentID != "" {
			params = append(params, "agent_id="+agentID)
		}
		if status != "" {
			params = append(params, "status="+status)
		}
		if len(params) > 0 {
			path += "?" + strings.Join(params, "&")
		}

		data, err := c.Get(path)
		if err != nil {
			return err
		}
		if cfg.OutputFormat != "table" {
			printer.Print(unmarshalList(data))
			return nil
		}
		tbl := output.NewTable("ID", "TYPE", "NAME", "AGENT", "STATUS")
		for _, ch := range unmarshalList(data) {
			tbl.AddRow(
				str(ch, "id"),
				str(ch, "channel_type"),
				str(ch, "name"),
				str(ch, "agent_name"),
				str(ch, "status"),
			)
		}
		printer.Print(tbl)
		return nil
	},
}

var channelsGetCmd = &cobra.Command{
	Use:   "get <id>",
	Short: "Get channel instance details",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := newHTTP()
		if err != nil {
			return err
		}
		data, err := c.Get("/v1/channels/" + args[0])
		if err != nil {
			return err
		}
		printer.Print(unmarshalMap(data))
		return nil
	},
}

var channelsInfoCmd = &cobra.Command{
	Use:   "info <id>",
	Short: "Show channel details with setup guide",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := newHTTP()
		if err != nil {
			return err
		}
		data, err := c.Get("/v1/channels/" + args[0])
		if err != nil {
			return err
		}
		ch := unmarshalMap(data)
		chType := str(ch, "channel_type")

		fmt.Printf("ID:      %s\n", str(ch, "id"))
		fmt.Printf("Type:    %s\n", chType)
		fmt.Printf("Name:    %s\n", str(ch, "name"))
		fmt.Printf("Agent:   %s\n", str(ch, "agent_id"))
		fmt.Printf("Status:  %s\n", str(ch, "status"))
		fmt.Printf("Enabled: %s\n", str(ch, "enabled"))

		if link, ok := channelOfficialLinks[chType]; ok && link != "" {
			fmt.Printf("\nOfficial link: %s\n", link)
		}

		if guide, ok := channelSetupGuides[chType]; ok {
			fmt.Printf("\nSetup Guide:\n")
			for _, line := range strings.Split(guide, "\n") {
				fmt.Printf("  %s\n", line)
			}
		}

		fmt.Printf("\nDocs: https://docs.qorven.ai/%s\n", chType)
		return nil
	},
}

var channelsStartCmd = &cobra.Command{
	Use:   "start <id>",
	Short: "Start a channel instance",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := newHTTP()
		if err != nil {
			return err
		}
		_, err = c.Post("/v1/channels/"+args[0]+"/start", nil)
		if err != nil {
			return err
		}
		printer.Success("Channel started")
		return nil
	},
}

var channelsStopCmd = &cobra.Command{
	Use:   "stop <id>",
	Short: "Stop a channel instance",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := newHTTP()
		if err != nil {
			return err
		}
		_, err = c.Post("/v1/channels/"+args[0]+"/stop", nil)
		if err != nil {
			return err
		}
		printer.Success("Channel stopped")
		return nil
	},
}

var channelsDeleteCmd = &cobra.Command{
	Use:   "delete <id>",
	Short: "Delete a channel instance",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if !tui.Confirm("Delete this channel?", cfg.Yes) {
			return nil
		}
		c, err := newHTTP()
		if err != nil {
			return err
		}
		_, err = c.Delete("/v1/channels/" + args[0])
		if err != nil {
			return err
		}
		printer.Success("Channel deleted")
		return nil
	},
}

var channelsCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a channel instance",
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := newHTTP()
		if err != nil {
			return err
		}
		chType, _ := cmd.Flags().GetString("type")
		agentID, _ := cmd.Flags().GetString("agent")
		name, _ := cmd.Flags().GetString("name")
		config, _ := cmd.Flags().GetString("config")

		body := buildBody(
			"channel_type", chType,
			"agent_id", agentID,
			"name", name,
			"enabled", true,
		)
		if config != "" {
			content, err := readContent(config)
			if err != nil {
				return err
			}
			var cfgMap map[string]any
			if err := json.Unmarshal([]byte(content), &cfgMap); err != nil {
				return fmt.Errorf("invalid config JSON: %w", err)
			}
			body["config"] = cfgMap
		}

		data, err := c.Post("/v1/channels", body)
		if err != nil {
			return err
		}
		m := unmarshalMap(data)
		printer.Success(fmt.Sprintf("Channel created: %s (%s)", str(m, "channel_type"), str(m, "id")))
		return nil
	},
}

var channelsEditCmd = &cobra.Command{
	Use:   "edit <id>",
	Short: "Update a channel's configuration",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := newHTTP()
		if err != nil {
			return err
		}
		config, _ := cmd.Flags().GetString("config")
		name, _ := cmd.Flags().GetString("name")
		if config == "" && name == "" {
			return fmt.Errorf("provide at least --config or --name")
		}

		body := buildBody("name", name)
		if config != "" {
			content, err := readContent(config)
			if err != nil {
				return err
			}
			var cfgMap map[string]any
			if err := json.Unmarshal([]byte(content), &cfgMap); err != nil {
				return fmt.Errorf("invalid config JSON: %w", err)
			}
			body["config"] = cfgMap
		}

		_, err = c.Put("/v1/channels/"+args[0], body)
		if err != nil {
			return err
		}
		printer.Success("Channel updated")
		return nil
	},
}

func init() {
	channelsListCmd.Flags().String("agent", "", "Filter by agent ID")
	channelsListCmd.Flags().String("status", "", "Filter by status (running|stopped|error)")
	channelsCreateCmd.Flags().String("type", "", "Channel type (telegram, slack, discord, etc.)")
	channelsCreateCmd.Flags().String("agent", "", "Agent ID to bind")
	channelsCreateCmd.Flags().String("name", "", "Display name for this channel instance")
	channelsCreateCmd.Flags().String("config", "", "Channel config JSON (or @file)")
	channelsEditCmd.Flags().String("config", "", "Channel config JSON (or @file)")
	channelsEditCmd.Flags().String("name", "", "New display name")
	_ = channelsCreateCmd.MarkFlagRequired("type")
	_ = channelsCreateCmd.MarkFlagRequired("agent")

	channelsCmd.AddCommand(channelsListCmd, channelsGetCmd, channelsCreateCmd,
		channelsStartCmd, channelsStopCmd, channelsDeleteCmd,
		channelsEditCmd, channelsInfoCmd)
	rootCmd.AddCommand(channelsCmd)
}
