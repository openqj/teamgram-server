import { execFileSync } from 'node:child_process';
import fs from 'node:fs';
import path from 'node:path';
import { createRequire } from 'node:module';
import { setTimeout as delay } from 'node:timers/promises';

const required = [
  'TEAMGRAM_GRAMJS_DIR', 'A_AUTH_B64', 'B_AUTH_B64', 'A_USER_ID', 'B_USER_ID',
  'PREMIUM_INVOICE_SLUG', 'RUN_BACKEND_RESTART',
];
for (const key of required) {
  if (!process.env[key]) throw new Error(`${key} is required`);
}

const gramjsDir = process.env.TEAMGRAM_GRAMJS_DIR!;
const dcId = Number(process.env.TG_DC_ID || '2');
const dcHost = process.env.TG_DC_HOST || '127.0.0.1';
const dcPort = Number(process.env.TG_DC_PORT || '11443');
const apiId = Number(process.env.TG_API_ID || '1');
const apiHash = process.env.TG_API_HASH || '0123456789abcdef0123456789abcdef';
const aUserId = BigInt(process.env.A_USER_ID!);
const bUserId = BigInt(process.env.B_USER_ID!);
const invoiceSlugBase = process.env.PREMIUM_INVOICE_SLUG!.trim();
const runRestart = process.env.RUN_BACKEND_RESTART === '1';
const startedAt = Math.floor(Date.now() / 1000);

if (!Number.isSafeInteger(dcId) || dcId <= 0 || !Number.isSafeInteger(dcPort) || dcPort <= 0) {
  throw new Error('TG_DC_ID and TG_DC_PORT must be positive integers');
}
if (!invoiceSlugBase) throw new Error('PREMIUM_INVOICE_SLUG must not be empty');
if (process.env.RUN_BACKEND_RESTART !== '0' && !runRestart) {
  throw new Error('RUN_BACKEND_RESTART must be 0 or 1');
}

const globalAny = globalThis as typeof globalThis & { self?: any; addEventListener?: () => void };
globalAny.self ??= globalThis;
globalAny.addEventListener ??= () => {};
globalAny.self.addEventListener ??= globalAny.addEventListener;

const requireModule = createRequire(import.meta.url);
const TelegramClient = requireModule(path.join(gramjsDir, 'src/lib/gramjs/client/TelegramClient.ts')).default;
const CallbackSession = requireModule(path.join(gramjsDir, 'src/lib/gramjs/sessions/CallbackSession.ts')).default;
const Api = requireModule(path.join(gramjsDir, 'src/lib/gramjs/tl/index.ts')).Api;
const ConnectionTCPObfuscated = requireModule(
  path.join(gramjsDir, 'src/lib/gramjs/network/connection/TCPObfuscated.ts'),
).ConnectionTCPObfuscated;
const quietLogger = { debug() {}, info() {}, warn() {}, error() {} };

function rpcName(value: any): string {
  return value?.className || value?.constructor?.name || typeof value;
}

function requireCondition(value: unknown, message: string): asserts value {
  if (!value) throw new Error(message);
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
    connection: ConnectionTCPObfuscated,
    fallbackConnection: ConnectionTCPObfuscated,
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
  (client as any)._borrowExportedSender = async () => undefined;
  (client as any)._updateLoop = async () => undefined;
  return client;
}

async function disconnect(client: any): Promise<void> {
  try {
    const timeoutMs = Number(process.env.DISCONNECT_TIMEOUT_MS || '5000');
    await Promise.race([
      client.disconnect(),
      delay(timeoutMs),
    ]);
  } catch {
    // Cleanup must not hide the assertion that failed in the probe.
  }
  try {
    client.destroy();
  } catch {
    // The client may already be destroyed after a transport failure.
  }
}

function sql(query: string): string {
  return execFileSync('docker', [
    'exec', process.env.ACCEPTANCE_POSTGRES_CONTAINER || 'teamgram-postgres',
    'psql', '-v', 'ON_ERROR_STOP=1', '-At', '-q', '-U',
    process.env.ACCEPTANCE_POSTGRES_USER || 'teamgram',
    '-d', process.env.ACCEPTANCE_POSTGRES_DATABASE || 'teamgram', '-c', query,
  ], { encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] }).trim();
}

function sqlJson(query: string): any {
  const raw = sql(query);
  if (!raw) return null;
  try {
    return JSON.parse(raw);
  } catch (error) {
    throw new Error(`PostgreSQL evidence was not JSON: ${String(error)}; raw=${raw.slice(0, 400)}`);
  }
}

function quote(value: string): string {
  return `'${value.replaceAll("'", "''")}'`;
}

function composeFiles(): string[] {
  const configured = process.env.ACCEPTANCE_COMPOSE_FILES?.trim();
  if (configured) return configured.split(/\s+/).filter(Boolean);
  const single = process.env.ACCEPTANCE_COMPOSE_FILE?.trim();
  if (single) return [single];
  const root = path.dirname(new URL(import.meta.url).pathname);
  return [
    '/Users/mac/open/tg/services/backend/docker-compose.yaml',
    path.join(root, 'backend-deploy-r29-prod.yaml'),
    '/tmp/teamgram-production-runtime.yaml',
    path.join(root, 'backend-e2e-provider-fixtures.yaml'),
    '/tmp/teamgram-passkey-image-userhandle.yaml',
  ];
}

async function waitForTcp(): Promise<void> {
  const timeoutMs = Number(process.env.RESTART_WAIT_SECONDS || '120') * 1000;
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    try {
      const net = await import('node:net');
      await new Promise<void>((resolve, reject) => {
        const socket = net.createConnection({ host: dcHost, port: dcPort });
        socket.once('connect', () => { socket.destroy(); resolve(); });
        socket.once('error', (error) => { socket.destroy(); reject(error); });
        socket.setTimeout(1000, () => { socket.destroy(); reject(new Error('TCP readiness timeout')); });
      });
      return;
    } catch {
      await delay(1000);
    }
  }
  throw new Error(`backend TCP endpoint ${dcHost}:${dcPort} did not become ready`);
}

function restartBackend(): void {
  const files = composeFiles();
  for (const file of files) requireCondition(file && fs.existsSync(file), `compose file not found: ${file}`);
  const args = ['compose'];
  const project = process.env.ACCEPTANCE_COMPOSE_PROJECT?.trim();
  const directory = process.env.ACCEPTANCE_COMPOSE_PROJECT_DIRECTORY?.trim();
  if (project) args.push('--project-name', project);
  if (directory) args.push('--project-directory', directory);
  const envFile = process.env.ACCEPTANCE_COMPOSE_ENV_FILE?.trim();
  if (envFile) {
    requireCondition(fs.existsSync(envFile), `Compose env file not found: ${envFile}`);
    args.push('--env-file', envFile);
  }
  for (const file of files) args.push('-f', file);
  args.push('restart', process.env.ACCEPTANCE_SERVICE || 'backend');
  execFileSync('docker', args, {
    stdio: ['ignore', 'inherit', 'inherit'],
    timeout: Number(process.env.RESTART_TIMEOUT_SECONDS || '300') * 1000,
  });
}

async function connectPair(): Promise<{ a: any; b: any; aMe: any; bMe: any }> {
  const a = makeClient(process.env.A_AUTH_B64!);
  const b = makeClient(process.env.B_AUTH_B64!);
  try {
    await Promise.all([a.connect(), b.connect()]);
    const [aMe, bMe] = await Promise.all([a.getMe(), b.getMe()]);
    requireCondition(String(aMe?.id) === aUserId.toString(), `A auth key user mismatch: ${String(aMe?.id)}`);
    requireCondition(String(bMe?.id) === bUserId.toString(), `B auth key user mismatch: ${String(bMe?.id)}`);
    return { a, b, aMe, bMe };
  } catch (error) {
    await Promise.all([disconnect(a), disconnect(b)]);
    throw error;
  }
}

async function connectPairWithRetry(): Promise<{ a: any; b: any; aMe: any; bMe: any }> {
  const attempts = Number(process.env.RESTART_CONNECT_RETRIES || '30');
  let lastError: unknown;
  for (let attempt = 1; attempt <= attempts; attempt += 1) {
    try {
      return await connectPair();
    } catch (error) {
      lastError = error;
      if (attempt < attempts) await delay(Number(process.env.RESTART_CONNECT_DELAY_MS || '1000'));
    }
  }
  throw new Error(`could not connect both Premium sessions after retries: ${String(lastError)}`);
}

function assertPaymentResult(value: any, label: string): void {
  const name = rpcName(value);
  requireCondition(name === 'payments.PaymentResult' || name === 'PaymentResult', `${label} returned ${name}`);
  requireCondition(value?.updates != null, `${label} did not contain Updates`);
}

async function purchasePremium(client: any, invoiceSlug: string): Promise<Record<string, unknown>> {
  const promo = await client.invoke(new Api.help.GetPremiumPromo({}));
  requireCondition(rpcName(promo) === 'help.PremiumPromo' || rpcName(promo) === 'HelpPremiumPromo', `help.getPremiumPromo returned ${rpcName(promo)}`);
  requireCondition(Array.isArray(promo.periodOptions) && promo.periodOptions.length > 0, 'Premium catalogue is empty');

  const purpose = new Api.InputStorePaymentPremiumSubscription({});
  const canPurchase = await client.invoke(new Api.payments.CanPurchaseStore({ purpose }));
  requireCondition(['BoolTrue', 'boolTrue', 'Boolean'].includes(rpcName(canPurchase)) || canPurchase === true, `payments.canPurchaseStore returned ${rpcName(canPurchase)}`);

  const invoice = new Api.InputInvoiceSlug({ slug: invoiceSlug });
  const form = await client.invoke(new Api.payments.GetPaymentForm({ invoice }));
  requireCondition(rpcName(form) === 'payments.PaymentForm' || rpcName(form) === 'PaymentForm', `payments.getPaymentForm returned ${rpcName(form)}`);
  requireCondition(BigInt(form.formId) > 0n, 'Premium payment form has no form id');

  const validated = await client.invoke(new Api.payments.ValidateRequestedInfo({
    invoice,
    info: new Api.PaymentRequestedInfo({}),
  }));
  requireCondition(rpcName(validated) === 'payments.ValidatedRequestedInfo' || rpcName(validated) === 'ValidatedRequestedInfo', `payments.validateRequestedInfo returned ${rpcName(validated)}`);

  // APIFull keys a Premium settlement by form, peer and message ID when the
  // invoice has no message peer. Use a probe-owned positive message ID so a
  // repeated acceptance run cannot collide with an earlier fixture purchase.
  const probeMsgId = 1_000_000_000 + (Date.now() % 900_000_000);
  const request = new Api.payments.SendPaymentForm({
    formId: form.formId,
    invoice,
    requestedInfoId: validated.id,
    peer: new Api.InputPeerSelf({}),
    msgId: probeMsgId,
    credentials: new Api.InputPaymentCredentials({
      data: new Api.DataJSON({ data: JSON.stringify({ provider: 'local-fixture', probe: invoiceSlug }) }),
    }),
  });
  const first = await client.invoke(request);
  const replay = await client.invoke(request);
  assertPaymentResult(first, 'first Premium settlement');
  assertPaymentResult(replay, 'replayed Premium settlement');
  return {
    promo: rpcName(promo),
    periodMonths: promo.periodOptions.map((option: any) => Number(option.months)),
    canPurchaseStore: rpcName(canPurchase),
    form: rpcName(form),
    formId: String(form.formId),
    validated: rpcName(validated),
    first: rpcName(first),
    replay: rpcName(replay),
    peer: 'self',
    msgId: probeMsgId,
  };
}

function premiumUserFromFull(full: any, userId: bigint): any {
  const users = Array.isArray(full?.users) ? full.users : [];
  return users.find((user: any) => String(user?.id) === userId.toString());
}

async function readPremium(a: any, b: any, aMe: any): Promise<Record<string, unknown>> {
  requireCondition(aMe?.premium === true, `A users.getMe did not show premium=true (got ${String(aMe?.premium)})`);
  const aAccessHash = aMe?.accessHash;
  requireCondition(aAccessHash !== undefined && aAccessHash !== null, 'A users.getMe did not return an access hash');
  const full = await b.invoke(new Api.users.GetFullUser({
    id: new Api.InputUser({ userId: aUserId, accessHash: BigInt(String(aAccessHash)) }),
  }));
  const fullType = rpcName(full);
  requireCondition(fullType === 'users.UserFull' || fullType === 'UserFull', `B users.getFullUser returned ${fullType}`);
  const target = premiumUserFromFull(full, aUserId);
  requireCondition(target != null, `B users.getFullUser did not include A in users[] (count=${Array.isArray(full?.users) ? full.users.length : 0})`);
  requireCondition(target.premium === true, `B users.getFullUser users[] did not show A premium=true (got ${String(target.premium)})`);
  const ownFull = await a.invoke(new Api.users.GetFullUser({ id: new Api.InputUserSelf() }));
  requireCondition(rpcName(ownFull) === 'users.UserFull' || rpcName(ownFull) === 'UserFull', `A users.getFullUser returned ${rpcName(ownFull)}`);
  return {
    aGetMePremium: Boolean(aMe.premium),
    bGetFullUser: fullType,
    bObservedUser: { id: String(target.id), premium: Boolean(target.premium) },
    aGetFullUser: rpcName(ownFull),
  };
}

function readLedgerEvidence(transactionId: string): Record<string, unknown> {
  const result = sqlJson(`
    SELECT json_build_object(
      'requestId', pr.id,
      'requestKey', pr.request_key,
      'requestState', pr.state,
      'transactionId', pr.transaction_id,
      'receiptTransactionId', rec.transaction_id,
      'outboxState', outbox.state,
      'outboxMonths', outbox.months,
      'grantTransactionId', premium_grant.transaction_id,
      'providerOperation', provider_op.operation,
      'providerRequestKey', provider_op.request_key,
      'providerReceipt', provider_receipt.transaction_id,
      'providerProduct', entitlement.product,
      'providerEntitlementState', entitlement.state,
      'providerEntitlementMonths', entitlement.months
    )
    FROM apifull_payment_request pr
    LEFT JOIN apifull_payment_receipt rec ON rec.request_id = pr.id
    LEFT JOIN apifull_payment_entitlement_outbox outbox ON outbox.request_id = pr.id
    LEFT JOIN user_premium_payment_grant premium_grant ON premium_grant.transaction_id = pr.transaction_id AND premium_grant.user_id = pr.user_id
    LEFT JOIN apifull_payment_provider_operation provider_op ON provider_op.transaction_id = pr.transaction_id AND provider_op.user_id = pr.user_id
    LEFT JOIN apifull_payment_provider_receipt provider_receipt ON provider_receipt.operation = provider_op.operation AND provider_receipt.transaction_id = pr.transaction_id
    LEFT JOIN apifull_payment_provider_entitlement entitlement ON entitlement.user_id = pr.user_id AND entitlement.product = 'premium_subscription' AND entitlement.transaction_id = pr.transaction_id
    WHERE pr.user_id = ${aUserId.toString()}
      AND pr.transaction_id = ${quote(transactionId)}
    ORDER BY pr.id DESC
    LIMIT 1
  `);
  requireCondition(result != null, `no local payment ledger row found for ${transactionId}`);
  for (const [field, expected] of [
    ['requestState', 'settled'], ['outboxState', 'complete'], ['providerEntitlementState', 'granted'],
  ] as const) {
    requireCondition(result[field] === expected, `ledger ${field}=${String(result[field])}, expected ${expected}`);
  }
  for (const field of ['receiptTransactionId', 'grantTransactionId', 'providerReceipt', 'providerOperation']) {
    requireCondition(result[field] != null && String(result[field]) !== '', `ledger ${field} is missing`);
  }
  requireCondition(String(result.transactionId) === transactionId, 'local transaction ID changed');
  requireCondition(String(result.receiptTransactionId) === transactionId, 'local receipt transaction differs');
  requireCondition(String(result.grantTransactionId) === transactionId, 'local premium grant transaction differs');
  requireCondition(String(result.providerReceipt) === transactionId, 'provider receipt transaction differs');
  requireCondition(String(result.providerEntitlementMonths) === '1' || Number(result.providerEntitlementMonths) === 1, 'provider entitlement months is not one');
  return result;
}

function readUserPremiumRow(): Record<string, unknown> {
  const result = sqlJson(`
    SELECT json_build_object('userId', id, 'premium', premium, 'premiumExpireDate', premium_expire_date)
    FROM users WHERE id = ${aUserId.toString()} LIMIT 1
  `);
  requireCondition(result != null && result.premium === true, `PostgreSQL users row did not show premium=true: ${JSON.stringify(result)}`);
  return result;
}

function findTransactionId(): string {
  const result = sqlJson(`
    SELECT json_build_object('transactionId', transaction_id, 'requestId', id, 'requestKey', request_key)
    FROM apifull_payment_request
    WHERE user_id = ${aUserId.toString()} AND state = 'settled' AND created_at >= ${startedAt}
    ORDER BY id DESC LIMIT 1
  `);
  requireCondition(result?.transactionId, 'settled Premium payment row was not found after purchase');
  return String(result.transactionId);
}

async function main(): Promise<void> {
  const invoiceSlug = `${invoiceSlugBase}-${Date.now()}-${process.pid}`;
  let pair = await connectPairWithRetry();
  const evidence: Record<string, unknown> = {
    transport: `DC${dcId} TCPObfuscated -> gateway -> session -> Premium BFF -> payment fixture`,
    aUserId: aUserId.toString(),
    bUserId: bUserId.toString(),
    invoiceSlug,
    restartRequested: runRestart,
  };
  let transactionId = '';
  try {
    evidence.purchase = await purchasePremium(pair.a, invoiceSlug);
    transactionId = findTransactionId();
    evidence.transactionId = transactionId;
    evidence.ledgerBeforeRestart = readLedgerEvidence(transactionId);
    evidence.userRowBeforeRestart = readUserPremiumRow();
    evidence.sessionsBeforeRestart = await readPremium(pair.a, pair.b, pair.aMe);

    await Promise.all([disconnect(pair.a), disconnect(pair.b)]);
    if (runRestart) {
      restartBackend();
      await waitForTcp();
      pair = await connectPairWithRetry();
      evidence.ledgerAfterRestart = readLedgerEvidence(transactionId);
      evidence.userRowAfterRestart = readUserPremiumRow();
      evidence.sessionsAfterRestart = await readPremium(pair.a, pair.b, pair.aMe);
    } else {
      evidence.restart = 'skipped (RUN_BACKEND_RESTART=0)';
    }
    console.log(JSON.stringify(evidence));
  } finally {
    await Promise.all([disconnect(pair.a), disconnect(pair.b)]);
  }
}

main().then(() => process.exit(0)).catch((error) => {
  console.error(`premium entitlement restart probe failed: ${error instanceof Error ? error.stack || error.message : String(error)}`);
  process.exit(1);
});
