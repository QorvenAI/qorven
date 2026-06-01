// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).
// Re-exports the auth guard from proxy.ts as the Next.js middleware entry point.
export { proxy as middleware, config } from './proxy';
