'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { useState } from 'react';
import { cn } from '@/lib/utils';
import { ChevronDown, ChevronLeft, ChevronRight, Upload, X, AlertCircle, CheckCircle, Info, AlertTriangle } from 'lucide-react';

// --- Calendar ---
interface CalendarBlockProps { title?: string; events: { date: string; title: string; color?: string; time?: string }[] }

export function CalendarBlock({ title, events }: CalendarBlockProps) {
  const [month, setMonth] = useState(new Date());
  const year = month.getFullYear(), mo = month.getMonth();
  const firstDay = new Date(year, mo, 1).getDay();
  const daysInMonth = new Date(year, mo + 1, 0).getDate();
  const days = Array.from({ length: 42 }, (_, i) => { const d = i - firstDay + 1; return d > 0 && d <= daysInMonth ? d : null; });
  const monthName = month.toLocaleString('default', { month: 'long', year: 'numeric' });
  const dateStr = (d: number) => `${year}-${String(mo + 1).padStart(2, '0')}-${String(d).padStart(2, '0')}`;
  const today = new Date(); const isToday = (d: number) => d === today.getDate() && mo === today.getMonth() && year === today.getFullYear();

  return (
    <div className="rounded-lg border border-border p-4">
      {title && <h3 className="text-sm font-medium mb-3">{title}</h3>}
      <div className="flex items-center justify-between mb-3">
        <button onClick={() => setMonth(new Date(year, mo - 1))} className="p-1 hover:bg-muted rounded"><ChevronLeft className="h-4 w-4" /></button>
        <span className="text-sm font-medium">{monthName}</span>
        <button onClick={() => setMonth(new Date(year, mo + 1))} className="p-1 hover:bg-muted rounded"><ChevronRight className="h-4 w-4" /></button>
      </div>
      <div className="grid grid-cols-7 gap-px text-center text-2xs text-muted-foreground mb-1">
        {['Su','Mo','Tu','We','Th','Fr','Sa'].map(d => <div key={d} className="py-1">{d}</div>)}
      </div>
      <div className="grid grid-cols-7 gap-px">
        {days.map((d, i) => {
          if (!d) return <div key={i} />;
          const dayEvents = events.filter(e => e.date === dateStr(d));
          return (
            <div key={i} className={cn('relative p-1 text-center text-xs rounded hover:bg-muted/50 cursor-pointer min-h-[32px]', isToday(d) && 'bg-primary/20 font-semibold')}>
              {d}
              {dayEvents.length > 0 && <div className="absolute bottom-0.5 left-1/2 -translate-x-1/2 flex gap-0.5">{dayEvents.slice(0, 3).map((e, j) => <div key={j} className="h-1 w-1 rounded-full" style={{ background: e.color || '#a3e635' }} />)}</div>}
            </div>
          );
        })}
      </div>
    </div>
  );
}

// --- Tabs ---
interface TabsBlockProps { title?: string; tabs: { id: string; label: string; content: string }[] }

export function TabsBlock({ title, tabs }: TabsBlockProps) {
  const [active, setActive] = useState(tabs[0]?.id);
  return (
    <div className="rounded-lg border border-border">
      {title && <div className="px-4 pt-3"><h3 className="text-sm font-medium">{title}</h3></div>}
      <div className="flex border-b border-border px-4">
        {tabs.map(t => (
          <button key={t.id} onClick={() => setActive(t.id)} className={cn('px-3 py-2 text-xs font-medium border-b-2 -mb-px transition-colors', active === t.id ? 'border-primary text-foreground' : 'border-transparent text-muted-foreground hover:text-foreground')}>{t.label}</button>
        ))}
      </div>
      <div className="p-4 text-sm">{tabs.find(t => t.id === active)?.content}</div>
    </div>
  );
}

// --- Accordion ---
interface AccordionBlockProps { title?: string; items: { id: string; title: string; content: string }[] }

export function AccordionBlock({ title, items }: AccordionBlockProps) {
  const [open, setOpen] = useState<string | null>(null);
  return (
    <div className="rounded-lg border border-border">
      {title && <div className="px-4 py-3 border-b border-border"><h3 className="text-sm font-medium">{title}</h3></div>}
      {items.map(item => (
        <div key={item.id} className="border-b border-border last:border-0">
          <button onClick={() => setOpen(o => o === item.id ? null : item.id)} className="flex items-center justify-between w-full px-4 py-3 text-sm font-medium hover:bg-muted/20">
            {item.title}<ChevronDown className={cn('h-4 w-4 transition-transform', open === item.id && 'rotate-180')} />
          </button>
          {open === item.id && <div className="px-4 pb-3 text-sm text-muted-foreground">{item.content}</div>}
        </div>
      ))}
    </div>
  );
}

// --- File Upload ---
interface FileUploadBlockProps { title?: string; accept?: string; multiple?: boolean; maxSize?: string }

export function FileUploadBlock({ title, accept, multiple, maxSize }: FileUploadBlockProps) {
  const [files, setFiles] = useState<File[]>([]);
  const handleDrop = (e: React.DragEvent) => { e.preventDefault(); setFiles(f => [...f, ...Array.from(e.dataTransfer.files)]); };
  const handleChange = (e: React.ChangeEvent<HTMLInputElement>) => { if (e.target.files) setFiles(f => [...f, ...Array.from(e.target.files!)]); };
  return (
    <div className="rounded-lg border border-border p-4">
      {title && <h3 className="text-sm font-medium mb-3">{title}</h3>}
      <div onDrop={handleDrop} onDragOver={e => e.preventDefault()} className="border-2 border-dashed border-border rounded-lg p-6 text-center hover:border-primary/50 transition-colors cursor-pointer" onClick={() => document.getElementById('block-file-input')?.click()}>
        <Upload className="h-8 w-8 mx-auto text-muted-foreground mb-2" />
        <p className="text-sm text-muted-foreground">Drop files here or click to browse</p>
        {maxSize && <p className="text-2xs text-muted-foreground mt-1">Max: {maxSize}</p>}
        <input id="block-file-input" type="file" accept={accept} multiple={multiple} onChange={handleChange} className="hidden" />
      </div>
      {files.length > 0 && (
        <div className="mt-3 space-y-1">{files.map((f, i) => (
          <div key={i} className="flex items-center justify-between rounded bg-muted/50 px-3 py-1.5 text-xs">
            <span className="truncate">{f.name}</span>
            <button onClick={() => setFiles(fs => fs.filter((_, j) => j !== i))}><X className="h-3 w-3 text-muted-foreground hover:text-foreground" /></button>
          </div>
        ))}</div>
      )}
    </div>
  );
}

// --- Alert ---
const alertStyles = { info: { icon: Info, color: 'border-blue-500/30 bg-blue-500/10 text-blue-400' }, success: { icon: CheckCircle, color: 'border-emerald-500/30 bg-emerald-500/10 text-emerald-400' }, warning: { icon: AlertTriangle, color: 'border-amber-500/30 bg-amber-500/10 text-amber-400' }, error: { icon: AlertCircle, color: 'border-red-500/30 bg-red-500/10 text-red-400' } };

interface AlertBlockProps { type: 'info' | 'success' | 'warning' | 'error'; title?: string; message: string }

export function AlertBlock({ type, title, message }: AlertBlockProps) {
  const { icon: Icon, color } = alertStyles[type];
  return (
    <div className={cn('flex gap-3 rounded-lg border p-3', color)}>
      <Icon className="h-4 w-4 shrink-0 mt-0.5" />
      <div><p className="text-sm font-medium">{title || type}</p><p className="text-xs opacity-80 mt-0.5">{message}</p></div>
    </div>
  );
}

// --- Avatar Group ---
interface AvatarGroupProps { avatars: { name: string; image?: string; color?: string }[]; max?: number }

export function AvatarGroup({ avatars, max = 5 }: AvatarGroupProps) {
  const shown = avatars.slice(0, max);
  const extra = avatars.length - max;
  return (
    <div className="flex -space-x-2">
      {shown.map((a, i) => (
        <div key={i} className="h-8 w-8 rounded-full border-2 border-background flex items-center justify-center text-2xs font-semibold" style={{ background: a.color || '#374151' }} title={a.name}>
          {a.image ? <img src={a.image} alt={a.name} className="h-full w-full rounded-full object-cover" /> : a.name.split(' ').map(n => n[0]).join('').slice(0, 2)}
        </div>
      ))}
      {extra > 0 && <div className="h-8 w-8 rounded-full border-2 border-background bg-muted flex items-center justify-center text-2xs font-medium">+{extra}</div>}
    </div>
  );
}

// --- Skeleton ---
export function SkeletonBlock({ rows = 3, type = 'lines' }: { rows?: number; type?: 'lines' | 'card' | 'table' }) {
  if (type === 'card') return (
    <div className="rounded-lg border border-border p-4 space-y-3 animate-pulse">
      <div className="h-4 w-1/3 rounded bg-muted" /><div className="h-8 w-1/2 rounded bg-muted" /><div className="h-3 w-2/3 rounded bg-muted" />
    </div>
  );
  if (type === 'table') return (
    <div className="rounded-lg border border-border overflow-hidden animate-pulse">
      <div className="h-10 bg-muted/50" />
      {Array.from({ length: rows }).map((_, i) => <div key={i} className="h-12 border-t border-border flex items-center gap-4 px-4"><div className="h-3 flex-1 rounded bg-muted" /><div className="h-3 w-20 rounded bg-muted" /></div>)}
    </div>
  );
  return (
    <div className="space-y-2 animate-pulse">{Array.from({ length: rows }).map((_, i) => <div key={i} className="h-3 rounded bg-muted" style={{ width: `${70 + Math.random() * 30}%` }} />)}</div>
  );
}
