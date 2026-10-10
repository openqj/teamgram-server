import { createHash } from 'node:crypto';
import { execFileSync } from 'node:child_process';
import path from 'node:path';
import { createRequire } from 'node:module';

for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) {
  delete process.env[key];
}

const userId = process.env.PAYMENT_SAVED_INFO_USER_ID || '136907713';
const dcId = Number(process.env.TG_DC_ID || '2');
const dcHost = process.env.TG_DC_HOST || '127.0.0.1';
const dcPort = Number(process.env.TG_DC_PORT || '11443');
const gramjsDir = process.env.TEAMGRAM_GRAMJS_DIR || '/Users/mac/open/test/telegram-tt-master';
const keys = [`pay:info:${userId}`, `pay:cred:${userId}`];

const globalAny = globalThis as typeof globalThis & { self?: any; addEventListener?: () => void };
globalAny.self ??= globalThis;
globalAny.addEventListener ??= () => {};
globalAny.self.addEventListener ??= globalAny.addEventListener;

function postgres(query: string): string {
  return execFileSync('docker', [
    'exec', 'teamgram-postgres', 'psql', '-v', 'ON_ERROR_STOP=1', '-At', '-q', '-F', '\t', '-U', 'teamgram', '-d', 'teamgram', '-c', query,
  ], { encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'] }).trim();
}

function snapshot(): Record<string, { rows: number; hash: string; value: string }> {
  return Object.fromEntries(keys.map((key) => {
    const row = postgres(`SELECT COUNT(*), COALESCE(MAX(encode(digest(v::text, 'sha256'), 'hex')), ''), COALESCE(MAX(encode(convert_to(v::text, 'UTF8'), 'hex')), '') FROM apifull_kv WHERE k='${key}'`);
    const [rows, hash, valueHex] = row.split('\t');
    return [key, {
      rows: Number(rows),
      hash: hash || '',
      value: Buffer.from(valueHex || '', 'hex').toString('utf8'),
    }];
  }));
}

function loadAuthKey(): string {
  const body = postgres(`
    SELECT encode(k.body, 'base64')
    FROM auth_users u
    JOIN auth_keys k USING (auth_key_id)
    JOIN auth_key_infos i USING (auth_key_id)
    WHERE u.user_id=${userId} AND u.state=0 AND u.deleted=0
      AND k.deleted=0 AND i.deleted=0 AND i.auth_key_type=0
    ORDER BY u.date_active DESC, u.id DESC
    LIMIT 1
  `);
  const key = Buffer.from(body, 'base64');
  if (key.length !== 256) throw new Error('active production auth key is absent or invalid');
  return key.toString('hex');
}

const requireModule = createRequire(import.meta.url);
const TelegramClient = requireModule(path.join(gramjsDir, 'src/lib/gramjs/client/TelegramClient.ts')).default;
const CallbackSession = requireModule(path.join(gramjsDir, 'src/lib/gramjs/sessions/CallbackSession.ts')).default;
const Connection = requireModule(path.join(gramjsDir, 'src/lib/gramjs/network/connection/TCPObfuscated.ts')).ConnectionTCPObfuscated;
const quietLogger = { debug() {}, info() {}, warn() {}, error() {} };

function rpcName(value: any): string {
  return value?.className || value?.constructor?.name || typeof value;
}

function int32(value: number): Uint8Array {
  const out = new Uint8Array(4);
  new DataView(out.buffer).setInt32(0, value, true);
  return out;
}

function request(): any {
  const constructorId = 0x227d824b;
  return {
    className: 'payments.GetSavedInfo',
    classType: 'request',
    CONSTRUCTOR_ID: constructorId,
    getBytes: () => int32(constructorId),
    readResult: (reader: any) => reader.tgReadObject(),
  };
}

function makeClient(authKeyHex: string): any {
  const session = new CallbackSession({ mainDcId: dcId, keys: { [dcId]: authKeyHex } }, () => {});
  const load = session.load.bind(session);
  session.load = async () => {
    await load();
    (session as any)._serverAddress = dcHost;
    (session as any)._port = dcPort;
  };
  const client = new TelegramClient(session, 1, '0123456789abcdef0123456789abcdef', {
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
  (client as any)._borrowExportedSender = async () => undefined;
  (client as any)._updateLoop = async () => undefined;
  return client;
}

async function main() {
  const before = snapshot();
  const client = makeClient(loadAuthKey());
  try {
    await client.connect();
    const me = await client.getMe();
    if (String(me?.id) !== userId) throw new Error('production auth key user mismatch');
    const result = await client.invoke(request());
    if (!rpcName(result).endsWith('SavedInfo')) throw new Error(`payments.getSavedInfo returned ${rpcName(result)}`);
    if (before[keys[0]].rows === 0 && before[keys[1]].rows === 0) {
      if (result.hasSavedCredentials === true || result.savedInfo?.name || result.savedInfo?.phone || result.savedInfo?.email) {
        throw new Error('empty payment KV rows did not return empty saved-info defaults');
      }
    }
    const after = snapshot();
    if (JSON.stringify(before) !== JSON.stringify(after)) throw new Error('payment saved-info KV changed during read-only probe');
    console.log(JSON.stringify({
      transport: `DC${dcId} WebSocket -> gateway -> session -> APIFull/PostgreSQL`,
      resultType: rpcName(result),
      hasSavedCredentials: result.hasSavedCredentials === true,
      savedInfoPresent: Boolean(result.savedInfo),
      kvUnchanged: true,
      kvHashes: Object.fromEntries(keys.map((key) => [key, before[key].hash])),
      writes: 0,
    }));
  } finally {
    client.destroy();
  }
}

main().then(() => process.exit(0)).catch((error) => {
  console.error(`payment saved-info production probe failed: ${error instanceof Error ? error.stack || error.message : String(error)}`);
  process.exit(1);
});
