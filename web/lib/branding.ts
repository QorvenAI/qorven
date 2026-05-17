// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

// Branding configuration — change these to rebrand the entire platform
// All UI text references import from here instead of hardcoding

export const brand = {
  platformName: 'Qorven',
  agentName: 'Qor',
  agentNamePlural: 'Qors',
  supervisorName: 'Prime',
  tagline: 'Multi-agent AI platform',
  company: 'Qorven',
  website: 'https://qorven.ai',
};

// Page titles
export const titles = {
  agents: brand.agentNamePlural,
  sessions: 'Sessions',
  channels: 'Channels',
  workflows: 'Workflows',
  billing: 'Billing',
  audit: 'Audit Log',
  supervisor: `${brand.supervisorName} Dashboard`,
  council: 'LLM Council',
  tools: 'Tools',
  calendar: 'Calendar',
  templates: 'Templates',
  research: 'Deep Research',
  sandbox: 'Code Sandbox',
  training: 'Training Export',
  usage: 'Usage Analytics',
  memory: 'Memory Manager',
  outbound: 'Outbound Approvals',
  settings: 'Settings',
  connections: 'Connections',
  connectors: 'Connectors',
  marketplace: 'Marketplace',
  drive: 'Drive',
  goals: 'Goals',
  tasks: 'Tasks',
  rooms: 'Hubs',
  orgChart: 'Org Chart',
  scenarios: 'Scenarios',
  analytics: 'Analytics',
  notifications: 'Notifications',
  mail: 'Mail',
  voice: 'Voice',
  pipeline: 'Pipeline',
  routing: 'Routing',
  modelsHub: 'Models Hub',
};

// Placeholders
export const placeholders = {
  chatInput: `Message this ${brand.agentName}…`,
  searchPlatforms: 'Search platforms...',
  councilInput: 'Ask a complex question...',
  researchInput: 'Research topic...',
  eventTitle: 'Event title',
  systemPrompt: 'Instructions for this agent...',
  globalSearch: `Search ${brand.agentNamePlural.toLowerCase()}, pages, settings...`,
  searchChat: 'Search chat',
  searchModels: 'Search models...',
  searchSkills: 'Search skills...',
  searchMemories: 'Search memories…',
  displayName: 'Display name',
  taskTitle: 'Task title',
  goalTitle: 'Goal title',
};

// Button labels
export const buttons = {
  newAgent: `New ${brand.agentName}`,
  createAgent: `Create ${brand.agentName}`,
  approve: 'Approve',
  reject: 'Reject',
  save: 'Save',
  delete: 'Delete',
  cancel: 'Cancel',
  install: 'Install',
  run: 'Run',
  copy: 'Copy',
  copied: 'Copied',
  retry: 'Retry',
  export: 'Export',
  connect: 'Connect',
  setBudget: 'Set Budget',
  savePrompt: 'Save System Prompt',
  creating: 'Creating...',
  running: 'Running...',
};

// Empty states
export const emptyStates = {
  agents: {
    title: `No ${brand.agentNamePlural} yet`,
    description: `Create your first ${brand.agentName} to get started. Each ${brand.agentName} is an AI agent with its own identity, tools, and channels.`,
    action: `Create your first ${brand.agentName}`,
  },
  activity: {
    title: 'No activity yet',
    description: `Activity will appear here in real-time as your ${brand.agentNamePlural} work.`,
  },
  channels: {
    title: 'No channels',
    description: `Connect channels from each ${brand.agentName}'s detail page.`,
  },
  sessions: {
    title: 'No sessions',
    description: 'Start a conversation to create a session.',
  },
  events: {
    title: 'No events scheduled',
    description: 'Create an event to get started.',
  },
};

// Descriptions
export const descriptions = {
  agents: `${brand.agentNamePlural} in your workspace`,
  channels: `All channel instances across all ${brand.agentNamePlural}`,
  workflows: `Multi-step ${brand.agentName} Flows`,
  rooms: `Multi-${brand.agentName} hub collaboration — agents collaborate in real-time`,
  audit: 'Every action tracked — who did what, when',
  supervisor: `${brand.supervisorName}'s view of the agent team`,
  council: 'Multi-model consensus — 3 models respond, rank each other, chairman synthesizes',
  tools: 'tools available',
  research: 'Async research jobs with multi-source search and synthesis',
  sandbox: 'Run code in an isolated environment',
  training: 'Export conversation data for fine-tuning LLMs',
  usage: 'Token usage and costs across all agents',
  memory: 'Manage company and supervisor memories',
  outbound: 'Review and approve outbound actions (emails, posts, webhooks)',
  settings: 'Manage your workspace and preferences',
  connections: 'Link external services',
  marketplace: 'Browse and install skills',
  billing: 'Usage costs and budgets',
};
