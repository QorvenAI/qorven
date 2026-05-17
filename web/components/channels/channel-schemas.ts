// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import type { ChannelType } from '@/types';

export interface ChannelField {
  key: string;
  label: string;
  type: 'text' | 'password' | 'toggle' | 'textarea' | 'select';
  placeholder?: string;
  help?: string;
  required?: boolean;
  options?: { value: string; label: string }[];
  // Only show this field when another field has a specific value
  showWhen?: { key: string; value: string };
}

export interface ChannelSchema {
  label: string;
  icon: string;        // emoji icon for the channel card
  description: string; // short description shown in picker
  color: string;       // tailwind bg-* class for the icon badge
  fields: ChannelField[];
  officialLink: string;   // URL to create bot/token; empty string if self-hosted
  docsSlug: string;       // appended to https://docs.qorven.ai/
  setupSteps: string[];   // 2–4 plain-English steps shown in the drawer
}

export const channelFormSchemas: Record<ChannelType, ChannelSchema> = {
  telegram: {
    label: 'Telegram',
    icon: '✈️',
    description: 'Connect via Telegram Bot API',
    color: 'bg-blue-500/10 text-blue-400',
    officialLink: 'https://t.me/BotFather',
    docsSlug: 'telegram',
    setupSteps: [
      'Open Telegram and search for @BotFather',
      'Send /newbot — choose a name and username for your bot',
      'Copy the bot token BotFather gives you and paste it below',
    ],
    fields: [
      {
        key: 'bot_token',
        label: 'Bot Token',
        type: 'password',
        placeholder: '123456789:ABC-DEF1234...',
        help: 'Create a bot via @BotFather on Telegram, then copy the token it gives you.',
        required: true,
      },
      {
        key: 'bot_name',
        label: 'Bot Username',
        type: 'text',
        placeholder: '@my_bot',
        help: 'Optional — the @username of the bot (shown in the UI only).',
      },
      {
        key: 'group_policy',
        label: 'Group Policy',
        type: 'select',
        options: [
          { value: '', label: 'Default (open)' },
          { value: 'open', label: 'Open — reply to anyone' },
          { value: 'mention_only', label: 'Mention only — only when @mentioned' },
          { value: 'admin_only', label: 'Admin only' },
          { value: 'disabled', label: 'Disabled in groups' },
        ],
      },
      {
        key: 'require_mention',
        label: 'Require @mention in groups',
        type: 'toggle',
      },
    ],
  },

  discord: {
    label: 'Discord',
    icon: '🎮',
    description: 'Connect a Discord bot to a server',
    color: 'bg-indigo-500/10 text-indigo-400',
    officialLink: 'https://discord.com/developers/applications',
    docsSlug: 'discord',
    setupSteps: [
      'Go to discord.com/developers/applications → New Application',
      'Open the Bot tab → Reset Token → copy the token',
      'Enable Message Content Intent under Privileged Gateway Intents',
    ],
    fields: [
      {
        key: 'bot_token',
        label: 'Bot Token',
        type: 'password',
        placeholder: 'MTM...your-discord-bot-token',
        help: 'Go to discord.com/developers → Your App → Bot → Reset Token.',
        required: true,
      },
      {
        key: 'guild_id',
        label: 'Guild (Server) ID',
        type: 'text',
        placeholder: '123456789012345678',
        help: 'Right-click your server icon → Copy Server ID (Developer Mode must be on).',
      },
      {
        key: 'dm_policy',
        label: 'DM Policy',
        type: 'select',
        options: [
          { value: '', label: 'Default (open)' },
          { value: 'open', label: 'Open — respond to all DMs' },
          { value: 'disabled', label: 'Disabled — ignore DMs' },
        ],
      },
      {
        key: 'require_mention',
        label: 'Require @mention in channels',
        type: 'toggle',
      },
    ],
  },

  slack: {
    label: 'Slack',
    icon: '#',
    description: 'Connect to a Slack workspace via Socket Mode',
    color: 'bg-green-500/10 text-green-400',
    officialLink: 'https://api.slack.com/apps',
    docsSlug: 'slack',
    setupSteps: [
      'Go to api.slack.com/apps → Create New App → From scratch',
      'Enable Socket Mode under Settings; generate an App-Level Token (xapp-) with connections:write scope',
      'OAuth & Permissions → Install to Workspace; copy the Bot Token (xoxb-)',
    ],
    fields: [
      {
        key: 'bot_token',
        label: 'Bot Token (xoxb-)',
        type: 'password',
        placeholder: 'xoxb-...',
        help: 'OAuth & Permissions → Bot User OAuth Token.',
        required: true,
      },
      {
        key: 'app_token',
        label: 'App-Level Token (xapp-)',
        type: 'password',
        placeholder: 'xapp-...',
        help: 'Basic Information → App-Level Tokens → Generate (connections:write scope). Required for Socket Mode.',
        required: true,
      },
      {
        key: 'dm_policy',
        label: 'DM Policy',
        type: 'select',
        options: [
          { value: '', label: 'Default (open)' },
          { value: 'open', label: 'Open' },
          { value: 'disabled', label: 'Disabled' },
        ],
      },
      {
        key: 'require_mention',
        label: 'Require @mention in channels',
        type: 'toggle',
      },
    ],
  },

  whatsapp: {
    label: 'WhatsApp',
    icon: '💬',
    description: 'Connect via Meta Cloud API or self-hosted bridge',
    color: 'bg-emerald-500/10 text-emerald-400',
    officialLink: 'https://business.facebook.com',
    docsSlug: 'whatsapp',
    setupSteps: [
      'Cloud API: Meta Business → WhatsApp → API Setup → copy Phone Number ID and generate a permanent access token',
      'Bridge: run the Baileys sidecar (see docs), paste its URL below, then scan the QR code after saving',
      'Set a Webhook Verify Token (any string) and enter it in the Meta webhook configuration',
    ],
    fields: [
      {
        key: 'mode',
        label: 'Connection Mode',
        type: 'select',
        required: true,
        options: [
          { value: 'cloud', label: 'Cloud API (Meta Business)' },
          { value: 'bridge', label: 'Bridge (self-hosted Baileys — scan QR)' },
        ],
      },
      // Cloud API fields
      {
        key: 'phone_number_id',
        label: 'Phone Number ID',
        type: 'text',
        placeholder: '123456789012345',
        help: 'Meta Business → WhatsApp → API Setup → Phone number ID.',
        showWhen: { key: 'mode', value: 'cloud' },
      },
      {
        key: 'access_token',
        label: 'Permanent Access Token',
        type: 'password',
        help: 'Meta Business → System Users → Generate token with whatsapp_business_messaging scope.',
        showWhen: { key: 'mode', value: 'cloud' },
      },
      {
        key: 'verify_token',
        label: 'Webhook Verify Token',
        type: 'text',
        placeholder: 'any-random-string',
        help: 'A secret string you choose — enter the same value in Meta webhook config.',
        showWhen: { key: 'mode', value: 'cloud' },
      },
      {
        key: 'app_secret',
        label: 'App Secret',
        type: 'password',
        help: 'Meta App Dashboard → Basic Settings → App Secret.',
        showWhen: { key: 'mode', value: 'cloud' },
      },
      // Bridge fields
      {
        key: 'bridge_url',
        label: 'Bridge URL',
        type: 'text',
        placeholder: 'http://localhost:3001',
        help: 'URL of your running Baileys/whatsapp-web.js sidecar. After saving, scan the QR code shown below.',
        showWhen: { key: 'mode', value: 'bridge' },
      },
      {
        key: 'allowlist',
        label: 'Allowed Numbers (allowlist)',
        type: 'textarea',
        placeholder: '+15551234567\n+447700900000',
        help: 'One number per line (E.164 format). Leave empty to allow all senders. Admins can also approve numbers via the Pending Senders panel.',
        showWhen: { key: 'mode', value: 'bridge' },
      },
      // Shared
      {
        key: 'dm_policy',
        label: 'DM Policy',
        type: 'select',
        options: [
          { value: '', label: 'Default (open)' },
          { value: 'open', label: 'Open' },
          { value: 'disabled', label: 'Disabled' },
        ],
      },
    ],
  },

  email: {
    label: 'Email',
    icon: '✉️',
    description: 'Connect via IMAP (receive) + SMTP (send)',
    color: 'bg-orange-500/10 text-orange-400',
    officialLink: 'https://myaccount.google.com/apppasswords',
    docsSlug: 'email',
    setupSteps: [
      'Enable IMAP in Gmail → Settings → See all settings → Forwarding and POP/IMAP',
      'Create an App Password at myaccount.google.com/apppasswords (requires 2-Step Verification)',
      'Fill in the IMAP and SMTP fields below using your email and the App Password',
    ],
    fields: [
      { key: 'email',     label: 'Email Address', type: 'text',     placeholder: 'agent@gmail.com', required: true },
      { key: 'password',  label: 'Password / App Password', type: 'password', required: true,
        help: 'For Gmail, create an App Password at myaccount.google.com/apppasswords.' },
      { key: 'imap_host', label: 'IMAP Host', type: 'text', placeholder: 'imap.gmail.com', required: true },
      { key: 'imap_port', label: 'IMAP Port', type: 'text', placeholder: '993' },
      { key: 'smtp_host', label: 'SMTP Host', type: 'text', placeholder: 'smtp.gmail.com', required: true },
      { key: 'smtp_port', label: 'SMTP Port', type: 'text', placeholder: '587' },
      { key: 'poll_seconds', label: 'Poll Interval (seconds)', type: 'text', placeholder: '60' },
      { key: 'folder',       label: 'IMAP Folder', type: 'text', placeholder: 'INBOX',
        help: 'The mailbox folder to monitor. Leave blank for INBOX.' },
      { key: 'soul_name',    label: 'Sender Display Name', type: 'text', placeholder: 'Qorven Assistant',
        help: 'Name shown in the From: field when the agent sends emails.' },
      { key: 'spam_filter',  label: 'Spam Filter', type: 'toggle',
        help: 'Silently drop newsletters, auto-replies, and no-reply addresses.' },
      { key: 'auto_ack',     label: 'Auto-Acknowledge', type: 'toggle',
        help: 'Send an immediate acknowledgment reply while the agent prepares a full response.' },
      { key: 'html_reply',   label: 'HTML Replies', type: 'toggle',
        help: 'Send outbound emails as HTML. Disable to send plain text only.' },
    ],
  },

  sms: {
    label: 'SMS',
    icon: '📱',
    description: 'Send and receive SMS via Twilio',
    color: 'bg-red-500/10 text-red-400',
    officialLink: 'https://console.twilio.com',
    docsSlug: 'sms',
    setupSteps: [
      'Create a Twilio project at console.twilio.com and buy a phone number',
      'Go to Account → API keys & tokens; copy your Account SID and Auth Token',
      'Enter the Twilio phone number as From Number and paste the credentials below',
    ],
    fields: [
      {
        key: 'provider',
        label: 'Provider',
        type: 'select',
        options: [{ value: 'twilio', label: 'Twilio' }],
        required: true,
      },
      { key: 'from_number', label: 'From Number', type: 'text', placeholder: '+14155552671', required: true,
        help: 'The Twilio phone number that sends messages.' },
      { key: 'api_key',    label: 'Account SID / API Key', type: 'password', required: true },
      { key: 'api_secret', label: 'Auth Token / API Secret', type: 'password', required: true },
    ],
  },

  teams: {
    label: 'Microsoft Teams',
    icon: '🟦',
    description: 'Connect a Teams Bot via Azure Bot Framework',
    color: 'bg-blue-600/10 text-blue-400',
    officialLink: 'https://portal.azure.com/#view/Microsoft_AAD_RegisteredApps',
    docsSlug: 'teams',
    setupSteps: [
      'Azure Portal → App registrations → New registration; note the Application (client) ID and Directory (tenant) ID',
      'Certificates & secrets → New client secret; copy the value immediately',
      'Register your bot at dev.botframework.com using the same App ID and secret',
    ],
    fields: [
      { key: 'app_id',       label: 'App ID (Client ID)',     type: 'text',     required: true,
        help: 'Azure Portal → App registrations → Application (client) ID.' },
      { key: 'app_secret',   label: 'App Password (Secret)', type: 'password', required: true,
        help: 'Azure Portal → App registrations → Certificates & secrets → New client secret.' },
      { key: 'tenant_id',    label: 'Tenant ID',             type: 'text',     required: true,
        help: 'Azure Portal → App registrations → Directory (tenant) ID.' },
    ],
  },

  github: {
    label: 'GitHub',
    icon: '🐙',
    description: 'Connect a GitHub App for PR reviews and issue comments',
    color: 'bg-neutral-500/10 text-neutral-300',
    officialLink: 'https://github.com/settings/apps/new',
    docsSlug: 'github',
    setupSteps: [
      'GitHub → Settings → Developer settings → GitHub Apps → New GitHub App',
      'Set the webhook URL to your Qorven instance; generate and download a private key',
      'Install the app on your repo or org; note the Installation ID from the URL',
    ],
    fields: [
      { key: 'app_id',          label: 'App ID',          type: 'text',     required: true,
        help: 'GitHub → Settings → Developer settings → GitHub Apps → your app → App ID.' },
      { key: 'installation_id', label: 'Installation ID', type: 'text',     required: true,
        help: 'In the GitHub App → Install → the numeric ID in the URL.' },
      { key: 'private_key',     label: 'Private Key (PEM)', type: 'textarea', required: true,
        help: 'Download from GitHub App → Private keys. Paste the full -----BEGIN RSA PRIVATE KEY----- block.' },
      { key: 'webhook_secret',  label: 'Webhook Secret',  type: 'password',
        help: 'Optional — used to verify incoming webhook payloads.' },
    ],
  },

  webchat: {
    label: 'Webchat',
    icon: '🌐',
    description: 'Embed a chat widget on any website',
    color: 'bg-violet-500/10 text-violet-400',
    officialLink: '',
    docsSlug: 'webchat',
    setupSteps: [
      'Save the channel configuration below',
      "Copy the embed snippet shown after saving and paste it into your website's <head>",
      'The chat widget will appear on your site immediately',
    ],
    fields: [
      { key: 'allowed_domains', label: 'Allowed Domains', type: 'text',
        placeholder: 'example.com, app.example.com',
        help: 'Comma-separated list of domains allowed to embed the widget. Leave blank for any.' },
      { key: 'widget_color', label: 'Widget Accent Color', type: 'text', placeholder: '#7C3AED' },
    ],
  },

  webhook: {
    label: 'Webhook',
    icon: '🔗',
    description: 'Receive messages from any external service via HTTP POST',
    color: 'bg-yellow-500/10 text-yellow-400',
    officialLink: '',
    docsSlug: 'webhook',
    setupSteps: [
      'Save the channel — Qorven will show you the inbound webhook URL',
      'Configure your external service to POST JSON payloads to that URL',
      'Optionally set a Secret to verify payloads via HMAC-SHA256 (X-Qorven-Signature header)',
    ],
    fields: [
      { key: 'secret', label: 'Webhook Secret', type: 'password',
        help: 'Optional — include this in the X-Webhook-Secret header to verify incoming requests.' },
    ],
  },

  // Placeholder schemas for new channel types (to be filled in Task 2)
  signal: {
    label: 'Signal',
    icon: '🔒',
    description: 'Connect via signal-cli or CallMeBot bridge',
    color: 'bg-blue-700/10 text-blue-300',
    officialLink: 'https://github.com/AsamK/signal-cli',
    docsSlug: 'signal',
    setupSteps: [
      'Install signal-cli and register your phone number: signal-cli -u +15551234567 register',
      'Start signal-cli in daemon mode: signal-cli daemon --socket /run/user/1000/signal-cli/socket',
      'Paste the socket path and the registered phone number below',
    ],
    fields: [
      { key: 'socket_path', label: 'signal-cli Socket Path', type: 'text',
        placeholder: '/run/user/1000/signal-cli/socket', required: true,
        help: 'Absolute path to the running signal-cli UNIX socket.' },
      { key: 'phone_number', label: 'Registered Phone Number', type: 'text',
        placeholder: '+15551234567', required: true,
        help: 'E.164 format. Must match the number registered with signal-cli.' },
    ],
  },

  imessage: {
    label: 'iMessage',
    icon: '🍎',
    description: 'Connect via BlueBubbles server on a Mac',
    color: 'bg-gray-500/10 text-gray-300',
    officialLink: 'https://bluebubbles.app',
    docsSlug: 'imessage',
    setupSteps: [
      'Install BlueBubbles server on a Mac that stays powered on',
      'Enable Cloud Sync in BlueBubbles → Settings → Connection and note your server URL and password',
      'Paste the server URL and password below',
    ],
    fields: [
      { key: 'server_url', label: 'BlueBubbles Server URL', type: 'text',
        placeholder: 'https://yourname.ngrok.io', required: true,
        help: 'The ngrok/CF Tunnel URL shown in BlueBubbles → Settings → Connection.' },
      { key: 'password', label: 'Server Password', type: 'password', required: true,
        help: 'Set in BlueBubbles → Settings → Private API → Password.' },
      { key: 'use_webhook', label: 'Use Webhook (push events)', type: 'toggle',
        help: 'Enable to receive messages via push instead of polling. Requires BlueBubbles v1.9+.' },
      { key: 'webhook_secret', label: 'Webhook Secret', type: 'password',
        help: 'Optional — verifies incoming webhook payloads from BlueBubbles.',
        showWhen: { key: 'use_webhook', value: 'true' } },
    ],
  },

  facebook: {
    label: 'Facebook Messenger',
    icon: '📘',
    description: 'Connect a Meta Page via Messenger Platform',
    color: 'bg-blue-600/10 text-blue-400',
    officialLink: 'https://developers.facebook.com/apps',
    docsSlug: 'facebook',
    setupSteps: [
      'Create a Meta App at developers.facebook.com → Add the Messenger product',
      'Subscribe your Facebook Page → Messenger → Settings → Generate Page Access Token',
      'Copy the Page Access Token and set a Webhook Verify Token (any string you choose)',
    ],
    fields: [
      { key: 'page_access_token', label: 'Page Access Token', type: 'password', required: true,
        help: 'Meta for Developers → your App → Messenger → Settings → Generate Page Access Token.' },
      { key: 'verify_token', label: 'Webhook Verify Token', type: 'text',
        placeholder: 'any-random-string', required: true,
        help: 'A string you choose — enter the same value in the Meta webhook configuration.' },
      { key: 'app_secret', label: 'App Secret', type: 'password',
        help: 'Meta App Dashboard → Basic Settings → App Secret. Used to verify webhook payloads.' },
    ],
  },

  line: {
    label: 'LINE',
    icon: '💚',
    description: 'Connect via LINE Messaging API',
    color: 'bg-green-600/10 text-green-400',
    officialLink: 'https://developers.line.biz/console/',
    docsSlug: 'line',
    setupSteps: [
      'Go to developers.line.biz → Create a new provider and channel → Messaging API',
      'Messaging API tab → Channel access token (long-lived) → Issue; copy the token',
      'Basic settings tab → copy Channel secret',
    ],
    fields: [
      { key: 'channel_access_token', label: 'Channel Access Token', type: 'password', required: true,
        help: 'LINE Developers → Messaging API tab → Channel access token (long-lived) → Issue.' },
      { key: 'channel_secret', label: 'Channel Secret', type: 'password', required: true,
        help: 'LINE Developers → Basic settings tab → Channel secret.' },
    ],
  },

  zalo: {
    label: 'Zalo',
    icon: '🇻🇳',
    description: 'Connect a Zalo Official Account (OA)',
    color: 'bg-blue-500/10 text-blue-300',
    officialLink: 'https://oa.zalo.me/home',
    docsSlug: 'zalo',
    setupSteps: [
      'Register a Zalo Official Account at oa.zalo.me',
      'OA Management → API → create an app to get App ID + App Secret',
      'Complete the OA authorization flow to obtain a Refresh Token; note your OA ID from OA Management → Info',
    ],
    fields: [
      { key: 'app_id',       label: 'App ID',       type: 'text',     required: true,
        help: 'Found in Zalo OA Management → API → your app credentials.' },
      { key: 'app_secret',   label: 'App Secret',   type: 'password', required: true },
      { key: 'refresh_token', label: 'Refresh Token', type: 'password', required: true,
        help: 'Obtained from the Zalo OA authorization flow. Expires every 90 days — re-issue before expiry.' },
      { key: 'oa_id', label: 'OA ID', type: 'text', required: true,
        help: 'Zalo Official Account ID — found in OA Management → Info.' },
    ],
  },

  feishu: {
    label: 'Feishu / Lark',
    icon: '🪁',
    description: 'Connect a Feishu/Lark Bot via Open Platform',
    color: 'bg-sky-500/10 text-sky-400',
    officialLink: 'https://open.feishu.cn/app',
    docsSlug: 'feishu',
    setupSteps: [
      'Go to open.feishu.cn → Create an app → copy App ID and App Secret',
      'Permissions & Scopes → add im:message and im:message:receive_v1',
      'Event Subscriptions → set callback URL and paste it into your Qorven webhook URL',
    ],
    fields: [
      { key: 'app_id',         label: 'App ID',         type: 'text',     required: true,
        help: 'Found at open.feishu.cn → your app → Credentials & Basic Info.' },
      { key: 'app_secret',     label: 'App Secret',     type: 'password', required: true },
      { key: 'encrypt_key',    label: 'Encrypt Key',    type: 'password',
        help: 'Optional — set in Feishu Event Subscriptions if you want payload encryption.' },
      { key: 'verification_token', label: 'Verification Token', type: 'password',
        help: 'From Feishu Event Subscriptions — used to verify webhook requests.' },
    ],
  },

  dingtalk: {
    label: 'DingTalk',
    icon: '📎',
    description: 'Connect a DingTalk bot via Alibaba Cloud',
    color: 'bg-orange-500/10 text-orange-400',
    officialLink: 'https://open.dingtalk.com',
    docsSlug: 'dingtalk',
    setupSteps: [
      'Go to open.dingtalk.com → Create an application → copy Client ID and Client Secret',
      'Permissions → add dingtalk.robot.message.receive and dingtalk.chat.message.sendToUser',
      'Event Subscription → configure the callback URL to point to your Qorven instance',
    ],
    fields: [
      { key: 'client_id',     label: 'Client ID',     type: 'text',     required: true,
        help: 'DingTalk Open Platform → app credentials → Client ID.' },
      { key: 'client_secret', label: 'Client Secret', type: 'password', required: true },
      { key: 'robot_code',    label: 'Robot Code',    type: 'text',
        help: 'The robot/app identifier shown in the DingTalk bot configuration.' },
    ],
  },

  wecom: {
    label: 'WeCom',
    icon: '💼',
    description: 'Connect WeCom (WeChat Work) application',
    color: 'bg-green-700/10 text-green-400',
    officialLink: 'https://work.weixin.qq.com/wework_admin',
    docsSlug: 'wecom',
    setupSteps: [
      'Log in to WeCom admin console → Apps → Create custom app',
      'Copy Corp ID (Company Information), Agent ID (your app), and App Secret (your app → Secret)',
      'Set the API receive message callback URL and configure the Token + EncodingAESKey',
    ],
    fields: [
      { key: 'corp_id',     label: 'Corp ID',     type: 'text',     required: true,
        help: 'WeCom admin → My Company → Company Information → Corp ID.' },
      { key: 'wecom_agent_id', label: 'Agent ID (WeCom)', type: 'text', required: true,
        help: 'AgentId shown in WeCom Apps → your custom app. (Not the Qorven agent ID.)' },
      { key: 'app_secret',  label: 'App Secret',  type: 'password', required: true,
        help: 'WeCom Apps → your custom app → Secret.' },
      { key: 'token',       label: 'Callback Token',  type: 'password',
        help: 'Set in WeCom app → Receive Messages → Token.' },
      { key: 'encoding_aes_key', label: 'EncodingAESKey', type: 'password',
        help: 'Set in WeCom app → Receive Messages → EncodingAESKey.' },
    ],
  },

  matrix: {
    label: 'Matrix',
    icon: '🔷',
    description: 'Connect a Matrix bot account',
    color: 'bg-teal-500/10 text-teal-400',
    officialLink: 'https://app.element.io',
    docsSlug: 'matrix',
    setupSteps: [
      'Create a bot account on your Matrix homeserver (e.g. matrix.org or your own)',
      'Log in as the bot account in Element → Settings → Help & About → Advanced → Access Token',
      'Paste the homeserver URL, full user ID (@bot:matrix.org), and the access token below',
    ],
    fields: [
      { key: 'homeserver_url', label: 'Homeserver URL', type: 'text',
        placeholder: 'https://matrix.org', required: true },
      { key: 'user_id',        label: 'User ID',        type: 'text',
        placeholder: '@mybot:matrix.org', required: true,
        help: 'Full Matrix user ID in @localpart:server format.' },
      { key: 'access_token',   label: 'Access Token',   type: 'password', required: true,
        help: 'Element → Settings → Help & About → Advanced → Access Token.' },
    ],
  },

  mattermost: {
    label: 'Mattermost',
    icon: '🗯️',
    description: 'Connect via Mattermost Bot Account',
    color: 'bg-cyan-500/10 text-cyan-400',
    officialLink: 'https://mattermost.com',
    docsSlug: 'mattermost',
    setupSteps: [
      'System Console → Integrations → Bot Accounts → Enable Bot Account Creation',
      'Integrations → Bot Accounts → Add Bot Account — copy the generated access token',
      'Paste your Mattermost server URL and the bot token below',
    ],
    fields: [
      { key: 'server_url',  label: 'Server URL',   type: 'text',
        placeholder: 'https://your-mattermost.com', required: true },
      { key: 'bot_token',   label: 'Bot Token',    type: 'password', required: true,
        help: 'Generated when you create the bot account in System Console.' },
      { key: 'team_name',   label: 'Team Name',    type: 'text',
        help: 'Optional — restrict the bot to a specific Mattermost team.' },
    ],
  },
};

export const CHANNEL_TYPES = Object.keys(channelFormSchemas) as ChannelType[];
