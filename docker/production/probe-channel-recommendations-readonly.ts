import path from 'node:path';
import { createRequire } from 'node:module';

for (const key of ['TEAMGRAM_GRAMJS_DIR', 'AUTH_B64', 'USER_ID', 'CHANNEL_ID', 'CHANNEL_ACCESS_HASH']) {
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

function summarize(value: any) {
  return {
    type: rpcName(value),
    chats: (value?.chats || []).map((chat: any) => ({
      type: rpcName(chat),
      id: String(chat?.id ?? ''),
      title: chat?.title || '',
      accessHash: chat?.accessHash === undefined ? '' : String(chat.accessHash),
    })),
  };
}

async function main() {
  const client = makeClient();
  try {
    (client as any)._borrowExportedSender = async () => undefined;
    (client as any)._updateLoop = async () => undefined;
    await client.connect();
    const me = await client.getMe();
    if (String(me?.id) !== String(process.env.USER_ID)) throw new Error('auth key user mismatch');
    const empty = await client.invoke(new Api.channels.GetChannelRecommendations({}));
    const excluded = await client.invoke(new Api.channels.GetChannelRecommendations({
      channel: new Api.InputChannel({
        channelId: BigInt(process.env.CHANNEL_ID!),
        accessHash: BigInt(process.env.CHANNEL_ACCESS_HASH!),
      }),
    }));
    const emptySummary = summarize(empty);
    const excludedSummary = summarize(excluded);
    if (emptySummary.type !== 'messages.Chats' || excludedSummary.type !== 'messages.Chats') {
      throw new Error(`unexpected result types: ${emptySummary.type}, ${excludedSummary.type}`);
    }
    if (emptySummary.chats.length === 0 || excludedSummary.chats.length === 0) {
      throw new Error('expected non-empty recommendations from canonical production channels');
    }
    if (excludedSummary.chats.some((chat: any) => chat.id === String(process.env.CHANNEL_ID))) {
      throw new Error('excluded channel was returned');
    }
    console.log(JSON.stringify({ userId: String(me.id), empty: emptySummary, excluded: excludedSummary }));
  } finally {
    client.destroy();
  }
}

main().then(() => process.exit(0)).catch((error) => {
  console.error(`channel recommendations probe failed: ${error instanceof Error ? error.stack || error.message : String(error)}`);
  process.exit(1);
});
