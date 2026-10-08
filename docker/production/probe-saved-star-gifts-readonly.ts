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

const globalAny = globalThis as typeof globalThis & { self?: any; addEventListener?: () => void };
globalAny.self ??= globalThis;
globalAny.addEventListener ??= () => {};
globalAny.self.addEventListener ??= globalAny.addEventListener;

function loadStoredSessions(): Array<{ authKeyHex: string; userId: string }> {
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

function className(value: any): string {
  return value?.className || value?.constructor?.name || typeof value;
}

function isType(value: any, name: string): boolean {
  const actual = className(value);
  return actual === name || actual === `payments.${name}`;
}

function tlInt(value: number): Uint8Array {
  const bytes = new Uint8Array(4);
  new DataView(bytes.buffer).setInt32(0, value, true);
  return bytes;
}

function tlString(value: string): Uint8Array {
  const body = Buffer.from(value, 'utf8');
  if (body.length >= 254) throw new Error('probe TL string is unexpectedly large');
  const bytes = new Uint8Array((1 + body.length + 3) & ~3);
  bytes[0] = body.length;
  bytes.set(body, 1);
  return bytes;
}

function rawRequest(constructorId: number, classNameValue: string, ...parts: Uint8Array[]): any {
  return {
    CONSTRUCTOR_ID: constructorId,
    SUBCLASS_OF_ID: 0,
    className: classNameValue,
    classType: 'request',
    getBytes: () => Uint8Array.from(Buffer.concat([
      Buffer.from(tlInt(constructorId)),
      ...parts.map((part) => Buffer.from(part)),
    ])),
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

function giftRows(userId: string): number {
  const output = execFileSync('docker', [
    'exec', 'teamgram-postgres', 'psql', '-v', 'ON_ERROR_STOP=1', '-At', '-q', '-F', '\t', '-U', 'teamgram', '-d', 'teamgram',
    '-c', `SELECT COUNT(*) FROM apifull_gift WHERE to_user=${Number(userId)};`,
  ], { encoding: 'utf8' });
  return Number.parseInt(output.trim(), 10) || 0;
}

async function main() {
  const configuredKey = process.env.CODEX_BROWSER_AUTH_B64;
  const sessions = configuredKey
    ? [{ authKeyHex: Buffer.from(configuredKey, 'base64').toString('hex'), userId: process.env.CODEX_BROWSER_USER_ID || 'unknown' }]
    : loadStoredSessions();
  const requestedIndex = Number.parseInt(process.env.CODEX_BROWSER_SESSION_INDEX || '0', 10);
  const ordered = Number.isInteger(requestedIndex) && requestedIndex >= 0
    ? [sessions[requestedIndex], ...sessions.filter((_, index) => index !== requestedIndex)].filter(Boolean)
    : sessions;
  let lastError: unknown;

  for (const stored of ordered) {
    const client = makeClient(stored.authKeyHex);
    try {
      (client as any)._borrowExportedSender = async () => undefined;
      (client as any)._updateLoop = async () => undefined;
      await client.connect();
      const me = await client.getMe();
      if (!me?.id || (stored.userId !== 'unknown' && String(me.id) !== stored.userId)) {
        throw new Error('stored session user does not match authenticated user');
      }
      const before = giftRows(String(me.id));
      if (before !== 0) throw new Error(`probe expects an empty saved-gift ledger, found ${before} rows`);
      const listed = await client.invoke(new Api.payments.GetSavedStarGifts({
        peer: new Api.InputPeerSelf(),
        offset: '',
        limit: 100,
      }));
      if (!isType(listed, 'SavedStarGifts') || Number(listed.count ?? -1) !== 0 || (listed.gifts?.length ?? 0) !== 0) {
        throw new Error(`unexpected getSavedStarGifts result: ${className(listed)} count=${listed.count} gifts=${listed.gifts?.length}`);
      }
      const slug = rawRequest(0x2085c238, 'inputSavedStarGiftSlug', tlString('prod-gift'));
      const bySlug = await client.invoke(rawRequest(
        0xb455a106,
        'payments.GetSavedStarGift',
        tlInt(0x1cb5c415),
        tlInt(1),
        slug.getBytes(),
      ));
      if (!isType(bySlug, 'SavedStarGifts') || Number(bySlug.count ?? -1) !== 0 || (bySlug.gifts?.length ?? 0) !== 0) {
        throw new Error(`unexpected getSavedStarGift result: ${className(bySlug)} count=${bySlug.count} gifts=${bySlug.gifts?.length}`);
      }
      const after = giftRows(String(me.id));
      if (after !== before) throw new Error(`saved-gift rows changed: before=${before} after=${after}`);
      console.log(JSON.stringify({
        userId: String(me.id),
        listType: className(listed),
        listCount: Number(listed.count),
        slugType: className(bySlug),
        slugCount: Number(bySlug.count),
        dbRowsBefore: before,
        dbRowsAfter: after,
        writes: 0,
      }));
      return;
    } catch (error) {
      lastError = error;
    } finally {
      client.destroy();
    }
  }
  throw lastError || new Error('no browser session found');
}

main().catch((error) => {
  console.error(error?.errorMessage || error?.message || String(error));
  process.exit(1);
}).then(() => process.exit(0));
