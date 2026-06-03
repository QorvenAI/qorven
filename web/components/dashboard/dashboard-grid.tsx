'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

// Import all widget files to trigger self-registration via registerWidget()
import '@/components/dashboard/widgets/metric';
import '@/components/dashboard/widgets/line-chart';
import '@/components/dashboard/widgets/bar-chart';
import '@/components/dashboard/widgets/activity-feed';
import '@/components/dashboard/widgets/agents-grid';
import '@/components/dashboard/widgets/tasks-approvals';
import '@/components/dashboard/widgets/donut';
import '@/components/dashboard/widgets/heatmap';
import '@/components/dashboard/widgets/external';
import '@/components/dashboard/widgets/progress';
import '@/components/dashboard/widgets/mini-chat';

import { useRef } from 'react';
import { GridLayout, useContainerWidth } from 'react-grid-layout';
import 'react-grid-layout/css/styles.css';

import { GripVertical, Settings, X, Plus } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';
import { WidgetRegistry, type WidgetConfig } from './widget-registry';
import type { Layout, LayoutItem } from 'react-grid-layout';

interface DashboardGridProps {
  layout: LayoutItem[];
  widgets: Record<string, WidgetConfig>;
  isEditing: boolean;
  onLayoutChange: (layout: Layout) => void;
  onRemoveWidget: (id: string) => void;
  onConfigWidget: (id: string) => void;
  onAddWidget: () => void;
}

const COLS = 12;
const ROW_HEIGHT = 60;
const ADD_WIDGET_ID = '__add_widget__';

export function DashboardGrid({
  layout,
  widgets,
  isEditing,
  onLayoutChange,
  onRemoveWidget,
  onConfigWidget,
  onAddWidget,
}: DashboardGridProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const { width, containerRef: widthRef } = useContainerWidth();

  // Combine the two refs
  const setRef = (el: HTMLDivElement | null) => {
    (containerRef as React.MutableRefObject<HTMLDivElement | null>).current = el;
    (widthRef as React.MutableRefObject<HTMLDivElement | null>).current = el;
  };

  // Add placeholder item in edit mode
  const gridLayout: LayoutItem[] = isEditing
    ? [
        ...layout,
        { i: ADD_WIDGET_ID, x: 0, y: 999, w: 3, h: 2, isResizable: false, isDraggable: false },
      ]
    : layout;

  function handleLayoutChange(newLayout: Layout) {
    // Filter out the add-widget placeholder
    const real = newLayout.filter((item) => item.i !== ADD_WIDGET_ID);
    onLayoutChange(real);
  }

  const containerWidth = width ?? 1200;

  return (
    <div ref={setRef} className="w-full">
      <GridLayout
        layout={gridLayout}
        width={containerWidth}
        gridConfig={{ cols: COLS, rowHeight: ROW_HEIGHT, margin: [12, 12] }}
        dragConfig={{
          enabled: isEditing,
          handle: '.drag-handle',
          threshold: 4,
          bounded: false,
        }}
        resizeConfig={{ enabled: isEditing }}
        onLayoutChange={handleLayoutChange}
        autoSize
      >
        {gridLayout.map((item) => {
          if (item.i === ADD_WIDGET_ID) {
            return (
              <div key={ADD_WIDGET_ID} className="flex">
                <button
                  onClick={onAddWidget}
                  className="w-full h-full flex flex-col items-center justify-center gap-1 rounded-xl border-2 border-dashed border-border text-muted-foreground hover:border-primary hover:text-primary transition-colors"
                >
                  <Plus className="size-5" />
                  <span className="text-xs font-medium">Add Widget</span>
                </button>
              </div>
            );
          }

          const config = widgets[item.i];
          if (!config) return <div key={item.i} />;

          const Widget = WidgetRegistry[config.type];

          return (
            <div key={item.i} className="relative group">
              {/* Drag handle — only visible in edit mode */}
              {isEditing && (
                <div className="drag-handle absolute top-0 inset-x-0 z-10 flex items-center justify-between px-3 py-1.5 bg-background/80 backdrop-blur-sm border-b border-border rounded-t-xl cursor-grab opacity-0 group-hover:opacity-100 transition-opacity">
                  <GripVertical className="size-3 text-muted-foreground" />
                  <span className="text-[10px] text-muted-foreground truncate mx-2">{config.title}</span>
                  <div className="flex items-center gap-1">
                    <button
                      onClick={() => onConfigWidget(item.i)}
                      className="size-5 flex items-center justify-center rounded hover:bg-accent text-muted-foreground hover:text-foreground"
                    >
                      <Settings className="size-3" />
                    </button>
                    <button
                      onClick={() => onRemoveWidget(item.i)}
                      className="size-5 flex items-center justify-center rounded hover:bg-destructive/10 text-muted-foreground hover:text-destructive"
                    >
                      <X className="size-3" />
                    </button>
                  </div>
                </div>
              )}

              {Widget ? (
                <div className={cn('w-full h-full', isEditing && 'pt-7')}>
                  <Widget config={config} />
                </div>
              ) : (
                <div className="w-full h-full flex items-center justify-center rounded-xl border border-border text-xs text-muted-foreground">
                  Unknown widget type: {config.type}
                </div>
              )}
            </div>
          );
        })}
      </GridLayout>
    </div>
  );
}
