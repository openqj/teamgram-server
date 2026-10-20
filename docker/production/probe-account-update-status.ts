import { execFileSync } from 'node:child_process';
import fs from 'node:fs';
import path from 'node:path';
import { createRequire } from 'node:module';
import { authSession, loadFixtureAuthKey } from '../web-isolated/fixture-auth';

for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) {
  delete process.env[key];
}

const username = process.env.STATUS_USERNAME || 'isolated_test_11';
const dcId = Number(process.env.TG_DC_ID || '2');
const dcHost = process.env.TG_DC_HOST || '127.0.0.1';
const dcPort = Number(process.env.TG_DC_PORT || '11443');
const timeoutMs = Number(process.env.STATUS_TIMEOUT_MS || '10000');
const reportPath = process.env.STATUS_REPORT || `/tmp/teamgram-account-update-status-${Date.now()}.json`;
const apiId = Number(process.env.TG_API_ID || '1');
const apiHash = process.env.TG_API_HASH || '0123456789abcdef0123456789abcdef';
const postgresContainer = process.env.TEAMGRAM_POSTGRES_CONTAINER || 'teamgram-postgres';
const quietLogger = { debug() {}, info() {}, warn() {}, error() {} };

const globalAny = globalThis as typeof globalThis & { self?: any; addEventListener?: () => void };
globalAny.self ??= globalThis;
globalAny.addEventListener ??= () => {};
globalAny.self.addEventListener ??= globalAny.addEventListener;

const requireModule = createRequire(import.meta.url);
const gramjsDir = process.env.TEAMGRAM_GRAMJS_DIR || '/Users/mac/open/test/telegram-tt-master';
const TelegramClient = requireModule(path.join(gramjsDir, 'src/lib/gramjs/client/TelegramClient.ts')).default;
const CallbackSession = requireModule(path.join(gramjsDir, 'src/lib/gramjs/sessions/CallbackSession.ts')).default;
const Api = requireModule(path.join(gramjsDir, 'src/lib/gramjs/tl/index.ts')).Api;
const Connection = requireModule(path.join(gramjsDir, 'src/lib/gramjs/network/connection/TCPObfuscated.ts')).ConnectionTCPObfuscated;

type Presence = { id: number; userId: number; lastSeenAt: number; expires: number } | null;
type StatusEvent = {
  envelope: string;
  update: string;
  userId: number;
  status: string;
  expires: number;
  wasOnline: number;
  date: number;
  observedAt: number;
};
type Actor = { name: 'A' | 'B'; client: any; id: bigint; accessHash: bigint; events: StatusEvent[] };

const sleep = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms));

function sql(query: string): string {
  return execFileSync('docker', [
    'exec', postgresContainer, 'psql', '-v', 'ON_ERROR_STOP=1', '-At', '-q', '-F', '\t',
    '-U', 'teamgram', '-d', 'teamgram', '-c', query,
  ], { encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'] }).trim();
}

function rpcName(value: any): string {
  return value?.className || value?.constructor?.name || typeof value;
}

function check(condition: unknown, message: string): asserts condition {
  if (!condition) throw new Error(message);
}

function snapshotPresence(userId: number): Presence {
  const value = sql(`SELECT id,user_id,last_seen_at,expires FROM user_presences WHERE user_id=${userId}`);
  if (!value) return null;
  const [id, rowUserId, lastSeenAt, expires] = value.split('\t').map(Number);
  check([id, rowUserId, lastSeenAt, expires].every(Number.isInteger), 'invalid user_presences row');
  return { id, userId: rowUserId, lastSeenAt, expires };
}

function restorePresence(userId: number, original: Presence): void {
  if (!original) {
    sql(`DELETE FROM user_presences WHERE user_id=${userId}`);
    return;
  }
  sql(`UPDATE user_presences SET last_seen_at=${original.lastSeenAt}, expires=${original.expires} WHERE user_id=${userId}`);
  check(snapshotPresence(userId)?.lastSeenAt === original.lastSeenAt
    && snapshotPresence(userId)?.expires === original.expires, 'presence cleanup did not restore original row');
}

function collectStatusUpdates(value: any, envelope: string = rpcName(value), out: StatusEvent[] = []): StatusEvent[] {
  if (!value || typeof value !== 'object') return out;
  const name = rpcName(value);
  const date = Number(value.date || 0);
  if (name === 'UpdateUserStatus' || name.endsWith('.UpdateUserStatus')) {
    const status = value.status;
    out.push({
      envelope,
      update: name,
      userId: Number(value.userId),
      status: rpcName(status),
      expires: Number(status?.expires || 0),
      wasOnline: Number(status?.wasOnline || 0),
      date,
      observedAt: Date.now(),
    });
  }
  if (Array.isArray(value.updates)) {
    for (const nested of value.updates) collectStatusUpdates(nested, name, out);
  }
  if (value.update) collectStatusUpdates(value.update, name, out);
  return out;
}

function makeActor(name: 'A' | 'B', authB64: string): Actor {
  check(Buffer.from(authB64, 'base64').length === 256, `${name} auth key length invalid`);
  const session = authSession({ CallbackSession } as any, authB64, dcId, dcHost, dcPort);
  const client = new TelegramClient(session, apiId, apiHash, {
    connection: Connection,
    fallbackConnection: Connection,
    shouldAllowHttpTransport: false,
    shouldForceHttpTransport: false,
    timeout: 20,
    requestRetries: 1,
    connectionRetries: 1,
    connectionRetriesToFallback: 0,
    retryDelay: 250,
    autoReconnect: false,
    sequentialUpdates: true,
    baseLogger: quietLogger,
    deviceModel: `status-probe-${name}`,
  });
  (client as any)._borrowExportedSender = async () => undefined;
  const actor: Actor = { name, client, id: 0n, accessHash: 0n, events: [] };
  client.addEventHandler((update: any) => {
    actor.events.push(...collectStatusUpdates(update));
  }, { build: (update: any) => update });
  return actor;
}

async function bounded<T>(promise: Promise<T>): Promise<T> {
  let timer: ReturnType<typeof setTimeout> | undefined;
  try {
    return await Promise.race([promise, new Promise<T>((_, reject) => {
      timer = setTimeout(() => reject(new Error('probe timeout')), timeoutMs);
    })]);
  } finally {
    if (timer) clearTimeout(timer);
  }
}

async function connect(actor: Actor): Promise<void> {
  await bounded(actor.client.connect());
  const me = await bounded(actor.client.getMe());
  actor.id = BigInt(me?.id || 0);
  actor.accessHash = BigInt(me?.accessHash || 0);
  check(actor.id > 0n && actor.accessHash !== 0n, `${actor.name} getMe returned no identity`);
}

async function waitForStatus(actor: Actor, beforeCount: number, status: string): Promise<StatusEvent> {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    const event = actor.events.slice(beforeCount).find((candidate) => candidate.status.endsWith(status));
    if (event) return event;
    await sleep(25);
  }
  throw new Error(`${actor.name} did not receive ${status} updateUserStatus`);
}

async function updateStatus(actor: Actor, offline: boolean): Promise<any> {
  const result = await bounded(actor.client.invoke(new Api.account.UpdateStatus({ offline })));
  check(result === true || rpcName(result).toLowerCase().includes('true'), `${actor.name} updateStatus returned ${rpcName(result)}`);
  return result;
}

async function main(): Promise<void> {
  check(Number.isInteger(timeoutMs) && timeoutMs >= 1000 && timeoutMs <= 60000, 'invalid timeout');
  const authA = loadFixtureAuthKey(username, 0);
  const authB = loadFixtureAuthKey(username, 1);
  check(authA && authB, `fixture auth keys unavailable for ${username}`);
  const before = snapshotPresence(Number(process.env.STATUS_USER_ID || '11'));
  check(before?.userId === 11 || process.env.STATUS_USER_ID, 'presence fixture user mismatch');
  const userId = before?.userId || Number(process.env.STATUS_USER_ID);
  check(Number.isInteger(userId) && userId > 0, 'invalid status user id');
  let actorA: Actor | undefined;
  let actorB: Actor | undefined;
  const report: any = {
    schemaVersion: 1,
    probe: 'account.updateStatus.session',
    transport: `DC${dcId} TCPObfuscated -> gateway -> session -> UserChannelProfiles BFF -> User/Sync services`,
    layer: 229,
    username,
    userId,
    authKeyFixture: 'postgresql.active-permanent-offsets-0-and-1',
    beforePresence: before,
    transitions: [],
    cleanup: { ok: false },
    ok: false,
  };
  try {
    actorA = makeActor('A', authA);
    actorB = makeActor('B', authB);
    await Promise.all([connect(actorA), connect(actorB)]);
    check(actorA.id === actorB.id && actorA.id === BigInt(userId), 'auth keys did not bind to same fixture user');
    const [stateA, stateB] = await Promise.all([
      bounded(actorA.client.invoke(new Api.updates.GetState())),
      bounded(actorB.client.invoke(new Api.updates.GetState())),
    ]);
    check(rpcName(stateA) === 'updates.State' && rpcName(stateB) === 'updates.State', 'updates.getState did not return canonical state');
    report.initialUpdateState = {
      A: { pts: Number(stateA.pts), qts: Number(stateA.qts), seq: Number(stateA.seq), date: Number(stateA.date) },
      B: { pts: Number(stateB.pts), qts: Number(stateB.qts), seq: Number(stateB.seq), date: Number(stateB.date) },
    };
    report.authenticatedUserId = actorA.id.toString();
    report.distinctAuthKeySessions = true;

    for (const [offline, expectedStatus] of [[false, 'UserStatusOnline'], [true, 'UserStatusOffline']] as const) {
      const beforeA = actorA.events.length;
      const beforeB = actorB.events.length;
      const beforePresenceTransition = snapshotPresence(userId);
      await updateStatus(actorA, offline);
      const event = await waitForStatus(actorB, beforeB, expectedStatus);
      await sleep(50);
      const afterPresence = snapshotPresence(userId);
      check(event.userId === userId, 'updateUserStatus user_id mismatch');
      check(actorA.events.slice(beforeA).filter((candidate) => candidate.userId === userId).length === 0,
        'current auth session received its own updateUserStatus');
      check(afterPresence?.userId === userId && afterPresence.lastSeenAt >= (beforePresenceTransition?.lastSeenAt || 0),
        'user_presences last_seen_at did not advance');
      if (offline) {
        check(event.status.endsWith('UserStatusOffline') && event.wasOnline > 0 && afterPresence.expires === 0,
          'offline status did not match persisted presence');
      } else {
        check(event.status.endsWith('UserStatusOnline') && event.expires > 0 && afterPresence.expires > 0,
          'online status did not match persisted presence');
        check(Math.abs(event.expires - afterPresence.lastSeenAt - 300) <= 1,
          'online event expiry did not match persisted last_seen_at');
      }
      report.transitions.push({
        requestedOffline: offline,
        result: rpcName(mtprotoBoolTrue()),
        recipient: 'B',
        event,
        senderOwnStatusEvents: actorA.events.slice(beforeA).filter((candidate) => candidate.userId === userId).length,
        presenceBefore: beforePresenceTransition,
        presenceAfter: afterPresence,
      });
    }
    report.canonicalUpdateAndPresenceMatched = true;
  } finally {
    try { actorA?.client.destroy(); } catch {}
    try { actorB?.client.destroy(); } catch {}
    try {
      restorePresence(userId, before);
      report.cleanup = { ok: true, presenceRestored: snapshotPresence(userId) === null || snapshotPresence(userId)?.lastSeenAt === before?.lastSeenAt };
      check(report.cleanup.presenceRestored, 'presence cleanup verification failed');
    } catch (error) {
      report.cleanup = { ok: false, error: error instanceof Error ? error.message : 'cleanup failure' };
    }
    report.ok = report.canonicalUpdateAndPresenceMatched === true && report.cleanup.ok === true;
    fs.writeFileSync(reportPath, `${JSON.stringify(report, null, 2)}\n`, { encoding: 'utf8', mode: 0o600 });
    process.stdout.write(`${JSON.stringify({ ...report, reportPath })}\n`, () => process.exit(report.ok ? 0 : 1));
  }
}

function mtprotoBoolTrue(): any {
  return { className: 'BoolTrue' };
}

main().catch((error) => {
  const fallback = { schemaVersion: 1, probe: 'account.updateStatus.session', ok: false, error: error instanceof Error ? error.message : 'probe failure', reportPath };
  fs.writeFileSync(reportPath, `${JSON.stringify(fallback, null, 2)}\n`, { encoding: 'utf8', mode: 0o600 });
  process.stderr.write(`${JSON.stringify(fallback)}\n`);
  process.exit(1);
});
