import fs from 'node:fs';
import path from 'node:path';
import { execFileSync } from 'node:child_process';
import { createRequire } from 'node:module';

const gramjsDir = process.env.TEAMGRAM_GRAMJS_DIR || '/Users/mac/open/test/telegram-tt-master';
const leveldbDir = process.env.CODEX_BROWSER_LEVELDB
  || '/Users/mac/Library/Application Support/Codex/Default/Partitions/codex-browser-app/Local Storage/leveldb';
const dcId = Number(process.env.TG_DC_ID || '2');
const dcHost = process.env.TG_DC_HOST || '127.0.0.1';
const dcPort = Number(process.env.TG_DC_PORT || '11443');
const expectedUserId = process.env.EXPECTED_USER_ID || '';

const globalAny = globalThis as typeof globalThis & { self?: any; addEventListener?: () => void };
globalAny.self ??= globalThis;
globalAny.addEventListener ??= () => {};
globalAny.self.addEventListener ??= globalAny.addEventListener;

function rpcName(value: any): string {
  return value?.className || value?.constructor?.name || typeof value;
}

function loadStoredSessions(): Array<{ authKeyHex: string; userId: string }> {
  if (!fs.existsSync(leveldbDir)) return [];
  const files = fs.readdirSync(leveldbDir)
    .filter((name) => /\.(ldb|log)$/.test(name))
    .map((name) => path.join(leveldbDir, name));
  const matches: Array<{ mtime: number; authKeyHex: string; userId: string }> = [];
  for (const file of files) {
    let raw: string;
    try {
      raw = execFileSync('strings', [file], { encoding: 'utf8', maxBuffer: 16 * 1024 * 1024 });
    } catch {
      continue;
    }
    const re = /\{"dcId":2,"dc2_auth_key":"([0-9a-f]+)","userId":"(\d+)"/g;
    for (const match of raw.matchAll(re)) {
      matches.push({ mtime: fs.statSync(file).mtimeMs, authKeyHex: match[1], userId: match[2] });
    }
  }
  matches.sort((a, b) => b.mtime - a.mtime);
  const seen = new Set<string>();
  return matches.filter((session) => {
    if (seen.has(session.userId)) return false;
    seen.add(session.userId);
    return true;
  });
}

const requireModule = createRequire(import.meta.url);
const TelegramClient = requireModule(path.join(gramjsDir, 'src/lib/gramjs/client/TelegramClient.ts')).default;
const CallbackSession = requireModule(path.join(gramjsDir, 'src/lib/gramjs/sessions/CallbackSession.ts')).default;
const Api = requireModule(path.join(gramjsDir, 'src/lib/gramjs/tl/index.ts')).Api;
const Connection = requireModule(path.join(gramjsDir, 'src/lib/gramjs/network/connection/TCPObfuscated.ts')).ConnectionTCPObfuscated;

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
    baseLogger: { debug() {}, info() {}, warn() {}, error() {} },
  });
}

function isBoolTrue(value: any): boolean {
  if (value === true) return true;
  if (value instanceof Boolean) return value.valueOf();
  return rpcName(value).toLowerCase().includes('true');
}

function isBoolFalse(value: any): boolean {
  if (value === false) return true;
  if (value instanceof Boolean) return !value.valueOf();
  return rpcName(value).toLowerCase().includes('false');
}

async function main() {
  const configuredKey = process.env.CODEX_BROWSER_AUTH_B64;
  const sessions = configuredKey
    ? [{ authKeyHex: Buffer.from(configuredKey, 'base64').toString('hex'), userId: expectedUserId || 'unknown' }]
    : loadStoredSessions();
  const orderedSessions = expectedUserId
    ? sessions.filter((session) => session.userId === expectedUserId)
    : sessions;
  if (orderedSessions.length === 0) throw new Error('no stored production browser session found for the expected user');

  const base = `codexaudit${Date.now().toString(36)}${process.pid.toString(36)}`;
  const candidates = [base, `${base}a`, `${base}b`];
  let lastError: unknown;

  for (const stored of orderedSessions) {
    const client = makeClient(stored.authKeyHex);
    try {
      (client as any)._borrowExportedSender = async () => undefined;
      (client as any)._updateLoop = async () => undefined;
      await client.connect();
      const me = await client.getMe();
      if (!me?.id || (expectedUserId && String(me.id) !== expectedUserId)) {
        throw new Error(`auth key user mismatch: got ${String(me?.id)}, expected ${expectedUserId || stored.userId}`);
      }
      let selected: { username: string; result: any } | undefined;
      for (const username of candidates) {
        const result = await client.invoke(new Api.account.CheckUsername({ username }));
        if (isBoolTrue(result)) {
          selected = { username, result };
          break;
        }
        if (!isBoolFalse(result)) {
          throw new Error(`unexpected account.checkUsername result: ${rpcName(result)}`);
        }
      }
      if (!selected) throw new Error('all generated usernames were occupied');
      console.log(JSON.stringify({
        userId: String(me.id),
        username: selected.username,
        resultType: rpcName(selected.result),
        writes: 0,
        candidatesChecked: candidates.indexOf(selected.username) + 1,
      }));
      return;
    } catch (error) {
      lastError = error;
    } finally {
      client.destroy();
    }
  }
  throw lastError || new Error('no production session completed the probe');
}

main().then(() => process.exit(0)).catch((error) => {
  console.error(`account.checkUsername probe failed: ${error instanceof Error ? error.stack || error.message : String(error)}`);
  process.exit(1);
});
