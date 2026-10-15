import fs from 'node:fs';
import path from 'node:path';
import { createRequire } from 'node:module';
import { performance } from 'node:perf_hooks';

// Local Layer 229 capacity probe. The session file is generated in the
// disposable PostgreSQL lab and contains only synthetic auth keys.
const sessionFile = process.env.ONLINE_SESSIONS_FILE || '';
const requestedUsers = Number(process.env.ONLINE_USERS || '1000');
const batchSize = Number(process.env.ONLINE_BATCH_SIZE || '50');
const holdMs = Number(process.env.ONLINE_HOLD_MS || '20000');
const reconnectCount = Number(process.env.ONLINE_RECONNECT_COUNT || '10');
const timeoutMs = Number(process.env.ONLINE_TIMEOUT_MS || '20000');
const dcId = Number(process.env.TG_DC_ID || '2');
const dcHost = process.env.TG_DC_HOST || '127.0.0.1';
const dcPort = Number(process.env.TG_DC_PORT || '11443');
const metricsPort = Number(process.env.METRICS_PORT || '19090');
const apiLayer = 229;
let Api: any;
let TelegramClient: any;
let CallbackSession: any;
let Connection: any;

type Seed = { userId: string; accessHash: string; authB64: string };
type ClientRow = { seed: Seed; client: any; index: number };

const report: any = {
  schemaVersion: 1,
  probe: 'messaging.online-sessions',
  evidenceClass: 'local_probe',
  productionCapacityEvidence: false,
  requestedUsers,
  batchSize,
  holdMs,
  reconnectCount,
  ok: false,
  connect: { requested: requestedUsers, connected: 0, failed: 0, failures: [], batches: [] },
  hold: { samples: [], minConnected: 0, maxConnected: 0, serverGaugeMin: null },
  reconnect: { requested: reconnectCount, connected: 0, failures: [] },
  serviceGauge: { samples: [], min: null, max: null, observed: false },
};

const quietLogger = { debug() {}, info() {}, warn() {}, error() {} };
const sleep = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms));

function check(condition: unknown, code: string): asserts condition {
  if (!condition) throw Object.assign(new Error(code), { probeCode: code });
}

function errorCode(error: any): string {
  if (typeof error?.probeCode === 'string') return error.probeCode;
  return /^[A-Z][A-Z0-9_]{0,79}$/.test(error?.errorMessage || '')
    ? error.errorMessage : 'RPC_OR_TRANSPORT_ERROR';
}

async function bounded<T>(promise: Promise<T>): Promise<T> {
  let timer: ReturnType<typeof setTimeout> | undefined;
  try {
    return await Promise.race([promise, new Promise<T>((_, reject) => {
      timer = setTimeout(() => reject(Object.assign(new Error('PROBE_TIMEOUT'), { probeCode: 'PROBE_TIMEOUT' })), timeoutMs);
    })]);
  } finally {
    if (timer) clearTimeout(timer);
  }
}

function rpcName(value: any): string {
  return value?.className || value?.constructor?.name || typeof value;
}

async function queryActive(): Promise<number | null> {
  try {
    const query = encodeURIComponent('gnetway_conn_active');
    const response = await fetch(`http://127.0.0.1:${metricsPort}/api/v1/query?query=${query}`);
    if (!response.ok) return null;
    const body: any = await response.json();
    const values = body?.data?.result?.map((row: any) => Number(row?.value?.[1]))
      .filter((value: number) => Number.isFinite(value));
    return values?.length ? values.reduce((sum: number, value: number) => sum + value, 0) : null;
  } catch {
    return null;
  }
}

function updateGauge(value: number | null, elapsedMs: number): void {
  report.serviceGauge.samples.push({ elapsedMs, active: value });
  if (value === null) return;
  report.serviceGauge.observed = true;
  report.serviceGauge.min = report.serviceGauge.min === null ? value : Math.min(report.serviceGauge.min, value);
  report.serviceGauge.max = report.serviceGauge.max === null ? value : Math.max(report.serviceGauge.max, value);
}

async function waitForGaugeAtLeast(expected: number, started: number): Promise<number | null> {
  const deadline = performance.now() + Math.max(10000, Math.min(30000, holdMs));
  let latest: number | null = null;
  while (performance.now() < deadline) {
    latest = await queryActive();
    updateGauge(latest, performance.now() - started);
    if (latest !== null && latest >= expected) return latest;
    await sleep(500);
  }
  return latest;
}

async function waitForGaugeAtMost(expected: number, started: number): Promise<number | null> {
  const deadline = performance.now() + 15000;
  let latest: number | null = null;
  while (performance.now() < deadline) {
    latest = await queryActive();
    updateGauge(latest, performance.now() - started);
    if (latest !== null && latest <= expected) return latest;
    await sleep(500);
  }
  return latest;
}

function makeClient(seed: Seed, index: number): any {
  const session = new CallbackSession({
    mainDcId: dcId,
    keys: { [dcId]: Buffer.from(seed.authB64, 'base64').toString('hex') },
  }, () => {});
  const load = session.load.bind(session);
  session.load = async () => {
    await load();
    session._serverAddress = dcHost;
    session._port = dcPort;
  };
  const client = new TelegramClient(session, Number(process.env.TG_API_ID || '1'),
    process.env.TG_API_HASH || '0123456789abcdef0123456789abcdef', {
      connection: Connection,
      fallbackConnection: Connection,
      shouldAllowHttpTransport: false,
      shouldForceHttpTransport: false,
      timeout: Math.ceil(timeoutMs / 1000),
      requestRetries: 1,
      connectionRetries: 1,
      connectionRetriesToFallback: 0,
      retryDelay: 250,
      autoReconnect: false,
      sequentialUpdates: true,
      baseLogger: quietLogger,
      deviceModel: `capacity-${index}`,
    });
  const initWith = client._initWith.bind(client);
  client._initWith = (query: any) => {
    const original = initWith(query);
    check(rpcName(original) === 'InvokeWithLayer' && rpcName(original.query) === 'InitConnection'
      && original.query.query === query, 'INVALID_INIT_CONNECTION_WRAPPER');
    return new Api.InvokeWithLayer({ layer: apiLayer, query: original.query });
  };
  client._borrowExportedSender = async () => undefined;
  return client;
}

async function connectOne(seed: Seed, index: number): Promise<ClientRow> {
  const client = makeClient(seed, index);
  try {
    await bounded(client.connect());
    const me: any = await bounded(client.getMe());
    check(String(me?.id) === seed.userId, 'AUTH_KEY_USER_MISMATCH');
    check(client.isConnected(), 'CLIENT_NOT_CONNECTED_AFTER_AUTH');
    return { seed, client, index };
  } catch (error) {
    try { client.disconnect(); } catch {}
    throw error;
  }
}

async function disconnect(row: ClientRow): Promise<void> {
  try { await bounded(Promise.resolve(row.client.disconnect())); } catch {}
}

async function connectBatches(seeds: Seed[]): Promise<ClientRow[]> {
  const clients: ClientRow[] = [];
  if (seeds.length > 0) {
    const warmupStarted = performance.now();
    const warmupFailures: string[] = [];
    for (let attempt = 0; attempt < 3 && !clients.length; attempt += 1) {
      try {
        clients.push(await connectOne(seeds[0], 0));
      } catch (error) {
        warmupFailures.push(errorCode(error));
        if (attempt < 2) await sleep(250);
      }
    }
    if (clients.length) {
      report.connect.warmup = { connected: 1, failed: 0, attempts: warmupFailures.length + 1,
        elapsedMs: performance.now() - warmupStarted,
        retryCodes: warmupFailures };
    } else {
      report.connect.warmup = { connected: 0, failed: 1, attempts: warmupFailures.length,
        elapsedMs: performance.now() - warmupStarted, retryCodes: warmupFailures };
      report.connect.failures.push({ index: 0, code: warmupFailures[warmupFailures.length - 1] || 'PROBE_TIMEOUT' });
    }
  }
  for (let offset = 1; offset < seeds.length; offset += batchSize) {
    const batch = seeds.slice(offset, offset + batchSize);
    const started = performance.now();
    let pending = batch.map((seed, index) => ({ seed, index: offset + index }));
    const failureCodes = new Map<number, string[]>();
    let attempts = 0;
    while (pending.length && attempts < 3) {
      const attempt = pending;
      const settled = await Promise.allSettled(attempt.map((item) => connectOne(item.seed, item.index)));
      const retry: typeof pending = [];
      settled.forEach((result, index) => {
        const item = attempt[index];
        if (result.status === 'fulfilled') {
          clients.push(result.value);
          failureCodes.delete(item.index);
        } else {
          const codes = failureCodes.get(item.index) || [];
          codes.push(errorCode(result.reason));
          failureCodes.set(item.index, codes);
          if (attempts < 2) retry.push(item);
        }
      });
      pending = retry;
      attempts += 1;
      if (pending.length && attempts < 3) await sleep(250);
    }
    for (const item of pending) {
      const codes = failureCodes.get(item.index) || [];
      report.connect.failures.push({ index: item.index, code: codes[codes.length - 1] || 'PROBE_TIMEOUT',
        attempts: codes.length });
    }
    report.connect.batches.push({ offset, requested: batch.length, connected: batch.length - pending.length,
      failed: pending.length, attempts, retryCount: attempts > 0 ? attempts - 1 : 0,
      elapsedMs: performance.now() - started });
  }
  report.connect.connected = clients.length;
  report.connect.failed = seeds.length - clients.length;
  return clients;
}

async function holdClients(clients: ClientRow[], started: number): Promise<void> {
  const deadline = performance.now() + holdMs;
  let min = Number.POSITIVE_INFINITY;
  let max = 0;
  let gaugeMin: number | null = null;
  while (performance.now() < deadline) {
    const connected = clients.reduce((count, row) => count + (row.client.isConnected() ? 1 : 0), 0);
    min = Math.min(min, connected);
    max = Math.max(max, connected);
    report.hold.samples.push({ elapsedMs: performance.now() - started, connected });
    const active = await queryActive();
    updateGauge(active, performance.now() - started);
    if (active !== null) gaugeMin = gaugeMin === null ? active : Math.min(gaugeMin, active);
    await sleep(Math.min(1000, Math.max(50, deadline - performance.now())));
  }
  report.hold.minConnected = Number.isFinite(min) ? min : 0;
  report.hold.maxConnected = max;
  report.hold.serverGaugeMin = gaugeMin;
}

async function reconnectSample(clients: ClientRow[]): Promise<void> {
  const sample = clients.slice(0, reconnectCount);
  for (const row of sample) await disconnect(row);
  await sleep(250);
  for (const row of sample) {
    try {
      await bounded(row.client.connect());
      const me: any = await bounded(row.client.getMe());
      check(String(me?.id) === row.seed.userId && row.client.isConnected(), 'RECONNECT_AUTH_FAILED');
      report.reconnect.connected += 1;
    } catch (error) {
      report.reconnect.failures.push({ index: row.index, code: errorCode(error) });
    }
  }
}

async function main(): Promise<void> {
  const started = performance.now();
  const clients: ClientRow[] = [];
  let configured = false;
  try {
    check(sessionFile && fs.existsSync(sessionFile), 'MISSING_ONLINE_SESSION_FILE');
    check(Number.isInteger(requestedUsers) && requestedUsers >= 1 && requestedUsers <= 5000, 'INVALID_ONLINE_USERS');
    check(Number.isInteger(batchSize) && batchSize >= 1 && batchSize <= 200, 'INVALID_ONLINE_BATCH_SIZE');
    check(Number.isInteger(holdMs) && holdMs >= 5000 && holdMs <= 120000, 'INVALID_ONLINE_HOLD_MS');
    check(Number.isInteger(reconnectCount) && reconnectCount >= 0 && reconnectCount <= requestedUsers, 'INVALID_RECONNECT_COUNT');
    check(Number.isInteger(timeoutMs) && timeoutMs >= 1000 && timeoutMs <= 60000, 'INVALID_ONLINE_TIMEOUT');
    check(Number.isInteger(dcId) && dcId > 0 && Number.isInteger(dcPort) && dcPort > 0 && dcPort < 65536, 'INVALID_DC_ENDPOINT');
    const seeds: Seed[] = JSON.parse(fs.readFileSync(sessionFile, 'utf8'));
    check(Array.isArray(seeds) && seeds.length === requestedUsers, 'ONLINE_SESSION_COUNT_MISMATCH');
    check(seeds.every((seed) => /^\d+$/.test(seed.userId) && /^-?\d+$/.test(seed.accessHash)
      && Buffer.from(seed.authB64, 'base64').length === 256), 'INVALID_ONLINE_SESSION_SEED');
    const globalAny = globalThis as any;
    globalAny.self ??= globalThis;
    globalAny.addEventListener ??= () => {};
    globalAny.self.addEventListener ??= globalAny.addEventListener;
    const gramjsDir = process.env.TEAMGRAM_GRAMJS_DIR;
    check(Boolean(gramjsDir), 'MISSING_TEAMGRAM_GRAMJS_DIR');
    const requireModule = createRequire(import.meta.url);
    TelegramClient = requireModule(path.join(gramjsDir!, 'src/lib/gramjs/client/TelegramClient.ts')).default;
    CallbackSession = requireModule(path.join(gramjsDir!, 'src/lib/gramjs/sessions/CallbackSession.ts')).default;
    Api = requireModule(path.join(gramjsDir!, 'src/lib/gramjs/tl/index.ts')).Api;
    Connection = requireModule(path.join(gramjsDir!, 'src/lib/gramjs/network/connection/TCPObfuscated.ts')).ConnectionTCPObfuscated;
    configured = true;
    const connected = await connectBatches(seeds);
    clients.push(...connected);
    const holdStarted = performance.now();
    if (connected.length === requestedUsers) await waitForGaugeAtLeast(requestedUsers, holdStarted);
    await holdClients(connected, holdStarted);
    await reconnectSample(connected);
    report.ok = report.connect.connected === requestedUsers
      && report.hold.minConnected === requestedUsers
      && report.reconnect.connected === reconnectCount
      && report.serviceGauge.observed
      && (report.hold.serverGaugeMin ?? 0) >= requestedUsers;
  } catch (error) {
    report.error = { code: errorCode(error) };
  } finally {
    for (const row of clients) await disconnect(row);
    report.serviceGauge.afterDisconnect = await waitForGaugeAtMost(0, performance.now());
    report.serviceGauge.cleanupObserved = report.serviceGauge.afterDisconnect === 0;
    report.durationMs = performance.now() - started;
    report.processMemory = process.memoryUsage();
    report.configured = configured;
    process.stdout.write(`${JSON.stringify(report)}\n`);
  }
}

main().then(() => process.exit(report.ok ? 0 : 1)).catch(() => {
  process.stdout.write(`${JSON.stringify({ ...report, ok: false, error: { code: 'UNEXPECTED_PROBE_ERROR' } })}\n`);
  process.exit(1);
});
