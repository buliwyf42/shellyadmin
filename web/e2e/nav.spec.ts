import { test, expect, type Page } from '@playwright/test';

// Credentials match `make dev-backend` (admin / dev-secret); override via env
// for other instances.
const USER = process.env.E2E_USER || 'admin';
const PASS = process.env.E2E_PASS || 'dev-secret';

async function login(page: Page) {
  await page.goto('/login');
  await page.fill('#login-username', USER);
  await page.fill('#login-password', PASS);
  await page.getByRole('button', { name: 'Sign In' }).click();
  await expect(page.locator('nav.topbar')).toBeVisible();
}

test('login lands on the shell with the nav', async ({ page }) => {
  await login(page);
  await expect(page.locator('.topnav-link', { hasText: 'Devices' })).toBeVisible();
  // Version badge renders a single leading "v" (regression guard for vv0.3.x).
  await expect(page.locator('.brand-version')).toHaveText(/^v(?!v)/);
});

test('desktop shows the horizontal nav, no hamburger', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 });
  await login(page);
  await expect(page.locator('.nav-toggle')).toBeHidden();
  await expect(page.locator('#topnav-collapse')).toBeVisible();
});

test('mobile collapses the nav into a hamburger drawer', async ({ page }) => {
  await page.setViewportSize({ width: 375, height: 812 });
  await login(page);

  const toggle = page.locator('.nav-toggle');
  const drawer = page.locator('#topnav-collapse');

  await expect(toggle).toBeVisible();
  await expect(drawer).toBeHidden();

  await toggle.click();
  await expect(drawer).toBeVisible();

  // Clicking a link navigates and closes the drawer (closeNav).
  await page.locator('.topnav-main a[href="/logs"]').click();
  await expect(page).toHaveURL(/\/logs$/);
  await expect(drawer).toBeHidden();
});
