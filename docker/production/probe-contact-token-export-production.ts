import { execFileSync } from 'node:child_process';
import path from 'node:path';
import { createRequire } from 'node:module';

for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) {
  delete process.env[key];
}

const userId = process.env.CONTACT_TOKEN_USER_ID || '136907714';
const dcId = Number(process.env.TG_DC_ID || '2');
const dcHost = process.env.TG_DC_HOST || '127.0.0.1';
const dcPort = Number(process.env.TG_DC_PORT || '11443');
const gramjsDir = process.env.TEAMGRAM_GRAMJS_DIR || '/Users/mac/open/test/telegram-tt-master';
const tokenPrefix = 'https://t.me/+';

const globalAny = globalThis as typeof globalThis & { self?: any; addEventListener?: () => void };
globalAny.self ??= globalThis;
globalAny.addEventListener ??= () => {};
globalAny.self.addEventListener ??= globalAny.addEventListener;

function postgres(query: string): string {
  return execFileSync('docker', [
    'exec', 'teamgram-postgres', 'psql', '-v', 'ON_ERROR_STOP=1', '-At', '-q', '-F', '\t', '-U', 'teamgram', '-d', 'teamgram', '-c', query,
  ], { encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'] }).trim();
}

function postgresWrite(query: string): void {
  execFileSync('docker', [
    'exec', 'teamgram-postgres', 'psql', '-v', 'ON_ERROR_STOP=1', '-At', '-q', '-F', '\t', '-U', 'teamgram', '-d', 'teamgram', '-c', query,
  ], { stdio: ['ignore', 'ignore', 'ignore'] });
}

function sqlString(value: string): string {
  return `'${value.replaceAll("'", "''")}'`;
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

function tokenRows(): Array<{ key: string; value: string }> {
  const raw = postgres('SELECT k,v FROM apifull_kv WHERE k LIKE "contact-token:%" ORDER BY k');
  if (!raw) return [];
  return raw.split('\n').map((line) => {
    const splitAt = line.indexOf('\t');
    return { key: line.slice(0, splitAt), value: line.slice(splitAt + 1) };
  });
}

function deleteToken(key: string, value: string): void {
  postgresWrite(`DELETE FROM apifull_kv WHERE k=${sqlString(key)} AND v=${sqlString(value)}`);
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

class ExportContactTokenRequest {
  CONSTRUCTOR_ID = 0xf8654027;
  SUBCLASS_OF_ID = 0;
  className = 'contacts.ExportContactToken';
  classType = 'request';

  getBytes(): Uint8Array {
    const constructor = Buffer.alloc(4);
    constructor.writeUInt32LE(this.CONSTRUCTOR_ID, 0);
    return Uint8Array.from(constructor);
  }

  readResult(reader: any): any {
    return reader.tgReadObject();
  }
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
  const before = tokenRows();
  const beforeKeys = new Set(before.map((row) => row.key));
  const startedAt = Math.floor(Date.now() / 1000);
  const client = makeClient(loadAuthKey());
  let token = '';
  let tokenKey = '';
  let tokenValue = '';
  let failure: unknown;
  let cleanupError: unknown;
  let evidence: Record<string, unknown> = {};

  try {
    await client.connect();
    const me = await client.getMe();
    if (String(me?.id) !== userId) throw new Error('production auth key user mismatch');

    const exported = await client.invoke(new ExportContactTokenRequest());
    if (!rpcName(exported).toLowerCase().includes('exportedcontacttoken')
      || typeof exported?.url !== 'string' || !exported.url.startsWith(tokenPrefix)) {
      throw new Error(`contacts.exportContactToken returned unexpected ${rpcName(exported)}`);
    }
    token = exported.url.slice(tokenPrefix.length);
    if (!/^[A-Za-z0-9_-]{32}$/.test(token)) throw new Error('exported token format is invalid');
    tokenKey = `contact-token:${token}`;
    if (beforeKeys.has(tokenKey)) throw new Error('exported token key existed before the probe');

    const row = postgres(`SELECT v FROM apifull_kv WHERE k=${sqlString(tokenKey)}`);
    if (!row) throw new Error('exported contact token was not stored in production PostgreSQL');
    tokenValue = row;
    const stored = JSON.parse(row) as { user_id?: number; expires?: number };
    const expires = Number(exported.expires);
    if (Number(stored.user_id) !== Number(userId) || Number(stored.expires) !== expires
      || expires <= startedAt || expires > startedAt + 86403) {
      throw new Error('exported contact token owner or 24-hour expiry did not match its PostgreSQL record');
    }
    evidence = {
      constructor: rpcName(exported),
      tokenLength: token.length,
      ttlSeconds: expires - startedAt,
      ownerMatches: true,
      storedExpiryMatches: true,
    };
  } catch (error) {
    failure = error;
  }

  try {
    if (tokenKey && tokenValue) {
      deleteToken(tokenKey, tokenValue);
    } else {
      const possible = tokenRows().filter((row) => {
        if (beforeKeys.has(row.key)) return false;
        try {
          const record = JSON.parse(row.value) as { user_id?: number; expires?: number };
          return Number(record.user_id) === Number(userId)
            && Number(record.expires) >= startedAt + 86400
            && Number(record.expires) <= startedAt + 86403;
        } catch {
          return false;
        }
      });
      if (possible.length === 1) deleteToken(possible[0].key, possible[0].value);
      else if (possible.length > 1) throw new Error(`cleanup found ${possible.length} ambiguous new tokens for the test account`);
    }
    const remaining = tokenKey
      ? Number(postgres(`SELECT COUNT(*) FROM apifull_kv WHERE k=${sqlString(tokenKey)}`))
      : tokenRows().filter((row) => {
        if (beforeKeys.has(row.key)) return false;
        try {
          const record = JSON.parse(row.value) as { user_id?: number; expires?: number };
          return Number(record.user_id) === Number(userId)
            && Number(record.expires) >= startedAt + 86400
            && Number(record.expires) <= startedAt + 86403;
        } catch {
          return false;
        }
      }).length;
    evidence.tokenRowsAfterCleanup = remaining;
    if (remaining !== 0) throw new Error('production contact token cleanup did not remove the new row');
  } catch (error) {
    cleanupError = error;
  }
  client.destroy();

  if (cleanupError) throw cleanupError;
  if (failure) throw failure;
  console.log(JSON.stringify({
    transport: `DC${dcId} WebSocket -> gateway -> session -> APIFull/User service/PostgreSQL`,
    ...evidence,
    tokenExposed: false,
  }));
}

main().then(() => process.exit(0)).catch((error) => {
  console.error(`contact token export production probe failed: ${error instanceof Error ? error.stack || error.message : String(error)}`);
  process.exit(1);
});
