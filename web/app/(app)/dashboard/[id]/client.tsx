'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { useState, useEffect } from 'react';
import { useParams } from 'next/navigation';
import { BlockRenderer } from '@/components/blocks';
import type { BlockConfig } from '@/components/blocks/types';
import { Loader2 } from 'lucide-react';

const getToken = () => typeof window !== 'undefined' ? (localStorage.getItem('qorven_token') || '') : '';

const API = '/api/v1';
const headers = { 'Content-Type': 'application/json', Authorization: `Bearer ${getToken()}` };

// Sample data generator for demo blocks
function generateSampleData(block: any): any {
  switch (block.type) {
    case 'stat-row':
      return { ...block, stats: [
        { value: '142', label: 'Total', change: '+12%', changeType: 'up' },
        { value: '$48.2K', label: 'Revenue', change: '+8%', changeType: 'up' },
        { value: '23', label: 'Active', change: '-2', changeType: 'down' },
        { value: '94%', label: 'Rate', changeType: 'neutral' },
      ]};
    case 'pipeline':
      return { ...block, stages: [
        { name: 'Lead', count: 45, color: 'var(--chart-5)' },
        { name: 'Qualified', count: 28, color: 'var(--chart-3)' },
        { name: 'Proposal', count: 12, color: 'var(--chart-1)' },
        { name: 'Closed', count: 8, value: '$24K', color: 'var(--chart-2)' },
      ]};
    case 'chart':
      return { ...block, chartType: 'bar', data: [
        { name: 'Jan', value: 12 }, { name: 'Feb', value: 19 },
        { name: 'Mar', value: 15 }, { name: 'Apr', value: 25 },
        { name: 'May', value: 22 }, { name: 'Jun', value: 30 },
      ]};
    case 'data-table':
      return { ...block, searchable: true, columns: [
        { key: 'name', label: 'Name', sortable: true },
        { key: 'status', label: 'Status' },
        { key: 'value', label: 'Value', align: 'right', sortable: true },
      ], rows: [
        { name: 'Acme Corp', status: 'Active', value: '$12,000' },
        { name: 'Beta Inc', status: 'Pending', value: '$8,500' },
        { name: 'Gamma LLC', status: 'Active', value: '$15,200' },
        { name: 'Delta Co', status: 'Closed', value: '$6,800' },
      ]};
    case 'contacts':
      return { ...block, contacts: [
        { id: '1', name: 'Alice Johnson', company: 'Acme Corp', email: 'alice@acme.com', status: 'active', lastContact: '2d ago' },
        { id: '2', name: 'Bob Smith', company: 'Beta Inc', email: 'bob@beta.io', status: 'active', lastContact: '1w ago' },
        { id: '3', name: 'Carol Davis', company: 'Gamma LLC', lastContact: '3d ago' },
      ]};
    case 'feed':
      return { ...block, items: [
        { id: '1', actor: 'Prospector', action: 'found 3 new leads from', target: 'LinkedIn', time: '5 min ago' },
        { id: '2', actor: 'Outreach', action: 'sent follow-up email to', target: 'Acme Corp', time: '1 hour ago' },
        { id: '3', actor: 'Analyst', action: 'generated weekly report', time: '3 hours ago' },
      ]};
    case 'kanban':
      return { ...block, columns: [
        { id: 'todo', title: 'To Do', color: 'var(--muted-foreground)' },
        { id: 'progress', title: 'In Progress', color: 'var(--chart-3)' },
        { id: 'done', title: 'Done', color: 'var(--chart-2)' },
      ], items: [
        { id: '1', columnId: 'todo', title: 'Draft Q2 content plan', tags: ['strategy'] },
        { id: '2', columnId: 'progress', title: 'Write blog post on AI trends', tags: ['content'] },
        { id: '3', columnId: 'done', title: 'Schedule Twitter thread', tags: ['social'] },
      ]};
    case 'calendar':
      return { ...block, events: [
        { date: new Date().toISOString().split('T')[0], title: 'Team standup', color: 'var(--chart-5)' },
        { date: new Date(Date.now() + 86400000 * 2).toISOString().split('T')[0], title: 'Content review', color: 'var(--chart-3)' },
      ]};
    case 'timeline':
      return { ...block, events: [
        { date: 'Today', title: 'Research started', description: 'Analyzing 15 sources' },
        { date: 'Yesterday', title: 'Report delivered', description: 'Q1 market analysis complete' },
      ]};
    default:
      return block;
  }
}

export default function DashboardPage() {
  const { id } = useParams();
  const [config, setConfig] = useState<BlockConfig | null>(null);
  const [templateName, setTemplateName] = useState('');
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    // Try to load saved dashboard, fallback to template default
    fetch(`${API}/templates/${id}/dashboard`, { headers })
      .then(r => r.ok ? r.json() : null)
      .then(dash => {
        if (dash) {
          const blocks = dash.blocks.map(generateSampleData);
          setConfig({ layout: dash.layout || 'grid-2col', blocks, title: '' });
        }
      })
      .catch(() => {})
      .finally(() => setLoading(false));

    // Get template name
    fetch(`${API}/templates`, { headers })
      .then(r => r.json())
      .then((list: any[]) => {
        const t = list?.find((t: any) => t.id === id);
        if (t) setTemplateName(`${t.icon} ${t.name}`);
      })
      .catch(() => {});
  }, [id]);

  if (loading) return <div className="flex items-center justify-center h-64"><Loader2 className="h-6 w-6 animate-spin text-muted-foreground" /></div>;

  if (!config) return (
    <div className="mx-auto max-w-3xl p-8 text-center">
      <p className="text-muted-foreground">Dashboard not found. <a href="/marketplace" className="text-primary hover:underline">Install a template first.</a></p>
    </div>
  );

  return (
    <div className="mx-auto max-w-6xl p-6 space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-xl font-semibold">{templateName || 'Dashboard'}</h1>
        <a href="/marketplace" className="text-xs text-muted-foreground hover:text-foreground">← Marketplace</a>
      </div>
      <BlockRenderer config={config} />
    </div>
  );
}
