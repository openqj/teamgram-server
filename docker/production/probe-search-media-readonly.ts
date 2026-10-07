import { execFileSync } from 'node:child_process';
import path from 'node:path';
import { createRequire } from 'node:module';

const userId = '136907714';
const channelId = '3';
const gramjsDir = process.env.TEAMGRAM_GRAMJS_DIR || '/Users/mac/open/test/telegram-tt-master';
const dcId = Number(process.env.TG_DC_ID || '2');
const dcHost = process.env.TG_DC_HOST || '127.0.0.1';
const dcPort = Number(process.env.TG_DC_PORT || '11443');

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

function mysql(query: string): string {
  return execFileSync('docker', [
    'exec', 'mysql', 'mysql', '-N', '-s', '-uteamgram', '-pteamgram', '-Dteamgram', '-e', query,
  ], { encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'] }).trim();
}

function requiredValue(query: string, name: string): string {
  const value = mysql(query);
  if (!value) throw new Error(`${name} is absent from production MySQL`);
  return value;
}

function rpcName(value: any): string {
  return value?.className || value?.constructor?.name || typeof value;
}

function isType(value: any, name: string): boolean {
  const actual = rpcName(value);
  return actual === name || actual.endsWith(`.${name}`);
}

function intBytes(value: number): Uint8Array {
  const bytes = new Uint8Array(4);
  new DataView(bytes.buffer).setInt32(0, value, true);
  return bytes;
}

function tlString(value: string): Uint8Array {
  const body = Buffer.from(value, 'utf8');
  const prefixLength = body.length < 254 ? 1 : 4;
  if (body.length >= 254) throw new Error('probe TL string is unexpectedly large');
  const paddedLength = (prefixLength + body.length + 3) & ~3;
  const bytes = new Uint8Array(paddedLength);
  bytes[0] = body.length;
  bytes.set(body, 1);
  return bytes;
}

function rawRequest(
  constructorId: number,
  className: string,
  parts: Uint8Array[],
  readResult: (reader: any) => any = (reader) => reader.tgReadObject(),
): any {
  return {
    CONSTRUCTOR_ID: constructorId,
    SUBCLASS_OF_ID: 0,
    className,
    classType: 'request',
    getBytes: () => Uint8Array.from(Buffer.concat([
      Buffer.from(intBytes(constructorId)),
      ...parts.map((part) => Buffer.from(part)),
    ])),
    readResult,
  };
}

function makeClient(authKeyB64: string): any {
  const session = new CallbackSession({
    mainDcId: dcId,
    keys: { [dcId]: Buffer.from(authKeyB64, 'base64').toString('hex') },
  }, () => {});
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
  const authKeyB64 = requiredValue(`
    SELECT k.body
    FROM auth_users u
    JOIN auth_keys k USING (auth_key_id)
    JOIN auth_key_infos i USING (auth_key_id)
    WHERE u.user_id=${userId} AND u.state=0 AND u.deleted=0
      AND k.deleted=0 AND i.deleted=0 AND i.auth_key_type=0
    ORDER BY u.date_active DESC,u.id DESC
    LIMIT 1
  `, 'active production auth key');
  const channelAccessHash = requiredValue(
    `SELECT access_hash FROM channels WHERE id=${channelId} AND deleted=0 LIMIT 1`,
    'production channel access hash',
  );
  const expectedCounter = Number(mysql(`
    SELECT COUNT(*) FROM messages
    WHERE user_id=${userId} AND dialog_id1=-4 AND dialog_id2=${channelId}
      AND message_filter_type=7 AND deleted=0
  `));
  const expectedPositions = Number(mysql(`
    SELECT COUNT(*) FROM messages
    WHERE user_id=${userId} AND dialog_id1=${userId} AND dialog_id2=${userId}
      AND message_filter_type IN (0,7,8) AND deleted=0
  `));
  const expectedSent = Number(mysql(`
    SELECT COUNT(*) FROM messages
    WHERE user_id=${userId} AND sender_user_id=${userId}
      AND message_filter_type=7 AND deleted=0
  `));
  if (![expectedCounter, expectedPositions, expectedSent].every(Number.isSafeInteger)) {
    throw new Error('production media count query returned invalid values');
  }

  const client = makeClient(authKeyB64);
  try {
    await client.connect();
    const me = await client.getMe();
    if (String(me?.id) !== userId) throw new Error('auth key user mismatch');

    const photoFilter = new Api.InputMessagesFilterPhotos();
    const channel = new Api.InputPeerChannel({ channelId: BigInt(channelId), accessHash: BigInt(channelAccessHash) });
    const self = new Api.InputPeerSelf();
    const counters = await client.invoke(rawRequest(0x1bbcf300, 'messages.GetSearchCounters', [
      intBytes(0),
      channel.getBytes(),
      intBytes(0x1cb5c415),
      intBytes(1),
      photoFilter.getBytes(),
    ], (reader) => reader.tgReadVector()));
    const counter = Array.isArray(counters) ? counters[0] : undefined;
    if (!Array.isArray(counters) || counters.length !== 1 || !isType(counter, 'SearchCounter')
      || Number(counter.count) !== expectedCounter || Number(counter.count) <= 0
      || rpcName(counter.filter) !== rpcName(photoFilter)) {
      throw new Error(`getSearchCounters malformed or mismatched: type=${rpcName(counters)}, itemType=${rpcName(counter)}, filterType=${rpcName(counter?.filter)}, count=${String(counter?.count)}, db=${expectedCounter}`);
    }

    const positions = await client.invoke(rawRequest(0x9c7f2f10, 'messages.GetSearchResultsPositions', [
      intBytes(0),
      self.getBytes(),
      photoFilter.getBytes(),
      intBytes(0),
      intBytes(10),
    ]));
    if (!isType(positions, 'SearchResultsPositions')
      || Number(positions.count) !== expectedPositions
      || !Array.isArray(positions.positions)
      || positions.positions.length !== Math.min(expectedPositions, 10)) {
      throw new Error(`getSearchResultsPositions malformed or mismatched: type=${rpcName(positions)}, count=${String(positions?.count)}, positions=${positions?.positions?.length}, db=${expectedPositions}`);
    }

    const sentMedia = await client.invoke(rawRequest(0x107e31a0, 'messages.SearchSentMedia', [
      tlString(''),
      photoFilter.getBytes(),
      intBytes(50),
    ]));
    const sentMessages = Array.isArray(sentMedia?.messages) ? sentMedia.messages : [];
    if (!isType(sentMedia, 'Messages') || sentMessages.length !== Math.min(expectedSent, 50)) {
      throw new Error(`searchSentMedia malformed or mismatched: type=${rpcName(sentMedia)}, messages=${sentMessages.length}, db=${expectedSent}`);
    }

    console.log(JSON.stringify({
      userId: String(me.id),
      transport: `DC${dcId} WebSocket -> gateway -> session -> BFF`,
      getSearchCounters: {
        type: rpcName(counters),
        itemType: rpcName(counter),
        filter: rpcName(counter.filter),
        count: Number(counter.count),
        mysqlCount: expectedCounter,
      },
      getSearchResultsPositions: {
        type: rpcName(positions),
        count: Number(positions.count),
        positions: positions.positions.length,
        mysqlCount: expectedPositions,
      },
      searchSentMedia: {
        type: rpcName(sentMedia),
        messages: sentMessages.length,
        mysqlCount: expectedSent,
        messageIds: sentMessages.map((item: any) => Number(item?.id)),
      },
      writes: 0,
    }));
  } finally {
    client.destroy();
  }
}

main().then(() => process.exit(0)).catch((error) => {
  console.error(`search media production probe failed: ${error instanceof Error ? error.stack || error.message : String(error)}`);
  process.exit(1);
});
