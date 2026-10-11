import { defineConfig } from '@playwright/test';
import { rmSync, cpSync } from 'node:fs';
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

// Playwright evaluates this file in the main process and again in every
// worker, and environment changes made here do not reach the workers. So
// the directory must be a deterministic path, created only by the main
// process (workers carry TEST_WORKER_INDEX): a random mkdtemp per
// evaluation would leave the tests writing to a copy the running server
// is not watching.
const fixtureDir = path.join(tmpdir(), 'gloss-e2e-fixture');
if (process.env.TEST_WORKER_INDEX === undefined) {
  rmSync(fixtureDir, { recursive: true, force: true });
  cpSync(fixtureSrc, fixtureDir, { recursive: true });
}

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
