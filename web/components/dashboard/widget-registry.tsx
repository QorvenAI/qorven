'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import type React from 'react';

// --- Widget type definitions ---

export type WidgetType =
  | 'metric'
  | 'line'
  | 'area'
  | 'bar'
  | 'donut'
  | 'activity'
  | 'agents'
  | 'tasks'
  | 'heatmap'
  | 'progress'
  | 'external';

export interface WidgetConfig {
  id: string;
  title: string;
  type: WidgetType;
  dataSource: string;
  grid: { w: number; h: number };
  config?: {
    xKey?: string;
    yKey?: string;
    color?: string;
    prefix?: string;
    suffix?: string;
    aggregation?: 'sum' | 'avg' | 'count' | 'last';
    timeRange?: '1h' | '24h' | '7d' | '30d';
    showTrend?: boolean;
  };
}

export interface WidgetTypeMeta {
  type: WidgetType;
  name: string;
  description: string;
  icon: string;
  defaultGrid: { w: number; h: number };
}

export const WIDGET_TYPES: WidgetTypeMeta[] = [
  {
    type: 'metric',
    name: 'Metric Card',
    description: 'Single value with optional trend arrow',
    icon: '🔢',
    defaultGrid: { w: 3, h: 2 },
  },
  {
    type: 'line',
    name: 'Line Chart',
    description: 'Time-series line chart',
    icon: '📈',
    defaultGrid: { w: 6, h: 3 },
  },
  {
    type: 'area',
    name: 'Area Chart',
    description: 'Filled area chart for volume over time',
    icon: '📊',
    defaultGrid: { w: 6, h: 3 },
  },
  {
    type: 'bar',
    name: 'Bar Chart',
    description: 'Grouped or stacked bar chart',
    icon: '📉',
    defaultGrid: { w: 6, h: 3 },
  },
  {
    type: 'donut',
    name: 'Donut Chart',
    description: 'Proportional breakdown as a donut',
    icon: '🍩',
    defaultGrid: { w: 4, h: 3 },
  },
  {
    type: 'activity',
    name: 'Activity Feed',
    description: 'Scrollable list of recent events',
    icon: '📋',
    defaultGrid: { w: 4, h: 4 },
  },
  {
    type: 'agents',
    name: 'Agents Grid',
    description: 'Compact status grid of all agents',
    icon: '🤖',
    defaultGrid: { w: 4, h: 3 },
  },
  {
    type: 'tasks',
    name: 'Tasks & Approvals',
    description: 'Pending approval requests with action buttons',
    icon: '✅',
    defaultGrid: { w: 6, h: 4 },
  },
  {
    type: 'heatmap',
    name: 'Heatmap',
    description: 'Activity intensity grid by day and hour',
    icon: '🗓️',
    defaultGrid: { w: 8, h: 3 },
  },
  {
    type: 'progress',
    name: 'Progress Gauge',
    description: 'Circular or linear progress indicator',
    icon: '🎯',
    defaultGrid: { w: 3, h: 2 },
  },
  {
    type: 'external',
    name: 'External Data',
    description: 'Embed a custom data source or URL',
    icon: '🔗',
    defaultGrid: { w: 6, h: 3 },
  },
];

// --- Widget registry ---

// Populated by each widget file via registerWidget() at module level.
// dashboard-grid.tsx imports all widget files to trigger self-registration.
export const WidgetRegistry: Partial<Record<WidgetType, React.ComponentType<{ config: WidgetConfig }>>> = {};

export function registerWidget(
  type: WidgetType,
  component: React.ComponentType<{ config: WidgetConfig }>,
): void {
  WidgetRegistry[type] = component;
}
