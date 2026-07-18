import { defineConfig } from '@playwright/test';
import { mkdtempSync, cpSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { fileURLToPath } from 'node:url';
import path from 'node:path';

// Milestone 4's exit criterion is "browse and read any file in a
// directory via localhost" -- exercised here against the same fixture
// corpus internal/connector's own tests use (testdata/connector/fixture),
// copied to a scratch dir so the running `gloss` server's .gloss/ doesn't
// land in a tracked directory.
const here = path.dirname(fileURLToPath(import.meta.url));
const fixtureSrc = path.resolve(here, '../testdata/connector/fixture');
const fixtureDir = mkdtempSync(path.join(tmpdir(), 'gloss-e2e-'));
cpSync(fixtureSrc, fixtureDir, { recursive: true });

const PORT = 4747;

// Expose fixtureDir so delta e2e tests can write files into the live dir.
process.env.GLOSS_FIXTURE_DIR = fixtureDir;

export default defineConfig({
  testDir: './tests',
  fullyParallel: false,
  retries: process.env.CI ? 2 : 0,
  use: {
    baseURL: `http://127.0.0.1:${PORT}`,
    trace: 'on-first-retry',
    // Set only for sandboxes that pre-install a Chromium build pinned to
    // a different revision than this package.json's @playwright/test
    // expects (see the repo's Claude Code environment notes) -- CI and
    // normal dev machines download the matching browser via
    // `playwright install` instead and never set this.
    ...(process.env.PW_EXECUTABLE_PATH
      ? { launchOptions: { executablePath: process.env.PW_EXECUTABLE_PATH } }
      : {}),
  },
  webServer: {
    command: `../gloss -no-open -port ${PORT} "${fixtureDir}"`,
    url: `http://127.0.0.1:${PORT}/`,
    reuseExistingServer: !process.env.CI,
    timeout: 15_000,
  },
});
