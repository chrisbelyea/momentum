import assert from 'node:assert/strict';
import { execFile as execFileCallback } from 'node:child_process';
import { promisify } from 'node:util';
import { createServer as createHttpsServer } from 'node:https';
import { spawn } from 'node:child_process';
import { mkdtemp, rm, readFile } from 'node:fs/promises';
import { openSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { request } from 'node:https';
import { chromium } from 'playwright';

const execFile = promisify(execFileCallback);
const binary = process.env.MOMENTUM_E2E_BINARY || join(process.cwd(), 'bin', 'momentum-server');
const port = Number(process.env.MOMENTUM_BACKEND_E2E_PORT || 18445);
// Keep the backend workflow on the server's explicit IPv4 loopback listener.
const baseURL = `https://127.0.0.1:${port}`;
const password = 'browser-backend-e2e-password';
const email = `browser-backend-e2e-${Date.now()}@example.invalid`;
const caldavUsername = 'caldav-browser';
const caldavPassword = 'caldav-browser-password';
const dataDir = await mkdtemp(join(tmpdir(), 'momentum-backend-e2e-'));
const dbPath = join(dataDir, 'momentum.db');
const logPath = join(dataDir, 'server.log');
const certPath = join(dataDir, 'caldav-cert.pem');
const keyPath = join(dataDir, 'caldav-key.pem');
let passed = false;
let browser;
let caldavServer;

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

async function makeCalDAVFixture() {
  await execFile('openssl', [
    'req', '-x509', '-newkey', 'rsa:2048', '-nodes', '-days', '1',
    '-keyout', keyPath, '-out', certPath, '-subj', '/CN=127.0.0.1',
    '-addext', 'subjectAltName=IP:127.0.0.1',
  ]);
  const key = await readFile(keyPath);
  const cert = await readFile(certPath);
  caldavServer = createHttpsServer({ key, cert }, (req, res) => {
    const expectedAuth = `Basic ${Buffer.from(`${caldavUsername}:${caldavPassword}`).toString('base64')}`;
    if (req.headers.authorization !== expectedAuth) {
      res.writeHead(401, { 'WWW-Authenticate': 'Basic realm="Momentum E2E"' });
      res.end();
      return;
    }
    if (req.method !== 'PROPFIND') {
      res.writeHead(405);
      res.end();
      return;
    }
    // ValidateConnection only needs a successful DAV response. Requiring the
    // expected Basic header proves the browser form sent credentials through
    // the real server, without storing or printing the password in this test.
    res.writeHead(207, { 'Content-Type': 'application/xml' });
    res.end('<?xml version="1.0"?><D:multistatus xmlns:D="DAV:"/>');
  });
  await new Promise((resolve, reject) => {
    caldavServer.once('error', reject);
    caldavServer.listen(0, '127.0.0.1', resolve);
  });
  return caldavServer.address().port;
}

const log = openSync(logPath, 'w');
const server = spawn(binary, [], {
  cwd: dataDir,
  env: { ...process.env, DB_PATH: dbPath, PORT: String(port), MOMENTUM_DEV_MODE: '1' },
  stdio: ['ignore', log, log],
});

try {
  const caldavPort = await makeCalDAVFixture();
  await waitForServer(`${baseURL}/health`);
  browser = await chromium.launch({
    headless: true,
    ...(process.env.MOMENTUM_E2E_CHROMIUM ? { executablePath: process.env.MOMENTUM_E2E_CHROMIUM } : {}),
  });
  const context = await browser.newContext({ ignoreHTTPSErrors: true });
  const page = await context.newPage();

  // A clean browser profile must onboard through the rendered first-run page;
  // every backend and task action below is performed through browser controls.
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

  await page.goto(`${baseURL}/settings/backends`);
  await page.getByRole('heading', { name: 'Configured backends' }).waitFor();
  await page.locator('#backend-name').fill('Browser CalDAV');
  await page.locator('#backend-type').selectOption('external_caldav');
  await page.locator('#backend-url').fill(`https://127.0.0.1:${caldavPort}/dav/tasks/`);
  await page.locator('#backend-username').fill(caldavUsername);
  await page.locator('#backend-password').fill(caldavPassword);
  await page.locator('#backend-skip-tls').check();
  await page.locator('#validate-backend').click();
  await page.waitForFunction(() => document.getElementById('backend-status')?.textContent !== 'Validating connection…');
  const validationMessage = await page.locator('#backend-status').textContent();
  assert.equal(validationMessage, 'Connection validated successfully', `validation failed: ${validationMessage}`);
  assert.doesNotMatch(await page.locator('body').textContent(), new RegExp(caldavPassword));

  await page.getByRole('button', { name: 'Save backend' }).click();
  await page.waitForURL(/\/\?backend_id=\d+$/);
  const selectedID = await page.locator('#task-backend').inputValue();
  assert.ok(selectedID, 'saved backend should be selected on the board');

  // Edit through the UI and verify the redacted password can be retained.
  await page.goto(`${baseURL}/settings/backends`);
  const card = page.locator('.backend-card', { hasText: 'Browser CalDAV' });
  await card.getByRole('button', { name: 'Edit' }).click();
  assert.equal(await page.locator('#backend-password').inputValue(), '');
  await page.locator('#backend-name').fill('Browser CalDAV renamed');
  await page.getByRole('button', { name: 'Save changes' }).click();
  await page.waitForURL(/\/\?backend_id=\d+$/);

  // Exercise task creation on the selected backend without calling a task API.
  await page.locator('#task-title').fill('Backend setup task');
  await Promise.all([
    page.waitForNavigation({ waitUntil: 'domcontentloaded' }),
    page.getByRole('button', { name: 'Create task' }).click(),
  ]);
  await page.getByText('Backend setup task').waitFor();

  await page.goto(`${baseURL}/settings/backends`);
  const renamedCard = page.locator('.backend-card', { hasText: 'Browser CalDAV renamed' });
  page.once('dialog', dialog => dialog.accept());
  await Promise.all([
    page.waitForNavigation({ waitUntil: 'domcontentloaded' }),
    renamedCard.getByRole('button', { name: 'Delete' }).click(),
  ]);
  assert.doesNotMatch(await page.locator('body').textContent(), /Browser CalDAV renamed/);
  assert.match(await page.locator('body').textContent(), /Local tasks/);
  console.log('Momentum backend browser E2E passed: validate, create, select, edit, retain secret, task, delete');
  passed = true;
} finally {
  if (browser) await browser.close();
  if (caldavServer) await new Promise(resolve => caldavServer.close(resolve));
  if (!server.killed) server.kill('SIGTERM');
  await new Promise(resolve => setTimeout(resolve, 250));
  if (!passed) console.error(await readFile(logPath, 'utf8').catch(() => ''));
  await rm(dataDir, { recursive: true, force: true });
}
