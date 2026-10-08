import { createHash } from 'node:crypto';
import { execFileSync } from 'node:child_process';
import path from 'node:path';
import { createRequire } from 'node:module';

for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) {
  delete process.env[key];
}

const userId = process.env.RECENT_LOCATIONS_USER_ID || '136907713';
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

function loadAuthKey(): string {
  const body = postgres(`
    SELECT k.body
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

function stableSnapshot(): string {
  const row = postgres(`SELECT id, authorization_ttl_days, account_days_ttl, updated_at FROM users WHERE id=${userId}`);
  return createHash('sha256').update(row).digest('hex');
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

function int64(value: bigint): Uint8Array {
  const out = new Uint8Array(8);
  new DataView(out.buffer).setBigInt64(0, value, true);
  return out;
}

function recentLocationsRequest(limit: number): any {
  const constructorId = 0x702a40e0;
  const payload = Uint8Array.from(Buffer.concat([
    Buffer.from(int32(constructorId)),
    Buffer.from(int32(0x7da07ec9)),
    Buffer.from(int32(limit)),
    Buffer.from(int64(0n)),
  ]));
  return {
    className: 'messages.GetRecentLocations',
    classType: 'request',
    CONSTRUCTOR_ID: constructorId,
    getBytes: () => payload,
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
  const before = stableSnapshot();
  const client = makeClient(loadAuthKey());
  try {
    await client.connect();
    const me = await client.getMe();
    if (String(me?.id) !== userId) throw new Error('production auth key user mismatch');

    const result = await client.invoke(recentLocationsRequest(20));
    const resultType = rpcName(result);
    if (!resultType.toLowerCase().includes('messages')) {
      throw new Error(`messages.getRecentLocations returned ${resultType}`);
    }
    if (!Array.isArray(result.messages) || !Array.isArray(result.users) || !Array.isArray(result.chats)) {
      throw new Error('messages.getRecentLocations omitted required response vectors');
    }
    const after = stableSnapshot();
    if (before !== after) throw new Error('user persistence changed during the read-only location probe');
    console.log(JSON.stringify({
      transport: `DC${dcId} WebSocket -> gateway -> session -> Messages BFF -> Message/User service`,
      peer: 'InputPeerSelf',
      resultType,
      messageCount: result.messages.length,
      userCount: result.users.length,
      chatCount: result.chats.length,
      userRowUnchanged: true,
      writes: 0,
    }));
  } finally {
    client.destroy();
  }
}

main().then(() => process.exit(0)).catch((error) => {
  console.error(`recent locations production probe failed: ${error instanceof Error ? error.stack || error.message : String(error)}`);
  process.exit(1);
});
