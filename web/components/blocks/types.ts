// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

// Block system types — JSON-driven dashboard composition

export type LayoutType = 'single' | 'grid-2col' | 'grid-3col' | 'grid-4col' | 'sidebar-left' | 'sidebar-right';

export interface BlockConfig {
  id?: string;
  title?: string;
  layout: LayoutType;
  blocks: Block[];
}

export type Block =
  | StatCardBlock
  | StatRowBlock
  | DataTableBlock
  | ChartBlock
  | KanbanBlock
  | ListBlock
  | FeedBlock
  | TimelineBlock
  | FormBlock
  | CalendarBlock
  | PipelineBlock
  | ContactsBlock
  | MarkdownBlock
  | EmbedBlock
  | ProgressBlock;

interface BaseBlock { type: string; title?: string; className?: string; span?: number }

export interface StatCardBlock extends BaseBlock {
  type: 'stat-card';
  value: string | number;
  label: string;
  change?: string;
  changeType?: 'up' | 'down' | 'neutral';
  icon?: string;
  color?: string;
}

export interface StatRowBlock extends BaseBlock {
  type: 'stat-row';
  stats: Omit<StatCardBlock, 'type'>[];
}

export interface DataTableBlock extends BaseBlock {
  type: 'data-table';
  columns: { key: string; label: string; sortable?: boolean; align?: 'left' | 'center' | 'right' }[];
  rows: Record<string, any>[];
  pageSize?: number;
  searchable?: boolean;
}

export interface ChartBlock extends BaseBlock {
  type: 'chart';
  chartType: 'bar' | 'line' | 'area' | 'pie' | 'donut';
  data: { name: string; value: number; [key: string]: any }[];
  xKey?: string;
  yKey?: string;
  colors?: string[];
  height?: number;
}

export interface KanbanBlock extends BaseBlock {
  type: 'kanban';
  columns: { id: string; title: string; color?: string }[];
  items: { id: string; columnId: string; title: string; description?: string; tags?: string[]; avatar?: string }[];
}

export interface ListBlock extends BaseBlock {
  type: 'list';
  items: { id: string; title: string; subtitle?: string; avatar?: string; badge?: string; badgeColor?: string; action?: string }[];
}

export interface FeedBlock extends BaseBlock {
  type: 'feed';
  items: { id: string; actor: string; action: string; target?: string; time: string; avatar?: string }[];
}

export interface TimelineBlock extends BaseBlock {
  type: 'timeline';
  events: { date: string; title: string; description?: string; color?: string }[];
}

export interface FormBlock extends BaseBlock {
  type: 'form';
  fields: { name: string; label: string; type: 'text' | 'email' | 'number' | 'select' | 'textarea' | 'date' | 'toggle'; options?: string[]; required?: boolean; placeholder?: string }[];
  submitLabel?: string;
}

export interface CalendarBlock extends BaseBlock {
  type: 'calendar';
  events: { date: string; title: string; color?: string; time?: string }[];
}

export interface PipelineBlock extends BaseBlock {
  type: 'pipeline';
  stages: { name: string; count: number; value?: string; color?: string }[];
}

export interface ContactsBlock extends BaseBlock {
  type: 'contacts';
  contacts: { id: string; name: string; company?: string; email?: string; avatar?: string; lastContact?: string; status?: string }[];
}

export interface MarkdownBlock extends BaseBlock {
  type: 'markdown';
  content: string;
}

export interface EmbedBlock extends BaseBlock {
  type: 'embed';
  url: string;
  height?: number;
}

export interface ProgressBlock extends BaseBlock {
  type: 'progress';
  steps: { label: string; status: 'done' | 'active' | 'pending' }[];
}

// Additional block types
export interface CalendarBlockType extends BaseBlock {
  type: 'calendar';
  events: { date: string; title: string; color?: string; time?: string }[];
}

export interface TabsBlockType extends BaseBlock {
  type: 'tabs';
  tabs: { id: string; label: string; content: string }[];
}

export interface AccordionBlockType extends BaseBlock {
  type: 'accordion';
  items: { id: string; title: string; content: string }[];
}

export interface FileUploadBlockType extends BaseBlock {
  type: 'file-upload';
  accept?: string;
  multiple?: boolean;
  maxSize?: string;
}

export interface AlertBlockType extends BaseBlock {
  type: 'alert';
  alertType: 'info' | 'success' | 'warning' | 'error';
  message: string;
}

export interface SkeletonBlockType extends BaseBlock {
  type: 'skeleton';
  rows?: number;
  skeletonType?: 'lines' | 'card' | 'table';
}
