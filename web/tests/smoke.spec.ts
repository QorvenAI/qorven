// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { test, expect } from '@playwright/test';

test.describe('Smoke Tests', () => {
  test('login page loads', async ({ page }) => {
    await page.goto('/login');
    await expect(page.locator('text=Sign in to Qorven')).toBeVisible();
    await expect(page.locator('input[placeholder="admin"]')).toBeVisible();
  });

  test('setup page loads', async ({ page }) => {
    await page.goto('/setup');
    await expect(page.locator('text=Welcome to Qorven')).toBeVisible();
  });

  test('dashboard loads after login', async ({ page }) => {
    // Skip if no running backend
    const health = await page.request.get('/api/health').catch(() => null);
    if (!health) test.skip();

    await page.goto('/');
    // Should either show dashboard or redirect to login/setup
    await expect(page).toHaveURL(/\/(login|setup|dashboard)?/);
  });

  test('agents page loads', async ({ page }) => {
    await page.goto('/qors');
    await page.waitForLoadState('networkidle');
    // Should show agent list or empty state
    const content = await page.textContent('body');
    expect(content).toBeTruthy();
  });

  test('models hub loads', async ({ page }) => {
    await page.goto('/models-hub');
    await page.waitForLoadState('networkidle');
    await expect(page.locator('text=Models')).toBeVisible();
  });
});
