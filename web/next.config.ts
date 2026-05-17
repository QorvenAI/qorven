// Copyright 2026 Tekky AI Academy LLP. Licensed under FSL-1.1-ALv2.

import type { NextConfig } from 'next';

// QORVEN_STATIC=1 switches the build into static-export mode —
// that's what the Go binary embeds via go:embed. The dev server
// (npm run dev) keeps the full Next.js feature set (rewrites, image
// optimisation) so local development isn't slowed down.
const isStaticBuild = process.env.QORVEN_STATIC === '1';

const nextConfig: NextConfig = {
  devIndicators: false,
  // Static export: produces ./out with plain HTML/CSS/JS that any
  // server (including our embedded FS) can serve with no Node
  // runtime. Rewrites + redirects are dropped because the export
  // has no server to evaluate them — the client talks to /v1/*
  // directly and the Go gateway serves both the static assets and
  // the API from the same origin.
  ...(isStaticBuild
    ? {
        output: 'export',
        images: { unoptimized: true },
        // Keep trailing slashes so nested routes resolve as index.html.
        trailingSlash: true,
      }
    : {
        async redirects() {
          return [
            { source: '/memory', destination: '/memories', permanent: true },
            { source: '/memory/:path*', destination: '/memories', permanent: true },
          ];
        },
        async rewrites() {
          return [
            {
              source: '/api/auth/:path*',
              destination: `${process.env.NEXT_PUBLIC_API_URL}/auth/:path*`,
            },
            {
              source: '/api/health',
              destination: `${process.env.NEXT_PUBLIC_API_URL}/health`,
            },
            {
              source: '/api/health/detailed',
              destination: `${process.env.NEXT_PUBLIC_API_URL}/health/detailed`,
            },
            {
              source: '/api/v1/:path*',
              destination: `${process.env.NEXT_PUBLIC_API_URL}/v1/:path*`,
            },
            {
              source: '/api/ws',
              destination: `${process.env.NEXT_PUBLIC_API_URL}/ws`,
            },
            {
              source: '/api/ws/voice',
              destination: `${process.env.NEXT_PUBLIC_API_URL}/ws/voice`,
            },
            {
              source: '/api/ws/realtime',
              destination: `${process.env.NEXT_PUBLIC_API_URL}/ws/realtime`,
            },
            // Direct WS paths — same as above without /api prefix so
            // wsBase('/ws/...') and wsBase('/v1/.../ws') connect through
            // the dev server proxy to the backend.
            {
              source: '/ws/realtime',
              destination: `${process.env.NEXT_PUBLIC_API_URL}/ws/realtime`,
            },
            {
              source: '/ws/voice',
              destination: `${process.env.NEXT_PUBLIC_API_URL}/ws/voice`,
            },
            {
              source: '/ws',
              destination: `${process.env.NEXT_PUBLIC_API_URL}/ws`,
            },
            {
              source: '/v1/:path*/ws',
              destination: `${process.env.NEXT_PUBLIC_API_URL}/v1/:path*/ws`,
            },
            {
              source: '/__qorven_runtime',
              destination: `${process.env.NEXT_PUBLIC_API_URL}/__qorven_runtime`,
            },
            {
              source: '/livez',
              destination: `${process.env.NEXT_PUBLIC_API_URL}/livez`,
            },
            {
              source: '/readyz',
              destination: `${process.env.NEXT_PUBLIC_API_URL}/readyz`,
            },
          ];
        },
      }),
};

export default nextConfig;
