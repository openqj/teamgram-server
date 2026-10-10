import { execFileSync } from 'node:child_process';
import path from 'node:path';
import { createRequire } from 'node:module';

for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) {
  delete process.env[key];
}

const userId = process.env.AUTHORIZATION_TTL_USER_ID || '136907713';
const dcId = Number(process.env.TG_DC_ID || '2');
const dcHost = process.env.TG_DC_HOST || '127.0.0.1';
const dcPort = Number(process.env.TG_DC_PORT || '11443');
const gramjsDir = process.env.TEAMGRAM_GRAMJS_DIR || '/Users/mac/open/test/telegram-tt-master';
const originalDays = 180;
const temporaryDays = 90;

const globalAny = globalThis as typeof globalThis & { self?: any; addEventListener?: () => void };
globalAny.self ??= globalThis;
globalAny.addEventListener ??= () => {};
globalAny.self.addEventListener ??= globalAny.addEventListener;

function postgres(query: string): string {
  return execFileSync('docker', [
    'exec', 'teamgram-postgres', 'psql', '-v', 'ON_ERROR_STOP=1', '-At', '-q', '-F', '\t', '-U', 'teamgram', '-d', 'teamgram', '-c', query,
  ], { encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'] }).trim();
}

function readState(): { days: number; updatedAt: string } {
  const row = postgres(`SELECT authorization_ttl_days, to_char(updated_at, 'YYYY-MM-DD HH24:MI:SS') FROM users WHERE id=${userId}`);
  const [days, updatedAt] = row.split('\t');
  if (days === undefined || !updatedAt) throw new Error('production user authorization TTL row is missing');
  return { days: Number(days), updatedAt };
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

function boolTrue(value: any): boolean {
  return value === true || value?.value === true || value?.value === 1
    || rpcName(value).toLowerCase().includes('true');
}

function int32(value: number): Uint8Array {
  const out = new Uint8Array(4);
  new DataView(out.buffer).setInt32(0, value, true);
  return out;
}

function setTtlRequest(days: number): any {
  const constructorId = 0xbf899aa0;
  const payload = new Uint8Array(8);
  payload.set(int32(constructorId), 0);
  payload.set(int32(days), 4);
  return {
    className: 'account.SetAuthorizationTTL',
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
  const before = readState();
  if (before.days !== originalDays) {
    throw new Error(`refusing to change authorization TTL from unexpected baseline ${before.days}`);
  }

  const client = makeClient(loadAuthKey());
  let connected = false;
  let writeStarted = false;
  let ownUpdatedAt = '';
  let primaryError: unknown;
  let cleanupError: unknown;
  let temporaryType = '';
  let restoreType = '';
  try {
    await client.connect();
    connected = true;
    const me = await client.getMe();
    if (String(me?.id) !== userId) throw new Error('production auth key user mismatch');

    writeStarted = true;
    const temporary = await client.invoke(setTtlRequest(temporaryDays));
    temporaryType = rpcName(temporary);
    if (!boolTrue(temporary)) throw new Error(`account.setAuthorizationTTL(90) returned ${temporaryType}`);
    let state = readState();
    if (state.days !== temporaryDays) throw new Error('temporary authorization TTL did not persist to PostgreSQL');
    ownUpdatedAt = state.updatedAt;

    const restored = await client.invoke(setTtlRequest(originalDays));
    restoreType = rpcName(restored);
    if (!boolTrue(restored)) throw new Error(`account.setAuthorizationTTL(180) returned ${restoreType}`);
    state = readState();
    if (state.days !== before.days) throw new Error('authorization TTL did not read back its original value');
    ownUpdatedAt = state.updatedAt;
  } catch (error) {
    primaryError = error;
  } finally {
    if (connected && writeStarted) {
      try {
        let current = readState();
        if (current.days === temporaryDays) {
          const restored = await client.invoke(setTtlRequest(originalDays));
          if (!boolTrue(restored)) throw new Error('failed to restore authorization TTL through RPC');
          current = readState();
          if (current.days !== originalDays) throw new Error('authorization TTL RPC cleanup readback mismatch');
          ownUpdatedAt = current.updatedAt;
        } else if (current.days !== originalDays) {
          throw new Error('authorization TTL changed outside the probe; left it untouched');
        }

        if (current.updatedAt !== before.updatedAt) {
          if (!ownUpdatedAt || current.updatedAt !== ownUpdatedAt) {
            throw new Error('authorization TTL audit timestamp changed outside the probe; left it untouched');
          }
          const restoredTimestamp = postgres(`UPDATE users SET updated_at='${before.updatedAt}' WHERE id=${userId} AND authorization_ttl_days=${originalDays} AND updated_at='${ownUpdatedAt}' RETURNING 1`);
          if (restoredTimestamp !== '1') throw new Error('guarded audit timestamp restoration did not update exactly one row');
        }
        const finalState = readState();
        if (finalState.days !== before.days || finalState.updatedAt !== before.updatedAt) {
          throw new Error('authorization TTL row did not return to its exact preflight state');
        }
      } catch (error) {
        cleanupError = error;
      }
    }
    client.destroy();
  }

  if (cleanupError) throw new Error(`authorization TTL cleanup failed: ${String(cleanupError)}`);
  if (primaryError) throw primaryError;
  console.log(JSON.stringify({
    transport: `DC${dcId} WebSocket -> gateway -> Authorization BFF -> User service/PostgreSQL`,
    temporaryType,
    restoreType,
    changedDays: [originalDays, temporaryDays, originalDays],
    exactRowRestored: true,
    rpcWrites: 2,
  }));
}

main().then(() => process.exit(0)).catch((error) => {
  console.error(`authorization TTL production probe failed: ${error instanceof Error ? error.stack || error.message : String(error)}`);
  process.exit(1);
});
