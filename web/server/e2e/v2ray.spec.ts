import { test, expect } from '@playwright/test';

test.describe('V2Ray Server Dashboard E2E Tests', () => {
  test.beforeEach(async ({ page }) => {
    // Inject mock authentication token
    await page.addInitScript(() => {
      window.localStorage.setItem('cc_auth', 'true');
      window.localStorage.setItem('cc_server_token', 'mock-server-token');
    });

    // Mock API requests
    await page.route('**/api/auth/session', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ authenticated: true }),
      });
    });

    await page.route('**/api/v2ray/nodes', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify([
          { ID: 1, name: 'SG-Edge-01', ip: '128.199.100.12', ssh_port: 22, status: 'online' },
        ]),
      });
    });

    await page.route('**/api/v2ray/inbounds', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify([
          { ID: 1, tag: 'vless-reality-443', port: 443, protocol: 'vless', network: 'tcp', tls_mode: 'reality' },
        ]),
      });
    });

    await page.route('**/api/v2ray/users', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify([
          { ID: 1, name: 'user01', uuid: 'some-uuid', used_upload: 1024, used_download: 2048, traffic_limit: 0 },
        ]),
      });
    });

    await page.route('**/api/v2ray/traffic/logs', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify([
          { name: 'user01', upload: 512, download: 1024 },
        ]),
      });
    });

    await page.route('**/api/v2ray/core/status', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ is_running: true }),
      });
    });
  });

  test('should load the server page and render all modular cards', async ({ page }) => {
    // Navigate to the server dashboard
    await page.goto('/v2ray-dashboard');

    // Wait for the lazy components to finish skeleton loading and render
    await page.waitForSelector('text=SG-Edge-01');

    // Check header title
    await expect(page.locator('h1')).toHaveText('V2Ray / Xray Server panel');

    // Check main panels
    await expect(page.locator('text=Remote VPS Edge Nodes')).toBeVisible();
    await expect(page.locator('text=Server Inbound Endpoints')).toBeVisible();
    await expect(page.locator('text=User Quota Auditing')).toBeVisible();
    await expect(page.locator('text=User Traffic Audits')).toBeVisible();
    await expect(page.locator('text=Server core engine')).toBeVisible();
    await expect(page.locator('text=WebDAV log access')).toBeVisible();
    await expect(page.locator('text=Fail2ban port protection')).toBeVisible();
    await expect(page.locator('text=MCP RPC diagnostic prober')).toBeVisible();
    await expect(page.locator('text=Realtime Webhook notifier')).toBeVisible();
  });
});
