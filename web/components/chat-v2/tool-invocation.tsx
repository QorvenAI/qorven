'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { useState } from 'react';
import { MailRuleCard, isMailCard } from './mail-rule-card';
import { ChevronDown, ChevronRight, Loader2, CheckCircle2, XCircle, Wrench,
  Terminal, FileText, FilePen, Globe, Zap, Brain, Search, Mail,
  Plane, Eye, MessageSquare, Send, Shield, Code, FolderSearch, FileSearch,
  Bug, GitBranch, Image, Mic, BookOpen, Users, Bot, CalendarDays,
  Plug, Rocket, Sparkles, Share2, MessageCircle, ListTodo, Radio } from 'lucide-react';
import { cn } from '@/lib/utils';

type IconType = typeof Wrench;

const toolMeta: Record<string, { icon: IconType; color: string; label: string }> = {
  exec:             { icon: Terminal,     color: 'text-primary/70 bg-primary/10',       label: 'Terminal' },
  read_file:        { icon: FileText,     color: 'text-blue-400 bg-blue-400/10',        label: 'Read file' },
  write_file:       { icon: FilePen,      color: 'text-amber-400 bg-amber-400/10',      label: 'Write file' },
  list_files:       { icon: FileText,     color: 'text-blue-400 bg-blue-400/10',        label: 'List files' },
  edit:             { icon: FilePen,      color: 'text-amber-400 bg-amber-400/10',      label: 'Edit file' },
  glob:             { icon: FolderSearch, color: 'text-primary bg-primary/10',          label: 'Find files' },
  grep:             { icon: FileSearch,   color: 'text-primary bg-primary/10',          label: 'Search code' },
  diagnostics:      { icon: Bug,          color: 'text-red-400 bg-red-400/10',          label: 'Diagnostics' },
  apply_patch:      { icon: GitBranch,    color: 'text-amber-400 bg-amber-400/10',      label: 'Apply patch' },
  lsp:              { icon: Code,         color: 'text-primary bg-primary/10',          label: 'Code nav' },
  web_search:       { icon: Search,       color: 'text-emerald-400 bg-emerald-400/10',  label: 'Web search' },
  web_fetch:        { icon: Globe,        color: 'text-emerald-400 bg-emerald-400/10',  label: 'Web fetch' },
  crawl:            { icon: Globe,        color: 'text-emerald-400 bg-emerald-400/10',  label: 'Crawl site' },
  research:         { icon: Eye,          color: 'text-emerald-400 bg-emerald-400/10',  label: 'Deep research' },
  browse_and_act:   { icon: Eye,          color: 'text-emerald-400 bg-emerald-400/10',  label: 'Browse & act' },
  memory_search:    { icon: Brain,        color: 'text-pink-400 bg-pink-400/10',        label: 'Memory search' },
  memory_get:       { icon: Brain,        color: 'text-pink-400 bg-pink-400/10',        label: 'Memory' },
  knowledge_graph_search: { icon: Share2, color: 'text-pink-400 bg-pink-400/10',        label: 'Knowledge graph' },
  email_send:       { icon: Mail,         color: 'text-cyan-400 bg-cyan-400/10',        label: 'Send email' },
  email_read:       { icon: Mail,         color: 'text-cyan-400 bg-cyan-400/10',        label: 'Read email' },
  set_mail_rule:    { icon: Mail,         color: 'text-cyan-400 bg-cyan-400/10',        label: 'Mail rule' },
  set_mail_policy:  { icon: Mail,         color: 'text-cyan-400 bg-cyan-400/10',        label: 'Mail policy' },
  send_telegram:    { icon: Send,         color: 'text-cyan-400 bg-cyan-400/10',        label: 'Telegram' },
  message:          { icon: MessageCircle,color: 'text-cyan-400 bg-cyan-400/10',        label: 'Message' },
  delegate:         { icon: Zap,          color: 'text-orange-400 bg-orange-400/10',    label: 'Delegate' },
  delegate_to_soul: { icon: Zap,          color: 'text-orange-400 bg-orange-400/10',    label: 'Delegate' },
  spawn:            { icon: Bot,          color: 'text-orange-400 bg-orange-400/10',    label: 'Spawn agent' },
  list_agents:      { icon: Users,        color: 'text-orange-400 bg-orange-400/10',    label: 'List agents' },
  flight_search:    { icon: Plane,        color: 'text-sky-400 bg-sky-400/10',          label: 'Flight search' },
  create_image:     { icon: Image,        color: 'text-violet-400 bg-violet-400/10',    label: 'Generate image' },
  tts:              { icon: Mic,          color: 'text-violet-400 bg-violet-400/10',    label: 'Text-to-speech' },
  task_create:      { icon: ListTodo,     color: 'text-indigo-400 bg-indigo-400/10',    label: 'Create task' },
  task_update:      { icon: ListTodo,     color: 'text-indigo-400 bg-indigo-400/10',    label: 'Update task' },
  cron:             { icon: CalendarDays, color: 'text-indigo-400 bg-indigo-400/10',    label: 'Schedule' },
  execute_action:   { icon: Plug,         color: 'text-fuchsia-400 bg-fuchsia-400/10',  label: 'Run action' },
  list_mcp_tools:   { icon: Rocket,       color: 'text-fuchsia-400 bg-fuchsia-400/10',  label: 'MCP tools' },
  self_improve:     { icon: Sparkles,     color: 'text-yellow-400 bg-yellow-400/10',    label: 'Self-improve' },
  room_post:        { icon: Radio,        color: 'text-blue-400 bg-blue-400/10',        label: 'Room post' },
  room_list:        { icon: MessageSquare,color: 'text-blue-400 bg-blue-400/10',        label: 'Room list' },
  book_read:        { icon: BookOpen,     color: 'text-amber-400 bg-amber-400/10',      label: 'Read book' },
  shield:           { icon: Shield,       color: 'text-red-400 bg-red-400/10',          label: 'Security' },
};

function getToolMeta(toolName: string) {
  return toolMeta[toolName] ?? { icon: Wrench, color: 'text-muted-foreground bg-muted', label: toolName };
}

function formatArgs(input: unknown): string {
  if (!input || (typeof input === 'object' && Object.keys(input as object).length === 0)) return '';
  try { return JSON.stringify(input, null, 2); } catch { return String(input); }
}

function formatOutput(output: unknown): string {
  if (output === null || output === undefined) return '';
  if (typeof output === 'string') return output;
  try { return JSON.stringify(output, null, 2); } catch { return String(output); }
}

type InvocationState = 'calling' | 'result' | 'error';

interface ToolInvocationProps {
  toolName: string;
  toolCallId: string;
  input?: unknown;
  output?: unknown;
  state: InvocationState;
  durationMs?: number;
  agentId?: string;
}

export function ToolInvocation({ toolName, input, output, state, durationMs, agentId }: ToolInvocationProps) {
  const [expanded, setExpanded] = useState(false);
  const { icon: Icon, color, label } = getToolMeta(toolName);

  const args = formatArgs(input);
  const result = formatOutput(output);
  const hasDetails = args || result;

  return (
    <div className="my-1.5 rounded-xl border border-border/60 bg-card/40 overflow-hidden text-sm">
      <button
        type="button"
        onClick={() => hasDetails && setExpanded(!expanded)}
        className={cn(
          'flex w-full items-center gap-2.5 px-3 py-2.5 text-left transition-colors',
          hasDetails ? 'hover:bg-accent/30 cursor-pointer' : 'cursor-default',
        )}
      >
        {/* Tool icon */}
        <span className={cn('flex h-6 w-6 shrink-0 items-center justify-center rounded-md text-xs', color)}>
          <Icon className="h-3.5 w-3.5" />
        </span>

        {/* Name + label */}
        <span className="flex-1 min-w-0">
          <span className="font-medium text-foreground/90">{label}</span>
          {toolName !== label && (
            <span className="ml-1.5 font-mono text-2xs text-muted-foreground">{toolName}</span>
          )}
        </span>

        {/* Status */}
        <span className="flex items-center gap-1.5 text-2xs text-muted-foreground shrink-0">
          {state === 'calling' && (
            <>
              <Loader2 className="h-3 w-3 animate-spin text-amber-400" />
              <span className="text-amber-400">Running…</span>
            </>
          )}
          {state === 'result' && (
            <>
              <CheckCircle2 className="h-3 w-3 text-emerald-500" />
              {durationMs != null && <span>{durationMs}ms</span>}
            </>
          )}
          {state === 'error' && (
            <>
              <XCircle className="h-3 w-3 text-destructive" />
              <span className="text-destructive">Error</span>
            </>
          )}
          {hasDetails && (
            expanded
              ? <ChevronDown className="h-3 w-3 ml-0.5" />
              : <ChevronRight className="h-3 w-3 ml-0.5" />
          )}
        </span>
      </button>

      {/* Mail rule/policy confirmation card */}
      {state === 'result' && isMailCard(output) && agentId && (
        <div className="border-t border-border/40 px-3 py-3">
          <MailRuleCard agentId={agentId} output={typeof output === 'string' ? output : ''} />
        </div>
      )}

      {/* Expanded details */}
      {expanded && hasDetails && !isMailCard(output) && (
        <div className="border-t border-border/40 divide-y divide-border/20">
          {args && (
            <div className="px-3 py-2">
              <p className="text-2xs font-medium text-muted-foreground mb-1">Input</p>
              <pre className="text-xs text-foreground/80 font-mono whitespace-pre-wrap break-words max-h-48 overflow-y-auto">
                {args}
              </pre>
            </div>
          )}
          {result && (
            <div className="px-3 py-2">
              <p className="text-2xs font-medium text-muted-foreground mb-1">Output</p>
              <pre className={cn(
                'text-xs font-mono whitespace-pre-wrap break-words max-h-64 overflow-y-auto',
                state === 'error' ? 'text-destructive/90' : 'text-foreground/80',
              )}>
                {result}
              </pre>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
