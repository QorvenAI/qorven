// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { cn } from '@/lib/utils';
import { brand } from '@/lib/branding';
import {
  Users, MessageSquare, Zap, Link2, Clock, CheckSquare, GitBranch, Sparkles,
  Plug, Activity, Share2, Cpu, Settings, Shield, FileText, Mic, Mail,
  BarChart3, ClipboardList, BookOpen, Layers, Target, UserCheck, Globe,
  Code, Beaker, Calendar, HardDrive, Bell, LayoutGrid, Route,
  type LucideIcon,
} from 'lucide-react';

interface EmptyStateProps {
  icon: LucideIcon;
  title: string;
  description: string;
  actionLabel?: string;
  onAction?: () => void;
  className?: string;
}

export function EmptyState({ icon: Icon, title, description, actionLabel, onAction, className }: EmptyStateProps) {
  return (
    <div className={cn('flex flex-col items-center justify-center py-16 text-center', className)}>
      <div className="flex h-14 w-14 items-center justify-center rounded-2xl bg-muted">
        <Icon className="h-7 w-7 text-muted-foreground" />
      </div>
      <h3 className="mt-4 text-base font-medium">{title}</h3>
      <p className="mt-1 max-w-sm text-sm text-muted-foreground">{description}</p>
      {actionLabel && onAction && (
        <button
          onClick={onAction}
          className="mt-4 rounded-lg bg-primary px-4 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90"
        >
          {actionLabel}
        </button>
      )}
    </div>
  );
}

export const emptyStates = {
  souls: { icon: Users, title: `No ${brand.agentNamePlural} yet`, description: `Create your first ${brand.agentName} to get started. Each ${brand.agentName} is an AI agent with its own identity, tools, and channels.`, actionLabel: `Create your first ${brand.agentName}` },
  sessions: { icon: MessageSquare, title: 'No sessions yet', description: `Start a conversation with any ${brand.agentName} to create a session.`, actionLabel: 'Start a chat' },
  live: { icon: Zap, title: 'No activity yet', description: `Activity will appear here in real-time as your ${brand.agentNamePlural} work.` },
  connectors: { icon: Link2, title: 'No connectors', description: `Connect external services like GitHub, Notion, or Stripe to extend your ${brand.agentNamePlural}.`, actionLabel: 'Add connector' },
  cron: { icon: Clock, title: 'No scheduled jobs', description: `Schedule recurring tasks for your ${brand.agentNamePlural} — weather reports, digests, monitoring.`, actionLabel: 'Create schedule' },
  tasks: { icon: CheckSquare, title: 'No tasks yet', description: `Tasks appear here when ${brand.agentNamePlural} create or receive work items.`, actionLabel: 'Create task' },
  workflows: { icon: GitBranch, title: 'No workflows', description: `Build multi-step workflows that coordinate multiple ${brand.agentNamePlural}.`, actionLabel: 'Create workflow' },
  skills: { icon: Sparkles, title: 'No skills installed', description: `Browse the marketplace to find skills for your ${brand.agentNamePlural}.`, actionLabel: 'Browse marketplace' },
  channels: { icon: Plug, title: 'No channels connected', description: `Connect channels like Telegram, Discord, or Slack so your ${brand.agentNamePlural} can communicate.`, actionLabel: 'Add channel' },
  heartbeat: { icon: Activity, title: 'No heartbeat data', description: `Heartbeat monitoring will appear here once your ${brand.agentNamePlural} are active.` },
  knowledgeGraph: { icon: Share2, title: 'No knowledge graph data', description: `The knowledge graph builds automatically as your ${brand.agentNamePlural} learn and interact.` },
  modelsHub: { icon: Cpu, title: 'No models configured', description: 'Add an LLM provider to start using AI models.', actionLabel: 'Configure provider' },
  connections: { icon: Link2, title: 'No connections', description: 'Link external services like GitHub, Jira, or Shopify.', actionLabel: 'Add connection' },
  council: { icon: Users, title: 'No council sessions', description: 'Ask a complex question and let multiple models debate and synthesize an answer.', actionLabel: 'Start council' },
  marketplace: { icon: Layers, title: 'Marketplace coming soon', description: 'Browse and install community-built skills, tools, and templates.' },
  pairing: { icon: UserCheck, title: 'No paired devices', description: 'Pair your phone or other devices to chat with agents via DM.' },
  research: { icon: Globe, title: 'No research jobs', description: 'Start a deep research job to search, analyze, and synthesize information.', actionLabel: 'New research' },
  sandbox: { icon: Code, title: 'No sandbox sessions', description: 'Run code in an isolated environment.', actionLabel: 'Open sandbox' },
  scenarios: { icon: Beaker, title: 'No scenarios', description: 'Create test scenarios to evaluate agent performance.', actionLabel: 'Create scenario' },
  supervisor: { icon: Shield, title: 'No supervisor data', description: `${brand.supervisorName} dashboard shows agent team health and coordination.` },
  system: { icon: Settings, title: 'System settings', description: 'Configure platform-wide settings.' },
  templates: { icon: FileText, title: 'No templates', description: `Create reusable ${brand.agentName} templates with pre-configured tools and prompts.`, actionLabel: 'Create template' },
  tools: { icon: Sparkles, title: 'Tools loaded', description: 'All built-in tools are available. Custom tools can be added via MCP.' },
  usage: { icon: BarChart3, title: 'No usage data yet', description: `Usage analytics will appear once your ${brand.agentNamePlural} start processing requests.` },
  voice: { icon: Mic, title: 'No voice sessions', description: 'Start a voice conversation with any agent.', actionLabel: 'Start voice chat' },
  mail: { icon: Mail, title: 'No messages', description: 'Inbound and outbound messages will appear here.' },
  rooms: { icon: MessageSquare, title: 'No hubs yet', description: `Hubs are group conversations where multiple ${brand.agentNamePlural} coordinate on a task, project, or topic. Prime coordinates the team.`, actionLabel: 'Create hub' },
  teams: { icon: Users, title: 'No teams', description: `Organize your ${brand.agentNamePlural} into teams with shared goals.`, actionLabel: 'Create team' },
  mcp: { icon: Plug, title: 'No MCP servers', description: 'Connect external tool servers via Model Context Protocol.', actionLabel: 'Add MCP server' },
  memories: { icon: BookOpen, title: 'No memories', description: `Memories are created automatically as ${brand.agentNamePlural} learn from conversations.` },
  approvals: { icon: ClipboardList, title: 'No pending approvals', description: 'Outbound actions requiring approval will appear here.' },
  audit: { icon: Shield, title: 'No audit events', description: 'All actions are logged here once agents start working.' },
  billing: { icon: BarChart3, title: 'No billing data', description: 'Usage costs and budgets will appear once agents start processing.' },
  goals: { icon: Target, title: 'No goals', description: 'Set goals for your agents to track progress.', actionLabel: 'Create goal' },
  drive: { icon: HardDrive, title: 'No files', description: 'Files uploaded by agents or users will appear here.' },
  notifications: { icon: Bell, title: 'No notifications', description: 'Notifications will appear here as events occur.' },
  pipeline: { icon: LayoutGrid, title: 'No pipeline stages', description: 'Create a pipeline to manage agent workflows visually.', actionLabel: 'Create pipeline' },
  routing: { icon: Route, title: 'No routing rules', description: 'Configure how incoming messages are routed to agents.', actionLabel: 'Add rule' },
  schedule: { icon: Calendar, title: 'No scheduled events', description: 'Schedule events and recurring tasks.', actionLabel: 'Create event' },
  providerKeys: { icon: Cpu, title: 'No provider keys', description: 'Add API keys for LLM providers like OpenAI, Anthropic, or use AWS Bedrock.', actionLabel: 'Add provider key' },
  training: { icon: BookOpen, title: 'No training data', description: 'Export conversation data for fine-tuning LLMs.' },
  orgChart: { icon: Users, title: 'No org chart', description: `Your ${brand.agentName} hierarchy will appear here once you create teams.` },
  outbound: { icon: Mail, title: 'No outbound messages', description: 'Messages pending review will appear here.' },
} as const;
