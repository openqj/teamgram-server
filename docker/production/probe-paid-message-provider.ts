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
const userId = BigInt(process.env.USER_ID!);
const targetId = BigInt(process.env.TARGET_USER_ID || '10');
const targetAccessHash = BigInt(process.env.TARGET_ACCESS_HASH || '900000000000000010');
const startedAt = Math.floor(Date.now() / 1000);

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

function sql(query: string): string {
  return execFileSync('docker', [
    'exec', 'teamgram-postgres', 'psql', '-v', 'ON_ERROR_STOP=1', '-At', '-q', '-F', '\t',
    '-U', 'teamgram', '-d', 'teamgram', '-c', query,
  ], { encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] }).trim();
}

function quote(value: string): string {
  return `'${value.replaceAll("'", "''")}'`;
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

function targetUser(): any {
  return new Api.InputUser({ userId: targetId, accessHash: targetAccessHash });
}

function targetPeer(): any {
  return new Api.InputPeerUser({ userId: targetId, accessHash: targetAccessHash });
}

function cleanup(): void {
  if (process.env.KEEP_ROWS === '1') return;
  const boundary = `to_timestamp(${startedAt})`;
  sql(`DELETE FROM apifull_payment_provider_operation
       WHERE user_id=${userId} AND created_at >= ${boundary}
         AND operation IN ('get_paid_messages_revenue','toggle_no_paid_messages_exception')`);
}

async function run(): Promise<void> {
  const client = makeClient(process.env.AUTH_B64!);
  try {
    await client.connect();
    const me = await client.getMe();
    if (BigInt(me?.id) !== userId) throw new Error('the authenticated account does not match USER_ID');

    const revenueRequest = new Api.account.GetPaidMessagesRevenue({
      parentPeer: targetPeer(),
      userId: targetUser(),
    });
    const revenue = await client.invoke(revenueRequest);
    const replay = await client.invoke(new Api.account.GetPaidMessagesRevenue({
      parentPeer: targetPeer(),
      userId: targetUser(),
    }));
    if (!['account.PaidMessagesRevenue', 'PaidMessagesRevenue'].includes(typeName(revenue))) {
      throw new Error(`unexpected revenue constructor ${typeName(revenue)}`);
    }
    if (String(revenue.starsAmount) !== String(replay.starsAmount) || Number(revenue.starsAmount) < 0) {
      throw new Error('paid-message revenue replay was not stable and non-negative');
    }

    const exception = await client.invoke(new Api.account.ToggleNoPaidMessagesException({
      refundCharged: true,
      requirePayment: true,
      parentPeer: targetPeer(),
      userId: targetUser(),
    }));
    if (!['Bool', 'Boolean', 'boolTrue', 'boolFalse'].includes(typeName(exception))) {
      throw new Error(`unexpected exception constructor ${typeName(exception)}`);
    }

    const rows = sql(`SELECT count(*) FROM apifull_payment_provider_operation
      WHERE user_id=${userId} AND created_at >= to_timestamp(${startedAt})
        AND operation IN ('get_paid_messages_revenue','toggle_no_paid_messages_exception')`);
    cleanup();
    const remaining = sql(`SELECT count(*) FROM apifull_payment_provider_operation
      WHERE user_id=${userId} AND created_at >= to_timestamp(${startedAt})
        AND operation IN ('get_paid_messages_revenue','toggle_no_paid_messages_exception')`);
    console.log(JSON.stringify({
      transport: `DC${dcId} TCPObfuscated -> gateway -> session -> APIFull/payment provider fixture/PostgreSQL`,
      userId: String(userId),
      targetId: String(targetId),
      methods: {
        getPaidMessagesRevenue: { constructor: typeName(revenue), starsAmount: String(revenue.starsAmount), replay: typeName(replay) },
        toggleNoPaidMessagesException: { constructor: typeName(exception) },
      },
      providerOperationRowsBeforeCleanup: rows,
      providerOperationRowsRemaining: remaining,
    }));
  } finally {
    client.destroy();
    cleanup();
  }
}

run().then(() => process.exit(0)).catch((error) => {
  try { cleanup(); } catch { /* best effort cleanup */ }
  console.error(`Paid-message provider probe failed: ${error instanceof Error ? error.stack || error.message : String(error)}`);
  process.exit(1);
});
