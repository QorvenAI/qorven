// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

// End-to-end rendering tests for every widget card. Uses the
// /__cardfixture route as a harness — the fixture page reads a
// base64-encoded JSON widget from ?data= and renders it via the
// real MessageParts / WidgetPartRenderer pipeline. If the registry
// dispatch breaks, or a card silently regresses, these fail.
//
// No backend required — fixture is entirely client-side.

import { test, expect } from '@playwright/test';

// Small helper: encode a widget and navigate to the fixture page.
function cardURL(widget: { type: string; data: unknown }): string {
  const b64 = Buffer.from(JSON.stringify(widget)).toString('base64');
  return `/__cardfixture?data=${b64}`;
}

test.describe('Card rendering', () => {
  test('weather — current + forecast', async ({ page }) => {
    await page.goto(cardURL({
      type: 'weather',
      data: {
        location: 'Tokyo',
        temperature: 21.4,
        feels_like: 20.1,
        condition: 'partly cloudy',
        humidity: 58,
        wind_speed: 12.3,
        daily: {
          time: ['2026-04-21', '2026-04-22', '2026-04-23'],
          temperature_2m_max: [23, 20, 22],
          temperature_2m_min: [15, 14, 16],
          weather_code: [2, 61, 3],
          precipitation_probability_max: [0, 60, 10],
        },
      },
    }));
    await expect(page.locator('text=Tokyo')).toBeVisible();
    // Temperature appears somewhere in the card (card formats
    // differently depending on unit — look for the integer).
    await expect(page.locator('text=/21/').first()).toBeVisible();
  });

  test('flights — single option with direct + stop', async ({ page }) => {
    await page.goto(cardURL({
      type: 'flights',
      data: {
        query: 'SFO to JFK, Mar 12',
        options: [
          {
            segments: [{
              airline: 'Delta', airline_code: 'DL',
              from_airport: 'SFO', to_airport: 'JFK',
              depart_time: '10:00', arrive_time: '18:30',
              duration: '5h 30m', stops: 0,
            }],
            price: 420,
            currency: 'USD',
            provider: 'Google Flights',
            booking_url: 'https://example.com/flight/1',
          },
          {
            segments: [{
              airline: 'United', airline_code: 'UA',
              from_airport: 'SFO', to_airport: 'JFK',
              depart_time: '12:00', arrive_time: '22:45',
              duration: '7h 45m', stops: 1,
            }],
            price: 310,
            currency: 'USD',
            provider: 'Skyscanner',
          },
        ],
      },
    }));
    await expect(page.locator('text=SFO')).toHaveCount(2);
    await expect(page.locator('text=JFK')).toHaveCount(2);
    await expect(page.locator('text=/direct/i')).toBeVisible();
    await expect(page.locator('text=/1 stop/i')).toBeVisible();
    // Prices rendered by Intl.NumberFormat — check the dollar sign.
    await expect(page.locator('text=/\\$420/')).toBeVisible();
    await expect(page.locator('text=/\\$310/')).toBeVisible();
  });

  test('hotels — price, rating, amenities', async ({ page }) => {
    await page.goto(cardURL({
      type: 'hotels',
      data: {
        options: [
          {
            name: 'Hotel Okura',
            city: 'Tokyo',
            rating: 4.7,
            rating_count: 2340,
            price_per_night: 380,
            currency: 'USD',
            amenities: ['wifi', 'breakfast', 'gym', 'pool'],
            distance_km: 1.2,
            provider: 'Booking.com',
            booking_url: 'https://example.com/hotel/1',
          },
        ],
      },
    }));
    await expect(page.locator('text=Hotel Okura')).toBeVisible();
    await expect(page.locator('text=/4\\.7/')).toBeVisible();
    await expect(page.locator('text=wifi')).toBeVisible();
    await expect(page.locator('text=pool')).toBeVisible();
  });

  test('sports score — live badge + team scores', async ({ page }) => {
    await page.goto(cardURL({
      type: 'sports_score',
      data: {
        sport: 'cricket',
        league: 'IPL 2026',
        is_live: true,
        home: { name: 'Mumbai Indians', short: 'MI', score: '187/4', subscore: '19.2 overs' },
        away: { name: 'Chennai Super Kings', short: 'CSK', score: '132/6', subscore: '16.1 overs' },
        venue: 'Wankhede Stadium',
      },
    }));
    await expect(page.locator('text=IPL 2026')).toBeVisible();
    await expect(page.locator('text=/LIVE/')).toBeVisible();
    await expect(page.locator('text=MI')).toBeVisible();
    await expect(page.locator('text=CSK')).toBeVisible();
    await expect(page.locator('text=187/4')).toBeVisible();
  });

  test('sql result — columns + rows + truncation', async ({ page }) => {
    await page.goto(cardURL({
      type: 'sql_result',
      data: {
        columns: ['id', 'name', 'email', 'active'],
        rows: [
          [1, 'Alice', 'alice@example.com', true],
          [2, 'Bob', 'bob@example.com', false],
          [3, 'Carol', null, true],
        ],
        truncated: false,
        query: 'SELECT id, name, email, active FROM users LIMIT 3',
        connection: 'prod',
      },
    }));
    // Headers
    await expect(page.locator('th:has-text("id")')).toBeVisible();
    await expect(page.locator('th:has-text("email")')).toBeVisible();
    // Rows
    await expect(page.locator('td:has-text("Alice")')).toBeVisible();
    await expect(page.locator('td:has-text("Bob")')).toBeVisible();
    // Null rendered visually
    await expect(page.locator('text=null').first()).toBeVisible();
    // Query shown in header
    await expect(page.locator('text=/SELECT id/')).toBeVisible();
  });

  test('sql result — truncation banner fires', async ({ page }) => {
    await page.goto(cardURL({
      type: 'sql_result',
      data: {
        columns: ['x'],
        rows: [[1], [2]],
        truncated: true,
        total_rows: 1000,
      },
    }));
    await expect(page.locator('text=/truncated/i')).toBeVisible();
  });

  test('screenshot — dimensions overlay', async ({ page }) => {
    const tinyPng = 'data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABAQMAAAAl21bKAAAAA1BMVEUAAACnej3aAAAAAXRSTlMAQObYZgAAAApJREFUCNdjYAAAAAIAAeIhvDMAAAAASUVORK5CYII=';
    await page.goto(cardURL({
      type: 'screenshot',
      data: {
        url: tinyPng,
        width: 1920,
        height: 1080,
        source: 'browser',
        url_context: 'https://news.ycombinator.com',
      },
    }));
    await expect(page.locator('text=Agent browser')).toBeVisible();
    await expect(page.locator('text=/1920.1080/')).toBeVisible();
    await expect(page.locator('text=news.ycombinator.com')).toBeVisible();
  });

  test('browser step — action + thoughts + screenshot', async ({ page }) => {
    await page.goto(cardURL({
      type: 'browser_step',
      data: {
        step: 3,
        url: 'https://example.com/login',
        action_type: 'click',
        thoughts: 'Clicking the Sign In button',
        result: 'clicked (520, 340)',
        duration_ms: 450,
      },
    }));
    await expect(page.locator('text=/Click/i')).toBeVisible();
    await expect(page.locator('text=#3')).toBeVisible();
    await expect(page.locator('text=Clicking the Sign In button')).toBeVisible();
    await expect(page.locator('text=/450ms/')).toBeVisible();
  });

  test('unknown widget type — fixture doesn\'t crash', async ({ page }) => {
    // Registry's default branch returns null — MessageParts skips.
    // The page renders but our widget slot is empty.
    await page.goto(cardURL({ type: 'fake_widget_type', data: {} }));
    await expect(page.locator('[data-testid="fixture-container"]')).toBeVisible();
    // No error banner.
    await expect(page.locator('[data-testid="fixture-error"]')).toHaveCount(0);
  });

  test('missing data param — clear error', async ({ page }) => {
    await page.goto('/__cardfixture');
    await expect(page.locator('[data-testid="fixture-error"]')).toBeVisible();
    await expect(page.locator('text=/missing/i')).toBeVisible();
  });

  test('malformed data param — clear error', async ({ page }) => {
    await page.goto('/__cardfixture?data=not-valid-base64-json');
    await expect(page.locator('[data-testid="fixture-error"]')).toBeVisible();
  });
});
