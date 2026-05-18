'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useState } from 'react';
import { cn } from '@/lib/utils';
import { FileChangeChip } from '@/components/code/file-change-chip';
import {
  Terminal, FileText, FilePen, Globe, Zap, Brain, Wrench, Search, Mail,
  ChevronDown, ChevronRight, Plane, Compass, Eye, MessageSquare, Send,
  Clock, Shield, Code, Undo2, FolderSearch, FileSearch, Bug, GitBranch,
  Image, FileAudio, Mic, BookOpen, Users, Bot, Moon, CalendarDays,
  Plug, HelpCircle, Download, Rocket, FileCheck, BookMarked, Sparkles,
  Share2, DoorOpen, DoorClosed, MessageCircle, ListTodo, Radio
} from 'lucide-react';

const toolMeta: Record<string, { icon: typeof Wrench; color: string; label: string }> = {
  // Filesystem
  exec:           { icon: Terminal, color: 'text-primary/70 bg-primary/10', label: 'Terminal' },
  read_file:      { icon: FileText, color: 'text-blue-400 bg-blue-400/10', label: 'Read file' },
  write_file:     { icon: FilePen, color: 'text-amber-400 bg-amber-400/10', label: 'Write file' },
  list_files:     { icon: FileText, color: 'text-blue-400 bg-blue-400/10', label: 'List files' },
  edit:           { icon: FilePen, color: 'text-amber-400 bg-amber-400/10', label: 'Edit file' },

  // Coding (from OpenCode)
  glob:           { icon: FolderSearch, color: 'text-primary bg-primary/10', label: 'Find files' },
  grep:           { icon: FileSearch, color: 'text-primary bg-primary/10', label: 'Search code' },
  diagnostics:    { icon: Bug, color: 'text-red-400 bg-red-400/10', label: 'Diagnostics' },
  apply_patch:    { icon: GitBranch, color: 'text-amber-400 bg-amber-400/10', label: 'Apply patch' },
  undo:           { icon: Undo2, color: 'text-amber-400 bg-amber-400/10', label: 'Undo' },
  lsp:            { icon: Code, color: 'text-primary bg-primary/10', label: 'Code navigation' },

  // Web & Research
  web_search:     { icon: Search, color: 'text-emerald-400 bg-emerald-400/10', label: 'Web search' },
  web_fetch:      { icon: Globe, color: 'text-emerald-400 bg-emerald-400/10', label: 'Web fetch' },
  crawl:          { icon: Globe, color: 'text-emerald-400 bg-emerald-400/10', label: 'Crawl site' },
  scrape:         { icon: Globe, color: 'text-emerald-400 bg-emerald-400/10', label: 'Scrape page' },
  research:       { icon: Compass, color: 'text-emerald-400 bg-emerald-400/10', label: 'Deep research' },
  browse_and_act: { icon: Eye, color: 'text-emerald-400 bg-emerald-400/10', label: 'Browse & act' },
  browser:        { icon: Eye, color: 'text-emerald-400 bg-emerald-400/10', label: 'Browser' },

  // Memory & Knowledge
  memory_search:  { icon: Brain, color: 'text-pink-400 bg-pink-400/10', label: 'Memory search' },
  memory_get:     { icon: Brain, color: 'text-pink-400 bg-pink-400/10', label: 'Memory' },
  knowledge_graph_search: { icon: Share2, color: 'text-pink-400 bg-pink-400/10', label: 'Knowledge graph' },

  // Communication
  email_send:     { icon: Mail, color: 'text-cyan-400 bg-cyan-400/10', label: 'Send email' },
  email_read:     { icon: Mail, color: 'text-cyan-400 bg-cyan-400/10', label: 'Read email' },
  send_dm:        { icon: MessageSquare, color: 'text-cyan-400 bg-cyan-400/10', label: 'Send DM' },
  send_telegram:  { icon: Send, color: 'text-cyan-400 bg-cyan-400/10', label: 'Telegram' },
  message:        { icon: MessageCircle, color: 'text-cyan-400 bg-cyan-400/10', label: 'Message' },

  // Multi-Agent
  delegate:       { icon: Zap, color: 'text-orange-400 bg-orange-400/10', label: 'Delegate' },
  delegate_to_soul: { icon: Zap, color: 'text-orange-400 bg-orange-400/10', label: 'Delegate' },
  handoff_to_soul:  { icon: Zap, color: 'text-orange-400 bg-orange-400/10', label: 'Handoff' },
  list_agents:    { icon: Users, color: 'text-orange-400 bg-orange-400/10', label: 'List agents' },
  spawn:          { icon: Bot, color: 'text-orange-400 bg-orange-400/10', label: 'Spawn agent' },

  // Travel
  flight_search:  { icon: Plane, color: 'text-sky-400 bg-sky-400/10', label: 'Flight search' },
  datetime:       { icon: Clock, color: 'text-muted-foreground bg-muted', label: 'Date/time' },

  // Social
  social_monitor: { icon: Radio, color: 'text-rose-400 bg-rose-400/10', label: 'Social monitor' },

  // Scheduling
  cron:           { icon: CalendarDays, color: 'text-indigo-400 bg-indigo-400/10', label: 'Schedule' },
  heartbeat:      { icon: Clock, color: 'text-indigo-400 bg-indigo-400/10', label: 'Heartbeat' },

  // Self-building
  self_knowledge: { icon: Shield, color: 'text-purple-400 bg-purple-400/10', label: 'Self-knowledge' },
  self_patch:     { icon: GitBranch, color: 'text-purple-400 bg-purple-400/10', label: 'Self-patch' },
  project:        { icon: Code, color: 'text-purple-400 bg-purple-400/10', label: 'Project' },

  // Skills
  skill_search:   { icon: BookOpen, color: 'text-teal-400 bg-teal-400/10', label: 'Search skills' },
  skill_manage:   { icon: BookOpen, color: 'text-teal-400 bg-teal-400/10', label: 'Manage skill' },
  use_skill:      { icon: Sparkles, color: 'text-teal-400 bg-teal-400/10', label: 'Use skill' },

  // Media
  read_image:     { icon: Image, color: 'text-fuchsia-400 bg-fuchsia-400/10', label: 'Analyze image' },
  read_document:  { icon: FileText, color: 'text-fuchsia-400 bg-fuchsia-400/10', label: 'Read document' },
  create_image:   { icon: Image, color: 'text-fuchsia-400 bg-fuchsia-400/10', label: 'Generate image' },
  tts:            { icon: Mic, color: 'text-fuchsia-400 bg-fuchsia-400/10', label: 'Text to speech' },

  // Team
  team_tasks:     { icon: ListTodo, color: 'text-lime-400 bg-lime-400/10', label: 'Team tasks' },
  team_message:   { icon: MessageSquare, color: 'text-lime-400 bg-lime-400/10', label: 'Team message' },
  join_room:      { icon: DoorOpen, color: 'text-lime-400 bg-lime-400/10', label: 'Join room' },
  leave_room:     { icon: DoorClosed, color: 'text-lime-400 bg-lime-400/10', label: 'Leave room' },
  room_post:      { icon: MessageCircle, color: 'text-lime-400 bg-lime-400/10', label: 'Room post' },

  // Connectors & Utils
  execute_action: { icon: Plug, color: 'text-yellow-400 bg-yellow-400/10', label: 'Connector action' },
  clarify:        { icon: HelpCircle, color: 'text-muted-foreground bg-muted', label: 'Clarify' },
  qorven_download: { icon: Download, color: 'text-muted-foreground bg-muted', label: 'Download' },
  qorven_fly:     { icon: Rocket, color: 'text-muted-foreground bg-muted', label: 'Deploy' },
  qorven_lint:    { icon: FileCheck, color: 'text-muted-foreground bg-muted', label: 'Lint' },
  qorven_report:  { icon: FileText, color: 'text-muted-foreground bg-muted', label: 'Report' },
  qorven_wiki:    { icon: BookMarked, color: 'text-muted-foreground bg-muted', label: 'Wiki' },

  // GitHub — autonomous dev loop
  gh_repo_info:      { icon: GitBranch, color: 'text-sky-400 bg-sky-400/10', label: 'Repo info' },
  gh_list_issues:    { icon: GitBranch, color: 'text-sky-400 bg-sky-400/10', label: 'List issues' },
  gh_read_issue:     { icon: GitBranch, color: 'text-sky-400 bg-sky-400/10', label: 'Read issue' },
  gh_create_issue:   { icon: GitBranch, color: 'text-sky-400 bg-sky-400/10', label: 'Create issue' },
  gh_create_branch:  { icon: GitBranch, color: 'text-amber-400 bg-amber-400/10', label: 'Create branch' },
  gh_push_file:      { icon: FilePen,   color: 'text-amber-400 bg-amber-400/10', label: 'Push file' },
  gh_open_pr:        { icon: GitBranch, color: 'text-emerald-400 bg-emerald-400/10', label: 'Open PR' },
  gh_post_comment:   { icon: MessageCircle, color: 'text-sky-400 bg-sky-400/10', label: 'Comment' },
  gh_list_pr_checks: { icon: FileCheck, color: 'text-emerald-400 bg-emerald-400/10', label: 'CI checks' },
  gh_merge_pr:       { icon: GitBranch, color: 'text-emerald-400 bg-emerald-400/10', label: 'Merge PR' },
  gh_task_register:  { icon: Rocket,    color: 'text-violet-400 bg-violet-400/10', label: 'Start task' },

  // QOROS
  sleep:          { icon: Moon, color: 'text-indigo-400 bg-indigo-400/10', label: 'Sleep' },
  daily_log:      { icon: CalendarDays, color: 'text-indigo-400 bg-indigo-400/10', label: 'Daily log' },

  // Sessions
  sessions_list:    { icon: FileText, color: 'text-muted-foreground bg-muted', label: 'Sessions' },
  sessions_history: { icon: FileText, color: 'text-muted-foreground bg-muted', label: 'History' },
  session_status:   { icon: FileText, color: 'text-muted-foreground bg-muted', label: 'Status' },
};

// Detect diff content in tool results
function isDiffContent(text: string): boolean {
  return text.includes('--- a/') || text.includes('+++ b/') || text.includes('*** Begin Patch');
}

function DiffView({ content }: { content: string }) {
  const lines = content.split('\n');
  return (
    <pre className="rounded-lg bg-muted px-2.5 py-2 text-2sm overflow-x-auto max-h-60 font-mono">
      {lines.map((line, i) => {
        let cls = '';
        if (line.startsWith('+') && !line.startsWith('+++')) cls = 'text-emerald-400 bg-emerald-400/5';
        else if (line.startsWith('-') && !line.startsWith('---')) cls = 'text-red-400 bg-red-400/5';
        else if (line.startsWith('@@')) cls = 'text-blue-400';
        else if (line.startsWith('***')) cls = 'text-yellow-400 font-semibold';
        return <div key={i} className={cls}>{line}</div>;
      })}
    </pre>
  );
}

interface ToolCallBlockProps {
  name: string;
  args?: string;
  result?: string;
}

const FILE_WRITE_TOOLS = new Set(['write_file', 'edit', 'apply_patch', 'gh_push_file']);

function extractFilePath(toolName: string, args?: string): string | null {
  if (!args || !FILE_WRITE_TOOLS.has(toolName)) return null;
  try {
    const parsed = JSON.parse(args);
    return parsed.path || parsed.file_path || null;
  } catch { return null; }
}

export function ToolCallBlock({ name, args, result }: ToolCallBlockProps) {
  const [expanded, setExpanded] = useState(false);
  const meta = toolMeta[name] ?? { icon: Wrench, color: 'text-muted-foreground bg-muted', label: name.replace(/_/g, ' ') };
  const Icon = meta.icon;
  const isDone = !!result;
  const hasDiff = result ? isDiffContent(result) : false;
  const filePath = extractFilePath(name, args);

  // For file-write tools with a known path, render a Windsurf-style chip
  if (filePath) {
    const linesAdded = (() => {
      if (!result) return 0;
      const m = result.match(/\+(\d+)\s+lines?/);
      return m ? parseInt(m[1]!, 10) : 0;
    })();
    return (
      <FileChangeChip
        path={filePath}
        linesAdded={linesAdded}
        linesRemoved={0}
      />
    );
  }

  return (
    <button onClick={() => setExpanded(!expanded)}
      className="w-full text-left rounded-xl border border-border bg-card/50 px-3 py-2 text-xs transition-colors hover:bg-card">
      <div className="flex items-center gap-2.5">
        <span className={cn('flex h-5 w-5 items-center justify-center rounded-md', meta.color)}>
          <Icon className="h-3 w-3" />
        </span>
        <span className="font-medium text-foreground">{meta.label}</span>
        <span className="ml-auto flex items-center gap-1">
          {isDone
            ? <span className="text-emerald-400 text-2xs font-medium">✓ Done</span>
            : <span className="flex items-center gap-1 text-amber-400 text-2xs">
                <span className="h-1.5 w-1.5 animate-pulse rounded-full bg-amber-400" />Running
              </span>}
          {expanded ? <ChevronDown className="h-3 w-3 text-muted-foreground ml-1" /> : <ChevronRight className="h-3 w-3 text-muted-foreground ml-1" />}
        </span>
      </div>
      {expanded && (
        <div className="mt-2 space-y-1.5 border-t border-border pt-2">
          {args && (
            <div>
              <p className="text-2xs font-medium text-muted-foreground uppercase mb-0.5">Input</p>
              <pre className="rounded-lg bg-muted px-2.5 py-2 text-2sm overflow-x-auto max-h-32">{args}</pre>
            </div>
          )}
          {result && (
            <div>
              <p className="text-2xs font-medium text-muted-foreground uppercase mb-0.5">Output</p>
              {hasDiff ? <DiffView content={result} /> : (
                <pre className="rounded-lg bg-muted px-2.5 py-2 text-2sm overflow-x-auto max-h-40">{result}</pre>
              )}
            </div>
          )}
        </div>
      )}
    </button>
  );
}
