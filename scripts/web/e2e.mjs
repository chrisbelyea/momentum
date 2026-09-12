import assert from 'node:assert/strict';
import { execFile as execFileCallback, spawn } from 'node:child_process';
import { mkdtemp, rm } from 'node:fs/promises';
import { openSync } from 'node:fs';
import { readFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { request } from 'node:https';
import { promisify } from 'node:util';
import { chromium } from 'playwright';

const execFile = promisify(execFileCallback);
const binary = process.env.MOMENTUM_E2E_BINARY || join(process.cwd(), 'bin', 'momentum-server');
const port = Number(process.env.MOMENTUM_E2E_PORT || 18444);
// The packaged server binds IPv4 loopback by default. Avoid localhost's
// IPv6-first resolution on hosts where ::1 is tried before 127.0.0.1.
const baseURL = `https://127.0.0.1:${port}`;
const password = 'browser-e2e-password';
const email = `browser-e2e-${Date.now()}@example.invalid`;
const dataDir = await mkdtemp(join(tmpdir(), 'momentum-browser-e2e-'));
const dbPath = join(dataDir, 'momentum.db');
const logPath = join(dataDir, 'server.log');
let passed = false;

function waitForServer(url, timeoutMs = 15000) {
  const deadline = Date.now() + timeoutMs;
  return new Promise((resolve, reject) => {
    const poll = () => {
      const req = request(url, { rejectUnauthorized: false }, response => {
        response.resume();
        if (response.statusCode === 200) return resolve();
        retry();
      });
      req.on('error', retry);
      req.end();
    };
    const retry = () => {
      if (Date.now() >= deadline) return reject(new Error(`server did not become ready; see ${logPath}`));
      setTimeout(poll, 250);
    };
    poll();
  });
}

const log = openSync(logPath, 'w');
const server = spawn(binary, [], {
  cwd: dataDir,
  env: { ...process.env, DB_PATH: dbPath, PORT: String(port), MOMENTUM_DEV_MODE: '1' },
  stdio: ['ignore', log, log],
});
let browser;
try {
  await waitForServer(`${baseURL}/health`);
  const launchOptions = { headless: true };
  if (process.env.MOMENTUM_E2E_CHROMIUM) launchOptions.executablePath = process.env.MOMENTUM_E2E_CHROMIUM;
  browser = await chromium.launch(launchOptions);
  const context = await browser.newContext({ ignoreHTTPSErrors: true });
  const page = await context.newPage();

  // A clean browser profile must be able to onboard without direct API auth
  // injection. The root navigation redirects to the first-run account page.
  await page.goto(`${baseURL}/`);
  assert.match(page.url(), /\/login\?/);
  await page.getByRole('link', { name: 'Create your account' }).click();
  await page.getByLabel('Email').fill(email);
  await page.getByLabel('Password').fill(password);
  await Promise.all([
    page.waitForNavigation({ waitUntil: 'domcontentloaded' }),
    page.getByRole('button', { name: 'Create account' }).click(),
  ]);
  assert.equal(new URL(page.url()).pathname, '/');

  await page.goto(`${baseURL}/`);
  assert.equal(await page.title(), 'Momentum - Kanban Board');
  const backendID = await page.locator('#task-backend').inputValue();
  assert.ok(backendID, 'registration should select the adopted local backend');
  assert.equal(await page.locator('.task-card').count(), 0);

  // Create a rich task via accessible controls and verify the board reload.
  await page.locator('#task-title').fill('Browser workflow task');
  await page.locator('#task-description').fill('Created through the real board');
  await page.locator('#task-due').fill('2026-12-31');
  await page.locator('#task-priority').selectOption('2');
  await page.locator('#task-tags').fill('release, browser, release');
  await Promise.all([
    page.waitForNavigation({ waitUntil: 'domcontentloaded' }),
    page.getByRole('button', { name: 'Create task' }).click(),
  ]);
  const card = page.locator('.task-card', { hasText: 'Browser workflow task' });
  await card.waitFor();
  assert.match(await card.textContent(), /Browser workflow task/);
  assert.equal(await card.getAttribute('data-due-at'), '2026-12-31');
  assert.equal(await card.getAttribute('data-priority'), '2');
  const taskID = await card.getAttribute('data-task-id');
  const initialVersion = await card.getAttribute('data-version');
  assert.ok(taskID && initialVersion, 'board task card must expose id and version');

  // Edit rich fields and status using the keyboard-accessible dialog controls.
  await card.getByRole('button', { name: 'Edit' }).click();
  await page.locator('#edit-title').fill('Browser workflow updated');
  await page.locator('#edit-description').fill('Updated through the browser dialog');
  await page.locator('#edit-status-select').selectOption('IN-PROCESS');
  await page.locator('#edit-tags').fill('verified, browser');
  const richUpdateResponse = page.waitForResponse(response => response.url().includes('/api/tasks/') && response.request().method() === 'PUT');
  await Promise.all([
    page.waitForNavigation({ waitUntil: 'domcontentloaded' }),
    page.getByRole('button', { name: 'Save changes' }).click(),
  ]);
  const richUpdate = await richUpdateResponse;
  assert.equal(richUpdate.status(), 200, `rich update returned ${richUpdate.status()}`);
  const updatedCard = page.locator('.task-card', { hasText: 'Browser workflow updated' });
  await updatedCard.waitFor();
  assert.equal(await updatedCard.getAttribute('data-status'), 'IN-PROCESS');

  // The list view is a separate rendered workflow and preserves backend state.
  await page.goto(`${baseURL}/list?backend_id=${backendID}&tag=verified&sort=title&order=asc`);
  assert.equal(new URL(page.url()).searchParams.get('backend_id'), backendID);
  assert.match(await page.locator('body').textContent(), /Browser workflow updated/);

  // Keep the first context stale while a second browser context changes the
  // task. The stale first context must receive 409 and reconcile instead of
  // silently overwriting that change.
  await page.goto(`${baseURL}/?backend_id=${backendID}`);
  const staleCard = page.locator('.task-card', { hasText: 'Browser workflow updated' });
  await staleCard.getByRole('button', { name: 'Edit' }).click();
  await page.locator('#edit-title').fill('Stale overwrite must fail');

  const otherContext = await browser.newContext({ ignoreHTTPSErrors: true, storageState: await context.storageState() });
  const otherPage = await otherContext.newPage();
  await otherPage.goto(`${baseURL}/?backend_id=${backendID}`);
  const otherCard = otherPage.locator('.task-card', { hasText: 'Browser workflow updated' });
  await otherCard.getByRole('button', { name: 'Edit' }).click();
  await otherPage.locator('#edit-title').fill('Concurrent device wins');
  await Promise.all([
    otherPage.waitForNavigation({ waitUntil: 'domcontentloaded' }),
    otherPage.getByRole('button', { name: 'Save changes' }).click(),
  ]);

  const conflictResponse = page.waitForResponse(response => response.url().includes(`/api/tasks/${taskID}`) && response.request().method() === 'PUT');
  await page.getByRole('button', { name: 'Save changes' }).click();
  assert.equal((await conflictResponse).status(), 409);
  await page.waitForLoadState('domcontentloaded');
  assert.match(await page.locator('body').textContent(), /Concurrent device wins/);
  assert.doesNotMatch(await page.locator('body').textContent(), /Stale overwrite must fail/);
  await otherContext.close();

  // A failed status mutation must reload the board to the server state rather
  // than leave the optimistic card in the wrong column.
  await page.goto(`${baseURL}/?backend_id=${backendID}`);
  const failedCard = page.locator('.task-card', { hasText: 'Concurrent device wins' });
  await page.route(`**/api/tasks/${taskID}/status`, route => route.fulfill({ status: 503, body: 'simulated outage' }));
  await failedCard.dragTo(page.locator('#done-column'));
  await page.unroute(`**/api/tasks/${taskID}/status`);
  await page.waitForLoadState('domcontentloaded');
  const reconciledCard = page.locator('.task-card', { hasText: 'Concurrent device wins' });
  assert.equal(await reconciledCard.getAttribute('data-status'), 'IN-PROCESS');

  // Delete through the visible board control and verify it is gone in list.
  // The delete form performs a document navigation. Wait for the navigation
  // it triggers before starting the list navigation, otherwise Chromium can
  // abort the second goto while the first response is still being committed.
  await Promise.all([
    page.waitForNavigation({ waitUntil: 'domcontentloaded' }),
    reconciledCard.getByRole('button', { name: 'Delete' }).click(),
  ]);
  await page.goto(`${baseURL}/list?backend_id=${backendID}`);
  assert.doesNotMatch(await page.locator('body').textContent(), /Concurrent device wins/);

  // Logout revokes the browser session, and an invalid subsequent login is
  // rejected before a valid sign-in restores access.
  await page.getByRole('button', { name: 'Sign out' }).click();
  await page.waitForURL(/\/login$/);
  await page.goto(`${baseURL}/`);
  assert.match(page.url(), /\/login\?/);
  await page.getByLabel('Email').fill(email);
  await page.getByLabel('Password').fill('wrong password');
  await page.getByRole('button', { name: 'Sign in' }).click();
  await page.getByRole('alert').waitFor();
  assert.match(await page.getByRole('alert').textContent(), /Invalid email or password/);
  // The failed POST re-renders a fresh form, so the browser must refill both
  // fields before retrying rather than relying on the discarded form values.
  await page.getByLabel('Email').fill(email);
  await page.getByLabel('Password').fill(password);
  await Promise.all([
    page.waitForNavigation({ waitUntil: 'domcontentloaded' }),
    page.getByRole('button', { name: 'Sign in' }).click(),
  ]);
  assert.equal(new URL(page.url()).pathname, '/');

  // Expire the active session in the test database and verify that browser
  // navigation treats it like a revoked session. Python's stdlib sqlite3 is
  // available on the Ubuntu browser runner and avoids adding a production
  // database dependency just for this assertion.
  await execFile('python3', ['-c', `
import sqlite3
import sys
connection = sqlite3.connect(sys.argv[1])
connection.execute("UPDATE sessions SET expires_at = CURRENT_TIMESTAMP")
connection.commit()
connection.close()
`, dbPath]);
  await page.goto(`${baseURL}/`);
  assert.match(page.url(), /\/login\?/);

  console.log('Momentum browser E2E passed: first-run onboarding, login failure/logout, expired-session route protection, authenticated board/list create, rich edit, conflict, failure reconciliation, delete');
  passed = true;
} finally {
  if (browser) await browser.close();
  if (!server.killed) server.kill('SIGTERM');
  await new Promise(resolve => setTimeout(resolve, 250));
  if (!passed) console.error(await readFile(logPath, 'utf8').catch(() => ''));
  await rm(dataDir, { recursive: true, force: true });
}
