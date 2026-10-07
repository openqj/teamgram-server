import path from 'node:path';
import { createRequire } from 'node:module';

for (const key of ['TEAMGRAM_GRAMJS_DIR', 'A_AUTH_B64', 'A_USER_ID', 'A_ACCESS_HASH', 'B_USER_ID', 'B_ACCESS_HASH']) {
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

function name(value: any): string {
  return value?.className || value?.constructor?.name || typeof value;
}

function makeClient(): any {
  const session = new CallbackSession({
    mainDcId: dcId,
    keys: { [dcId]: Buffer.from(process.env.A_AUTH_B64!, 'base64').toString('hex') },
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
    requestRetries: 2,
    connectionRetries: 2,
    connectionRetriesToFallback: 0,
    retryDelay: 250,
    autoReconnect: false,
    baseLogger: quietLogger,
  });
}

async function main() {
  const client = makeClient();
  try {
    await client.connect();
    const peer = new Api.InputPeerUser({
      userId: BigInt(process.env.B_USER_ID!),
      accessHash: BigInt(process.env.B_ACCESS_HASH!),
    });
    const sendAs = new Api.InputPeerUser({
      userId: BigInt(process.env.A_USER_ID!),
      accessHash: BigInt(process.env.A_ACCESS_HASH!),
    });
    const result = await client.invoke(new Api.messages.SaveDefaultSendAs({ peer, sendAs }));
    console.log(JSON.stringify({ result: name(result), peer: String(process.env.B_USER_ID), sendAs: String(process.env.A_USER_ID) }));
  } finally {
    client.destroy();
  }
}

main().then(() => process.exit(0)).catch((error) => {
  console.error(String(error));
  process.exit(1);
});
