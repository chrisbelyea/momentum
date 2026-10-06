import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { mkdtemp, rm } from 'node:fs/promises';
import { openSync } from 'node:fs';
import { readFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { request } from 'node:https';
import { chromium } from 'playwright';

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
  // Service-worker script fetches do not consistently honor the context-level
  // ignoreHTTPSErrors option. The test server uses Momentum's documented
  // self-signed development certificate, so explicitly allow it in Chromium.
  const launchOptions = { headless: true, args: ['--ignore-certificate-errors'] };
  if (process.env.MOMENTUM_E2E_CHROMIUM) launchOptions.executablePath = process.env.MOMENTUM_E2E_CHROMIUM;
  browser = await chromium.launch(launchOptions);
  const context = await browser.newContext({ ignoreHTTPSErrors: true });
  const page = await context.newPage();

  // Authenticate through the real server, then drive the rendered board.
  await page.goto(`${baseURL}/health`);
  const registration = await page.evaluate(async ({ email: address, password: secret }) => {
    const response = await fetch('/auth/register', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ email: address, password: secret }),
    });
    return { status: response.status, body: await response.text() };
  }, { email, password });
  assert.equal(registration.status, 201, registration.body);

  await page.goto(`${baseURL}/`);
  assert.equal(await page.title(), 'Momentum - Kanban Board');
  const backendID = await page.locator('#task-backend').inputValue();
  assert.ok(backendID, 'registration should select the local backend');
  assert.equal(await page.locator('.task-card').count(), 0);

  // Validate the installable PWA contract in a real Chromium secure context.
  // The worker intentionally provides an offline static shell only: pages,
  // API responses, and mutations remain network/authentication dependent.
  const pwaManifest = await page.evaluate(async () => {
    const response = await fetch('/static/manifest.json', { cache: 'no-store' });
    return {
      status: response.status,
      contentType: response.headers.get('content-type'),
      body: await response.json(),
    };
  });
  assert.equal(pwaManifest.status, 200);
  assert.match(pwaManifest.contentType ?? '', /application\/json/);
  assert.equal(pwaManifest.body.name, 'Momentum');
  assert.equal(pwaManifest.body.start_url, '/');
  assert.equal(pwaManifest.body.scope, '/');
  assert.equal(pwaManifest.body.display, 'standalone');
  assert.deepEqual(
    pwaManifest.body.icons.map(icon => [icon.src, icon.sizes, icon.type]),
    [
      ['/static/icon-192.png', '192x192', 'image/png'],
      ['/static/icon-512.png', '512x512', 'image/png'],
    ],
  );
  for (const icon of pwaManifest.body.icons) {
    const iconResponse = await page.request.get(`${baseURL}${icon.src}`);
    assert.equal(iconResponse.status(), 200, `${icon.src} must be served by the release binary`);
    assert.match(iconResponse.headers()['content-type'] ?? '', /^image\/png/);
  }

  // The worker is root-scoped so it can control installed app navigations.
  // Its no-cache response header is what lets a newly installed release
  // trigger registration.update() instead of being hidden by browser caches.
  await page.reload({ waitUntil: 'domcontentloaded' });
  const worker = await page.evaluate(async () => {
    // Register explicitly as well as through the page template so this check
    // cannot hang forever if the root-scoped worker fails to install.
    const registration = await navigator.serviceWorker.register('/sw.js', { scope: '/' });
    const ready = await Promise.race([
      navigator.serviceWorker.ready,
      new Promise((_, reject) => setTimeout(() => reject(new Error('service worker did not become ready')), 10000)),
    ]);
    await ready.update();
    return {
      scope: registration.scope,
      scriptURL: registration.active?.scriptURL ?? '',
      state: registration.active?.state ?? '',
      controller: Boolean(navigator.serviceWorker.controller),
    };
  });
  assert.equal(worker.scope, `${baseURL}/`);
  assert.equal(worker.scriptURL, `${baseURL}/sw.js`);
  assert.equal(worker.state, 'activated');
  assert.equal(worker.controller, true);
  const workerResponse = await page.request.get(`${baseURL}/sw.js`);
  assert.equal(workerResponse.status(), 200);
  assert.match(workerResponse.headers()['content-type'] ?? '', /javascript/);
  assert.match(workerResponse.headers()['cache-control'] ?? '', /no-cache/);
  assert.equal(workerResponse.headers()['service-worker-allowed'], '/');

  const cacheState = await page.evaluate(async () => {
    const cachesByName = [];
    for (const name of await caches.keys()) {
      const cache = await caches.open(name);
      cachesByName.push({
        name,
        urls: (await cache.keys()).map(request => new URL(request.url).pathname),
      });
    }
    return cachesByName;
  });
  assert.ok(cacheState.some(cache => cache.name === 'momentum-static-v1'), 'worker must install its static cache');
  const cachedURLs = cacheState.flatMap(cache => cache.urls);
  assert.ok(cachedURLs.length > 0, 'worker static cache must contain the offline shell');
  assert.ok(
    cachedURLs.every(path => path.startsWith('/static/') && path !== '/static/sw.js'),
    `authenticated pages and API responses must not be cached: ${cachedURLs.join(', ')}`,
  );

  // Static assets remain readable when disconnected, while the authenticated
  // page/API cannot be served from a stale cache and therefore fails closed.
  await context.setOffline(true);
  const offlineManifest = await page.evaluate(async () => {
    const response = await fetch('/static/manifest.json');
    return { status: response.status, body: await response.json() };
  });
  assert.equal(offlineManifest.status, 200);
  assert.equal(offlineManifest.body.name, 'Momentum');
  const offlineAPI = await page.evaluate(async (id) => {
    try {
      // Chromium can leave an offline network request pending instead of
      // rejecting it immediately. Bound the probe so this acceptance test
      // deterministically verifies that no authenticated API data is cached.
      const response = await fetch(`/api/tasks?backend_id=${id}`, {
        cache: 'no-store',
        signal: AbortSignal.timeout(5000),
      });
      return { status: response.status };
    } catch (error) {
      return { error: String(error) };
    }
  }, backendID);
  assert.ok(offlineAPI.error, `authenticated API must not be cached offline: ${JSON.stringify(offlineAPI)}`);
  await context.setOffline(false);

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
  await Promise.all([
    page.waitForNavigation({ waitUntil: 'domcontentloaded' }),
    reconciledCard.getByRole('button', { name: 'Delete' }).click(),
  ]);
  await page.goto(`${baseURL}/list?backend_id=${backendID}`);
  assert.doesNotMatch(await page.locator('body').textContent(), /Concurrent device wins/);
  console.log('Momentum browser E2E passed: authenticated board/list create, rich edit, conflict, failure reconciliation, delete');
  passed = true;
} finally {
  if (browser) await browser.close();
  if (!server.killed) server.kill('SIGTERM');
  await new Promise(resolve => setTimeout(resolve, 250));
  if (!passed) console.error(await readFile(logPath, 'utf8').catch(() => ''));
  await rm(dataDir, { recursive: true, force: true });
}
