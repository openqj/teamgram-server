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
      const result = await client.invoke(new Api.payments.GetStarGifts({ hash: 0 }));
      const type = className(result);
      if (type !== 'StarGifts' && type !== 'payments.StarGifts') {
        throw new Error(`unexpected payments.getStarGifts result: ${type}`);
      }
      const gifts = Array.isArray(result?.gifts) ? result.gifts : [];
      if (gifts.length === 0) throw new Error('payments.getStarGifts returned no durable catalog records');
      if (gifts.some((gift: any) => className(gift) !== 'StarGift' || className(gift?.sticker) !== 'DocumentEmpty')) {
        throw new Error('payments.getStarGifts returned an invalid StarGift/document union');
      }
      const unique = await client.invoke(new Api.payments.GetUniqueStarGift({ slug: 'prod-gift' }));
      const uniqueType = className(unique);
      if (uniqueType !== 'UniqueStarGift' && uniqueType !== 'payments.UniqueStarGift') {
        throw new Error(`unexpected payments.getUniqueStarGift result: ${uniqueType}`);
      }
      if (!unique?.gift || className(unique.gift) !== 'StarGift' || className(unique.gift.sticker) !== 'DocumentEmpty') {
        throw new Error('payments.getUniqueStarGift returned an invalid StarGift/document union');
      }
      const giftSummary = gifts.map((gift: any) => ({
        id: String(gift.id ?? ''),
        stars: String(gift.stars ?? ''),
        convertStars: String(gift.convertStars ?? ''),
      }));
      const encoded = result.getBytes ? result.getBytes().length : undefined;
      console.log(JSON.stringify({
        userId: String(me.id),
        resultType: type,
        giftCount: gifts.length,
        gifts: giftSummary,
        uniqueType,
        uniqueGiftId: String(unique.gift.id ?? ''),
        stickerTypes: [...new Set(gifts.map((gift: any) => className(gift.sticker)))],
        wireBytes: encoded,
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
