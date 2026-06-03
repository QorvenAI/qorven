'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useState, useEffect } from 'react';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
  DialogBody,
} from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { WIDGET_TYPES, type WidgetConfig, type WidgetType } from './widget-registry';
import { AVAILABLE_DATA_SOURCES } from '@/contexts/dashboard-data';

interface WidgetConfigModalProps {
  open: boolean;
  widget: WidgetConfig | null;
  onClose: () => void;
  onSave: (config: WidgetConfig) => void;
  onRemove: (id: string) => void;
}

export function WidgetConfigModal({ open, widget, onClose, onSave, onRemove }: WidgetConfigModalProps) {
  const [form, setForm] = useState<WidgetConfig | null>(null);
  const [confirmRemove, setConfirmRemove] = useState(false);

  useEffect(() => {
    if (widget) {
      setForm({ ...widget, config: { ...widget.config } });
      setConfirmRemove(false);
    }
  }, [widget]);

  if (!form) return null;

  function set<K extends keyof WidgetConfig>(key: K, value: WidgetConfig[K]) {
    setForm((prev) => prev ? { ...prev, [key]: value } : prev);
  }

  function setConfigField<K extends keyof NonNullable<WidgetConfig['config']>>(
    key: K,
    value: NonNullable<WidgetConfig['config']>[K],
  ) {
    setForm((prev) =>
      prev ? { ...prev, config: { ...prev.config, [key]: value } } : prev,
    );
  }

  function handleSave() {
    if (form) { onSave(form); onClose(); }
  }

  function handleRemove() {
    if (!confirmRemove) { setConfirmRemove(true); return; }
    if (form) { onRemove(form.id); onClose(); }
  }

  return (
    <Dialog open={open} onOpenChange={(v) => { if (!v) onClose(); }}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Configure Widget</DialogTitle>
        </DialogHeader>

        <DialogBody className="space-y-4 overflow-y-auto max-h-[60vh]">
          {/* Title */}
          <div className="space-y-1">
            <label className="text-xs font-medium text-foreground">Title</label>
            <input
              type="text"
              value={form.title}
              onChange={(e) => set('title', e.target.value)}
              className="w-full h-9 rounded-lg border border-input bg-background px-3 text-sm outline-none focus:ring-2 focus:ring-ring"
            />
          </div>

          {/* Type */}
          <div className="space-y-1">
            <label className="text-xs font-medium text-foreground">Widget Type</label>
            <select
              value={form.type}
              onChange={(e) => set('type', e.target.value as WidgetType)}
              className="w-full h-9 rounded-lg border border-input bg-background px-3 text-sm outline-none focus:ring-2 focus:ring-ring"
            >
              {WIDGET_TYPES.map((wt) => (
                <option key={wt.type} value={wt.type}>
                  {wt.icon} {wt.name}
                </option>
              ))}
            </select>
          </div>

          {/* Data source */}
          <div className="space-y-1">
            <label className="text-xs font-medium text-foreground">Data Source</label>
            <select
              value={form.dataSource}
              onChange={(e) => set('dataSource', e.target.value)}
              className="w-full h-9 rounded-lg border border-input bg-background px-3 text-sm outline-none focus:ring-2 focus:ring-ring"
            >
              {Object.entries(AVAILABLE_DATA_SOURCES).map(([key, desc]) => (
                <option key={key} value={key}>
                  {key} — {desc}
                </option>
              ))}
            </select>
          </div>

          {/* Optional config fields */}
          <div className="grid grid-cols-2 gap-3">
            <div className="space-y-1">
              <label className="text-xs font-medium text-foreground">X Key</label>
              <input
                type="text"
                value={form.config?.xKey ?? ''}
                placeholder="e.g. date"
                onChange={(e) => setConfigField('xKey', e.target.value || undefined)}
                className="w-full h-9 rounded-lg border border-input bg-background px-3 text-sm outline-none focus:ring-2 focus:ring-ring"
              />
            </div>
            <div className="space-y-1">
              <label className="text-xs font-medium text-foreground">Y Key</label>
              <input
                type="text"
                value={form.config?.yKey ?? ''}
                placeholder="e.g. value"
                onChange={(e) => setConfigField('yKey', e.target.value || undefined)}
                className="w-full h-9 rounded-lg border border-input bg-background px-3 text-sm outline-none focus:ring-2 focus:ring-ring"
              />
            </div>
            <div className="space-y-1">
              <label className="text-xs font-medium text-foreground">Prefix</label>
              <input
                type="text"
                value={form.config?.prefix ?? ''}
                placeholder="e.g. $"
                onChange={(e) => setConfigField('prefix', e.target.value || undefined)}
                className="w-full h-9 rounded-lg border border-input bg-background px-3 text-sm outline-none focus:ring-2 focus:ring-ring"
              />
            </div>
            <div className="space-y-1">
              <label className="text-xs font-medium text-foreground">Suffix</label>
              <input
                type="text"
                value={form.config?.suffix ?? ''}
                placeholder="e.g. ms"
                onChange={(e) => setConfigField('suffix', e.target.value || undefined)}
                className="w-full h-9 rounded-lg border border-input bg-background px-3 text-sm outline-none focus:ring-2 focus:ring-ring"
              />
            </div>
          </div>

          {/* Show trend toggle (metric only) */}
          {form.type === 'metric' && (
            <div className="flex items-center gap-2">
              <input
                id="show-trend"
                type="checkbox"
                checked={form.config?.showTrend ?? false}
                onChange={(e) => setConfigField('showTrend', e.target.checked)}
                className="rounded border-input"
              />
              <label htmlFor="show-trend" className="text-xs font-medium text-foreground cursor-pointer">
                Show trend arrow
              </label>
            </div>
          )}
        </DialogBody>

        <DialogFooter className="flex-row items-center justify-between">
          <Button
            variant="destructive"
            size="sm"
            onClick={handleRemove}
          >
            {confirmRemove ? 'Confirm Remove' : 'Remove Widget'}
          </Button>
          <div className="flex gap-2">
            <Button variant="ghost" size="sm" onClick={onClose}>Cancel</Button>
            <Button variant="primary" size="sm" onClick={handleSave}>Save</Button>
          </div>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
