// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.
//
// Unit tests for channel-schemas completeness.
// Run with: pnpm test:unit

import { test } from 'node:test';
import assert from 'node:assert/strict';

// Import the compiled schema — node strips types natively in Node 24.
import { channelFormSchemas, CHANNEL_TYPES } from '../../components/channels/channel-schemas.ts';

const ALL_20: string[] = [
  'telegram', 'discord', 'slack', 'whatsapp', 'email', 'sms',
  'teams', 'github', 'webchat', 'webhook',
  'signal', 'imessage', 'facebook', 'line', 'zalo',
  'feishu', 'dingtalk', 'wecom', 'matrix', 'mattermost',
];

test('CHANNEL_TYPES contains all 20 types', () => {
  assert.equal(CHANNEL_TYPES.length, 20, `expected 20 types, got ${CHANNEL_TYPES.length}: ${CHANNEL_TYPES.join(', ')}`);
  for (const t of ALL_20) {
    assert.ok(CHANNEL_TYPES.includes(t as any), `missing type: ${t}`);
  }
});

test('every schema has officialLink (string)', () => {
  for (const type of CHANNEL_TYPES) {
    const schema = channelFormSchemas[type];
    assert.equal(typeof schema.officialLink, 'string', `${type}.officialLink must be a string`);
  }
});

test('every schema has docsSlug (non-empty string)', () => {
  for (const type of CHANNEL_TYPES) {
    const schema = channelFormSchemas[type];
    assert.ok(schema.docsSlug.length > 0, `${type}.docsSlug must be non-empty`);
  }
});

test('every schema has at least 2 setupSteps', () => {
  for (const type of CHANNEL_TYPES) {
    const schema = channelFormSchemas[type];
    assert.ok(schema.setupSteps.length >= 2, `${type} needs at least 2 setupSteps, got ${schema.setupSteps.length}`);
  }
});

test('every schema has at least 1 field', () => {
  for (const type of CHANNEL_TYPES) {
    const schema = channelFormSchemas[type];
    assert.ok(schema.fields.length > 0, `${type} must have at least 1 field`);
  }
});

test('every schema has non-empty label, icon, description, color', () => {
  for (const type of CHANNEL_TYPES) {
    const schema = channelFormSchemas[type];
    assert.ok(schema.label.length > 0, `${type}.label is empty`);
    assert.ok(schema.icon.length > 0, `${type}.icon is empty`);
    assert.ok(schema.description.length > 0, `${type}.description is empty`);
    assert.ok(schema.color.length > 0, `${type}.color is empty`);
  }
});
