import fs from 'node:fs';
import path from 'node:path';
import { execFileSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { createRequire } from 'node:module';

const gramjsDir = process.env.TEAMGRAM_GRAMJS_DIR || '/Users/mac/open/test/telegram-tt-master';
const leveldbDir = process.env.CODEX_BROWSER_LEVELDB
  || '/Users/mac/Library/Application Support/Codex/Default/Partitions/codex-browser-app/Local Storage/leveldb';
const dcId = Number(process.env.TG_DC_ID || '2');
const dcHost = process.env.TG_DC_HOST || '127.0.0.1';
const dcPort = Number(process.env.TG_DC_PORT || '11443');
const expectedUserId = process.env.USER_ID?.trim();
const query = process.env.SPONSORED_QUERY || '';
const expectedCollectibleId = process.env.COLLECTIBLE_ID?.trim();

const globalAny = globalThis as typeof globalThis & { self?: any; addEventListener?: () => void };
globalAny.self ??= globalThis;
globalAny.addEventListener ??= () => {};
globalAny.self.addEventListener ??= globalAny.addEventListener;

type StoredSession = { authKeyHex: string; userId: string; authKeyId: string };

function authKeyId(authKeyHex: string): string {
  const digest = createHash('sha1').update(Buffer.from(authKeyHex, 'hex')).digest();
  return Buffer.from(digest.subarray(-8)).reverse().readBigUInt64BE(0).toString();
}

function loadStoredSessions(): StoredSession[] {
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
  return matches.flatMap((session) => {
    const id = authKeyId(session.authKeyHex);
    if (seen.has(id)) return [];
    seen.add(id);
    return [{ ...session, authKeyId: id }];
  });
}

const requireModule = createRequire(import.meta.url);
const TelegramClient = requireModule(path.join(gramjsDir, 'src/lib/gramjs/client/TelegramClient.ts')).default;
const CallbackSession = requireModule(path.join(gramjsDir, 'src/lib/gramjs/sessions/CallbackSession.ts')).default;
const Api = requireModule(path.join(gramjsDir, 'src/lib/gramjs/tl/index.ts')).Api;
const ConnectionTCPObfuscated = requireModule(
  path.join(gramjsDir, 'src/lib/gramjs/network/connection/TCPObfuscated.ts'),
).ConnectionTCPObfuscated;
const quietLogger = { debug() {}, info() {}, warn() {}, error() {} };

function className(value: any): string {
  return value?.className || value?.constructor?.name || typeof value;
}

function isClass(value: any, name: string): boolean {
  const actual = className(value);
  return actual === name || actual.endsWith(`.${name}`);
}

function makeClient(authKeyHex: string): any {
  const session = new CallbackSession({ mainDcId: dcId, keys: { [dcId]: authKeyHex } }, () => {});
  const load = session.load.bind(session);
  session.load = async () => {
    await load();
    (session as any)._serverAddress = dcHost;
    (session as any)._port = dcPort;
  };
  const client = new TelegramClient(session, Number(process.env.TG_API_ID || '1'), process.env.TG_API_HASH || '0123456789abcdef0123456789abcdef', {
    connection: ConnectionTCPObfuscated,
    fallbackConnection: ConnectionTCPObfuscated,
    shouldAllowHttpTransport: false,
    shouldForceHttpTransport: false,
    timeout: 20,
    requestRetries: 1,
    connectionRetries: 1,
    connectionRetriesToFallback: 0,
    retryDelay: 250,
    autoReconnect: false,
    baseLogger: quietLogger,
  });
  (client as any)._borrowExportedSender = async () => undefined;
  (client as any)._updateLoop = async () => undefined;
  return client;
}

function chooseSessions(): StoredSession[] {
  if (process.env.AUTH_B64) {
    const authKeyHex = Buffer.from(process.env.AUTH_B64, 'base64').toString('hex');
    return [{ authKeyHex, userId: expectedUserId || 'unknown', authKeyId: authKeyId(authKeyHex) }];
  }
  const sessions = loadStoredSessions();
  if (!expectedUserId) return sessions;
  return sessions.filter((session) => session.userId === expectedUserId);
}

async function main(): Promise<void> {
  if (!query) throw new Error('SPONSORED_QUERY is required');
  const sessions = chooseSessions();
  if (sessions.length === 0) throw new Error(`no browser auth session found for USER_ID=${expectedUserId || '(any)'}`);
  let lastError: unknown;
  for (const stored of sessions) {
    const client = makeClient(stored.authKeyHex);
    try {
      await client.connect();
      const me = await client.getMe();
      const userId = String(me?.id ?? stored.userId);
      if (expectedUserId && userId !== expectedUserId) {
        throw new Error(`authenticated account ${userId} does not match USER_ID=${expectedUserId}`);
      }

      const peers = await client.invoke(new Api.contacts.GetSponsoredPeers({ q: query }));
      const statuses = await client.invoke(new Api.account.GetCollectibleEmojiStatuses({ hash: 0n }));
      const statusItems = Array.isArray(statuses?.statuses) ? statuses.statuses : [];
      if (!isClass(peers, 'SponsoredPeers') && !isClass(peers, 'SponsoredPeersEmpty')) {
        throw new Error(`contacts.getSponsoredPeers returned ${className(peers)}`);
      }
      if (!isClass(statuses, 'EmojiStatuses') && !isClass(statuses, 'AccountEmojiStatuses')) {
        throw new Error(`account.getCollectibleEmojiStatuses returned ${className(statuses)}`);
      }
      if (statusItems.length === 0) throw new Error('account.getCollectibleEmojiStatuses returned no fixture status');
      if (!isClass(statusItems[0], 'EmojiStatusCollectible')) {
        throw new Error(`collectible status returned ${className(statusItems[0])}`);
      }
      if (expectedCollectibleId && String(statusItems[0].collectibleId) !== expectedCollectibleId) {
        throw new Error(`collectible id ${String(statusItems[0].collectibleId)} does not match fixture ${expectedCollectibleId}`);
      }

      const same = await client.invoke(new Api.account.GetCollectibleEmojiStatuses({ hash: statuses.hash }));
      if (!isClass(same, 'EmojiStatusesNotModified') && !isClass(same, 'AccountEmojiStatusesNotModified')) {
        throw new Error(`collectible hash replay returned ${className(same)}`);
      }
      console.log(JSON.stringify({
        event: 'sponsored_collectible_fixture_rpc',
        userId,
        authKeyId: stored.authKeyId,
        query,
        sponsoredPeersType: className(peers),
        sponsoredPeerCount: Array.isArray(peers?.peers) ? peers.peers.length : 0,
        collectibleType: className(statuses),
        collectibleStatusType: className(statusItems[0]),
        collectibleId: String(statusItems[0].collectibleId),
        documentId: String(statusItems[0].documentId),
        collectibleHash: String(statuses.hash),
        collectibleHashReplayType: className(same),
      }));
      return;
    } catch (error) {
      lastError = error;
      console.error(JSON.stringify({ event: 'sponsored_collectible_rpc_failed', authKeyId: stored.authKeyId, error: error?.errorMessage || error?.message || String(error) }));
    } finally {
      try { client.destroy(); } catch { /* best effort */ }
    }
  }
  throw lastError || new Error('all browser auth sessions failed');
}

void main().then(() => process.exit(0)).catch((error) => {
  console.error(error?.errorMessage || error?.message || String(error));
  process.exit(1);
});
