import { createHash } from 'node:crypto';
import { execFileSync } from 'node:child_process';
import path from 'node:path';
import { createRequire } from 'node:module';

const gramjsDir = process.env.TEAMGRAM_GRAMJS_DIR || '/Users/mac/open/test/telegram-tt-master';
const dcId = Number(process.env.TG_DC_ID || '2');
const userId = process.env.USER_ID || '136907714';

if (!/^\d+$/.test(userId)) throw new Error('USER_ID must be numeric');

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

class HidePromoDataRequest {
  CONSTRUCTOR_ID = 0x1e251c95;
  SUBCLASS_OF_ID = 0;
  className = 'help.hidePromoData';
  classType = 'request';

  constructor(public peer: any) {}

  getBytes() {
    const constructorBytes = new Uint8Array(4);
    new DataView(constructorBytes.buffer).setUint32(0, this.CONSTRUCTOR_ID, true);
    const peerBytes = this.peer.getBytes();
    const bytes = new Uint8Array(constructorBytes.length + peerBytes.length);
    bytes.set(constructorBytes, 0);
    bytes.set(peerBytes, constructorBytes.length);
    return bytes;
  }

  readResult(reader: any) {
    return reader.tgReadObject();
  }
}

function rpcName(value: any): string {
  return value?.className || value?.constructor?.name || typeof value;
}

function isTrue(value: any): boolean {
  if (value === true) return true;
  if (value instanceof Boolean) return value.valueOf();
  return rpcName(value).toLowerCase().includes('true');
}

function sql(query: string): string {
  return execFileSync('docker', [
    'exec', 'teamgram-postgres', 'psql', '-v', 'ON_ERROR_STOP=1', '-At', '-q', '-F', '\t', '-U', 'teamgram', '-d', 'teamgram', '-c', query,
  ], { encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'] }).trim();
}

function loadAuthKey(): string {
  const body = sql(`
    SELECT encode(k.body, 'base64')
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
    retryDelay: 250,
    autoReconnect: false,
    baseLogger: quietLogger,
  });
}

type RedisSnapshot = { exists: boolean; raw: Buffer };

function redisSnapshot(key: string): RedisSnapshot {
  const exists = execFileSync('docker', ['exec', 'redis', 'redis-cli', '--raw', 'EXISTS', key], {
    encoding: 'utf8',
    stdio: ['ignore', 'pipe', 'ignore'],
  }).trim() === '1';
  if (!exists) return { exists: false, raw: Buffer.alloc(0) };

  const output = execFileSync('docker', ['exec', 'redis', 'redis-cli', '--raw', 'GET', key]) as Buffer;
  // redis-cli appends one line-feed; the stored promo payload is JSON and has no trailing line-feed.
  const raw = output.length > 0 && output[output.length - 1] === 0x0a
    ? output.subarray(0, output.length - 1)
    : output;
  return { exists: true, raw };
}

function restoreRedis(key: string, snapshot: RedisSnapshot): void {
  if (!snapshot.exists) {
    execFileSync('docker', ['exec', 'redis', 'redis-cli', '--raw', 'DEL', key], {
      encoding: 'utf8',
      stdio: ['ignore', 'pipe', 'ignore'],
    });
    return;
  }
  execFileSync('docker', ['exec', '-i', 'redis', 'redis-cli', '--raw', '-x', 'SET', key], {
    input: snapshot.raw,
    encoding: 'utf8',
    stdio: ['pipe', 'pipe', 'ignore'],
  });
}

function rawHash(snapshot: RedisSnapshot): string | null {
  return snapshot.exists ? createHash('sha256').update(snapshot.raw).digest('hex') : null;
}

function peerID(promo: any): bigint {
  const peer = promo?.peer;
  const id = peer?.userId ?? peer?.chatId ?? peer?.channelId;
  if (id === undefined) throw new Error(`help.getPromoData returned ${rpcName(promo)} without a peer`);
  return BigInt(id);
}

function isPromoEmpty(value: any): boolean {
  return rpcName(value).toLowerCase().endsWith('promodataempty');
}

async function main() {
  const authKey = loadAuthKey();
  const redisKey = `promo:hide:${userId}`;
  const original = redisSnapshot(redisKey);
  const client = makeClient(authKey);
  let restoreNeeded = false;

  try {
    (client as any)._borrowExportedSender = async () => undefined;
    (client as any)._updateLoop = async () => undefined;
    await client.connect();
    const me = await client.getMe();
    if (String(me?.id) !== userId) throw new Error(`auth key user mismatch: got ${String(me?.id)}, expected ${userId}`);

    const before = await client.invoke(new Api.help.GetPromoData());
    if (isPromoEmpty(before)) {
      throw new Error('help.getPromoData returned PromoDataEmpty before hide; no reversible peer is available');
    }
    const beforePeerID = peerID(before);
    // The request can commit before the response is decoded, so arm cleanup first.
    restoreNeeded = true;
    const hide = await client.invoke(new HidePromoDataRequest(
      new Api.InputPeerUser({ userId: beforePeerID, accessHash: 0n }),
    ));
    if (!isTrue(hide)) {
      throw new Error(`help.hidePromoData returned ${rpcName(hide)}`);
    }
    const redisAfterHide = redisSnapshot(redisKey);
    if (!redisAfterHide.exists) {
      throw new Error('help.hidePromoData returned true but did not persist promo state in Redis');
    }

    const after = await client.invoke(new Api.help.GetPromoData());
    const afterPeerID = isPromoEmpty(after) ? undefined : peerID(after);
    if (afterPeerID !== undefined && afterPeerID === beforePeerID) {
      throw new Error(`help.getPromoData after hide returned the hidden peer ${String(afterPeerID)}`);
    }

    restoreRedis(redisKey, original);
    const restored = redisSnapshot(redisKey);
    if (restored.exists !== original.exists || !restored.raw.equals(original.raw)) {
      throw new Error('promo Redis value did not restore byte-for-byte');
    }
    restoreNeeded = false;

    console.log(JSON.stringify({
      userId: String(me.id),
      redisKey,
      redisBeforeExists: original.exists,
      redisBeforeSha256: rawHash(original),
      beforeType: rpcName(before),
      beforePeerId: String(beforePeerID),
      hideType: rpcName(hide),
      redisAfterHideExists: redisAfterHide.exists,
      redisAfterHideSha256: rawHash(redisAfterHide),
      afterType: rpcName(after),
      afterPeerId: afterPeerID === undefined ? null : String(afterPeerID),
      redisRestoredExists: restored.exists,
      redisRestoredSha256: rawHash(restored),
      redisRestoredByteEqual: true,
    }));
  } finally {
    try {
      if (restoreNeeded) {
        restoreRedis(redisKey, original);
        const restored = redisSnapshot(redisKey);
        if (restored.exists !== original.exists || !restored.raw.equals(original.raw)) {
          throw new Error('promo Redis cleanup did not restore the original value');
        }
      }
    } finally {
      client.destroy();
    }
  }
}

main().then(() => process.exit(0)).catch((error) => {
  console.error(`promo data production probe failed: ${error instanceof Error ? error.stack || error.message : String(error)}`);
  process.exit(1);
});
