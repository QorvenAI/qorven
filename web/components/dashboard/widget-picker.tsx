'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useState } from 'react';
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetDescription,
  SheetFooter,
} from '@/components/ui/sheet';
import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';
import { WIDGET_TYPES, type WidgetConfig, type WidgetType } from './widget-registry';
import { AVAILABLE_DATA_SOURCES } from '@/contexts/dashboard-data';

interface WidgetPickerProps {
  open: boolean;
  onClose: () => void;
  onAdd: (config: WidgetConfig) => void;
}

export function WidgetPicker({ open, onClose, onAdd }: WidgetPickerProps) {
  const [selectedType, setSelectedType] = useState<WidgetType>('metric');
  const [selectedSource, setSelectedSource] = useState<string>(Object.keys(AVAILABLE_DATA_SOURCES)[0] ?? '');
  const [title, setTitle] = useState('');

  const meta = WIDGET_TYPES.find((t) => t.type === selectedType);

  function handleAdd() {
    if (!meta) return;
    const config: WidgetConfig = {
      id: crypto.randomUUID(),
      title: title.trim() || meta.name,
      type: selectedType,
      dataSource: selectedSource,
      grid: { w: meta.defaultGrid.w, h: meta.defaultGrid.h },
    };
    onAdd(config);
    onClose();
    // Reset state
    setTitle('');
    setSelectedType('metric');
    setSelectedSource(Object.keys(AVAILABLE_DATA_SOURCES)[0] ?? '');
  }

  return (
    <Sheet open={open} onOpenChange={(v) => { if (!v) onClose(); }}>
      <SheetContent side="right" className="w-full sm:max-w-md flex flex-col gap-0 p-0 overflow-y-auto">
        <SheetHeader className="px-5 py-4 border-b border-border">
          <SheetTitle>Add Widget</SheetTitle>
          <SheetDescription>Choose a widget type and data source, then add it to your dashboard.</SheetDescription>
        </SheetHeader>

        <div className="flex-1 overflow-y-auto px-5 py-4 space-y-5">
          {/* Widget title */}
          <div className="space-y-1.5">
            <label className="text-xs font-medium text-foreground">Widget Title</label>
            <input
              type="text"
              value={title}
              onChange={(e) => setTitle(e.target.value)}
              placeholder={meta?.name ?? 'Widget title'}
              className="w-full h-9 rounded-lg border border-input bg-background px-3 text-sm outline-none focus:ring-2 focus:ring-ring"
            />
          </div>

          {/* Widget type grid */}
          <div className="space-y-1.5">
            <label className="text-xs font-medium text-foreground">Widget Type</label>
            <div className="grid grid-cols-2 gap-2">
              {WIDGET_TYPES.map((wt) => (
                <button
                  key={wt.type}
                  onClick={() => setSelectedType(wt.type)}
                  className={cn(
                    'flex flex-col items-start gap-1 rounded-lg border p-3 text-left transition-colors',
                    selectedType === wt.type
                      ? 'border-primary bg-primary/5 text-foreground'
                      : 'border-border bg-background text-muted-foreground hover:border-muted-foreground hover:text-foreground',
                  )}
                >
                  <span className="text-xl leading-none">{wt.icon}</span>
                  <span className="text-xs font-medium leading-tight">{wt.name}</span>
                  <span className="text-[10px] leading-tight">{wt.description}</span>
                  <span className="text-[10px] text-muted-foreground">
                    Default: {wt.defaultGrid.w}×{wt.defaultGrid.h}
                  </span>
                </button>
              ))}
            </div>
          </div>

          {/* Data source */}
          <div className="space-y-1.5">
            <label className="text-xs font-medium text-foreground">Data Source</label>
            <select
              value={selectedSource}
              onChange={(e) => setSelectedSource(e.target.value)}
              className="w-full h-9 rounded-lg border border-input bg-background px-3 text-sm outline-none focus:ring-2 focus:ring-ring"
            >
              {Object.entries(AVAILABLE_DATA_SOURCES).map(([key, desc]) => (
                <option key={key} value={key}>
                  {key} — {desc}
                </option>
              ))}
            </select>
          </div>
        </div>

        <SheetFooter className="px-5 py-4 border-t border-border">
          <Button variant="ghost" onClick={onClose} className="mr-2">
            Cancel
          </Button>
          <Button variant="primary" onClick={handleAdd} disabled={!selectedType || !selectedSource}>
            Add to Dashboard
          </Button>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  );
}
