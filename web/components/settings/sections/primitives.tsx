'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { useEffect, useRef, useState } from 'react';
import { Loader2, ChevronDown } from 'lucide-react';
import { cn } from '@/lib/utils';
import { userPrefs } from '@/lib/api';

export function Card({ id, title, description, headerRight, children }: {
  id?: string; title: string; description?: string;
  headerRight?: React.ReactNode; children: React.ReactNode;
}) {
  return (
    <div className="rounded-xl border border-border bg-card shadow-sm overflow-hidden">
      <div className="flex items-center justify-between px-6 py-4 border-b border-border" id={id}>
        <div>
          <h3 className="text-sm font-semibold text-foreground">{title}</h3>
          {description && <p className="text-xs text-muted-foreground mt-0.5 max-w-lg">{description}</p>}
        </div>
        {headerRight && <div className="flex items-center gap-2 shrink-0 ml-4">{headerRight}</div>}
      </div>
      <div className="px-6 py-5 space-y-5">{children}</div>
    </div>
  );
}

export function Row({ label, hint, children }: { label: string; hint?: string; children: React.ReactNode }) {
  return (
    <div className="flex items-baseline flex-wrap lg:flex-nowrap gap-2.5">
      <div className="w-full max-w-[13rem] shrink-0">
        <span className="text-sm font-medium">{label}</span>
        {hint && <p className="text-xs text-muted-foreground mt-0.5 leading-relaxed">{hint}</p>}
      </div>
      <div className="flex-1 min-w-0">{children}</div>
    </div>
  );
}

export function Input({ value, onChange, type = 'text', placeholder, readOnly, suffix, className }: {
  value?: string; onChange?: (v: string) => void; type?: string;
  placeholder?: string; readOnly?: boolean; suffix?: React.ReactNode; className?: string;
}) {
  return (
    <div className="relative">
      <input
        type={type} value={value ?? ''}
        onChange={e => onChange?.(e.target.value)}
        placeholder={placeholder} readOnly={readOnly}
        className={cn(
          'w-full rounded-lg border border-border bg-input px-3 py-2 text-sm outline-none transition-colors',
          'placeholder:text-muted-foreground/40',
          readOnly ? 'text-muted-foreground cursor-default select-none' : 'focus:border-primary',
          suffix && 'pr-10',
          className,
        )}
      />
      {suffix && <div className="absolute right-2.5 top-1/2 -translate-y-1/2">{suffix}</div>}
    </div>
  );
}

export function Btn({ children, onClick, loading, disabled, variant = 'primary', className }: {
  children: React.ReactNode; onClick?: () => void; loading?: boolean;
  disabled?: boolean; variant?: 'primary' | 'ghost' | 'danger'; className?: string;
}) {
  return (
    <button onClick={onClick} disabled={disabled || loading}
      className={cn(
        'inline-flex items-center gap-1.5 rounded-lg px-4 py-2 text-sm font-medium transition-colors disabled:opacity-50 cursor-pointer',
        variant === 'primary' && 'bg-primary text-primary-foreground hover:bg-primary/90',
        variant === 'ghost'   && 'border border-border hover:bg-accent text-foreground',
        variant === 'danger'  && 'bg-destructive/10 text-destructive hover:bg-destructive/20 border border-destructive/20',
        className,
      )}>
      {loading && <Loader2 className="h-3.5 w-3.5 animate-spin" />}
      {children}
    </button>
  );
}

export function Toggle({ checked, onChange }: { checked: boolean; onChange: (v: boolean) => void }) {
  return (
    <button onClick={() => onChange(!checked)}
      className={cn('relative inline-flex h-5 w-9 shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors', checked ? 'bg-primary' : 'bg-muted')}>
      <span className={cn('inline-block h-4 w-4 rounded-full bg-background shadow-sm transition-transform', checked ? 'translate-x-4' : 'translate-x-0')} />
    </button>
  );
}

export function SaveBar({ saving, onSave, label = 'Save Changes' }: { saving?: boolean; onSave: () => void; label?: string }) {
  return (
    <div className="flex justify-end pt-3 border-t border-border/70 -mx-6 px-6 -mb-5 mt-2 pb-1">
      <Btn onClick={onSave} loading={saving}>{label}</Btn>
    </div>
  );
}

export function usePrefs() {
  const [prefs, setPrefs] = useState<Record<string, any>>({});
  const loaded = useRef(false);

  useEffect(() => {
    if (loaded.current) return;
    loaded.current = true;
    userPrefs.get().then(d => setPrefs(d ?? {})).catch(() => {});
  }, []);

  const setPref = (key: string, value: any) =>
    setPrefs(p => ({ ...p, [key]: value }));

  const savePrefs = async (patch: Record<string, any>) => {
    const merged = { ...prefs, ...patch };
    setPrefs(merged);
    await userPrefs.save(merged);
  };

  return { prefs, setPref, savePrefs };
}

const COMMON_TIMEZONES = [
  'UTC',
  'America/New_York', 'America/Chicago', 'America/Denver', 'America/Los_Angeles',
  'America/Toronto', 'America/Vancouver', 'America/Sao_Paulo', 'America/Mexico_City',
  'Europe/London', 'Europe/Paris', 'Europe/Berlin', 'Europe/Moscow', 'Europe/Istanbul',
  'Asia/Dubai', 'Asia/Kolkata', 'Asia/Dhaka', 'Asia/Bangkok', 'Asia/Singapore',
  'Asia/Shanghai', 'Asia/Tokyo', 'Asia/Seoul', 'Australia/Sydney', 'Pacific/Auckland',
  'Africa/Cairo', 'Africa/Lagos', 'Africa/Johannesburg',
];

export function TimezoneSelect({ value, onChange }: { value: string; onChange: (v: string) => void }) {
  const [search, setSearch] = useState('');
  const [open, setOpen] = useState(false);
  const dropRef = useRef<HTMLDivElement>(null);

  const options = search
    ? COMMON_TIMEZONES.filter(tz => tz.toLowerCase().includes(search.toLowerCase()))
    : COMMON_TIMEZONES;

  useEffect(() => {
    const handler = (e: MouseEvent) => {
      if (dropRef.current && !dropRef.current.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener('mousedown', handler);
    return () => document.removeEventListener('mousedown', handler);
  }, []);

  return (
    <div ref={dropRef} className="relative w-full">
      <button type="button" onClick={() => setOpen(!open)}
        className="w-full flex items-center justify-between rounded-lg border border-border bg-background px-3 py-2 text-sm hover:border-primary/50 transition-colors">
        <span className="truncate">{value || 'Select timezone…'}</span>
        <ChevronDown className={cn('h-4 w-4 text-muted-foreground shrink-0 transition-transform ml-2', open && 'rotate-180')} />
      </button>
      {open && (
        <div className="absolute top-full left-0 right-0 z-50 mt-1 rounded-lg border border-border bg-popover shadow-lg overflow-hidden">
          <div className="p-2 border-b border-border">
            <input
              type="text"
              value={search}
              onChange={e => setSearch(e.target.value)}
              placeholder="Search timezone…"
              className="w-full bg-transparent text-sm outline-none px-1"
              autoFocus
            />
          </div>
          <div className="max-h-56 overflow-y-auto py-1">
            {options.map(tz => (
              <button key={tz} type="button"
                onClick={() => { onChange(tz); setOpen(false); setSearch(''); }}
                className={cn('flex w-full items-center px-3 py-1.5 text-sm hover:bg-accent',
                  tz === value && 'text-primary font-medium')}>
                {tz}
              </button>
            ))}
            {options.length === 0 && <p className="px-3 py-2 text-sm text-muted-foreground">No matches</p>}
          </div>
        </div>
      )}
    </div>
  );
}
