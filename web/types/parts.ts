// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

// Message Parts — typed segments of assistant messages
// Each part renders with its own component. No more parsing markdown to detect cards.

export interface MessagePart {
  type: 'text' | 'reasoning' | 'tool-call' | 'tool-result' | 'widget' | 'source' | 'code' | 'image' | 'error';
  // Text / Reasoning
  content?: string;
  // Tool
  toolName?: string;
  toolCallId?: string;
  toolArgs?: any;
  toolResult?: any;
  duration?: number;
  // Widget
  widgetType?: string;
  widgetData?: any;
  // Source
  sources?: { index: number; title: string; url: string }[];
  // Code
  language?: string;
  code?: string;
  output?: string;
  exitCode?: number;
  // Image
  url?: string;
  alt?: string;
}
