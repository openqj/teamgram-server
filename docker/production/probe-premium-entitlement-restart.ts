import { createHmac } from 'node:crypto';
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
const webhookURL = process.env.PAYMENT_WEBHOOK_URL || 'http://127.0.0.1:8801/payment/provider/webhook';
let webhookSigningKey = '';
let providerSigningKey = '';
let providerName = 'external';
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

async function withTimeout<T>(promise: Promise<T>, label: string): Promise<T> {
  const timeoutMs = Number(process.env.PREMIUM_CONNECT_TIMEOUT_MS || '15000');
  let timer: ReturnType<typeof setTimeout> | undefined;
  try {
    return await Promise.race([
      promise,
      new Promise<T>((_, reject) => {
        timer = setTimeout(() => reject(new Error(`${label} timed out after ${timeoutMs}ms`)), timeoutMs);
      }),
    ]);
  } finally {
    if (timer) clearTimeout(timer);
  }
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

function hydrateComposeProviderEnvironment(): void {
  const project = process.env.ACCEPTANCE_COMPOSE_PROJECT?.trim() || 'backend';
  const service = process.env.ACCEPTANCE_SERVICE?.trim() || 'backend';
  const container = process.env.ACCEPTANCE_BACKEND_CONTAINER?.trim() || `${project}-${service}-1`;
  let raw: string;
  try {
    raw = execFileSync('docker', [
      'inspect', container, '--format', '{{range .Config.Env}}{{println .}}{{end}}',
    ], { encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'] });
  } catch {
    return;
  }
  const values = new Map<string, string>();
  for (const line of raw.split(/\r?\n/)) {
    const separator = line.indexOf('=');
    if (separator > 0) values.set(line.slice(0, separator), line.slice(separator + 1));
  }
  // The production Compose layer names these values PROVIDER_KEY/SECRET,
  // while the running backend keeps the legacy CODE_KEY/SECRET names.
  for (const [target, source] of [
    ['TEAMGRAM_CODE_PROVIDER_KEY', 'TEAMGRAM_CODE_KEY'],
    ['TEAMGRAM_CODE_PROVIDER_SECRET', 'TEAMGRAM_CODE_SECRET'],
    ['TEAMGRAM_CODE_CHALLENGE_SECRET', 'TEAMGRAM_CODE_CHALLENGE_SECRET'],
    ['TEAMGRAM_PAYMENT_PROVIDER_KEY', 'TEAMGRAM_PAYMENT_PROVIDER_KEY'],
    ['TEAMGRAM_PAYMENT_PROVIDER_SIGNING_KEY', 'TEAMGRAM_PAYMENT_PROVIDER_SIGNING_KEY'],
    ['TEAMGRAM_PAYMENT_PROVIDER_WEBHOOK_SIGNING_KEY', 'TEAMGRAM_PAYMENT_PROVIDER_WEBHOOK_SIGNING_KEY'],
    ['TEAMGRAM_PAYMENT_PROVIDER_NAME', 'TEAMGRAM_PAYMENT_PROVIDER_NAME'],
  ] as const) {
    if (!process.env[target] && values.has(source)) process.env[target] = values.get(source)!;
  }
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
  hydrateComposeProviderEnvironment();
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
    await Promise.all([
      withTimeout(a.connect(), 'A Telegram connection'),
      withTimeout(b.connect(), 'B Telegram connection'),
    ]);
    const [aMe, bMe] = await Promise.all([
      withTimeout(a.getMe(), 'A users.getMe'),
      withTimeout(b.getMe(), 'B users.getMe'),
    ]);
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

async function readPremium(a: any, b: any, expected: boolean): Promise<Record<string, unknown>> {
  const aMe = await a.getMe();
  requireCondition(Boolean(aMe?.premium) === expected, `A users.getMe premium=${String(aMe?.premium)}, expected ${String(expected)}`);
  const aAccessHash = aMe?.accessHash;
  requireCondition(aAccessHash !== undefined && aAccessHash !== null, 'A users.getMe did not return an access hash');
  const full = await b.invoke(new Api.users.GetFullUser({
    id: new Api.InputUser({ userId: aUserId, accessHash: BigInt(String(aAccessHash)) }),
  }));
  const fullType = rpcName(full);
  requireCondition(fullType === 'users.UserFull' || fullType === 'UserFull', `B users.getFullUser returned ${fullType}`);
  const target = premiumUserFromFull(full, aUserId);
  requireCondition(target != null, `B users.getFullUser did not include A in users[] (count=${Array.isArray(full?.users) ? full.users.length : 0})`);
  requireCondition(Boolean(target.premium) === expected, `B users.getFullUser users[] premium=${String(target.premium)}, expected ${String(expected)}`);
  const ownFull = await a.invoke(new Api.users.GetFullUser({ id: new Api.InputUserSelf() }));
  requireCondition(rpcName(ownFull) === 'users.UserFull' || rpcName(ownFull) === 'UserFull', `A users.getFullUser returned ${rpcName(ownFull)}`);
  return {
    aGetMePremium: Boolean(aMe.premium),
    bGetFullUser: fullType,
    bObservedUser: { id: String(target.id), premium: Boolean(target.premium) },
    aGetFullUser: rpcName(ownFull),
  };
}

async function readPremiumState(a: any, b: any, expected: boolean): Promise<Record<string, unknown>> {
  const aMe = await a.getMe();
  requireCondition(Boolean(aMe?.premium) === expected, `A users.getMe premium=${String(aMe?.premium)}, expected ${String(expected)}`);
  const aAccessHash = aMe?.accessHash;
  requireCondition(aAccessHash !== undefined && aAccessHash !== null, 'A users.getMe did not return an access hash');
  const full = await b.invoke(new Api.users.GetFullUser({
    id: new Api.InputUser({ userId: aUserId, accessHash: BigInt(String(aAccessHash)) }),
  }));
  const fullType = rpcName(full);
  requireCondition(fullType === 'users.UserFull' || fullType === 'UserFull', `B users.getFullUser returned ${fullType}`);
  const target = premiumUserFromFull(full, aUserId);
  requireCondition(target != null, `B users.getFullUser did not include A in users[]`);
  requireCondition(Boolean(target.premium) === expected, `B users.getFullUser premium=${String(target.premium)}, expected ${String(expected)}`);
  return {
    aGetMePremium: Boolean(aMe.premium),
    bObservedUser: { id: String(target.id), premium: Boolean(target.premium) },
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

function readReversalEvidence(transactionId: string, eventID: string): Record<string, unknown> {
  const result = sqlJson(`
    SELECT json_build_object(
      'webhookState', (SELECT state FROM apifull_payment_provider_webhook WHERE event_id = ${quote(eventID)}),
      'webhookAttempts', (SELECT attempts FROM apifull_payment_provider_webhook WHERE event_id = ${quote(eventID)}),
      'reversalCount', (SELECT count(*) FROM apifull_payment_provider_reversal WHERE provider = ${quote(providerName)} AND transaction_id = ${quote(transactionId)}),
      'reversalEventId', (SELECT event_id FROM apifull_payment_provider_reversal WHERE provider = ${quote(providerName)} AND transaction_id = ${quote(transactionId)}),
      'requestState', (SELECT state FROM apifull_payment_request WHERE user_id = ${aUserId.toString()} AND transaction_id = ${quote(transactionId)}),
      'reversedLedgerCount', (SELECT count(*) FROM apifull_payment_ledger WHERE user_id = ${aUserId.toString()} AND transaction_id = ${quote(transactionId)} AND state = 'reversed'),
      'entitlementState', (SELECT state FROM apifull_payment_provider_entitlement WHERE user_id = ${aUserId.toString()} AND product = 'premium_subscription' AND transaction_id = ${quote(transactionId)}),
      'grantRevokedAt', (SELECT revoked_at FROM user_premium_payment_grant WHERE user_id = ${aUserId.toString()} AND transaction_id = ${quote(transactionId)}),
      'activeGrantCount', (SELECT count(*) FROM user_premium_payment_grant WHERE user_id = ${aUserId.toString()} AND revoked_at = 0 AND (expires_at = 0 OR expires_at > EXTRACT(EPOCH FROM CURRENT_TIMESTAMP)::bigint)),
      'userPremium', (SELECT premium FROM users WHERE id = ${aUserId.toString()}),
      'reconciliationState', (SELECT state FROM apifull_payment_provider_reconciliation WHERE provider = ${quote(providerName)} AND user_id = ${aUserId.toString()} AND transaction_id = ${quote(transactionId)}),
      'reconciliationStatus', (SELECT provider_status FROM apifull_payment_provider_reconciliation WHERE provider = ${quote(providerName)} AND user_id = ${aUserId.toString()} AND transaction_id = ${quote(transactionId)})
    )
  `);
  requireCondition(result != null, `no reversal evidence found for ${transactionId}`);
  return result;
}

function markProviderEntitlementRevoked(transactionId: string): Record<string, unknown> {
  const result = sqlJson(`
    UPDATE apifull_payment_provider_entitlement
    SET state = 'revoked', updated_at = CURRENT_TIMESTAMP
    WHERE user_id = ${aUserId.toString()} AND product = 'premium_subscription' AND transaction_id = ${quote(transactionId)}
    RETURNING json_build_object('transactionId', transaction_id, 'state', state)
  `);
  requireCondition(result != null && result.state === 'revoked', `provider entitlement ${transactionId} was not marked revoked`);
  return result;
}

function armPaymentReconciliation(transactionId: string): Record<string, unknown> {
  const result = sqlJson(`
    UPDATE apifull_payment_provider_reconciliation
    SET state = 'pending', next_check_at = EXTRACT(EPOCH FROM CURRENT_TIMESTAMP)::bigint,
        lease_until = 0, claim_token = '', updated_at = EXTRACT(EPOCH FROM CURRENT_TIMESTAMP)::bigint
    WHERE provider = ${quote(providerName)} AND user_id = ${aUserId.toString()} AND transaction_id = ${quote(transactionId)}
    RETURNING json_build_object('provider', provider, 'transactionId', transaction_id, 'state', state)
  `);
  requireCondition(result != null && result.state === 'pending', `payment reconciliation row for ${transactionId} was not armed`);
  return result;
}

async function waitForPaymentReconciliationRow(transactionId: string): Promise<Record<string, unknown>> {
  const deadline = Date.now() + Number(process.env.RECONCILIATION_SEED_TIMEOUT_SECONDS || '120') * 1000;
  let last: Record<string, unknown> | null = null;
  while (Date.now() < deadline) {
    last = sqlJson(`
      SELECT json_build_object('state', state, 'providerStatus', provider_status, 'transactionId', transaction_id)
      FROM apifull_payment_provider_reconciliation
      WHERE provider = ${quote(providerName)} AND user_id = ${aUserId.toString()} AND transaction_id = ${quote(transactionId)}
      LIMIT 1
    `);
    if (last != null) return last;
    await delay(1000);
  }
  throw new Error(`payment reconciliation row was not created: ${JSON.stringify(last)}`);
}

async function sendRefundWebhook(transactionId: string, requestKey: string, eventID: string): Promise<Record<string, unknown>> {
  const payload = {
    event_id: eventID,
    event_type: 'payment.refunded',
    provider: providerName,
    user_id: Number(aUserId),
    request_key: requestKey,
    transaction_id: transactionId,
    product: 'premium_subscription',
    months: 1,
    stars: 0,
    occurred_at: Math.floor(Date.now() / 1000),
    reason: 'acceptance fixture refund',
  };
  const body = JSON.stringify(payload);
  const deadline = Date.now() + Number(process.env.WEBHOOK_CONNECT_TIMEOUT_SECONDS || '60') * 1000;
  let lastError = '';
  while (Date.now() < deadline) {
    try {
      const response = await fetch(webhookURL, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'X-Teamgram-Payment-Provider-Webhook-Signature': `sha256=${createHmac('sha256', webhookSigningKey).update(body).digest('hex')}`,
        },
        body,
      });
      const responseBody = await response.text();
      if (response.status === 202) {
        return { url: webhookURL, status: response.status, body: responseBody, eventID };
      }
      lastError = `HTTP ${response.status}: ${responseBody.slice(0, 300)}`;
      if (response.status !== 503) break;
    } catch (error) {
      lastError = String(error);
    }
    await delay(1000);
  }
  throw new Error(`payment webhook was not accepted: ${lastError}`);
}

async function waitForReversal(transactionId: string, eventID: string): Promise<Record<string, unknown>> {
  const deadline = Date.now() + Number(process.env.WEBHOOK_PROCESS_TIMEOUT_SECONDS || '180') * 1000;
  let last: Record<string, unknown> | null = null;
  while (Date.now() < deadline) {
    last = readReversalEvidence(transactionId, eventID);
    const activeGrantCount = Number(last.activeGrantCount || 0);
    if (last.webhookState === 'complete' && Number(last.reversalCount) === 1 &&
      last.requestState === 'reversed' && Number(last.reversedLedgerCount) === 1 &&
      last.entitlementState === 'revoked' && Number(last.grantRevokedAt) > 0 &&
      last.reconciliationState === 'complete' && last.reconciliationStatus === 'revoked' &&
      (activeGrantCount > 0 ? last.userPremium === true : last.userPremium === false)) {
      return last;
    }
    await delay(1000);
  }
  throw new Error(`payment reversal did not converge: ${JSON.stringify(last)}`);
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
  hydrateComposeProviderEnvironment();
  webhookSigningKey = (process.env.PAYMENT_WEBHOOK_SIGNING_KEY || process.env.TEAMGRAM_PAYMENT_PROVIDER_WEBHOOK_SIGNING_KEY || '').trim();
  providerSigningKey = (process.env.TEAMGRAM_PAYMENT_PROVIDER_SIGNING_KEY || '').trim();
  providerName = (process.env.TEAMGRAM_PAYMENT_PROVIDER_NAME || 'external').trim();
  requireCondition(webhookSigningKey, 'PAYMENT_WEBHOOK_SIGNING_KEY or TEAMGRAM_PAYMENT_PROVIDER_WEBHOOK_SIGNING_KEY is required');
  requireCondition(providerSigningKey, 'TEAMGRAM_PAYMENT_PROVIDER_SIGNING_KEY is required for reconciliation');
  requireCondition(providerName.length > 0 && providerName.length <= 32, 'TEAMGRAM_PAYMENT_PROVIDER_NAME must be 1-32 characters');
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
    evidence.sessionsBeforeRestart = await readPremium(pair.a, pair.b, true);

    await Promise.all([disconnect(pair.a), disconnect(pair.b)]);
    if (runRestart) {
      restartBackend();
      await waitForTcp();
      pair = await connectPairWithRetry();
      evidence.ledgerAfterRestart = readLedgerEvidence(transactionId);
      evidence.userRowAfterRestart = readUserPremiumRow();
      evidence.sessionsAfterRestart = await readPremium(pair.a, pair.b, true);
    } else {
      evidence.restart = 'skipped (RUN_BACKEND_RESTART=0)';
    }
    const requestKey = String((evidence.ledgerBeforeRestart as Record<string, unknown>).requestKey || '');
    requireCondition(requestKey, 'settled Premium request key is missing');
    const webhookEventID = `probe-refund-${Date.now()}-${process.pid}`;
    evidence.reconciliationSeed = await waitForPaymentReconciliationRow(transactionId);
    evidence.providerEntitlementRevoked = markProviderEntitlementRevoked(transactionId);
    evidence.reconciliationArmed = armPaymentReconciliation(transactionId);
    evidence.refundWebhook = await sendRefundWebhook(transactionId, requestKey, webhookEventID);
    evidence.refundRevoke = await waitForReversal(transactionId, webhookEventID);
    evidence.sessionsAfterRevoke = await readPremiumState(pair.a, pair.b, Number((evidence.refundRevoke as Record<string, unknown>).activeGrantCount || 0) > 0);
    console.log(JSON.stringify(evidence));
  } finally {
    await Promise.all([disconnect(pair.a), disconnect(pair.b)]);
  }
}

main().then(() => process.exit(0)).catch((error) => {
  console.error(`premium entitlement restart probe failed: ${error instanceof Error ? error.stack || error.message : String(error)}`);
  process.exit(1);
});
