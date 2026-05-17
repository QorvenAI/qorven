'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import React, { useState } from 'react';
import { Settings2 } from 'lucide-react';
import { cn } from '@/lib/utils';
import { prettyModel } from '@/components/setup/setup-config';

export function QorvenSpinner({ className }: { className?: string }) {
  return (
    <img
      src="/logo/qorven-mark.svg"
      alt=""
      className={cn('animate-spin', className)}
      style={{ animationDuration: '1.4s', animationTimingFunction: 'linear' }}
    />
  );
}

export function SectionTitle({ icon: Icon, title, subtitle }: {
  icon: React.ElementType; title: string; subtitle?: string;
}) {
  return (
    <div className="flex items-start gap-2">
      <div className="flex h-7 w-7 items-center justify-center rounded-lg bg-primary/10 text-primary shrink-0">
        <Icon className="h-3.5 w-3.5" />
      </div>
      <div>
        <h3 className="text-base font-semibold text-foreground">{title}</h3>
        {subtitle && <p className="text-sm text-muted-foreground mt-0.5">{subtitle}</p>}
      </div>
    </div>
  );
}

export function LabeledInput(p: {
  label: string; value: string; onChange: (v: string) => void;
  placeholder?: string; autoFocus?: boolean; type?: string;
}) {
  return (
    <div>
      <label className="block text-sm font-medium text-foreground mb-1.5">{p.label}</label>
      <input
        type={p.type}
        value={p.value}
        onChange={e => p.onChange(e.target.value)}
        placeholder={p.placeholder}
        autoFocus={p.autoFocus}
        className="qr-input" />
    </div>
  );
}

export function ProviderLogo({ id, name, size = 'sm' }: { id: string; name: string; size?: 'sm' | 'md' }) {
  const [srcIndex, setSrcIndex] = useState(0);
  const dim = size === 'md' ? 'h-7 w-7' : 'h-5 w-5';
  const iconDim = size === 'md' ? 'h-4 w-4' : 'h-3.5 w-3.5';

  // Custom provider → gear icon
  if (id === 'custom') {
    return (
      <div className={`${dim} rounded-md bg-muted-foreground/15 flex items-center justify-center shrink-0`}>
        <Settings2 className={`${iconDim} text-muted-foreground`} />
      </div>
    );
  }

  const srcs = [`/icons/providers/${id}.svg`, `/icons/providers/${id}.webp`, `/icons/providers/${id}.jpeg`];

  if (srcIndex >= srcs.length) {
    return (
      <div className={`${dim} rounded-md bg-primary/20 flex items-center justify-center text-xs font-semibold text-primary shrink-0`}>
        {name.charAt(0).toUpperCase()}
      </div>
    );
  }
  return (
    <img
      src={srcs[srcIndex]}
      alt={name}
      onError={() => setSrcIndex(i => i + 1)}
      className={`${dim} rounded-md object-contain shrink-0`}
    />
  );
}

export function ModelPicker(p: {
  label: string; value: string; onChange: (v: string) => void;
  options: string[]; recommend: string;
}) {
  return (
    <div>
      <label className="flex items-center justify-between text-sm font-medium text-muted-foreground mb-1.5">
        <span>{p.label}</span>
        {p.value === p.recommend && <span className="text-xs text-emerald-400">recommended</span>}
      </label>
      <select
        value={p.value}
        onChange={e => p.onChange(e.target.value)}
        className="qr-select">
        {p.options.map(m => <option key={m} value={m}>{prettyModel(m)}</option>)}
      </select>
    </div>
  );
}
