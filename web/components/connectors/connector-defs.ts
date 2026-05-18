// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

export interface ConnectorDef {
  id: string;
  name: string;
  icon: string;
  category: string;
  authType: 'api_key' | 'oauth' | 'credentials';
  fields: { key: string; label: string; type: 'text' | 'password'; placeholder?: string; required?: boolean }[];
  actions: string[];
  triggers: string[];
}

export const goldConnectors: ConnectorDef[] = [
  {
    id: 'github', name: 'GitHub', icon: '🐙', category: 'Development',
    authType: 'api_key',
    fields: [{ key: 'api_key', label: 'Personal Access Token', type: 'password', required: true }],
    actions: ['create_issue', 'create_pr', 'push_commit', 'list_repos'],
    triggers: ['push', 'pull_request', 'issue'],
  },
  {
    id: 'gmail', name: 'Gmail', icon: '📧', category: 'Communication',
    authType: 'api_key',
    fields: [{ key: 'api_key', label: 'App Password', type: 'password', required: true }, { key: 'email', label: 'Email Address', type: 'text', required: true }],
    actions: ['send_email', 'read_inbox', 'search_emails'],
    triggers: ['new_email'],
  },
  {
    id: 'google-calendar', name: 'Google Calendar', icon: '📅', category: 'Productivity',
    authType: 'api_key',
    fields: [{ key: 'api_key', label: 'API Key', type: 'password', required: true }],
    actions: ['create_event', 'list_events', 'update_event'],
    triggers: ['event_start'],
  },
  {
    id: 'slack', name: 'Slack', icon: '💬', category: 'Communication',
    authType: 'api_key',
    fields: [{ key: 'api_key', label: 'Bot Token (xoxb-)', type: 'password', required: true }],
    actions: ['send_message', 'list_channels', 'upload_file'],
    triggers: ['message', 'reaction'],
  },
  {
    id: 'notion', name: 'Notion', icon: '📝', category: 'Productivity',
    authType: 'api_key',
    fields: [{ key: 'api_key', label: 'Integration Token', type: 'password', required: true }],
    actions: ['create_page', 'query_database', 'update_page'],
    triggers: ['page_updated'],
  },
  {
    id: 'jira', name: 'Jira', icon: '🎯', category: 'Development',
    authType: 'credentials',
    fields: [
      { key: 'domain', label: 'Jira Domain', type: 'text', placeholder: 'yourteam.atlassian.net', required: true },
      { key: 'email', label: 'Email', type: 'text', required: true },
      { key: 'api_token', label: 'API Token', type: 'password', required: true },
    ],
    actions: ['create_issue', 'update_issue', 'transition_issue', 'search_issues'],
    triggers: ['issue_created', 'issue_updated'],
  },
  {
    id: 'stripe', name: 'Stripe', icon: '💳', category: 'Finance',
    authType: 'api_key',
    fields: [{ key: 'api_key', label: 'Secret Key', type: 'password', required: true }],
    actions: ['create_charge', 'list_customers', 'create_invoice'],
    triggers: ['payment_succeeded', 'invoice_paid'],
  },
  {
    id: 'google-sheets', name: 'Google Sheets', icon: '📊', category: 'Productivity',
    authType: 'api_key',
    fields: [{ key: 'api_key', label: 'API Key', type: 'password', required: true }],
    actions: ['read_sheet', 'write_sheet', 'append_row'],
    triggers: [],
  },
  {
    id: 'weather', name: 'Weather', icon: '🌤️', category: 'Data',
    authType: 'api_key',
    fields: [{ key: 'api_key', label: 'API Key', type: 'password', placeholder: 'OpenWeatherMap key' }],
    actions: ['current_weather', 'forecast'],
    triggers: [],
  },
];
