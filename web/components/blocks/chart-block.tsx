'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { BarChart, Bar, LineChart, Line, AreaChart, Area, PieChart, Pie, Cell, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer, Legend } from 'recharts';

const COLORS = ['#a3e635', '#3b82f6', '#f59e0b', '#ef4444', '#8b5cf6', '#06b6d4', '#ec4899', '#10b981'];

interface ChartBlockProps {
  title?: string;
  chartType: 'bar' | 'line' | 'area' | 'pie' | 'donut';
  data: { name: string; value: number; [key: string]: any }[];
  xKey?: string;
  yKey?: string;
  colors?: string[];
  height?: number;
}

export function ChartBlock({ title, chartType, data, xKey = 'name', yKey = 'value', colors = COLORS, height = 300 }: ChartBlockProps) {
  const palette = colors.length ? colors : COLORS;

  return (
    <div className="rounded-lg border border-border bg-card p-4">
      {title && <h3 className="text-sm font-medium mb-3">{title}</h3>}
      <ResponsiveContainer width="100%" height={height}>
        {chartType === 'bar' ? (
          <BarChart data={data}><CartesianGrid strokeDasharray="3 3" stroke="var(--border)" /><XAxis dataKey={xKey} tick={{ fontSize: 11 }} /><YAxis tick={{ fontSize: 11 }} /><Tooltip contentStyle={{ background: 'var(--card)', border: '1px solid var(--border)', borderRadius: 8, fontSize: 12 }} /><Bar dataKey={yKey} fill={palette[0]} radius={[4, 4, 0, 0]} /></BarChart>
        ) : chartType === 'line' ? (
          <LineChart data={data}><CartesianGrid strokeDasharray="3 3" stroke="var(--border)" /><XAxis dataKey={xKey} tick={{ fontSize: 11 }} /><YAxis tick={{ fontSize: 11 }} /><Tooltip contentStyle={{ background: 'var(--card)', border: '1px solid var(--border)', borderRadius: 8, fontSize: 12 }} /><Line type="monotone" dataKey={yKey} stroke={palette[0]} strokeWidth={2} dot={{ r: 3 }} /></LineChart>
        ) : chartType === 'area' ? (
          <AreaChart data={data}><CartesianGrid strokeDasharray="3 3" stroke="var(--border)" /><XAxis dataKey={xKey} tick={{ fontSize: 11 }} /><YAxis tick={{ fontSize: 11 }} /><Tooltip contentStyle={{ background: 'var(--card)', border: '1px solid var(--border)', borderRadius: 8, fontSize: 12 }} /><Area type="monotone" dataKey={yKey} stroke={palette[0]} fill={palette[0]} fillOpacity={0.2} /></AreaChart>
        ) : (
          <PieChart><Pie data={data} dataKey={yKey} nameKey={xKey} cx="50%" cy="50%" innerRadius={chartType === 'donut' ? 60 : 0} outerRadius={100} paddingAngle={2} label={{ fontSize: 11 }}>{data.map((_, i) => <Cell key={i} fill={palette[i % palette.length]} />)}</Pie><Tooltip contentStyle={{ background: 'var(--card)', border: '1px solid var(--border)', borderRadius: 8, fontSize: 12 }} /><Legend /></PieChart>
        )}
      </ResponsiveContainer>
    </div>
  );
}
