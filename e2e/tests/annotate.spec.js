import { test, expect } from '@playwright/test';

// Milestone 5's exit criterion: "full human review loop works
// end-to-end under Playwright" -- create a session, anchor a thread to
// a line in a file, reply, and resolve/reopen it, all through the UI.

test('full annotation loop: session, thread, reply, resolve, reopen', async ({ page }) => {
  await page.goto('/');

  // The session switcher on the plain browse page starts expanded (no
  // active session to collapse it around), so the new-session form is
  // already visible.
  await page.getByPlaceholder('New session name').fill('Full loop review');
  await page.getByRole('button', { name: '+ New session' }).click();

  await expect(page).toHaveURL(/\/s\/[^/]+$/);
  const sessionURL = page.url();

  // Open a file within the session and select its first line.
  await page.getByRole('link', { name: 'README.md' }).click();
  await expect(page).toHaveURL(/\/s\/[^/]+\/files\/README\.md$/);

  await page.locator('[data-line="1"]').click();
  await expect(page.getByPlaceholder('Add a comment...')).toBeVisible();
  await page.getByPlaceholder('Add a comment...').fill('What is this file for?');
  await page.getByRole('button', { name: 'Comment', exact: true }).click();

  // The new thread and its root comment show in the thread panel, and
  // the annotated line is highlighted in the gutter.
  await expect(page.getByText('What is this file for?')).toBeVisible();
  await expect(page.locator('[data-line="1"]')).toHaveClass(/bg-amber-50/);

  // Reply to the thread. Scope to the thread-level reply form so this
  // doesn't collide with each comment's own "Reply" toggle button.
  const replyForm = page.locator('form', { has: page.getByPlaceholder('Reply...') });
  await replyForm.getByPlaceholder('Reply...').fill('It is the project README.');
  await replyForm.getByRole('button', { name: 'Reply', exact: true }).click();
  await expect(page.getByText('It is the project README.')).toBeVisible();

  // Resolve, then reopen.
  await page.getByRole('button', { name: 'Resolve' }).click();
  await expect(page.getByRole('button', { name: 'Reopen' })).toBeVisible();
  await page.getByRole('button', { name: 'Reopen' }).click();
  await expect(page.getByRole('button', { name: 'Resolve' })).toBeVisible();

  // The thread also shows up in the session-wide thread list, labeled
  // with the file it's anchored to (distinct from the plain "README.md"
  // tree link text, so this also confirms it's the thread-panel label).
  await page.goto(sessionURL);
  await expect(page.getByText('What is this file for?')).toBeVisible();
  await expect(page.getByText('README.md · line 1')).toBeVisible();
});
