// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

export interface FileNode {
  name: string; path: string; type: 'file' | 'dir'; children?: FileNode[];
}

export interface FileTab {
  path: string; name: string; content: string; dirty: boolean;
}

export interface ChatMsg {
  role: 'user' | 'assistant';
  content: string;
  streaming?: boolean;
  tools?: { name: string; args?: string; result?: string }[];
}

export interface CodeProject {
  id: string; name: string; path: string; session_id: string;
  tasks: any[]; notes: string;
  build_phase?: string; build_room_id?: string; build_plan?: string;
  display_name?: string; preview_url?: string;
}

export interface BuildEntry {
  type: 'text' | 'tool_start' | 'tool_result' | 'file_created' | 'file_chip' | 'pr_card' | 'error' | 'done';
  content: string;
  tool?: string;
  path?: string;
  ts?: number; // unix ms — set at push time for timeline duration calc
  // file_chip fields
  linesAdded?: number;
  linesRemoved?: number;
  totalLines?: number;
  // pr_card fields
  prUrl?: string;
  prTitle?: string;
  prNumber?: number;
  prRepo?: string;
}
