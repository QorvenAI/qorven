// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

// Barrel re-export — consumers that import from '@/lib/api' continue to work unchanged.
// New code should import directly from the domain module.

export * from './api-core';
export * from './api-agents';
export * from './api-providers';
export * from './api-workspace';
export * from './api-content';
export * from './api-github';
export * from './api-inbound';
export * from './api-apps';
export * from './api-dashboard';
export * from './api-permissions';
