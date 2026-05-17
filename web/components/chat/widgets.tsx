'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { cn } from '@/lib/utils';
import { Droplets, Wind, Thermometer, Eye, CloudRain } from 'lucide-react';

interface WeatherWidgetProps {
  data: {
    location: string;
    temperature: number;
    feels_like: number;
    humidity: number;
    wind_speed: number;
    condition: string;
    icon: string;
    unit: string;
    daily?: {
      time?: string[];
      temperature_2m_max?: number[];
      temperature_2m_min?: number[];
      weather_code?: number[];
      precipitation_probability_max?: number[];
    };
  };
}

const dayName = (dateStr: string, i: number) => {
  if (i === 0) return 'Today';
  try { return new Date(dateStr).toLocaleDateString('en', { weekday: 'short' }); } catch { return `Day ${i + 1}`; }
};

const wxIcon = (code: number) => {
  if (code === 0) return '☀️';
  if (code <= 3) return '⛅';
  if (code <= 48) return '🌫️';
  if (code <= 67) return '🌧️';
  if (code <= 77) return '❄️';
  if (code <= 86) return '🌨️';
  return '⛈️';
};

export function WeatherWidget({ data }: WeatherWidgetProps) {
  const daily = data.daily;
  const days = daily?.time?.slice(0, 5) || [];

  return (
    <div className="rounded-xl border border-border overflow-hidden my-3 max-w-md">
      {/* Current conditions */}
      <div className="bg-gradient-to-br from-blue-900/30 to-indigo-900/30 p-5">
        <p className="text-xs text-muted-foreground mb-1">{data.location?.split(',').slice(0, 2).join(',')}</p>
        <div className="flex items-center justify-between">
          <div>
            <div className="flex items-baseline gap-1">
              <span className="text-4xl font-semibold">{Math.round(data.temperature)}</span>
              <span className="text-xl text-muted-foreground">{data.unit}</span>
            </div>
            <p className="text-sm font-medium mt-1">{data.condition}</p>
          </div>
          <span className="text-5xl">{data.icon}</span>
        </div>
        {/* Stats row */}
        <div className="flex gap-4 mt-3 text-xs text-muted-foreground">
          <span className="flex items-center gap-1"><Thermometer className="h-3 w-3" />Feels {Math.round(data.feels_like)}{data.unit}</span>
          <span className="flex items-center gap-1"><Droplets className="h-3 w-3" />{data.humidity}%</span>
          <span className="flex items-center gap-1"><Wind className="h-3 w-3" />{data.wind_speed} km/h</span>
        </div>
      </div>

      {/* Forecast */}
      {days.length > 0 && (
        <div className="flex divide-x divide-border">
          {days.map((d, i) => (
            <div key={i} className="flex-1 py-2 px-1.5 text-center">
              <p className="text-2xs text-muted-foreground font-medium">{dayName(d, i)}</p>
              <p className="text-sm my-0.5">{wxIcon(daily?.weather_code?.[i] || 0)}</p>
              <p className="text-xs font-medium">{Math.round(daily?.temperature_2m_max?.[i] || 0)}°</p>
              <p className="text-2xs text-muted-foreground">{Math.round(daily?.temperature_2m_min?.[i] || 0)}°</p>
              {daily?.precipitation_probability_max?.[i] != null && daily.precipitation_probability_max[i] > 0 && (
                <p className="text-2xs text-blue-400 flex items-center justify-center gap-0.5 mt-0.5">
                  <CloudRain className="h-2.5 w-2.5" />{daily.precipitation_probability_max[i]}%
                </p>
              )}
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

interface CalcWidgetProps {
  expression: string;
  result: number;
}

export function CalcWidget({ expression, result }: CalcWidgetProps) {
  return (
    <div className="rounded-xl border border-border overflow-hidden my-3 max-w-xs">
      <div className="p-4 space-y-2">
        <code className="text-xs text-muted-foreground block">{expression}</code>
        <div className="flex items-center gap-2">
          <span className="text-xs text-muted-foreground">=</span>
          <span className="text-2xl font-semibold">{result}</span>
        </div>
      </div>
    </div>
  );
}
