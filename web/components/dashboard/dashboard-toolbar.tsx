'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { Check, Edit3, Plus, Sparkles } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';

interface DashboardToolbarProps {
  isEditing: boolean;
  onToggleEdit: () => void;
  onAddWidget: () => void;
  onAskAI: () => void;
  onSave: () => void;
  dashboardName: string;
}

export function DashboardToolbar({
  isEditing,
  onToggleEdit,
  onAddWidget,
  onAskAI,
  onSave,
  dashboardName,
}: DashboardToolbarProps) {
  return (
    <div className="flex items-center justify-between gap-3 px-1 py-2 mb-2">
      {/* Left: dashboard name */}
      <p className="text-sm font-medium text-foreground truncate">{dashboardName}</p>

      {/* Right: action buttons */}
      <div className="flex items-center gap-2">
        {isEditing && (
          <Button
            variant="ghost"
            size="sm"
            onClick={onAddWidget}
            className="h-8 gap-1.5 text-xs"
          >
            <Plus className="size-3.5" />
            Add Widget
          </Button>
        )}

        <Button
          variant="ghost"
          size="sm"
          onClick={onAskAI}
          className="h-8 gap-1.5 text-xs"
        >
          <Sparkles className="size-3.5" />
          Ask AI
        </Button>

        {isEditing ? (
          <Button
            variant="primary"
            size="sm"
            onClick={() => { onSave(); onToggleEdit(); }}
            className="h-8 gap-1.5 text-xs"
          >
            <Check className="size-3.5" />
            Done
          </Button>
        ) : (
          <Button
            variant="outline"
            size="sm"
            onClick={onToggleEdit}
            className="h-8 gap-1.5 text-xs"
          >
            <Edit3 className="size-3.5" />
            Edit
          </Button>
        )}
      </div>
    </div>
  );
}
