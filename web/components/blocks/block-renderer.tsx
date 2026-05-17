'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { cn } from '@/lib/utils';
import type { Block, BlockConfig, LayoutType } from './types';
import { StatCard, StatRow } from './stat-blocks';
import { DataTable } from './data-table';
import { ChartBlock } from './chart-block';
import { KanbanBoard } from './kanban-block';
import { ListBlock, FeedBlock, TimelineBlock } from './list-blocks';
import { PipelineBlock, ContactsBlock, ProgressBlock } from './domain-blocks';
import { DynamicForm } from './form-block';
import { CalendarBlock, TabsBlock, AccordionBlock, FileUploadBlock, AlertBlock, SkeletonBlock } from './ui-blocks';
import ReactMarkdown from 'react-markdown';

const layoutClasses: Record<LayoutType, string> = {
  'single': 'grid grid-cols-1 gap-4',
  'grid-2col': 'grid grid-cols-1 md:grid-cols-2 gap-4',
  'grid-3col': 'grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4',
  'grid-4col': 'grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4',
  'sidebar-left': 'grid grid-cols-1 md:grid-cols-[280px_1fr] gap-4',
  'sidebar-right': 'grid grid-cols-1 md:grid-cols-[1fr_280px] gap-4',
};

export function BlockRenderer({ config }: { config: BlockConfig }) {
  return (
    <div className="space-y-4">
      {config.title && <h2 className="text-lg font-semibold">{config.title}</h2>}
      <div className={layoutClasses[config.layout] || layoutClasses.single}>
        {config.blocks.map((block, i) => (
          <div key={block.title || i} className={cn(block.span ? `col-span-${block.span}` : '', block.className)}>
            <BlockSwitch block={block} />
          </div>
        ))}
      </div>
    </div>
  );
}

function BlockSwitch({ block }: { block: Block | any }) {
  switch (block.type) {
    case 'stat-card': return <StatCard {...block} />;
    case 'stat-row': return <StatRow stats={block.stats} />;
    case 'data-table': return <DataTable {...block} />;
    case 'chart': return <ChartBlock {...block} />;
    case 'kanban': return <KanbanBoard {...block} />;
    case 'list': return <ListBlock {...block} />;
    case 'feed': return <FeedBlock {...block} />;
    case 'timeline': return <TimelineBlock {...block} />;
    case 'form': return <DynamicForm {...block} />;
    case 'pipeline': return <PipelineBlock {...block} />;
    case 'contacts': return <ContactsBlock {...block} />;
    case 'progress': return <ProgressBlock {...block} />;
    case 'calendar': return <CalendarBlock {...block} />;
    case 'tabs': return <TabsBlock {...block} />;
    case 'accordion': return <AccordionBlock {...block} />;
    case 'file-upload': return <FileUploadBlock {...block} />;
    case 'alert': return <AlertBlock type={block.alertType} title={block.title} message={block.message} />;
    case 'skeleton': return <SkeletonBlock rows={block.rows} type={block.skeletonType} />;
    case 'markdown': return <div className="prose prose-sm prose-invert max-w-none"><ReactMarkdown>{block.content}</ReactMarkdown></div>;
    case 'embed': return <iframe src={block.url} className="w-full rounded-lg border border-border" style={{ height: block.height || 400 }} />;
    default: return <div className="rounded-lg border border-border p-4 text-sm text-muted-foreground">Unknown block: {block.type}</div>;
  }
}

export { BlockRenderer as default };
