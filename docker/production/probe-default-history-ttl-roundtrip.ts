import { execFileSync } from 'node:child_process';
import path from 'node:path';
import { createRequire } from 'node:module';

const gramjsDir = process.env.TEAMGRAM_GRAMJS_DIR || '/Users/mac/open/test/telegram-tt-master';
const dcId = Number(process.env.TG_DC_ID || '2');
const userId = '136907714';

const globalAny = globalThis as typeof globalThis & { self?: any; addEventListener?: () => void };
globalAny.self ??= globalThis;
globalAny.addEventListener ??= () => {};
globalAny.self.addEventListener ??= globalAny.addEventListener;

const requireModule = createRequire(import.meta.url);
const TelegramClient = requireModule(path.join(gramjsDir, 'src/lib/gramjs/client/TelegramClient.ts')).default;
const CallbackSession = requireModule(path.join(gramjsDir, 'src/lib/gramjs/sessions/CallbackSession.ts')).default;
const Api = requireModule(path.join(gramjsDir, 'src/lib/gramjs/tl/index.ts')).Api;
const Connection = requireModule(path.join(gramjsDir, 'src/lib/gramjs/network/connection/TCPObfuscated.ts')).ConnectionTCPObfuscated;
const quietLogger = { debug() {}, info() {}, warn() {}, error() {} };

function sql(query: string): string {
  return execFileSync('docker', [
    'exec', 'mysql', 'mysql', '-N', '-s', '-uteamgram', '-pteamgram', '-Dteamgram', '-e', query,
  ], { encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'] }).trim();
}

function loadAuthKey(): string {
  const body = sql(`
    SELECT k.body
    FROM auth_users u
    JOIN auth_keys k USING (auth_key_id)
    JOIN auth_key_infos i USING (auth_key_id)
    WHERE u.user_id=${userId} AND u.state=0 AND u.deleted=0
      AND k.deleted=0 AND i.deleted=0 AND i.auth_key_type=0
    ORDER BY u.date_active DESC, u.id DESC
    LIMIT 1
  `);
  if (!body) throw new Error(`no active production auth key found for user ${userId}`);
  return Buffer.from(body, 'base64').toString('hex');
}

function storedPeriod(): number | undefined {
  const value = sql(`SELECT period FROM default_history_ttl WHERE user_id=${userId} LIMIT 1`);
  if (!value) return undefined;
  const period = Number(value);
  if (!Number.isInteger(period) || period < 0 || period > 366 * 86400) {
    throw new Error(`production default-history TTL row is invalid: ${value}`);
  }
  return period;
}

function rpcName(value: any): string {
  return value?.className || value?.constructor?.name || typeof value;
}

function isTrue(value: any): boolean {
  return value === true || rpcName(value).toLowerCase().includes('true');
}

function makeClient(authKeyHex: string): any {
  const session = new CallbackSession({ mainDcId: dcId, keys: { [dcId]: authKeyHex } }, () => {});
  const load = session.load.bind(session);
  session.load = async () => {
    await load();
    (session as any)._serverAddress = process.env.TG_DC_HOST || '127.0.0.1';
    (session as any)._port = Number(process.env.TG_DC_PORT || '11443');
  };
  return new TelegramClient(session, 1, '0123456789abcdef0123456789abcdef', {
    connection: Connection,
    fallbackConnection: Connection,
    shouldAllowHttpTransport: false,
    shouldForceHttpTransport: false,
    timeout: 20,
    requestRetries: 1,
    connectionRetries: 1,
    connectionRetriesToFallback: 0,
    autoReconnect: false,
    baseLogger: quietLogger,
  });
}

async function readPeriod(client: any): Promise<{ type: string; period: number }> {
  const result = await client.invoke(new Api.messages.GetDefaultHistoryTTL());
  if (rpcName(result) !== 'DefaultHistoryTTL') {
    throw new Error(`messages.getDefaultHistoryTTL returned ${rpcName(result)}`);
  }
  return { type: rpcName(result), period: Number(result.period) };
}

async function setPeriod(client: any, period: number): Promise<string> {
  const result = await client.invoke(new Api.messages.SetDefaultHistoryTTL({ period }));
  if (!isTrue(result)) throw new Error(`messages.setDefaultHistoryTTL returned ${rpcName(result)}`);
  return rpcName(result);
}

async function main() {
  const originalPeriod = storedPeriod();
  if (originalPeriod === undefined) {
    throw new Error(`refusing to mutate user ${userId}: no pre-existing TTL row to restore`);
  }

  const client = makeClient(loadAuthKey());
  const candidatePeriod = originalPeriod === 86400 ? 172800 : 86400;
  let restoreNeeded = false;
  try {
    (client as any)._borrowExportedSender = async () => undefined;
    (client as any)._updateLoop = async () => undefined;
    await client.connect();
    const me = await client.getMe();
    if (String(me?.id) !== userId) throw new Error('auth key user mismatch');

    const before = await readPeriod(client);
    if (before.period !== originalPeriod) {
      throw new Error(`API/database TTL mismatch before write: api=${before.period}, db=${originalPeriod}`);
    }

    restoreNeeded = true;
    const set = await setPeriod(client, candidatePeriod);
    const storedCandidate = storedPeriod();
    const after = await readPeriod(client);
    if (storedCandidate !== candidatePeriod || after.period !== candidatePeriod) {
      throw new Error(`TTL write readback mismatch: api=${after.period}, db=${storedCandidate}`);
    }

    const setBack = await setPeriod(client, originalPeriod);
    const restored = await readPeriod(client);
    const storedRestored = storedPeriod();
    if (restored.period !== originalPeriod || storedRestored !== originalPeriod) {
      throw new Error(`TTL restore mismatch: api=${restored.period}, db=${storedRestored}, expected=${originalPeriod}`);
    }
    restoreNeeded = false;

    console.log(JSON.stringify({
      userId: String(me.id),
      beforeType: before.type,
      originalPeriod,
      setType: set,
      candidatePeriod,
      candidateReadbackType: after.type,
      candidateDatabasePeriod: storedCandidate,
      restoreType: setBack,
      restoredReadbackType: restored.type,
      restoredPeriod: restored.period,
      restoredDatabasePeriod: storedRestored,
    }));
  } finally {
    try {
      if (restoreNeeded) {
        const current = storedPeriod();
        if (current === candidatePeriod) {
          await setPeriod(client, originalPeriod);
        } else if (current !== originalPeriod) {
          throw new Error(`refusing to overwrite concurrent TTL value ${String(current)}`);
        }
        const restored = await readPeriod(client);
        const storedRestored = storedPeriod();
        if (restored.period !== originalPeriod || storedRestored !== originalPeriod) {
          throw new Error(`TTL cleanup mismatch: api=${restored.period}, db=${storedRestored}, expected=${originalPeriod}`);
        }
      }
    } finally {
      client.destroy();
    }
  }
}

main().then(() => process.exit(0)).catch((error) => {
  console.error(`default history TTL production probe failed: ${error instanceof Error ? error.stack || error.message : String(error)}`);
  process.exit(1);
});
