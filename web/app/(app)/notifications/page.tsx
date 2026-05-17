'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { useEffect, useState } from 'react';
import { Bell, Check, CheckCheck, Filter, AlertCircle } from 'lucide-react';
import { cn } from '@/lib/utils';
import { EmptyState, emptyStates } from '@/components/empty-state';
import { TableSkeleton } from '@/components/page-skeleton';
import { ErrorBoundary } from '@/components/error-boundary';
import { notifications as notificationsApi } from '@/lib/api';
import type { Notification } from '@/types';

export default function NotificationsPage() {
  const [notifs, setNotifs] = useState<Notification[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [filter, setFilter] = useState<'all' | 'unread'>('all');

  const load = () => {
    setLoading(true);
    setError(null);
    notificationsApi
      .list()
      .then((d) => { setNotifs(d); setLoading(false); })
      .catch((e) => { setError(e.message); setLoading(false); });
  };

  useEffect(load, []);

  const markRead = async (id: string) => {
    await notificationsApi.markRead(id);
    setNotifs((prev) => prev.map((n) => (n.id === id ? { ...n, read: true } : n)));
  };

  const markAllRead = async () => {
    await notificationsApi.markAllRead();
    setNotifs((prev) => prev.map((n) => ({ ...n, read: true })));
  };

  const filtered = filter === 'unread' ? notifs.filter((n) => !n.read) : notifs;
  const unreadCount = notifs.filter((n) => !n.read).length;

  return (
    <ErrorBoundary fallbackTitle="Failed to load notifications">
      <div className="space-y-6">
        <div className="flex items-start justify-between">
          <div>
            <h1 className="text-lg font-semibold flex items-center gap-2">
              <Bell className="h-6 w-6" />
              Notifications
            </h1>
            <p className="text-sm text-muted-foreground">
              {loading ? 'Loading…' : `${unreadCount} unread`}
            </p>
          </div>
          {!loading && !error && (
            <div className="flex items-center gap-2">
              <button
                onClick={() => setFilter((f) => (f === 'all' ? 'unread' : 'all'))}
                className={cn(
                  'flex items-center gap-1.5 rounded-md border border-input px-3 py-1.5 text-sm transition-colors',
                  filter === 'unread' && 'bg-accent',
                )}
              >
                <Filter className="h-3.5 w-3.5" />
                {filter === 'all' ? 'All' : 'Unread'}
              </button>
              {unreadCount > 0 && (
                <button
                  onClick={markAllRead}
                  className="flex items-center gap-1.5 rounded-md border border-input px-3 py-1.5 text-sm transition-colors hover:bg-accent"
                >
                  <CheckCheck className="h-3.5 w-3.5" />
                  Mark all read
                </button>
              )}
            </div>
          )}
        </div>

        {loading ? (
          <TableSkeleton rows={5} />
        ) : error ? (
          <div className="flex flex-col items-center py-16 text-center">
            <AlertCircle className="h-8 w-8 text-destructive" />
            <p className="mt-2 text-sm text-destructive">{error}</p>
            <button
              onClick={load}
              className="mt-3 rounded-lg bg-primary px-4 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90"
            >
              Retry
            </button>
          </div>
        ) : filtered.length === 0 ? (
          <EmptyState
            {...emptyStates.notifications}
            title={filter === 'unread' ? 'No unread notifications' : 'No notifications yet'}
            description={
              filter === 'unread'
                ? 'All caught up! Switch to All to see past notifications.'
                : 'Notifications will appear here as events occur.'
            }
          />
        ) : (
          <div className="space-y-1">
            {filtered.map((n) => (
              <div
                key={n.id}
                onClick={() => !n.read && markRead(n.id)}
                className={cn(
                  'flex items-start gap-3 rounded-xl border p-4 transition-colors',
                  n.read
                    ? 'border-border bg-card opacity-60'
                    : 'border-primary/20 bg-primary/5 cursor-pointer hover:bg-primary/10',
                )}
              >
                <div
                  className={cn(
                    'mt-1 h-2 w-2 rounded-full shrink-0',
                    n.read ? 'bg-transparent' : 'bg-primary',
                  )}
                />
                <div className="flex-1 min-w-0">
                  <div className="text-sm font-medium text-foreground">{n.title}</div>
                  {n.highlight && (
                    <div className="text-xs text-muted-foreground mt-0.5">{n.highlight}</div>
                  )}
                  <div className="flex items-center gap-2 mt-1.5 text-xs text-muted-foreground">
                    <span>{new Date(n.created_at).toLocaleString()}</span>
                    {n.source && <span>• {n.source}</span>}
                    <span className="px-1.5 py-0.5 rounded bg-muted text-muted-foreground">
                      {n.type}
                    </span>
                  </div>
                </div>
                {!n.read && <Check className="h-4 w-4 text-muted-foreground shrink-0 mt-1" />}
              </div>
            ))}
          </div>
        )}
      </div>
    </ErrorBoundary>
  );
}
