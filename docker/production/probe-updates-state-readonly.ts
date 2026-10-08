import { execFileSync } from 'node:child_process';
import path from 'node:path';
import { createRequire } from 'node:module';

for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) {
  delete process.env[key];
}

const userId = process.env.UPDATES_STATE_USER_ID || '136907714';
const dcId = Number(process.env.TG_DC_ID || '2');
const dcHost = process.env.TG_DC_HOST || '127.0.0.1';
const dcPort = Number(process.env.TG_DC_PORT || '11443');
const gramjsDir = process.env.TEAMGRAM_GRAMJS_DIR || '/Users/mac/open/test/telegram-tt-master';

const globalAny = globalThis as typeof globalThis & { self?: any; addEventListener?: () => void };
globalAny.self ??= globalThis;
globalAny.addEventListener ??= () => {};
globalAny.self.addEventListener ??= globalAny.addEventListener;

function postgres(query: string): string {
  return execFileSync('docker', [
    'exec', 'teamgram-postgres', 'psql', '-v', 'ON_ERROR_STOP=1', '-At', '-q', '-F', '\t', '-U', 'teamgram', '-d', 'teamgram', '-c', query,
  ], { encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'] }).trim();
}

function redis(key: string): string {
  return execFileSync('docker', ['exec', 'redis', 'redis-cli', '--raw', 'GET', key], {
    encoding: 'utf8',
    stdio: ['ignore', 'pipe', 'ignore'],
  }).trim();
}

function loadAuth(): { authKeyId: string; authKeyHex: string } {
  const raw = postgres(`
    SELECT k.auth_key_id, k.body
    FROM auth_users u
    JOIN auth_keys k USING (auth_key_id)
    JOIN auth_key_infos i USING (auth_key_id)
    WHERE u.user_id=${userId} AND u.state=0 AND u.deleted=0
      AND k.deleted=0 AND i.deleted=0 AND i.auth_key_type=0
    ORDER BY u.date_active DESC, u.id DESC
    LIMIT 1
  `);
  const separator = raw.indexOf('\t');
  if (separator < 0) throw new Error(`no active production auth key found for user ${userId}`);
  const authKeyId = raw.slice(0, separator);
  const key = Buffer.from(raw.slice(separator + 1), 'base64');
  if (key.length !== 256) throw new Error('production auth key has an invalid length');
  return { authKeyId, authKeyHex: key.toString('hex') };
}

function stateSnapshot(authKeyId: string) {
  const secretState = postgres(`
    SELECT last_qts, confirmed_qts FROM apifull_secret_user_state WHERE user_id=${userId}
  `);
  if (!secretState) throw new Error(`no production secret-update state found for user ${userId}`);
  return {
    pts: redis(`pts_updates_ngen_${userId}`),
    seq: redis(`seq_updates_ngen_${authKeyId}`),
    idgenQts: redis(`qts_updates_ngen_${authKeyId}`),
    secretState,
  };
}

const requireModule = createRequire(import.meta.url);
const TelegramClient = requireModule(path.join(gramjsDir, 'src/lib/gramjs/client/TelegramClient.ts')).default;
const CallbackSession = requireModule(path.join(gramjsDir, 'src/lib/gramjs/sessions/CallbackSession.ts')).default;
const Api = requireModule(path.join(gramjsDir, 'src/lib/gramjs/tl/index.ts')).Api;
const Connection = requireModule(path.join(gramjsDir, 'src/lib/gramjs/network/connection/TCPObfuscated.ts')).ConnectionTCPObfuscated;
const quietLogger = { debug() {}, info() {}, warn() {}, error() {} };

function rpcName(value: any): string {
  return value?.className || value?.constructor?.name || typeof value;
}

function makeClient(authKeyHex: string): any {
  const session = new CallbackSession({ mainDcId: dcId, keys: { [dcId]: authKeyHex } }, () => {});
  const load = session.load.bind(session);
  session.load = async () => {
    await load();
    (session as any)._serverAddress = dcHost;
    (session as any)._port = dcPort;
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

async function main() {
  const { authKeyId, authKeyHex } = loadAuth();
  const before = stateSnapshot(authKeyId);
  const expectedPts = Number(before.pts);
  const [expectedQts] = before.secretState.split('\t').map(Number);
  const expectedSeq = Number(before.seq || 0) || -1;
  if (!Number.isSafeInteger(expectedPts) || expectedPts <= 0) {
    throw new Error('a non-zero existing PTS is required to keep updates.getState read-only');
  }
  if (!Number.isSafeInteger(expectedQts) || expectedQts < 0) {
    throw new Error(`invalid persisted secret QTS: ${before.secretState}`);
  }

  const client = makeClient(authKeyHex);
  try {
    (client as any)._borrowExportedSender = async () => undefined;
    (client as any)._updateLoop = async () => undefined;
    await client.connect();
    const me = await client.getMe();
    if (String(me?.id) !== userId) throw new Error(`auth key user mismatch: ${String(me?.id)}`);
    const state = await client.invoke(new Api.updates.GetState());
    const type = rpcName(state);
    const currentSecond = Math.floor(Date.now() / 1000);
    if (!type.toLowerCase().endsWith('state')
      || Number(state?.pts) !== expectedPts
      || Number(state?.seq) !== expectedSeq
      || Number(state?.qts) !== expectedQts
      || !Number.isSafeInteger(Number(state?.date))
      || Math.abs(currentSecond - Number(state.date)) > 60) {
      throw new Error(`updates.getState response mismatch: type=${type}, pts=${String(state?.pts)}, seq=${String(state?.seq)}, qts=${String(state?.qts)}`);
    }
    const after = stateSnapshot(authKeyId);
    if (JSON.stringify(after) !== JSON.stringify(before)) {
      throw new Error('updates.getState changed a Redis or PostgreSQL state counter');
    }
    console.log(JSON.stringify({
      userId: String(me.id),
      transport: `DC${dcId} WebSocket -> gateway -> session -> Updates BFF -> biz updates/IDGen + PostgreSQL secret-QTS`,
      type,
      pts: Number(state.pts),
      seq: Number(state.seq),
      qts: Number(state.qts),
      date: Number(state.date),
      redisPts: before.pts,
      redisSeq: before.seq || '0',
      idgenQts: before.idgenQts || '0',
      postgresSecretState: before.secretState,
      stateUnchanged: true,
      writes: 0,
    }));
  } finally {
    client.destroy();
  }
}

main().then(() => process.exit(0)).catch((error) => {
  console.error(`updates.getState production probe failed: ${error instanceof Error ? error.stack || error.message : String(error)}`);
  process.exit(1);
});
