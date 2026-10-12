import { createRequire } from 'node:module';
import path from 'node:path';

for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) {
  delete process.env[key];
}

for (const key of ['TEAMGRAM_GRAMJS_DIR', 'AUTH_B64', 'USER_ID']) {
  if (!process.env[key]) throw new Error(`${key} is required`);
}

const gramjsDir = process.env.TEAMGRAM_GRAMJS_DIR!;
const dcId = Number(process.env.TG_DC_ID || '2');
const dcHost = process.env.TG_DC_HOST || '127.0.0.1';
const dcPort = Number(process.env.TG_DC_PORT || '11443');
const userId = BigInt(process.env.USER_ID!);
const collectibleUsername = process.env.COLLECTIBLE_USERNAME || 'probe_collectible';
const callbackMessageId = Number(process.env.CALLBACK_MESSAGE_ID || '17');
const callbackData = process.env.CALLBACK_DATA || 'approve';
const liveRandomId = BigInt(process.env.LIVE_RANDOM_ID || '990001');

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

function rpcName(value: any): string {
  return value?.className || value?.constructor?.name || typeof value;
}

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

function concatBytes(...parts: Uint8Array[]): Uint8Array {
  const output = new Uint8Array(parts.reduce((size, part) => size + part.length, 0));
  let offset = 0;
  for (const part of parts) {
    output.set(part, offset);
    offset += part.length;
  }
  return output;
}

function tlInt(value: number): Uint8Array {
  const output = new Uint8Array(4);
  new DataView(output.buffer).setInt32(0, value, true);
  return output;
}

function tlLong(value: bigint): Uint8Array {
  const output = new Uint8Array(8);
  new DataView(output.buffer).setBigInt64(0, BigInt.asIntN(64, value), true);
  return output;
}

class RawRequest {
  SUBCLASS_OF_ID = 0;
  classType = 'request';

  constructor(
    public readonly CONSTRUCTOR_ID: number,
    private readonly payload: Uint8Array,
    public readonly className: string,
  ) {}

  getBytes(): Uint8Array {
    return concatBytes(tlInt(this.CONSTRUCTOR_ID), this.payload);
  }

  readResult(reader: any): any {
    return reader.tgReadObject();
  }
}

function startLiveRequest(api: any): RawRequest {
  const emptyVector = new Uint8Array([0x15, 0xc4, 0xb5, 0x1c, 0, 0, 0, 0]);
  return new RawRequest(
    0xd069ccde,
    concatBytes(tlInt(0), new api.InputPeerSelf().getBytes(), emptyVector, tlLong(liveRandomId)),
    'stories.startLive',
  );
}

async function invoke(client: any, name: string, request: any, results: Record<string, unknown>) {
  try {
    const result = await client.invoke(request);
    results[name] = { constructor: rpcName(result), value: result?.value, date: result?.date, seq: result?.seq };
  } catch (error) {
    results[name] = { error: error instanceof Error ? error.message : String(error) };
  }
}

async function main() {
  const client = makeClient();
  const results: Record<string, unknown> = {};
  try {
    await client.connect();
    const me = await client.getMe();
    if (String(me?.id) !== userId.toString()) throw new Error(`authenticated user mismatch: ${String(me?.id)}`);

    await invoke(client, 'fragment.getCollectibleInfo', new Api.fragment.GetCollectibleInfo({
      collectible: new Api.InputCollectibleUsername({ username: collectibleUsername }),
    }), results);
    await invoke(client, 'ephemeral.getCallbackAnswer', new Api.ephemeral.GetCallbackAnswer({
      peer: new Api.InputPeerSelf(), id: callbackMessageId, data: Buffer.from(callbackData),
    }), results);
    await invoke(client, 'stories.startLive', startLiveRequest(Api), results);
  } finally {
    client.destroy();
  }

  const failures = Object.values(results).filter((value: any) => value?.error);
  console.log(JSON.stringify({
    transport: `DC${dcId} TCPObfuscated -> gateway -> session -> APIFull/provider fixtures`,
    userId: userId.toString(),
    results,
  }));
  if (failures.length > 0) process.exitCode = 1;
}

main().catch((error) => {
  console.error(`business provider probe failed: ${error instanceof Error ? error.stack || error.message : String(error)}`);
  process.exitCode = 1;
});
