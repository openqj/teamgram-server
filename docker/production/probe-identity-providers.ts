import path from 'node:path';
import { createRequire } from 'node:module';

for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) {
  delete process.env[key];
}

for (const key of ['TEAMGRAM_GRAMJS_DIR', 'AUTH_B64', 'IDENTITY_WEB_TOKEN', 'IDENTITY_USER_ID', 'IDENTITY_BOT_ID', 'IDENTITY_BOT_ACCESS_HASH']) {
  if (!process.env[key]) throw new Error(`${key} is required`);
}

const gramjsDir = process.env.TEAMGRAM_GRAMJS_DIR!;
const dcId = Number(process.env.TG_DC_ID || '2');
const dcHost = process.env.TG_DC_HOST || '127.0.0.1';
const dcPort = Number(process.env.TG_DC_PORT || '11443');
const apiId = Number(process.env.TG_API_ID || '1');
const apiHash = process.env.TG_API_HASH || '0123456789abcdef0123456789abcdef';
const userId = BigInt(process.env.IDENTITY_USER_ID!);
const botId = BigInt(process.env.IDENTITY_BOT_ID!);
const botAccessHash = BigInt(process.env.IDENTITY_BOT_ACCESS_HASH!);

const globalAny = globalThis as typeof globalThis & { self?: any; addEventListener?: () => void };
globalAny.self ??= globalThis;
globalAny.addEventListener ??= () => {};
globalAny.self.addEventListener ??= globalAny.addEventListener;

const requireModule = createRequire(import.meta.url);
const TelegramClient = requireModule(path.join(gramjsDir, 'src/lib/gramjs/client/TelegramClient.ts')).default;
const CallbackSession = requireModule(path.join(gramjsDir, 'src/lib/gramjs/sessions/CallbackSession.ts')).default;
const MemorySession = requireModule(path.join(gramjsDir, 'src/lib/gramjs/sessions/Memory.ts')).default;
const Api = requireModule(path.join(gramjsDir, 'src/lib/gramjs/tl/index.ts')).Api;
const Connection = requireModule(path.join(gramjsDir, 'src/lib/gramjs/network/connection/TCPObfuscated.ts')).ConnectionTCPObfuscated;
const quietLogger = { debug() {}, info() {}, warn() {}, error() {} };

function rpcName(value: any): string {
  return value?.className || value?.constructor?.name || typeof value;
}

function isBoolTrue(value: any): boolean {
  return value === true || rpcName(value).toLowerCase().includes('booltrue');
}

function concatBytes(...parts: Uint8Array[]): Uint8Array {
  const out = new Uint8Array(parts.reduce((sum, part) => sum + part.length, 0));
  let offset = 0;
  for (const part of parts) {
    out.set(part, offset);
    offset += part.length;
  }
  return out;
}

function tlInt(value: number): Uint8Array {
  const out = new Uint8Array(4);
  new DataView(out.buffer).setInt32(0, value, true);
  return out;
}

function tlLong(value: bigint): Uint8Array {
  const out = new Uint8Array(8);
  new DataView(out.buffer).setBigInt64(0, BigInt.asIntN(64, value), true);
  return out;
}

function tlString(value: string): Uint8Array {
  const payload = new TextEncoder().encode(value);
  const headerLength = payload.length < 254 ? 1 : 4;
  const out = new Uint8Array((headerLength + payload.length + 3) & ~3);
  if (headerLength === 1) {
    out[0] = payload.length;
  } else {
    out[0] = 254;
    out[1] = payload.length & 0xff;
    out[2] = (payload.length >>> 8) & 0xff;
    out[3] = (payload.length >>> 16) & 0xff;
  }
  out.set(payload, headerLength);
  return out;
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

function confirmBotConnectionRequest(bot: any): RawRequest {
  return new RawRequest(
    0x67ed1f68,
    bot.getBytes(),
    'account.confirmBotConnection',
  );
}

function acceptAuthorizationRequest(botId: bigint, scope: string, credentials: any): RawRequest {
  const emptyHashes = new Uint8Array([0x15, 0xc4, 0xb5, 0x1c, 0, 0, 0, 0]);
  return new RawRequest(
    0xf3ed4c73,
    concatBytes(tlLong(botId), tlString(scope), tlString('identity-fixture-public-key'), emptyHashes, credentials.getBytes()),
    'account.acceptAuthorization',
  );
}

function importWebTokenAuthorizationRequest(token: string): RawRequest {
  return new RawRequest(
    0x2db873a9,
    concatBytes(tlInt(apiId), tlString(apiHash), tlString(token)),
    'auth.importWebTokenAuthorization',
  );
}

function makePermanentClient(authKeyBase64: string): any {
  const authKeyHex = Buffer.from(authKeyBase64, 'base64').toString('hex');
  const session = new CallbackSession({ mainDcId: dcId, keys: { [dcId]: authKeyHex } }, () => {});
  const load = session.load.bind(session);
  session.load = async () => {
    await load();
    (session as any)._serverAddress = dcHost;
    (session as any)._port = dcPort;
  };
  const client = new TelegramClient(session, apiId, apiHash, {
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

function makeUnboundClient(): any {
  const session = new MemorySession();
  session.setDC(dcId, dcHost, dcPort);
  const client = new TelegramClient(session, apiId, apiHash, {
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
  const userClient = makePermanentClient(process.env.AUTH_B64!);
  const unboundClient = makeUnboundClient();
  const result: Record<string, unknown> = {};
  try {
    await userClient.connect();
    const me = await userClient.getMe();
    if (String(me?.id) !== userId.toString()) throw new Error(`authenticated user mismatch: ${String(me?.id)}`);

    const confirmed = await userClient.invoke(confirmBotConnectionRequest(
      new Api.InputUser({ userId: botId, accessHash: botAccessHash }),
    ));
    if (!isBoolTrue(confirmed)) throw new Error(`account.confirmBotConnection returned ${rpcName(confirmed)}`);
    result.confirmBotConnection = rpcName(confirmed);

    const scope = JSON.stringify({ v: 1, nonce: 'identity-fixture', data: [] });
    const accepted = await userClient.invoke(acceptAuthorizationRequest(
      botId,
      scope,
      new Api.SecureCredentialsEncrypted({
        data: Buffer.from('identity-fixture-credentials'),
        hash: Buffer.from('identity-fixture-hash'),
        secret: Buffer.from('identity-fixture-secret'),
      }),
    ));
    if (!isBoolTrue(accepted)) throw new Error(`account.acceptAuthorization returned ${rpcName(accepted)}`);
    result.acceptAuthorization = rpcName(accepted);

    await unboundClient.connect();
    const authorization = await unboundClient.invoke(importWebTokenAuthorizationRequest(
      process.env.IDENTITY_WEB_TOKEN!,
    ));
    if (!rpcName(authorization).toLowerCase().includes('authorization')) {
      throw new Error(`auth.importWebTokenAuthorization returned ${rpcName(authorization)}`);
    }
    const authorizedUser = authorization?.user;
    if (String(authorizedUser?.id) !== userId.toString()) throw new Error(`web-token user mismatch: ${String(authorizedUser?.id)}`);
    result.importWebTokenAuthorization = { constructor: rpcName(authorization), userId: String(authorizedUser.id) };
  } finally {
    userClient.destroy();
    unboundClient.destroy();
  }
  console.log(JSON.stringify({
    transport: `DC${dcId} TCPObfuscated -> gateway -> session -> authorization/account/passport BFF`,
    userId: userId.toString(),
    botId: botId.toString(),
    result,
  }));
}

main().then(() => process.exit(0)).catch((error) => {
  console.error(`identity provider probe failed: ${error instanceof Error ? error.stack || error.message : String(error)}`);
  process.exit(1);
});
