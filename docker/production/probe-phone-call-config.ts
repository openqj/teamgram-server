import { createRequire } from 'node:module';
import path from 'node:path';

for (const key of ['TEAMGRAM_GRAMJS_DIR', 'AUTH_B64', 'USER_ID']) {
  if (!process.env[key]) throw new Error(`${key} is required`);
}

for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) {
  delete process.env[key];
}

const root = process.env.TEAMGRAM_GRAMJS_DIR!;
const dcId = Number(process.env.TG_DC_ID || '2');
const dcHost = process.env.TG_DC_HOST || '127.0.0.1';
const dcPort = Number(process.env.TG_DC_PORT || '11443');
const userId = BigInt(process.env.USER_ID!);

const globalAny = globalThis as typeof globalThis & { self?: any; addEventListener?: () => void };
globalAny.self ??= globalThis;
globalAny.addEventListener ??= () => {};
globalAny.self.addEventListener ??= globalAny.addEventListener;

const requireModule = createRequire(import.meta.url);
const TelegramClient = requireModule(path.join(root, 'src/lib/gramjs/client/TelegramClient.ts')).default;
const CallbackSession = requireModule(path.join(root, 'src/lib/gramjs/sessions/CallbackSession.ts')).default;
const Api = requireModule(path.join(root, 'src/lib/gramjs/tl/index.ts')).Api;
const Connection = requireModule(path.join(root, 'src/lib/gramjs/network/connection/TCPObfuscated.ts')).ConnectionTCPObfuscated;
const quietLogger = { debug() {}, info() {}, warn() {}, error() {} };

function makeClient(): any {
  const keyHex = Buffer.from(process.env.AUTH_B64!, 'base64').toString('hex');
  const session = new CallbackSession({ mainDcId: dcId, keys: { [dcId]: keyHex } }, () => {});
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

function rpcName(value: any): string {
  return value?.className || value?.constructor?.name || typeof value;
}

async function main() {
  const client = makeClient();
  try {
    await client.connect();
    const me = await client.getMe();
    if (String(me?.id) !== userId.toString()) throw new Error(`authenticated user mismatch: ${String(me?.id)}`);

    const result = await client.invoke(new Api.phone.GetCallConfig());
    if (rpcName(result) !== 'DataJSON') throw new Error(`unexpected result: ${rpcName(result)}`);
    const payload = JSON.parse(String(result.data || ''));
    const turn = payload?.turn;
    const ip = String(turn?.ip || '');
    const port = Number(turn?.port || 0);
    const username = String(turn?.username || '');
    const password = String(turn?.password || '');
    if (!ip || ip === '127.0.0.1' || ip === '::1' || ip === 'localhost') throw new Error(`invalid TURN host: ${ip}`);
    if (!Number.isInteger(port) || port < 1 || port > 65535) throw new Error(`invalid TURN port: ${port}`);
    if (!username || !password) throw new Error('TURN credentials are empty');
    console.log(JSON.stringify({
      transport: `DC${dcId} TCPObfuscated -> gateway -> session -> APIFull`,
      userId: userId.toString(),
      constructor: rpcName(result),
      turn: { ip, port, usernamePrefix: username.split(':')[0], passwordBytes: Buffer.byteLength(password) },
      audioFrameSize: payload.audio_frame_size,
      jitter: [payload.jitter_min_delay_60, payload.jitter_max_delay_60],
    }));
  } finally {
    try {
      await client.disconnect();
    } catch {
      // The session is read-only; a clean transport close is best effort.
    }
    client.destroy();
  }
}

main()
  .then(() => process.exit(0))
  .catch((error) => {
    console.error(`phone.getCallConfig probe failed: ${error instanceof Error ? error.stack || error.message : String(error)}`);
    process.exit(1);
  });
