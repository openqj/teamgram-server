import path from 'node:path';
import { createRequire } from 'node:module';

for (const key of ['TEAMGRAM_GRAMJS_DIR', 'AUTH_B64', 'USER_ID']) {
  if (!process.env[key]) throw new Error(`${key} is required`);
}

const root = process.env.TEAMGRAM_GRAMJS_DIR!;
const dcId = Number(process.env.TG_DC_ID || '2');
const requireModule = createRequire(import.meta.url);
const TelegramClient = requireModule(path.join(root, 'src/lib/gramjs/client/TelegramClient.ts')).default;
const CallbackSession = requireModule(path.join(root, 'src/lib/gramjs/sessions/CallbackSession.ts')).default;
const Api = requireModule(path.join(root, 'src/lib/gramjs/tl/index.ts')).Api;
const Connection = requireModule(path.join(root, 'src/lib/gramjs/network/connection/TCPObfuscated.ts')).ConnectionTCPObfuscated;
const quietLogger = { debug() {}, info() {}, warn() {}, error() {} };
const globalAny = globalThis as typeof globalThis & { self?: any; addEventListener?: () => void };
globalAny.self ??= globalThis;
globalAny.addEventListener ??= () => {};
globalAny.self.addEventListener ??= globalAny.addEventListener;

const rpcName = (value: any): string => value?.className || value?.constructor?.name || typeof value;

function summarize(value: any): Record<string, unknown> {
  const summary: Record<string, unknown> = { type: rpcName(value) };
  if (!value || typeof value !== 'object') return summary;

  for (const [key, item] of Object.entries(value)) {
    if (Array.isArray(item)) {
      summary[`${key}Count`] = item.length;
    } else if (item && typeof item === 'object') {
      const counts = Object.fromEntries(
        Object.entries(item)
          .filter(([, nested]) => Array.isArray(nested))
          .map(([nestedKey, nested]) => [nestedKey, (nested as unknown[]).length]),
      );
      if (Object.keys(counts).length) summary[`${key}ArrayCounts`] = counts;
    } else if (['hash', 'count', 'totalCount', 'total_count'].includes(key) && item !== undefined) {
      summary[key] = String(item);
    }
  }

  return summary;
}

function tlInt(value: number): Uint8Array {
  const bytes = new Uint8Array(4);
  new DataView(bytes.buffer).setInt32(0, value, true);
  return bytes;
}

function tlLong(value: bigint): Uint8Array {
  const bytes = new Uint8Array(8);
  new DataView(bytes.buffer).setBigInt64(0, value, true);
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

function rawRequest(constructorId: number, className: string, ...parts: Uint8Array[]): any {
  return {
    CONSTRUCTOR_ID: constructorId,
    SUBCLASS_OF_ID: 0,
    className,
    classType: 'request',
    getBytes: () => Uint8Array.from(Buffer.concat([
      Buffer.from(tlInt(constructorId)),
      ...parts.map((part) => Buffer.from(part)),
    ])),
    readResult: (reader: any) => reader.tgReadObject(),
  };
}

function makeClient(): any {
  const session = new CallbackSession({
    mainDcId: dcId,
    keys: { [dcId]: Buffer.from(process.env.AUTH_B64!, 'base64').toString('hex') },
  }, () => {});
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
    autoReconnect: false,
    baseLogger: quietLogger,
  });
}

async function main() {
  const client = makeClient();
  try {
    (client as any)._borrowExportedSender = async () => undefined;
    (client as any)._updateLoop = async () => undefined;
    await client.connect();
    const me = await client.getMe();
    if (String(me?.id) !== String(process.env.USER_ID)) throw new Error('auth key user mismatch');
    const calls: Array<[string, () => Promise<any>]> = [
      ['help.getAppConfig', () => client.invoke(new Api.help.GetAppConfig({ hash: 0 }))],
      ['help.getUserInfo', () => client.invoke(rawRequest(
        0x38a08d3,
        'help.GetUserInfo',
        Buffer.from(new Api.InputUserSelf().getBytes()),
      ))],
      ['help.getPeerColors', () => client.invoke(new Api.help.GetPeerColors({ hash: 0 }))],
      ['help.getPeerProfileColors', () => client.invoke(new Api.help.GetPeerProfileColors({ hash: 0 }))],
      ['messages.getAttachMenuBots', () => client.invoke(new Api.messages.GetAttachMenuBots({ hash: 0n }))],
      ['messages.getPinnedSavedDialogs', () => client.invoke(new Api.messages.GetPinnedSavedDialogs({}))],
      ['messages.getSavedDialogs', () => client.invoke(new Api.messages.GetSavedDialogs({
        offsetDate: 0,
        offsetId: 0,
        offsetPeer: new Api.InputPeerEmpty(),
        limit: 50,
        hash: 0n,
      }))],
      ['messages.getSavedHistory', () => client.invoke(new Api.messages.GetSavedHistory({
        peer: new Api.InputPeerSelf(),
        offsetId: 0,
        offsetDate: 0,
        addOffset: 0,
        limit: 50,
        maxId: 0,
        minId: 0,
        hash: 0n,
      }))],
      ['messages.getSavedDialogsByID', () => client.invoke(rawRequest(
        0x6f6f9c96,
        'messages.GetSavedDialogsByID',
        Buffer.from(tlInt(0)),
        Buffer.from(tlInt(0x1cb5c415)),
        Buffer.from(tlInt(1)),
        Buffer.from(tlInt(0x7da07ec9)),
      ))],
    ];
    const result: Record<string, unknown> = {};
    for (const [method, request] of calls) {
      const value = await request();
      result[method] = summarize(value);
    }
    const recentStatuses = await client.invoke(rawRequest(
      0xf578105,
      'account.GetRecentEmojiStatuses',
      Buffer.from(tlLong(0n)),
    ));
    const statuses = Array.isArray(recentStatuses?.statuses) ? recentStatuses.statuses : [];
    if (!rpcName(recentStatuses).toLowerCase().endsWith('emojistatuses') || statuses.length !== 0) {
      throw new Error(`unexpected account.getRecentEmojiStatuses result: ${rpcName(recentStatuses)} statuses=${statuses.length}`);
    }
    const recentNotModified = await client.invoke(rawRequest(
      0xf578105,
      'account.GetRecentEmojiStatuses',
      Buffer.from(tlLong(BigInt(recentStatuses.hash || 0))),
    ));
    if (!rpcName(recentNotModified).toLowerCase().endsWith('emojistatusesnotmodified')) {
      throw new Error(`unexpected recent emoji hash response: ${rpcName(recentNotModified)}`);
    }
    result['account.getRecentEmojiStatuses'] = {
      type: rpcName(recentStatuses),
      hash: String(recentStatuses.hash),
      statuses: statuses.length,
      hashResponseType: rpcName(recentNotModified),
    };
    if (process.env.BOT_USERNAME) {
      const request = rawRequest(
        0x87f2219b,
        'bots.CheckUsername',
        Buffer.from(tlString(process.env.BOT_USERNAME)),
      );
      request.readResult = (reader: any) => reader.tgReadBool();
      const available = await client.invoke(request);
      if (typeof available !== 'boolean' || !available) {
        throw new Error(`unexpected bots.checkUsername result: ${String(available)}`);
      }
      result['bots.checkUsername'] = { type: 'Boolean', available };
    }
    if (process.env.CHANNEL_ID || process.env.CHANNEL_ACCESS_HASH || process.env.CHANNEL_USERNAME) {
      if (!process.env.CHANNEL_ID || !process.env.CHANNEL_ACCESS_HASH || !process.env.CHANNEL_USERNAME) {
        throw new Error('CHANNEL_ID, CHANNEL_ACCESS_HASH and CHANNEL_USERNAME must be provided together');
      }
      const channel = new Api.InputChannel({
        channelId: BigInt(process.env.CHANNEL_ID),
        accessHash: BigInt(process.env.CHANNEL_ACCESS_HASH),
      });
      const request = rawRequest(
        0x10e6bd2c,
        'channels.CheckUsername',
        Buffer.from(channel.getBytes()),
        Buffer.from(tlString(process.env.CHANNEL_USERNAME)),
      );
      request.readResult = (reader: any) => reader.tgReadBool();
      const available = await client.invoke(request);
      if (typeof available !== 'boolean' || !available) {
        throw new Error(`unexpected channels.checkUsername result: ${String(available)}`);
      }
      result['channels.checkUsername'] = { type: 'Boolean', available };
    }
    if (process.env.PEER_ID || process.env.PEER_ACCESS_HASH) {
      if (!process.env.PEER_ID || !process.env.PEER_ACCESS_HASH) {
        throw new Error('PEER_ID and PEER_ACCESS_HASH must be provided together');
      }
      const value = await client.invoke(new Api.messages.GetCommonChats({
        userId: new Api.InputUser({
          userId: BigInt(process.env.PEER_ID),
          accessHash: BigInt(process.env.PEER_ACCESS_HASH),
        }),
        maxId: 0n,
        limit: 100,
      }));
      const chats = Array.isArray(value?.chats) ? value.chats : [];
      if (!rpcName(value).toLowerCase().includes('chats') || chats.length !== 2) {
        throw new Error(`unexpected messages.getCommonChats result: ${rpcName(value)} chats=${chats.length}`);
      }
      result['messages.getCommonChats'] = summarize(value);
    }
    console.log(JSON.stringify({ userId: String(me.id), results: result }));
  } finally {
    client.destroy();
  }
}

main().then(() => process.exit(0)).catch((error) => {
  console.error(`readonly contract probe failed: ${error instanceof Error ? error.stack || error.message : String(error)}`);
  process.exit(1);
});
