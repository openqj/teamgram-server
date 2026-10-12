import { createHmac } from 'node:crypto';
import path from 'node:path';
import { createRequire } from 'node:module';

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
const apiId = Number(process.env.TG_API_ID || '1');
const apiHash = process.env.TG_API_HASH || '0123456789abcdef0123456789abcdef';
const userId = BigInt(process.env.USER_ID!);

const globalAny = globalThis as typeof globalThis & { self?: any; addEventListener?: () => void };
globalAny.self ??= globalThis;
globalAny.addEventListener ??= () => {};
globalAny.self.addEventListener ??= globalAny.addEventListener;

const requireModule = createRequire(import.meta.url);
const TelegramClient = requireModule(path.join(gramjsDir, 'src/lib/gramjs/client/TelegramClient.ts')).default;
const CallbackSession = requireModule(path.join(gramjsDir, 'src/lib/gramjs/sessions/CallbackSession.ts')).default;
const Api = requireModule(path.join(gramjsDir, 'src/lib/gramjs/tl/index.ts')).Api;
const serializeBytes = requireModule(path.join(gramjsDir, 'src/lib/gramjs/tl/index.ts')).serializeBytes;
const Connection = requireModule(path.join(gramjsDir, 'src/lib/gramjs/network/connection/TCPObfuscated.ts')).ConnectionTCPObfuscated;
const quietLogger = { debug() {}, info() {}, warn() {}, error() {} };

function rpcName(value: any): string {
  return value?.className || value?.constructor?.name || typeof value;
}

function tlUInt(value: number): Uint8Array {
  const result = new Uint8Array(4);
  new DataView(result.buffer).setUint32(0, value >>> 0, true);
  return result;
}

function tlInt(value: number): Uint8Array {
  const result = new Uint8Array(4);
  new DataView(result.buffer).setInt32(0, value | 0, true);
  return result;
}

function tlLong(value: bigint): Uint8Array {
  const result = new Uint8Array(8);
  new DataView(result.buffer).setBigInt64(0, BigInt.asIntN(64, value), true);
  return result;
}

function concatBytes(...parts: Uint8Array[]): Uint8Array {
  const result = new Uint8Array(parts.reduce((size, part) => size + part.length, 0));
  let offset = 0;
  for (const part of parts) {
    result.set(part, offset);
    offset += part.length;
  }
  return result;
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
    return concatBytes(tlUInt(this.CONSTRUCTOR_ID), this.payload);
  }

  readResult(reader: any): any {
    return reader.tgReadObject();
  }
}

function noArgs(constructorId: number, className: string): RawRequest {
  return new RawRequest(constructorId, new Uint8Array(), className);
}

function tokenRequest(constructorId: number, className: string, token: Uint8Array, offset?: bigint, limit?: number): RawRequest {
  const fields = [serializeBytes(token)];
  if (offset !== undefined) fields.push(tlLong(offset));
  if (limit !== undefined) fields.push(tlInt(limit));
  return new RawRequest(constructorId, concatBytes(...fields), className);
}

function reuploadRequest(token: Uint8Array, requestToken: Uint8Array): RawRequest {
  return new RawRequest(
    0x9b2754a8,
    concatBytes(serializeBytes(token), serializeBytes(requestToken)),
    'upload.reuploadCdnFile',
  );
}

function makeClient(): any {
  const authKeyHex = Buffer.from(process.env.AUTH_B64!, 'base64').toString('hex');
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

function bytes(value: any): Uint8Array {
  return value instanceof Uint8Array ? value : new Uint8Array(value || []);
}

function requestTokenFor(token: Uint8Array): Uint8Array {
  const signingKey = process.env.CDN_PROVIDER_SIGNING_KEY || 'acceptance-cdn-signing-key-at-least-32-bytes';
  return createHmac('sha256', signingKey).update(token).digest();
}

async function main() {
  const client = makeClient();
  const result: Record<string, unknown> = {};
  try {
    await client.connect();
    const me = await client.getMe();
    if (String(me?.id) !== userId.toString()) throw new Error(`authenticated user mismatch: ${String(me?.id)}`);

    const config = await client.invoke(noArgs(0x52029342, 'help.getCdnConfig'));
    result.helpGetCdnConfig = { constructor: rpcName(config), publicKeys: config?.publicKeys?.length ?? 0 };
    if (rpcName(config).toLowerCase() !== 'cdnconfig' || !config?.publicKeys?.length) throw new Error(`invalid CDN config: ${rpcName(config)}`);

    const location = new Api.InputDocumentFileLocation({
      id: BigInt(42), accessHash: BigInt(99), fileReference: Buffer.alloc(0), thumbSize: '',
    });
    const redirect = await client.invoke(new Api.upload.GetFile({
      cdnSupported: true, location, offset: BigInt(0), limit: 1024,
    }));
    result.uploadGetFile = {
      constructor: rpcName(redirect), dcId: redirect?.dcId,
      tokenBytes: redirect?.fileToken?.length, keyBytes: redirect?.encryptionKey?.length,
      ivBytes: redirect?.encryptionIv?.length, hashCount: redirect?.fileHashes?.length,
    };
    if (rpcName(redirect).toLowerCase() !== 'upload.filecdnredirect' || redirect?.fileToken?.length !== 32 || redirect?.encryptionKey?.length !== 32 || redirect?.encryptionIv?.length !== 16) {
      throw new Error(`invalid CDN redirect: ${JSON.stringify(result.uploadGetFile)}`);
    }

    const fileToken = bytes(redirect.fileToken);
    const chunk = await client.invoke(tokenRequest(0x395f69da, 'upload.getCdnFile', fileToken, BigInt(0), 1024));
    result.uploadGetCdnFile = { constructor: rpcName(chunk), bytes: chunk?.bytes?.length, requestTokenBytes: chunk?.requestToken?.length };
    if (rpcName(chunk).toLowerCase() !== 'upload.cdnfile' || !chunk?.bytes?.length) throw new Error(`invalid CDN chunk: ${JSON.stringify(result.uploadGetCdnFile)}`);

    const hashes = await client.invoke(tokenRequest(0x91dc3f31, 'upload.getCdnFileHashes', fileToken, BigInt(0)));
    result.uploadGetCdnFileHashes = { constructor: rpcName(hashes), count: hashes?.length };
    if (!Array.isArray(hashes) || hashes.length === 0) throw new Error(`invalid CDN hashes: ${JSON.stringify(result.uploadGetCdnFileHashes)}`);

    const requestToken = chunk?.requestToken?.length ? bytes(chunk.requestToken) : requestTokenFor(fileToken);
    const reuploaded = await client.invoke(reuploadRequest(fileToken, requestToken));
    result.uploadReuploadCdnFile = { constructor: rpcName(reuploaded), count: reuploaded?.length };
    if (!Array.isArray(reuploaded) || reuploaded.length === 0) throw new Error(`invalid CDN reupload hashes: ${JSON.stringify(result.uploadReuploadCdnFile)}`);
  } finally {
    client.destroy();
  }
  console.log(JSON.stringify({
    transport: `DC${dcId} TCPObfuscated -> gateway -> session -> Files BFF/CDN provider/PostgreSQL`,
    userId: userId.toString(), result,
  }));
}

main().then(() => process.exit(0)).catch((error) => {
  console.error(`CDN provider probe failed: ${error instanceof Error ? error.stack || error.message : String(error)}`);
  process.exit(1);
});
