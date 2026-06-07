import { test, expect } from '@playwright/test';

test.describe('V2Ray Client Dashboard E2E Tests', () => {
  test.beforeEach(async ({ page }) => {
    // Inject mock authentication token
    await page.addInitScript(() => {
      window.localStorage.setItem('cc_auth', 'true');
      window.localStorage.setItem('cc_client_token', 'mock-client-token');
    });

    // Mock API requests
    await page.route('**/api/auth/session', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ authenticated: true }),
      });
    });

    await page.route('**/api/v2ray/inbounds', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify([
          { ID: 1, tag: 'vless-client-inbound', port: 10808, protocol: 'vless', network: 'tcp' },
        ]),
      });
    });

    await page.route('**/api/v2ray/core/status', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ is_running: true, socks_port: 10808, http_port: 10809 }),
      });
    });

    await page.route('**/api/v2ray/subscriptions', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify([
          { ID: 1, name: 'SG Premium Sub', url: 'https://example.com/sub' },
        ]),
      });
    });

    await page.route('**/api/v2ray/settings', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          core_binary: 'xray',
          socks_port: 10808,
          http_port: 10809,
          dpi_evasion: false,
          custom_routes: '',
        }),
      });
    });
  });

  test('should load the client page and render all modular cards', async ({ page }) => {
    // Navigate to the client dashboard
    await page.goto('/v2ray-dashboard');

    // Wait for the lazy components to finish skeleton loading and render
    await page.waitForSelector('text=Core Supervisor: RUNNING');

    // Check header title
    await expect(page.locator('h1')).toHaveText('V2Ray / Xray manager');

    // Check main panels
    await expect(page.locator('text=Subscriptions & Profiles')).toBeVisible();
    await expect(page.locator('text=CDN Scanner')).toBeVisible();
    await expect(page.locator('text=Settings & Presets')).toBeVisible();
    await expect(page.locator('text=Daemon console outputs')).toBeVisible();
    await expect(page.locator('text=TCP/UDP Port Diagnostics')).toBeVisible();
    await expect(page.locator('text=Device Discovery')).toBeVisible();
    await expect(page.locator('text=Wake on LAN')).toBeVisible();
    await expect(page.locator('text=Local CONNECT Interception Proxy')).toBeVisible();
    await expect(page.locator('text=Tray Options & Hotkeys')).toBeVisible();
  });
});
