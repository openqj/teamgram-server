import { execFileSync } from 'node:child_process';
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
const userId = String(process.env.USER_ID);
const startedAt = new Date().toISOString();

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

class AssignAppStoreTransactionCompat {
  static CONSTRUCTOR_ID = 0x80ed747d;
  static className = 'payments.assignAppStoreTransaction';
  static classType = 'request';
  CONSTRUCTOR_ID = AssignAppStoreTransactionCompat.CONSTRUCTOR_ID;
  className = AssignAppStoreTransactionCompat.className;
  classType = AssignAppStoreTransactionCompat.classType;
  receipt: Uint8Array;
  purpose: any;

  constructor(args: any) {
    Object.assign(this, args);
  }

  getBytes(): Uint8Array {
    return Buffer.concat([
      Buffer.from([0x7d, 0x74, 0xed, 0x80]),
      Buffer.from(serializeBytes(this.receipt)),
      Buffer.from(this.purpose.getBytes()),
    ]);
  }

  readResult(reader: any): any {
    return reader.tgReadObject();
  }
}

class AssignPlayMarketTransactionCompat {
  static CONSTRUCTOR_ID = 0xdffd50d3;
  static className = 'payments.assignPlayMarketTransaction';
  static classType = 'request';
  CONSTRUCTOR_ID = AssignPlayMarketTransactionCompat.CONSTRUCTOR_ID;
  className = AssignPlayMarketTransactionCompat.className;
  classType = AssignPlayMarketTransactionCompat.classType;
  receipt: any;
  purpose: any;

  constructor(args: any) {
    Object.assign(this, args);
  }

  getBytes(): Uint8Array {
    return Buffer.concat([
      Buffer.from([0xd3, 0x50, 0xfd, 0xdf]),
      Buffer.from(this.receipt.getBytes()),
      Buffer.from(this.purpose.getBytes()),
    ]);
  }

  readResult(reader: any): any {
    return reader.tgReadObject();
  }
}

const AssignAppStoreTransaction = Api.payments.AssignAppStoreTransaction ?? AssignAppStoreTransactionCompat;
const AssignPlayMarketTransaction = Api.payments.AssignPlayMarketTransaction ?? AssignPlayMarketTransactionCompat;

function typeName(value: any): string {
  return value?.className || value?.constructor?.name || typeof value;
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

function sql(query: string): string {
  return execFileSync('docker', [
    'exec', 'teamgram-postgres', 'psql', '-v', 'ON_ERROR_STOP=1', '-At', '-q', '-F', '\t',
    '-U', 'teamgram', '-d', 'teamgram', '-c', query,
  ], { encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] }).trim();
}

function quote(value: string): string {
  return `'${value.replaceAll("'", "''")}'`;
}

function cleanup(): void {
  if (process.env.KEEP_ROWS === '1') return;
  const boundary = quote(startedAt);
  sql(`DELETE FROM apifull_payment_provider_operation
    WHERE user_id=${Number(userId)}
      AND operation IN ('payments.assignAppStoreTransaction','payments.assignPlayMarketTransaction')
      AND created_at >= ${boundary}::timestamptz`);
}

async function run() {
  const tag = `store-assignment-${Date.now()}`;
  const client = makeClient(process.env.AUTH_B64!);
  const results: Record<string, unknown> = {};
  try {
    await client.connect();
    const me = await client.getMe();
    if (String(me?.id) !== userId) throw new Error('the authenticated account does not match USER_ID');

    const purpose = new Api.InputStorePaymentPremiumSubscription({});
    const appRequest = new AssignAppStoreTransaction({
      receipt: Buffer.from(`${tag}:app`),
      purpose,
    });
    const playRequest = new AssignPlayMarketTransaction({
      receipt: new Api.DataJSON({ data: JSON.stringify({ provider: 'fixture', token: `${tag}:play` }) }),
      purpose,
    });
    const appFirst = await client.invoke(appRequest);
    const appReplay = await client.invoke(appRequest);
    const playFirst = await client.invoke(playRequest);
    const playReplay = await client.invoke(playRequest);
    for (const [name, value] of [
      ['appFirst', appFirst], ['appReplay', appReplay], ['playFirst', playFirst], ['playReplay', playReplay],
    ] as const) {
      if (typeName(value) !== 'Updates' && typeName(value) !== 'updates') {
        throw new Error(`${name} returned ${typeName(value)}`);
      }
      results[name] = { constructor: typeName(value), date: value?.date, seq: value?.seq };
    }
    console.log(JSON.stringify({
      transport: `DC${dcId} TCPObfuscated -> gateway -> session -> Premium BFF -> payment fixture`,
      userId,
      results,
      operationRowsBeforeCleanup: sql(`SELECT count(*) FROM apifull_payment_provider_operation WHERE user_id=${Number(userId)} AND operation IN ('payments.assignAppStoreTransaction','payments.assignPlayMarketTransaction') AND created_at >= ${quote(startedAt)}::timestamptz`),
    }));
  } finally {
    client.destroy();
    cleanup();
  }
}

run().then(() => process.exit(0)).catch((error) => {
  try { cleanup(); } catch { /* best effort */ }
  console.error(`premium store assignment probe failed: ${error instanceof Error ? error.stack || error.message : String(error)}`);
  process.exit(1);
});
