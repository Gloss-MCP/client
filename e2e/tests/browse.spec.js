import { test, expect } from '@playwright/test';

test('shows the tracked file tree and hides ignored files', async ({ page }) => {
  await page.goto('/');

  await expect(page.getByRole('link', { name: 'README.md' })).toBeVisible();
  await expect(page.getByRole('link', { name: 'main.go' })).toBeVisible();

  // .glossignore excludes notes/ and *.draft in this (non-git) fixture.
  await expect(page.getByRole('link', { name: 'notes' })).toHaveCount(0);
  await expect(page.getByRole('link', { name: 'scratch.draft' })).toHaveCount(0);

  await expect(page.locator('#file-content')).toContainText('Select a file');
});

test('reads a file via the tree without a full page reload', async ({ page }) => {
  await page.goto('/');

  await page.getByRole('link', { name: 'README.md' }).click();

  await expect(page).toHaveURL(/\/files\/README\.md$/);
  await expect(page.locator('#file-content')).toContainText('Fixture repo');
  // The sidebar tree persists across the navigation -- proof it was an
  // HTMX partial swap of #file-content, not a full page reload.
  await expect(page.getByRole('link', { name: 'main.go' })).toBeVisible();
});

test('a direct URL to a file renders the full page with content', async ({ page }) => {
  await page.goto('/files/main.go');

  await expect(page.locator('#file-content')).toContainText('package fixture');
  await expect(page.getByRole('link', { name: 'README.md' })).toBeVisible();
});

test('an unknown or ignored path 404s', async ({ page }) => {
  const resp = await page.goto('/files/does/not/exist.txt');
  expect(resp.status()).toBe(404);
});
