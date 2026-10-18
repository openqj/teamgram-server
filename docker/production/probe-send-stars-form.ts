import { createHash } from 'node:crypto';
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
const userId = Number(process.env.USER_ID);
// Purpose-based Layer 229 invoices omit store_product. Use a probe-unique
// quote so an older active fixture offer cannot make catalog resolution
// ambiguous for this run.
const stars = 10000 + (Date.now() % 900000);
const currency = 'USD';
const amount = stars * 2;
const storeProduct = `probe-stars-${Date.now()}`;
const startedAt = Math.floor(Date.now() / 1000);
const keepRows = process.env.KEEP_ROWS === '1';

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

function typeName(value: any): string {
  return value?.className || value?.constructor?.name || typeof value;
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

function seed(): void {
  sql(`INSERT INTO apifull_stars_offer
    (kind, stars, store_product, currency, amount, extended, active)
    VALUES ('topup', ${stars}, ${quote(storeProduct)}, ${quote(currency)}, ${amount}, 0, 1)
    ON CONFLICT (kind, stars, store_product, currency, amount) DO UPDATE
      SET active=1, extended=0`);
}

function cleanup(): void {
  if (keepRows) return;
  const boundary = `to_timestamp(${startedAt})`;
  // Capture the exact request rows before deleting their dependent records.
  const rows = sql(`SELECT id, transaction_id FROM apifull_payment_request
    WHERE user_id=${userId} AND provider='external' AND created_at >= ${startedAt}
      AND request_key LIKE ${quote(`stars:${userId}:%`)} ORDER BY id`).split('\n').filter(Boolean);
  const requestIds: string[] = [];
  const transactionIds: string[] = [];
  for (const row of rows) {
    const [id, transactionID] = row.split('\t');
    if (/^\d+$/.test(id)) requestIds.push(id);
    if (transactionID) transactionIds.push(transactionID.replaceAll("'", "''"));
  }
  const ids = requestIds.length ? requestIds.join(',') : 'NULL';
  const txValues = transactionIds.map((value) => `'${value}'`).join(',') || 'NULL';
  const starsIdems = transactionIds.map((transactionID) => {
    const digest = createHash('sha256').update(`external\0${transactionID}`).digest('hex');
    return `'payment:${digest}'`;
  }).join(',') || 'NULL';
  sql(`BEGIN;
    DELETE FROM apifull_payment_entitlement_outbox WHERE request_id IN (${ids});
    DELETE FROM apifull_payment_receipt WHERE request_id IN (${ids});
    DELETE FROM apifull_payment_ledger WHERE request_id IN (${ids});
    DELETE FROM apifull_payment_request WHERE id IN (${ids});
    DELETE FROM apifull_star_tx WHERE user_id=${userId} AND idem IN (${starsIdems});
    DELETE FROM apifull_payment_provider_entitlement
      WHERE user_id=${userId} AND product='stars_topup' AND created_at >= ${boundary};
    DELETE FROM apifull_payment_provider_receipt
      WHERE user_id=${userId} AND operation='settle_stars' AND created_at >= ${boundary};
    DELETE FROM apifull_payment_provider_operation
      WHERE user_id=${userId} AND operation IN ('get_stars_form','settle_stars') AND created_at >= ${boundary};
    DELETE FROM apifull_stars_offer WHERE store_product=${quote(storeProduct)};
    COMMIT;`);
}

async function run() {
  const beforeBalance = Number(sql(`SELECT COALESCE(balance,0) FROM apifull_stars WHERE user_id=${userId}`) || '0');
  seed();
  const client = makeClient(process.env.AUTH_B64!);
  try {
    await client.connect();
    const me = await client.getMe();
    if (Number(me?.id) !== userId) throw new Error('the authenticated account does not match USER_ID');

    const options = await client.invoke(new Api.payments.GetStarsTopupOptions({}));
    const option = Array.isArray(options) ? options.find((value: any) => value?.storeProduct?.value === storeProduct || value?.storeProduct === storeProduct) : null;
    if (!option) throw new Error(`seeded Stars option ${storeProduct} was not returned`);

    const purpose = new Api.InputStorePaymentStarsTopup({ stars: BigInt(stars), currency, amount: BigInt(amount) });
    const invoice = new Api.InputInvoiceStars({ purpose });
    const form = await client.invoke(new Api.payments.GetPaymentForm({ invoice }));
    if (!['payments.PaymentFormStars', 'PaymentFormStars'].includes(typeName(form)) || BigInt(form.formId) <= 0n) {
      throw new Error(`payments.getPaymentForm returned ${typeName(form)} form=${String(form?.formId)}`);
    }

    const request = new Api.payments.SendStarsForm({ formId: form.formId, invoice });
    const first = await client.invoke(request);
    const replay = await client.invoke(request);
    if (!['payments.PaymentResult', 'PaymentResult'].includes(typeName(first)) || !['payments.PaymentResult', 'PaymentResult'].includes(typeName(replay))) {
      throw new Error(`sendStarsForm returned ${typeName(first)} / ${typeName(replay)}`);
    }
    const settled = sql(`SELECT count(*) FROM apifull_payment_request
      WHERE user_id=${userId} AND state='settled' AND created_at >= ${startedAt}
        AND request_key LIKE ${quote(`stars:${userId}:%`)}`);
    const chargeID = sql(`SELECT transaction_id FROM apifull_payment_request
      WHERE user_id=${userId} AND state='settled' AND created_at >= ${startedAt}
        AND request_key LIKE ${quote(`stars:${userId}:%`)} ORDER BY id DESC LIMIT 1`);
    const providerRows = sql(`SELECT count(*) FROM apifull_payment_provider_operation
      WHERE user_id=${userId} AND operation='settle_stars' AND created_at >= to_timestamp(${startedAt})`);
    const balance = Number(sql(`SELECT COALESCE(balance,0) FROM apifull_stars WHERE user_id=${userId}`) || '0');
    if (Number(settled) !== 1 || Number(providerRows) !== 1 || balance !== beforeBalance + stars) {
      throw new Error(`settlement evidence mismatch settled=${settled} provider=${providerRows} balance=${balance} before=${beforeBalance}`);
    }
    console.log(JSON.stringify({
      transport: `DC${dcId} TCPObfuscated -> gateway -> session -> APIFull/payment fixture/PostgreSQL`,
      userId,
      optionType: typeName(option),
      formType: typeName(form),
      formId: String(form.formId),
      firstResult: typeName(first),
      replayResult: typeName(replay),
      settledRequests: Number(settled),
      providerSettlements: Number(providerRows),
      chargeID,
      balanceBefore: beforeBalance,
      balanceAfter: balance,
    }));
  } finally {
    client.destroy();
    cleanup();
    const afterBalance = Number(sql(`SELECT COALESCE(balance,0) FROM apifull_stars WHERE user_id=${userId}`) || '0');
    if (!keepRows && afterBalance !== beforeBalance) {
      sql(`UPDATE apifull_stars SET balance=${beforeBalance} WHERE user_id=${userId}`);
    }
  }
}

run().then(() => process.exit(0)).catch((error) => {
  try { cleanup(); } catch { /* best effort cleanup */ }
  console.error(`sendStarsForm probe failed: ${error instanceof Error ? error.stack || error.message : String(error)}`);
  process.exit(1);
});
