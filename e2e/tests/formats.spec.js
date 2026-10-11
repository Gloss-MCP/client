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
  for (const name of ['doc.md', 'prices.csv', 'pixel.png', 'tone.wav', 'clip.webm']) {
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

// ---- Milestone 9: media ----

// mcpCall speaks streamable-HTTP MCP to the running server with
// Playwright's request client: initialize, then one tools/call.
async function mcpCall(request, tool, args) {
  const headers = { 'Content-Type': 'application/json', Accept: 'application/json, text/event-stream' };
  const parse = async (resp) => {
    expect(resp.ok(), await resp.text()).toBeTruthy();
    const text = await resp.text();
    const data = text.split('\n').filter((l) => l.startsWith('data:')).map((l) => l.slice(5).trim()).pop();
    return JSON.parse(data ?? text);
  };
  const init = await request.post('/mcp', {
    headers,
    data: { jsonrpc: '2.0', id: 1, method: 'initialize', params: { protocolVersion: '2025-06-18', capabilities: {}, clientInfo: { name: 'e2e-agent', version: '0' } } },
  });
  const sid = init.headers()['mcp-session-id'];
  const sessHeaders = sid ? { ...headers, 'Mcp-Session-Id': sid } : headers;
  await request.post('/mcp', { headers: sessHeaders, data: { jsonrpc: '2.0', method: 'notifications/initialized' } });
  const res = await parse(await request.post('/mcp', {
    headers: sessHeaders,
    data: { jsonrpc: '2.0', id: 2, method: 'tools/call', params: { name: tool, arguments: args } },
  }));
  expect(res.result?.isError, JSON.stringify(res)).toBeFalsy();
  return JSON.parse(res.result.content[0].text);
}

test('image: region anchors round-trip through UI, store and MCP, and orphan when the file changes', async ({ page, request }) => {
  await openInNewSession(page, 'Image format', 'pixel.png');
  const sessionID = page.url().match(/\/s\/([^/]+)\//)[1];

  const img = page.locator('[data-region-surface] img');
  await expect(img).toBeVisible();
  await expect.poll(() => img.evaluate((el) => el.naturalWidth)).toBe(240); // served by /raw

  // Draw a region from (25%,25%) to (75%,50%) of the image.
  const box = await img.boundingBox();
  await page.mouse.move(box.x + box.width * 0.25, box.y + box.height * 0.25);
  await page.mouse.down();
  await page.mouse.move(box.x + box.width * 0.5, box.y + box.height * 0.4, { steps: 4 });
  await page.mouse.move(box.x + box.width * 0.75, box.y + box.height * 0.5, { steps: 4 });
  await page.mouse.up();
  await expect(page.getByText(/Comment on region 25%,25% 50%×25%/)).toBeVisible();
  await comment(page, 'what is this patch?');

  // UI -> store: the thread shows its region label and an overlay box.
  await expect(page.getByText(/^region 25%,25% 50%×25%$/)).toBeVisible();
  await expect(page.getByRole('button', { name: 'Thread 1' })).toBeVisible();

  // Reply through the UI.
  await page.getByPlaceholder('Reply...').fill('looks like a checkerboard');
  await page.locator('form', { has: page.getByPlaceholder('Reply...') }).getByRole('button', { name: 'Reply', exact: true }).click();
  await expect(page.getByText('looks like a checkerboard')).toBeVisible();

  // MCP -> store -> UI: an agent's region thread appears as a second box.
  const created = await mcpCall(request, 'create_thread', {
    session_id: sessionID,
    file_path: 'formats/pixel.png',
    anchor: { type: 'region', x: 0.8, y: 0.1, width: 0.15, height: 0.2 },
    body: 'ai: is this corner intended?',
  });
  expect(created.anchor_type ?? created.anchor?.x).toBeTruthy();
  await page.reload();
  await expect(page.getByRole('button', { name: 'Thread 2' })).toBeVisible();
  await expect(page.getByText('ai: is this corner intended?')).toBeVisible();

  // Store -> MCP: the UI-created thread reads back with its geometry.
  const listed = await mcpCall(request, 'list_threads', { session_id: sessionID, file_path: 'formats/pixel.png' });
  const regions = listed.filter((t) => t.anchor.x !== undefined);
  expect(regions).toHaveLength(2);
  expect(regions[0].anchor).toMatchObject({ x: 0.25, y: 0.25, width: 0.5, height: 0.25 });

  // The file changes: positions on the old pixels mean nothing, so both
  // threads orphan and no longer draw boxes.
  const edited = Buffer.from(readFileSync(corpus('pixel.png')));
  edited[edited.length - 20] ^= 0xff;
  writeFileSync(live('pixel.png'), edited);
  await expectAfterReload(page, async () => {
    await expect(page.getByText('orphaned', { exact: true })).toHaveCount(2, { timeout: 500 });
  });
  await expect(page.locator('.gloss-mark')).toHaveCount(0);
});

test('image: a bare click does not start a selection', async ({ page }) => {
  // A click without dragging is below the minimum size: no composer.
  await openInNewSession(page, 'Image click', 'pixel.png');
  const img = page.locator('[data-region-surface] img');
  await expect(img).toBeVisible();
  const box = await img.boundingBox();
  await page.mouse.click(box.x + 20, box.y + 20);
  await expect(page.getByPlaceholder('Add a comment...')).toBeHidden();
});

test('audio: time anchors round-trip through UI, store and MCP, and orphan when the file changes', async ({ page, request }) => {
  await openInNewSession(page, 'Audio format', 'tone.wav');
  const sessionID = page.url().match(/\/s\/([^/]+)\//)[1];
  const audio = page.locator('[data-time-surface] audio');
  await expect(audio).toBeAttached();
  await expect.poll(() => audio.evaluate((a) => a.duration)).toBeGreaterThan(1.9); // served by /raw

  // Mark 0.5s .. 1.5s from the playhead.
  await audio.evaluate((a) => { a.currentTime = 0.5; });
  await page.getByRole('button', { name: 'Mark start', exact: true }).click();
  await audio.evaluate((a) => { a.currentTime = 1.5; });
  await page.getByRole('button', { name: 'Mark end', exact: true }).click();
  await expect(page.getByText('Comment on 0:00.5\u20130:01.5')).toBeVisible();
  await comment(page, 'the tone wobbles here');

  await expect(page.getByText('0.5s-1.5s', { exact: true })).toBeVisible();
  // The chip seeks the player back to the range start.
  await audio.evaluate((a) => { a.currentTime = 1.9; });
  await page.getByRole('button', { name: 'Thread 1', exact: true }).click();
  await expect.poll(() => audio.evaluate((a) => a.currentTime)).toBeCloseTo(0.5, 1);

  // MCP -> store -> UI.
  await mcpCall(request, 'create_thread', {
    session_id: sessionID, file_path: 'formats/tone.wav',
    anchor: { type: 'time', start_time: 1, end_time: 1.25 }, body: 'ai: is this a click?',
  });
  await page.reload();
  await expect(page.getByRole('button', { name: 'Thread 2', exact: true })).toBeVisible();

  // Store -> MCP.
  const listed = await mcpCall(request, 'list_threads', { session_id: sessionID, file_path: 'formats/tone.wav' });
  expect(listed).toHaveLength(2);
  expect(listed[0].anchor).toMatchObject({ start_time: 0.5, end_time: 1.5 });

  const edited = Buffer.from(readFileSync(corpus('tone.wav')));
  edited[edited.length - 10] ^= 0x7f;
  writeFileSync(live('tone.wav'), edited);
  await expectAfterReload(page, async () => {
    await expect(page.getByText('orphaned', { exact: true })).toHaveCount(2, { timeout: 500 });
  });
  await expect(page.locator('.gloss-time-mark')).toHaveCount(0);
});

test('video: region-time anchors round-trip through UI, store and MCP, and orphan when the file changes', async ({ page, request }) => {
  await openInNewSession(page, 'Video format', 'clip.webm');
  const sessionID = page.url().match(/\/s\/([^/]+)\//)[1];
  const video = page.locator('[data-region-surface] video');
  await expect(video).toBeVisible();
  await expect.poll(() => video.evaluate((v) => v.videoWidth)).toBe(160); // served by /raw

  // Pause on t=1s, draw a region, extend the range to 1.5s.
  await video.evaluate((v) => { v.currentTime = 1; });
  await page.getByRole('button', { name: 'Draw region', exact: true }).click();
  const box = await video.boundingBox();
  await page.mouse.move(box.x + box.width * 0.25, box.y + box.height * 0.2);
  await page.mouse.down();
  await page.mouse.move(box.x + box.width * 0.5, box.y + box.height * 0.35, { steps: 4 });
  await page.mouse.move(box.x + box.width * 0.75, box.y + box.height * 0.5, { steps: 4 });
  await page.mouse.up();
  await expect(page.getByText(/Comment on region 25%,20% 50%×30% at 0:01\.0/)).toBeVisible();
  await video.evaluate((v) => { v.currentTime = 1.5; });
  await page.getByRole('button', { name: 'Mark end', exact: true }).click();
  await expect(page.getByText(/at 0:01\.0\u20130:01\.5/)).toBeVisible();
  await comment(page, 'the logo flickers');

  await expect(page.getByText(/^region 25%,20% 50%×30% at 1s-1\.5s$/)).toBeVisible();
  // The box shows only while the playhead is inside the range.
  await video.evaluate((v) => { v.currentTime = 0; });
  await expect(page.locator('.gloss-mark')).toHaveCount(0);
  await page.getByRole('button', { name: /^Thread 1 at/ }).click();
  await expect(page.locator('.gloss-mark')).toHaveCount(1);

  // MCP -> store -> UI.
  await mcpCall(request, 'create_thread', {
    session_id: sessionID, file_path: 'formats/clip.webm',
    anchor: { type: 'region_time', x: 0.6, y: 0.6, width: 0.2, height: 0.2, start_time: 0.2, end_time: 0.4 },
    body: 'ai: what is this corner?',
  });
  await page.reload();
  await expect(page.getByRole('button', { name: /^Thread 2 at/ })).toBeVisible();

  // Store -> MCP.
  const listed = await mcpCall(request, 'list_threads', { session_id: sessionID, file_path: 'formats/clip.webm' });
  expect(listed).toHaveLength(2);
  expect(listed[0].anchor).toMatchObject({ x: 0.25, y: 0.2, width: 0.5, height: 0.3, start_time: 1, end_time: 1.5 });

  const edited = Buffer.from(readFileSync(corpus('clip.webm')));
  edited[edited.length - 30] ^= 0x55;
  writeFileSync(live('clip.webm'), edited);
  await expectAfterReload(page, async () => {
    await expect(page.getByText('orphaned', { exact: true })).toHaveCount(2, { timeout: 500 });
  });
  await expect(page.locator('.gloss-time-mark')).toHaveCount(0);
});
