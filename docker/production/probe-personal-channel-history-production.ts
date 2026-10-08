import { createHash } from 'node:crypto';
import { execFileSync } from 'node:child_process';
import path from 'node:path';
import { createRequire } from 'node:module';

for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) {
  delete process.env[key];
}

const userId = process.env.PERSONAL_HISTORY_USER_ID || '136907714';
const dcId = Number(process.env.TG_DC_ID || '2');
const dcHost = process.env.TG_DC_HOST || '127.0.0.1';
const dcPort = Number(process.env.TG_DC_PORT || '11443');
const userCacheKey = `user_data.${dcId}#${userId}`;
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

function postgresWrite(query: string): void {
  execFileSync('docker', [
    'exec', 'teamgram-postgres', 'psql', '-v', 'ON_ERROR_STOP=1', '-At', '-q', '-F', '\t', '-U', 'teamgram', '-d', 'teamgram', '-c', query,
  ], { stdio: ['ignore', 'ignore', 'ignore'] });
}

function readPersonalChannel(): string {
  return postgres(`SELECT personal_channel_id FROM users WHERE id=${userId} AND deleted=0 LIMIT 1`);
}

type CacheSnapshot = { present: boolean; value: string };
function cacheSnapshot(): CacheSnapshot {
  const present = execFileSync('docker', ['exec', 'redis', 'redis-cli', 'EXISTS', userCacheKey], { encoding: 'utf8' }).trim() === '1';
  const raw = present ? execFileSync('docker', ['exec', 'redis', 'redis-cli', '--raw', 'GET', userCacheKey], { encoding: 'utf8' }) : '';
  return { present, value: raw.endsWith('\n') ? raw.slice(0, -1) : raw };
}

function restoreCache(snapshot: CacheSnapshot): void {
  if (snapshot.present) {
    execFileSync('docker', ['exec', 'redis', 'redis-cli', 'SET', userCacheKey, snapshot.value], { stdio: 'ignore' });
  } else {
    execFileSync('docker', ['exec', 'redis', 'redis-cli', 'DEL', userCacheKey], { stdio: 'ignore' });
  }
}

function loadAuthKey(): string {
  const body = postgres(`
    SELECT k.body FROM auth_users u
    JOIN auth_keys k USING (auth_key_id)
    JOIN auth_key_infos i USING (auth_key_id)
    WHERE u.user_id=${userId} AND u.state=0 AND u.deleted=0
      AND k.deleted=0 AND i.deleted=0 AND i.auth_key_type=0
    ORDER BY u.date_active DESC, u.id DESC LIMIT 1
  `);
  const key = Buffer.from(body, 'base64');
  if (key.length !== 256) throw new Error('active production auth key is absent or invalid');
  return key.toString('hex');
}

function ownedChannelWithMessages(): { id: string; accessHash: string; messageId: string; messageHash: string; messageCount: number } {
  const row = postgres(`
    SELECT c.id, c.access_hash,
      (SELECT m.message_id FROM apifull_channel_message m
       WHERE m.channel_id=c.id AND NOT EXISTS (
         SELECT 1 FROM apifull_channel_message_hidden h
         WHERE h.user_id=${userId} AND h.channel_id=m.channel_id AND h.message_id=m.message_id)
       ORDER BY m.message_id DESC LIMIT 1),
      (SELECT encode(digest(m.message::text, 'sha256'), 'hex') FROM apifull_channel_message m
       WHERE m.channel_id=c.id AND NOT EXISTS (
         SELECT 1 FROM apifull_channel_message_hidden h
         WHERE h.user_id=${userId} AND h.channel_id=m.channel_id AND h.message_id=m.message_id)
       ORDER BY m.message_id DESC LIMIT 1),
      (SELECT COUNT(*) FROM apifull_channel_message m WHERE m.channel_id=c.id)
    FROM apifull_channel c
    WHERE c.creator_user_id=${userId}
      AND EXISTS (SELECT 1 FROM apifull_channel_message m WHERE m.channel_id=c.id)
    ORDER BY c.created_at DESC, c.id DESC LIMIT 1
  `);
  const [id, accessHash, messageId, messageHash, messageCount] = row.split('\t');
  if (!id || !accessHash || !messageId || !messageHash || Number(messageCount) <= 0) {
    throw new Error('no owned production APIFull channel with a visible message is available');
  }
  return { id, accessHash, messageId, messageHash, messageCount: Number(messageCount) };
}

function sha256(value: string): string {
  return createHash('sha256').update(value).digest('hex');
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

function boolTrue(value: any): boolean {
  return value === true || value?.value === true || value?.value === 1 || rpcName(value).toLowerCase().includes('true');
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

class GetPersonalChannelHistoryRequest {
  CONSTRUCTOR_ID = 0x55fb0996;
  SUBCLASS_OF_ID = 0;
  className = 'messages.GetPersonalChannelHistory';
  classType = 'request';

  constructor(private readonly user: any, private readonly limit: number) {}

  getBytes(): Uint8Array {
    const header = Buffer.alloc(4);
    header.writeUInt32LE(this.CONSTRUCTOR_ID, 0);
    const integers = Buffer.alloc(20);
    integers.writeInt32LE(this.limit, 0);
    integers.writeInt32LE(0, 4);
    integers.writeInt32LE(0, 8);
    integers.writeBigInt64LE(0n, 12);
    return Uint8Array.from(Buffer.concat([header, Buffer.from(this.user.getBytes()), integers]));
  }

  readResult(reader: any): any {
    return reader.tgReadObject();
  }
}

async function main() {
  const channel = ownedChannelWithMessages();
  const before = readPersonalChannel();
  if (before !== '0') throw new Error('selected account already has a personal channel; refusing to replace it');
  const beforeCache = cacheSnapshot();
  const client = makeClient(loadAuthKey());
  let connected = false;
  let setterAttempted = false;
  let primaryError: unknown;
  let cleanupError: unknown;
  let setterType = '';
  let historyType = '';
  let resultCount = 0;

  try {
    await client.connect();
    connected = true;
    const me = await client.getMe();
    if (String(me?.id) !== userId) throw new Error('production auth key user mismatch');

    setterAttempted = true;
    const updated = await client.invoke(new Api.account.UpdatePersonalChannel({
      channel: new Api.InputChannel({ channelId: BigInt(channel.id), accessHash: BigInt(channel.accessHash) }),
    }));
    setterType = rpcName(updated);
    if (!boolTrue(updated) || readPersonalChannel() !== channel.id) {
      throw new Error('account.updatePersonalChannel did not persist the selected owned channel');
    }

    const user = new Api.InputUserSelf();
    const request = typeof Api.messages.GetPersonalChannelHistory === 'function'
      ? new Api.messages.GetPersonalChannelHistory({ userId: user, limit: 100, maxId: 0, minId: 0, hash: 0n })
      : new GetPersonalChannelHistoryRequest(user, 100);
    const history = await client.invoke(request);
    historyType = rpcName(history);
    const matchingMessage = (history?.messages || []).find((message: any) =>
      String(message?.id) === channel.messageId && sha256(String(message?.message || '')) === channel.messageHash);
    const returnedChannel = Array.isArray(history?.chats)
      ? history.chats.find((chat: any) => String(chat?.id) === channel.id)
      : undefined;
    resultCount = Array.isArray(history?.messages) ? history.messages.length : 0;
    if (!historyType.toLowerCase().includes('messages') || !matchingMessage
      || rpcName(returnedChannel) !== 'Channel' || resultCount !== channel.messageCount) {
      throw new Error(`messages.getPersonalChannelHistory mismatch: type=${historyType}, count=${resultCount}`);
    }
  } catch (error) {
    primaryError = error;
  } finally {
    if (setterAttempted) {
      try {
        const current = readPersonalChannel();
        if (current === channel.id) {
          postgresWrite(`UPDATE users SET personal_channel_id=${before} WHERE id=${userId} AND personal_channel_id=${channel.id}`);
        } else if (current !== before) {
          throw new Error('personal channel changed to an unexpected value; refusing to overwrite it');
        }
        if (readPersonalChannel() !== before) throw new Error('personal_channel_id did not restore to its original value');
        restoreCache(beforeCache);
        const cache = cacheSnapshot();
        if (cache.present !== beforeCache.present || cache.value !== beforeCache.value) {
          throw new Error('Redis user cache did not restore to its original snapshot');
        }
      } catch (error) {
        cleanupError = error;
      }
    }
    client.destroy();
  }

  if (cleanupError) throw new Error(`personal channel cleanup failed: ${String(cleanupError)}`);
  if (primaryError) throw primaryError;
  if (!connected || !setterAttempted || readPersonalChannel() !== before) {
    throw new Error('personal channel history probe did not restore the original relation');
  }
  console.log(JSON.stringify({
    transport: `DC${dcId} WebSocket -> gateway -> session -> UserChannelProfiles + APIFull BFF/PostgreSQL`,
    updatePersonalChannel: setterType,
    getPersonalChannelHistory: historyType,
    visibleMessagesRead: resultCount,
    newestVisibleMessageMatchesPostgreSQL: true,
    personalChannelRestored: true,
    userCacheRestored: true,
    channelWrites: 0,
  }));
}

main().then(() => process.exit(0)).catch((error) => {
  console.error(`personal channel history production probe failed: ${error instanceof Error ? error.stack || error.message : String(error)}`);
  process.exit(1);
});
