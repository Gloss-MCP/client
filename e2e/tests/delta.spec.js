import { test, expect } from '@playwright/test';
import { writeFileSync, readFileSync } from 'node:fs';
import path from 'node:path';

// Milestone 7 exit criterion: "edit a file mid-review; threads follow or
// orphan visibly — never vanish."
//
// Each test creates a fresh session, anchors a thread to a line in
// sample.go, then modifies the file on disk. The watcher picks up the
// change within its debounce window (150 ms) and remaps or orphans the
// thread. Tests poll up to 5 s for the UI to reflect the change.

const fixtureDir = process.env.GLOSS_FIXTURE_DIR;
const samplePath = () => path.join(fixtureDir, 'sample.go');

// originalContent is the sample.go committed in testdata/connector/fixture.
// Keep in sync with that file — line numbers matter.
const originalContent = `package fixture

// sample.go — used by delta-tracking e2e tests.
// Do not remove or reorder these lines; the e2e tests anchor to specific
// line numbers and rely on exact context above and below.

func alpha() string {
\treturn "alpha"
}

func beta() string {
\treturn "beta"
}

func gamma() string {
\treturn "gamma"
}
`;

// Restore sample.go before every test so each runs against a clean copy.
test.beforeEach(() => {
  writeFileSync(samplePath(), originalContent);
});

// ---------------------------------------------------------------------------
// Helper: create a session and navigate to the sample.go file view, then
// anchor a thread at the given line. Returns the session URL.
// ---------------------------------------------------------------------------
async function createSessionWithThread(page, sessionName, line, body) {
  await page.goto('/');
  await page.getByPlaceholder('New session name').fill(sessionName);
  await page.getByRole('button', { name: '+ New session' }).click();
  await expect(page).toHaveURL(/\/s\/[^/]+$/);
  const sessionURL = page.url();

  await page.getByRole('link', { name: 'sample.go' }).click();
  await expect(page).toHaveURL(/\/s\/[^/]+\/files\/sample\.go$/);

  await page.locator(`[data-line="${line}"]`).click();
  await expect(page.getByPlaceholder('Add a comment...')).toBeVisible();
  await page.getByPlaceholder('Add a comment...').fill(body);
  await page.getByRole('button', { name: 'Comment', exact: true }).click();

  await expect(page.getByText(body)).toBeVisible();
  return sessionURL;
}

// ---------------------------------------------------------------------------
// Helper: poll until the thread panel contains text matching the predicate
// or the timeout elapses.
// ---------------------------------------------------------------------------
async function waitForThreadText(page, textOrRE, timeout = 5000) {
  await expect(page.getByText(textOrRE)).toBeVisible({ timeout });
}

// ---------------------------------------------------------------------------
// Test 1: Lines inserted before the anchor — thread follows.
// ---------------------------------------------------------------------------
test('remap: lines inserted before anchor shift the thread', async ({ page }) => {
  // Anchor on line 12: `\treturn "beta"` — context lines 9-11, 13-15.
  await createSessionWithThread(page, 'Delta remap test', 12, 'Check the beta return value');

  // Verify initial anchor label.
  await expect(page.getByText('line 12')).toBeVisible();

  // Insert 2 lines before func beta (before line 11). The anchor shifts
  // from line 12 → 14.
  const modified = originalContent.replace(
    'func beta() string {',
    'func betaHelper() {}\n\nfunc beta() string {'
  );
  writeFileSync(samplePath(), modified);

  // Poll until the thread panel reflects the new anchor position.
  // The watcher debounces at 150 ms; give it up to 5 s total.
  await expect(page.getByText('line 14')).toBeVisible({ timeout: 5000 });
  // Status must remain ACTIVE.
  await expect(page.getByText('ACTIVE')).toBeVisible();
});

// ---------------------------------------------------------------------------
// Test 2: File content replaced entirely — thread is orphaned.
// ---------------------------------------------------------------------------
test('orphan: thread orphaned when context is gone', async ({ page }) => {
  await createSessionWithThread(page, 'Orphan test', 12, 'Reviewing beta function');

  // Replace the file with completely different content.
  writeFileSync(samplePath(), 'package fixture\n\nfunc unrelated() {}\n');

  // Poll until the orphaned badge and Reopen button appear.
  await expect(page.getByText('ORPHANED')).toBeVisible({ timeout: 5000 });
  await expect(page.getByRole('button', { name: 'Reopen' })).toBeVisible();
  // Resolve button must NOT be shown for an orphaned thread.
  await expect(page.getByRole('button', { name: 'Resolve' })).not.toBeVisible();
  // Yellow styling — verify the thread card has the orphaned class.
  await expect(page.locator('.border-yellow-400')).toBeVisible();
});

// ---------------------------------------------------------------------------
// Test 3: Reopening an orphaned thread sets it back to ACTIVE.
// ---------------------------------------------------------------------------
test('reopen: orphaned thread can be reopened', async ({ page }) => {
  const sessionURL = await createSessionWithThread(page, 'Reopen test', 12, 'Will be orphaned');

  writeFileSync(samplePath(), 'package fixture\n\nfunc unrelated() {}\n');
  await expect(page.getByText('ORPHANED')).toBeVisible({ timeout: 5000 });

  await page.getByRole('button', { name: 'Reopen' }).click();

  await expect(page.getByText('ACTIVE')).toBeVisible();
  await expect(page.getByRole('button', { name: 'Resolve' })).toBeVisible();
  // Yellow border should be gone.
  await expect(page.locator('.border-yellow-400')).not.toBeVisible();
});

// ---------------------------------------------------------------------------
// Test 4: File deleted — all threads for that file are orphaned.
// ---------------------------------------------------------------------------
test('orphan: file deletion orphans all threads', async ({ page }) => {
  const sessionURL = await createSessionWithThread(page, 'Delete test', 8, 'Alpha return');

  const { unlinkSync } = await import('node:fs');
  unlinkSync(samplePath());

  await expect(page.getByText('ORPHANED')).toBeVisible({ timeout: 5000 });
});
