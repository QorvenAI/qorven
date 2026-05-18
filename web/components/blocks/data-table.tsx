'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useState, useMemo } from 'react';
import { cn } from '@/lib/utils';
import { ArrowUpDown, Search } from 'lucide-react';

interface DataTableProps {
  title?: string;
  columns: { key: string; label: string; sortable?: boolean; align?: 'left' | 'center' | 'right' }[];
  rows: Record<string, any>[];
  pageSize?: number;
  searchable?: boolean;
}

export function DataTable({ title, columns, rows, pageSize = 10, searchable }: DataTableProps) {
  const [sort, setSort] = useState<{ key: string; dir: 'asc' | 'desc' } | null>(null);
  const [search, setSearch] = useState('');
  const [page, setPage] = useState(0);

  const filtered = useMemo(() => {
    let data = rows;
    if (search) data = data.filter(r => Object.values(r).some(v => String(v).toLowerCase().includes(search.toLowerCase())));
    if (sort) data = [...data].sort((a, b) => { const av = a[sort.key], bv = b[sort.key]; return sort.dir === 'asc' ? (av > bv ? 1 : -1) : (av < bv ? 1 : -1); });
    return data;
  }, [rows, search, sort]);

  const paged = filtered.slice(page * pageSize, (page + 1) * pageSize);
  const totalPages = Math.ceil(filtered.length / pageSize);

  return (
    <div className="rounded-lg border border-border overflow-hidden">
      {(title || searchable) && (
        <div className="flex items-center justify-between px-4 py-3 border-b border-border bg-card">
          {title && <h3 className="text-sm font-medium">{title}</h3>}
          {searchable && (
            <div className="relative"><Search className="absolute left-2 top-2 h-3.5 w-3.5 text-muted-foreground" />
              <input value={search} onChange={e => { setSearch(e.target.value); setPage(0); }} placeholder="Search..." className="qr-input text-xs pl-7 w-48" />
            </div>
          )}
        </div>
      )}
      <div className="overflow-x-auto">
        <table className="w-full text-sm">
          <thead><tr className="border-b border-border bg-muted/30">
            {columns.map(col => (
              <th key={col.key} className={cn('px-4 py-2 text-xs font-medium text-muted-foreground', col.align === 'right' && 'text-right', col.align === 'center' && 'text-center', col.sortable && 'cursor-pointer select-none hover:text-foreground')}
                onClick={() => col.sortable && setSort(s => s?.key === col.key ? { key: col.key, dir: s.dir === 'asc' ? 'desc' : 'asc' } : { key: col.key, dir: 'asc' })}>
                <span className="flex items-center gap-1">{col.label}{col.sortable && <ArrowUpDown className="h-3 w-3" />}</span>
              </th>
            ))}
          </tr></thead>
          <tbody>{paged.map((row, i) => (
            <tr key={i} className="border-b border-border last:border-0 hover:bg-muted/20">
              {columns.map(col => <td key={col.key} className={cn('px-4 py-2.5', col.align === 'right' && 'text-right', col.align === 'center' && 'text-center')}>{row[col.key]}</td>)}
            </tr>
          ))}</tbody>
        </table>
      </div>
      {totalPages > 1 && (
        <div className="flex items-center justify-between px-4 py-2 border-t border-border text-xs text-muted-foreground">
          <span>{filtered.length} rows</span>
          <div className="flex gap-1">
            <button onClick={() => setPage(p => Math.max(0, p - 1))} disabled={page === 0} className="px-2 py-1 rounded hover:bg-muted disabled:opacity-30">Prev</button>
            <span className="px-2 py-1">{page + 1}/{totalPages}</span>
            <button onClick={() => setPage(p => Math.min(totalPages - 1, p + 1))} disabled={page >= totalPages - 1} className="px-2 py-1 rounded hover:bg-muted disabled:opacity-30">Next</button>
          </div>
        </div>
      )}
    </div>
  );
}
