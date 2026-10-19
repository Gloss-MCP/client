import { test, expect } from '@playwright/test';

// Theme picker: light / dark / system (default), persisted in localStorage.
// <html data-theme> always holds the resolved light|dark.

const theme = (page) => page.locator('html').getAttribute('data-theme');
const pressed = (page, name) =>
  expect(page.getByRole('button', { name, exact: true })).toHaveAttribute('aria-pressed', 'true');

test.describe('system dark', () => {
  test.use({ colorScheme: 'dark' });

  test('defaults to system and follows the OS', async ({ page }) => {
    await page.goto('/');
    expect(await theme(page)).toBe('dark');
    await pressed(page, 'System');

    await page.emulateMedia({ colorScheme: 'light' });
    await expect.poll(() => theme(page)).toBe('light');
  });

  test('an explicit choice persists across reloads and beats the OS', async ({ page }) => {
    await page.goto('/');
    await page.getByRole('button', { name: 'Light', exact: true }).click();
    expect(await theme(page)).toBe('light');
    expect(await page.evaluate(() => localStorage.getItem('gloss-theme'))).toBe('light');

    await page.reload();
    expect(await theme(page)).toBe('light');
    await pressed(page, 'Light');

    // Choosing System clears the stored preference and follows the OS again.
    await page.getByRole('button', { name: 'System', exact: true }).click();
    expect(await theme(page)).toBe('dark');
    expect(await page.evaluate(() => localStorage.getItem('gloss-theme'))).toBeNull();
  });
});

test('dark repaints the page', async ({ page }) => {
  await page.goto('/');
  const bg = () => page.evaluate(() => getComputedStyle(document.body).backgroundColor);
  await page.getByRole('button', { name: 'Light', exact: true }).click();
  const light = await bg();
  await page.getByRole('button', { name: 'Dark', exact: true }).click();
  expect(await bg()).not.toBe(light);
});
