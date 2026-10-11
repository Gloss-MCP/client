import { test, expect } from '@playwright/test';
import { writeFileSync, readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';

// Milestone 8 exit criterion: "fixture corpus renders and annotates
// correctly per format." Each test annotates a file in one format through
// the UI, edits the file on disk, and checks the thread follows.

const here = path.dirname(fileURLToPath(import.meta.url));
const corpus = (name) => path.resolve(here, '../../testdata/plugins', name);
const live = (name) => path.join(process.env.GLOSS_FIXTURE_DIR ?? path.join(process.env.TMPDIR ?? '/tmp', 'gloss-e2e-fixture'), 'formats', name);

// Restore the corpus files these tests modify.
test.beforeEach(() => {
  for (const name of ['doc.md', 'prices.csv']) {
    writeFileSync(live(name), readFileSync(corpus(name)));
  }
});

// The UI does not live-refresh; reload until the watcher's result shows.
async function expectAfterReload(page, assertion, timeout = 8000) {
  await expect(async () => {
    await page.reload();
    await assertion();
  }).toPass({ timeout });
}

async function openInNewSession(page, name, file) {
  await page.goto('/');
  await page.getByPlaceholder('New session name').fill(name);
  await page.getByRole('button', { name: '+ New session' }).click();
  await expect(page).toHaveURL(/\/s\/[^/]+$/);
  await page.goto(`${page.url()}/files/formats/${file}`);
}

async function comment(page, body) {
  await expect(page.getByPlaceholder('Add a comment...')).toBeVisible();
  await page.getByPlaceholder('Add a comment...').fill(body);
  await page.getByRole('button', { name: 'Comment', exact: true }).click();
  await expect(page.getByText(body)).toBeVisible();
}

test('code: highlighted tokens are annotatable and multi-line tokens keep numbering', async ({ page }) => {
  await openInNewSession(page, 'Code format', 'code.go');
  await expect(page.locator('.gloss-code span.k').first()).toBeVisible(); // highlighted keyword
  // Lines inside the multi-line block comment are individually addressable.
  await page.locator('[data-line="4"]').click();
  await expect(page.getByText('Comment on line 4')).toBeVisible();
  await comment(page, 'inside the block comment');
  await expect(page.locator('.border-gray-200').getByText('line 4', { exact: true })).toBeVisible();
});

test('json: raw view is annotatable, formatted view is read-only', async ({ page }) => {
  await openInNewSession(page, 'Json format', 'config.json');
  await page.getByRole('button', { name: 'Formatted', exact: true }).click();
  await expect(page.locator('.gloss-readonly')).toBeVisible();
  await expect(page.locator('.gloss-readonly [data-line]')).toHaveCount(0);
  await page.getByRole('button', { name: 'Raw', exact: true }).click();
  await page.locator('[data-line="1"]').click();
  await comment(page, 'is "tags" needed?');
});

test('markdown: annotate a rendered block; the thread follows an edit', async ({ page }) => {
  await openInNewSession(page, 'Markdown format', 'doc.md');
  // The intro paragraph spans source lines 3-4.
  await page.locator('.gloss-md [data-line="3"]').click();
  await expect(page.getByText('Comment on line 3-4')).toBeVisible();
  await comment(page, 'clarify the intro');
  await expect(page.locator('.border-gray-200').getByText('line 3-4', { exact: true })).toBeVisible();

  // Two lines inserted above the paragraph: the thread moves to 5-6.
  const src = readFileSync(corpus('doc.md'), 'utf8');
  writeFileSync(live('doc.md'), src.replace('# Corpus document\n', '# Corpus document\n\nA new line.\n'));
  await expectAfterReload(page, () => expect(page.getByText('line 5-6', { exact: true })).toBeVisible({ timeout: 500 }));
});

test('markdown: source tab annotates by line', async ({ page }) => {
  await openInNewSession(page, 'Markdown source', 'doc.md');
  await page.getByRole('button', { name: 'Source', exact: true }).click();
  await page.locator('.gloss-line[data-line="1"]').click();
  await expect(page.getByText('Comment on line 1')).toBeVisible();
  await comment(page, 'title looks fine');
});

test('csv: annotate one cell; the thread follows a column insertion', async ({ page }) => {
  await openInNewSession(page, 'Csv format', 'prices.csv');
  // apple's price: record on line 2, column 3.
  await page.locator('tr[data-line="2"] td[data-col="3"]').click();
  await expect(page.getByText('Comment on line 2, column 3')).toBeVisible();
  await expect(page.locator('.gloss-cell-selected')).toHaveCount(1);
  await comment(page, 'is this price right?');
  await expect(page.getByText('line 2, col 3 (price)')).toBeVisible();

  // A sku column is inserted before price and a new row is added: the
  // thread must follow to line 3, column 4 (price), not stay on column 3.
  const modified = [
    'id,name,sku,price,notes',
    '0,zero,z0,0.00,',
    '1,apple,a1,0.50,fresh',
    '2,banana,b2,0.25,"two-line',
    'note"',
    '3,cherry,c3,4.00,',
    '4,"date, medjool",d4,3.00,<b>bold</b>',
    '',
  ].join('\n');
  writeFileSync(live('prices.csv'), modified);
  await expectAfterReload(page, () => expect(page.getByText('line 3, col 4 (price)')).toBeVisible({ timeout: 500 }));
});

test('csv: removing the anchored column orphans the thread visibly', async ({ page }) => {
  await openInNewSession(page, 'Csv orphan', 'prices.csv');
  await page.locator('tr[data-line="2"] td[data-col="3"]').click();
  await comment(page, 'price question');

  writeFileSync(live('prices.csv'), 'id,name,notes\n1,apple,fresh\n2,banana,"two-line\nnote"\n3,cherry,\n4,"date, medjool",<b>bold</b>\n');
  await expectAfterReload(page, () => expect(page.locator('[id^="thread-"] span.uppercase')).toHaveText('orphaned', { timeout: 500 }));
  await expect(page.getByText('price question')).toBeVisible(); // never vanishes
});

test('csv: quoted-newline record is one selectable row', async ({ page }) => {
  await openInNewSession(page, 'Csv multiline', 'prices.csv');
  const row = page.locator('tr[data-line="3"]');
  await expect(row).toHaveAttribute('data-line-end', '4');
  await row.locator('td[data-col="4"]').click();
  await expect(page.getByText('Comment on line 3-4, column 4')).toBeVisible();
});
