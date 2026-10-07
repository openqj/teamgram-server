import path from 'node:path';
import { createRequire } from 'node:module';

for (const key of ['TEAMGRAM_GRAMJS_DIR', 'AUTH_B64', 'USER_ID', 'PEER_ID', 'PEER_ACCESS_HASH']) {
  if (!process.env[key]) throw new Error(`${key} is required`);
}

const root = process.env.TEAMGRAM_GRAMJS_DIR!;
const dcId = Number(process.env.TG_DC_ID || '2');
const requireModule = createRequire(import.meta.url);
const TelegramClient = requireModule(path.join(root, 'src/lib/gramjs/client/TelegramClient.ts')).default;
const CallbackSession = requireModule(path.join(root, 'src/lib/gramjs/sessions/CallbackSession.ts')).default;
const Api = requireModule(path.join(root, 'src/lib/gramjs/tl/index.ts')).Api;
const Connection = requireModule(path.join(root, 'src/lib/gramjs/network/connection/TCPObfuscated.ts')).ConnectionTCPObfuscated;
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
    baseLogger: { debug() {}, info() {}, warn() {}, error() {} },
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
    const result = await client.invoke(new Api.messages.GetPeerSettings({
      peer: new Api.InputPeerUser({
        userId: BigInt(process.env.PEER_ID!),
        accessHash: BigInt(process.env.PEER_ACCESS_HASH!),
      }),
    }));
    const type = rpcName(result);
    if (type !== 'PeerSettings' && type !== 'messages.PeerSettings') {
      throw new Error(`unexpected messages.getPeerSettings result: ${type}`);
    }
    const users = Array.isArray(result?.users) ? result.users : [];
    if (users.length !== 1 || String(users[0]?.id) !== String(process.env.PEER_ID)) {
      throw new Error(`unexpected hydrated users: ${users.length}`);
    }
    console.log(JSON.stringify({
      userId: String(me.id),
      resultType: type,
      settingsType: rpcName(result.settings),
      userCount: users.length,
      peerId: String(users[0].id),
      chats: Array.isArray(result?.chats) ? result.chats.length : 0,
      writes: 0,
    }));
  } finally {
    client.destroy();
  }
}

main().then(() => process.exit(0)).catch((error) => {
  console.error(`peer settings probe failed: ${error instanceof Error ? error.stack || error.message : String(error)}`);
  process.exit(1);
});
