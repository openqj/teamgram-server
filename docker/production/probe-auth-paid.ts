import path from 'node:path';
import { createRequire } from 'node:module';

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
const phoneNumber = process.env.PAID_AUTH_PHONE || '+12025550105';
const phoneCodeHash = process.env.PAID_AUTH_CODE_HASH || 'paid-code-hash';
const formId = Number(process.env.PAID_AUTH_FORM_ID || '990001');

const globalAny = globalThis as typeof globalThis & { self?: any; addEventListener?: () => void };
globalAny.self ??= globalThis;
globalAny.addEventListener ??= () => {};
globalAny.self.addEventListener ??= globalAny.addEventListener;

const requireModule = createRequire(import.meta.url);
const TelegramClient = requireModule(path.join(gramjsDir, 'src/lib/gramjs/client/TelegramClient.ts')).default;
const CallbackSession = requireModule(path.join(gramjsDir, 'src/lib/gramjs/sessions/CallbackSession.ts')).default;
const serializeBytes = requireModule(path.join(gramjsDir, 'src/lib/gramjs/tl/index.ts')).serializeBytes;
const Connection = requireModule(path.join(gramjsDir, 'src/lib/gramjs/network/connection/TCPObfuscated.ts')).ConnectionTCPObfuscated;
const quietLogger = { debug() {}, info() {}, warn() {}, error() {} };

// The checked-in Web runtime currently ships the Layer 229 type declaration
// but not the corresponding runtime constructor. Keep the acceptance probe
// wire-compatible until the generated runtime bundle is refreshed.
class CheckPaidAuthRequestCompat {
  static classType = 'request';
  classType = 'request';
  className = 'auth.CheckPaidAuth';
  CONSTRUCTOR_ID = 0x56e59f9c;
  private readonly parts: Uint8Array[];

  constructor(parts: Uint8Array[]) {
    this.parts = parts;
  }

  getBytes(): Uint8Array {
    const constructor = Buffer.alloc(4);
    constructor.writeUInt32LE(this.CONSTRUCTOR_ID >>> 0);
    return Buffer.concat([constructor, ...this.parts.map((part) => Buffer.from(part))]);
  }

  readResult(reader: any): any {
    return reader.tgReadObject();
  }
}

function int64(value: bigint): Uint8Array {
  const bytes = Buffer.alloc(8);
  bytes.writeBigInt64LE(BigInt.asIntN(64, value));
  return bytes;
}

function makeClient(authKeyBase64: string): any {
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

function typeName(value: any): string {
  return value?.className || value?.constructor?.name || typeof value;
}

async function run(): Promise<void> {
  const client = makeClient(process.env.AUTH_B64!);
  try {
    await client.connect();
    const me = await client.getMe();
    if (BigInt(me?.id) !== userId) throw new Error('the authenticated account does not match USER_ID');
    const result = await client.invoke(new CheckPaidAuthRequestCompat([
      serializeBytes(phoneNumber),
      serializeBytes(phoneCodeHash),
      int64(BigInt(formId)),
    ]));
    if (!['auth.SentCodePaymentRequired', 'SentCodePaymentRequired'].includes(typeName(result))) {
      throw new Error(`unexpected auth.checkPaidAuth constructor ${typeName(result)}`);
    }
    if (String(result.storeProduct || '') !== 'telegram-premium-monthly' || String(result.phoneCodeHash || '') !== phoneCodeHash || Number(result.premiumDays) !== 30 || Number(result.amount) !== 499) {
      throw new Error('auth.checkPaidAuth returned unexpected provider-bound fields');
    }
    console.log(JSON.stringify({
      transport: `DC${dcId} TCPObfuscated -> gateway -> session -> Authorization/identity provider fixture`,
      userId: String(userId),
      method: { constructor: typeName(result), storeProduct: String(result.storeProduct), premiumDays: String(result.premiumDays), currency: String(result.currency), amount: String(result.amount) },
    }));
  } finally {
    client.destroy();
  }
}

run().then(() => process.exit(0)).catch((error) => {
  console.error(`auth.checkPaidAuth probe failed: ${error instanceof Error ? error.stack || error.message : String(error)}`);
  process.exit(1);
});
