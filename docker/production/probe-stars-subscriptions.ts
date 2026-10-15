import { execFileSync } from 'node:child_process';
import path from 'node:path';
import { createRequire } from 'node:module';

const gramjsDir = process.env.TEAMGRAM_GRAMJS_DIR || '/Users/mac/open/test/telegram-tt-master';
const dcId = Number(process.env.TG_DC_ID || '2');
const dcHost = process.env.TG_DC_HOST || '127.0.0.1';
const dcPort = Number(process.env.TG_DC_PORT || '11443');
const apiId = Number(process.env.TG_API_ID || '1');
const apiHash = process.env.TG_API_HASH || '0123456789abcdef0123456789abcdef';
const userId = BigInt(process.env.USER_ID || '7');
const subscriptionId = process.env.STARS_SUBSCRIPTION_ID || `probe-stars-${userId}-${Date.now()}`;
const startedAt = new Date().toISOString();

const globalAny = globalThis as typeof globalThis & { self?: any; addEventListener?: () => void };
globalAny.self ??= globalThis;
globalAny.addEventListener ??= () => {};
globalAny.self.addEventListener ??= globalAny.addEventListener;

const requireModule = createRequire(import.meta.url);
const TelegramClient = requireModule(path.join(gramjsDir, 'src/lib/gramjs/client/TelegramClient.ts')).default;
const CallbackSession = requireModule(path.join(gramjsDir, 'src/lib/gramjs/sessions/CallbackSession.ts')).default;
const Api = requireModule(path.join(gramjsDir, 'src/lib/gramjs/tl/index.ts')).Api;
const { concat, writeUint32LE } = requireModule(path.join(gramjsDir, 'src/util/encoding/buffer.ts'));
const { serializeBytes } = requireModule(path.join(gramjsDir, 'src/lib/gramjs/tl/generationHelpers.ts'));
const Connection = requireModule(path.join(gramjsDir, 'src/lib/gramjs/network/connection/TCPObfuscated.ts')).ConnectionTCPObfuscated;
const quietLogger = { debug() {}, info() {}, warn() {}, error() {} };

function sql(query: string): string {
  const container = process.env.TEAMGRAM_POSTGRES_CONTAINER || 'teamgram-postgres';
  return execFileSync('docker', [
    'exec', container, 'psql', '-v', 'ON_ERROR_STOP=1', '-At', '-q', '-F', '\t',
    '-U', 'teamgram', '-d', 'teamgram', '-c', query,
  ], { encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] }).trim();
}

function loadAuthKey(): string {
  return sql(`SELECT encode(k.body, 'base64') FROM auth_users u JOIN auth_keys k USING (auth_key_id)
    JOIN auth_key_infos i USING (auth_key_id) WHERE u.user_id=${userId} AND u.state=0
    AND NOT u.deleted AND NOT k.deleted AND NOT i.deleted AND i.auth_key_type=0
    ORDER BY u.date_active DESC,u.id DESC LIMIT 1`);
}

function quote(value: string): string {
  return `'${value.replaceAll("'", "''")}'`;
}

function typeName(value: any): string {
  return value?.className || value?.constructor?.name || typeof value;
}

function assertType(value: any, ...names: string[]): void {
  if (!names.includes(typeName(value))) {
    throw new Error(`expected ${names.join(' or ')}, got ${typeName(value)}`);
  }
}

function makeClient(): any {
  const auth = process.env.AUTH_B64 || loadAuthKey();
  if (!auth) throw new Error(`no active production auth key found for user ${userId}`);
  const keyHex = Buffer.from(auth, 'base64').toString('hex');
  const session = new CallbackSession({ mainDcId: dcId, keys: { [dcId]: keyHex } }, () => {});
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

function makeBotCancelStarsSubscription(user: any, chargeId: string, restore: boolean): any {
  // The reduced GramJS schema used by the web bundle omits this bot-only
  // request at runtime even though its generated type declaration exists.
  // Reuse the generated Bool request decoder and provide the exact TL wire
  // constructor for the acceptance call.
  const request: any = new Api.payments.FulfillStarsSubscription({
    peer: new Api.InputPeerSelf(),
    subscriptionId: chargeId,
  });
  request.className = 'payments.BotCancelStarsSubscription';
  request.CONSTRUCTOR_ID = 0x6dfa0622;
  request.classType = 'request';
  request.userId = user;
  request.chargeId = chargeId;
  request.restore = restore;
  request.getBytes = () => {
    const constructor = new Uint8Array(4);
    const flags = new Uint8Array(4);
    writeUint32LE(constructor, request.CONSTRUCTOR_ID);
    writeUint32LE(flags, restore ? 1 : 0);
    return concat(constructor, flags, user.getBytes(), serializeBytes(chargeId));
  };
  return request;
}

function seed(): void {
  // TL int64 fields are encoded as JSON numbers in the provider contract.
  // Keep the BigInt conversion explicit so the seed matches Go's mtproto
  // decoder instead of producing a string-valued Peer.user_id.
  const peer = `{"predicate_name":"peerUser","constructor":1498486562,"user_id":${userId.toString()}}`;
  sql(`INSERT INTO apifull_payment_provider_stars_account (user_id, balance)
       VALUES (${userId}, 100)
       ON CONFLICT (user_id) DO UPDATE SET balance=EXCLUDED.balance, updated_at=CURRENT_TIMESTAMP`);
  sql(`INSERT INTO apifull_payment_provider_stars_subscription
       (user_id, subscription_id, peer, until_date, pricing_period, pricing_amount,
        canceled, can_refulfill, missing_balance, bot_canceled, title, invoice_slug)
       VALUES (${userId}, ${quote(subscriptionId)}, ${quote(peer)}::jsonb,
               EXTRACT(EPOCH FROM CURRENT_TIMESTAMP)::INTEGER + 2592000, 2592000, 100,
               FALSE, FALSE, FALSE, FALSE, ${quote('Probe recurring subscription')}, ${quote(`probe-invoice-${subscriptionId}`)})
       ON CONFLICT (user_id, subscription_id) DO UPDATE
         SET peer=EXCLUDED.peer, until_date=EXCLUDED.until_date, pricing_period=EXCLUDED.pricing_period,
             pricing_amount=EXCLUDED.pricing_amount, canceled=FALSE, can_refulfill=FALSE,
             missing_balance=FALSE, bot_canceled=FALSE, title=EXCLUDED.title,
             invoice_slug=EXCLUDED.invoice_slug, updated_at=CURRENT_TIMESTAMP`);
}

function cleanup(): void {
  if (process.env.KEEP_ROWS === '1') return;
  sql(`DELETE FROM apifull_payment_provider_stars_subscription WHERE user_id=${userId} AND subscription_id=${quote(subscriptionId)}`);
  sql(`DELETE FROM apifull_payment_provider_stars_account WHERE user_id=${userId}
       AND NOT EXISTS (SELECT 1 FROM apifull_payment_provider_stars_subscription WHERE user_id=${userId})`);
  sql(`DELETE FROM apifull_payment_provider_operation
       WHERE user_id=${userId} AND operation IN ('change_stars_subscription','fulfill_stars_subscription','bot_cancel_stars_subscription')
         AND created_at >= ${quote(startedAt)}::timestamptz`);
}

async function run(): Promise<void> {
  seed();
  const client = makeClient();
  const observed: Record<string, unknown> = {};
  try {
    await client.connect();
    const me = await client.getMe();
    if (String(me?.id) !== userId.toString()) throw new Error(`auth mismatch: ${String(me?.id)}`);
    const peer = new Api.InputPeerSelf();
    const getSubscriptions = () => client.invoke(new Api.payments.GetStarsSubscriptions({ peer, offset: '0' }));
    const statusBefore = await getSubscriptions();
    assertType(statusBefore, 'payments.StarsStatus', 'PaymentsStarsStatus', 'StarsStatus');
    if (!Array.isArray(statusBefore.subscriptions) || statusBefore.subscriptions.length !== 1 || String(statusBefore.subscriptions[0].id) !== subscriptionId) {
      throw new Error('provider did not return the seeded Stars subscription');
    }
    const changeRequest = new Api.payments.ChangeStarsSubscription({ peer, subscriptionId, canceled: true });
    const changed = await client.invoke(changeRequest);
    const changedReplay = await client.invoke(changeRequest);
    assertType(changed, 'BoolTrue', 'Boolean', 'boolTrue');
    assertType(changedReplay, 'BoolTrue', 'Boolean', 'boolTrue');
    const statusCanceled = await getSubscriptions();
    if (!statusCanceled.subscriptions[0].canceled || !statusCanceled.subscriptions[0].canRefulfill) {
      throw new Error('cancel mutation was not persisted by the provider');
    }
    const fulfillRequest = new Api.payments.FulfillStarsSubscription({ peer, subscriptionId });
    const fulfilled = await client.invoke(fulfillRequest);
    const fulfilledReplay = await client.invoke(fulfillRequest);
    assertType(fulfilled, 'BoolTrue', 'Boolean', 'boolTrue');
    assertType(fulfilledReplay, 'BoolTrue', 'Boolean', 'boolTrue');
    const statusFulfilled = await getSubscriptions();
    if (statusFulfilled.subscriptions[0].canceled === true || statusFulfilled.subscriptions[0].canRefulfill === true) {
      throw new Error('fulfill mutation was not persisted by the provider');
    }
    const botCancelRequest = makeBotCancelStarsSubscription(new Api.InputUser({ userId, accessHash: BigInt(0) }), subscriptionId, false);
    const botCanceled = await client.invoke(botCancelRequest);
    const botCanceledReplay = await client.invoke(botCancelRequest);
    assertType(botCanceled, 'BoolTrue', 'Boolean', 'boolTrue');
    assertType(botCanceledReplay, 'BoolTrue', 'Boolean', 'boolTrue');
    const statusBotCanceled = await getSubscriptions();
    if (!statusBotCanceled.subscriptions[0].canceled || !statusBotCanceled.subscriptions[0].botCanceled) {
      throw new Error('bot cancellation was not persisted by the provider');
    }
    const operationCounts = sql(`SELECT operation || '=' || count(*)::text
      FROM apifull_payment_provider_operation
      WHERE user_id=${userId} AND operation IN ('change_stars_subscription','fulfill_stars_subscription','bot_cancel_stars_subscription')
        AND created_at >= ${quote(startedAt)}::timestamptz
      GROUP BY operation ORDER BY operation`);
    if (operationCounts !== 'bot_cancel_stars_subscription=1\nchange_stars_subscription=1\nfulfill_stars_subscription=1') {
      throw new Error(`provider mutation idempotency rows = ${operationCounts}`);
    }
    observed.statusBefore = { type: typeName(statusBefore), subscriptions: statusBefore.subscriptions.length, balance: String(statusBefore.balance) };
    observed.cancel = { type: typeName(changed), replayType: typeName(changedReplay), canceled: Boolean(statusCanceled.subscriptions[0].canceled) };
    observed.fulfill = { type: typeName(fulfilled), replayType: typeName(fulfilledReplay), canceled: Boolean(statusFulfilled.subscriptions[0].canceled) };
    observed.botCancel = { type: typeName(botCanceled), replayType: typeName(botCanceledReplay), botCanceled: statusBotCanceled.subscriptions[0].botCanceled };
    observed.operationCounts = operationCounts.split('\n');
    console.log(JSON.stringify({ transport: `DC${dcId} TCPObfuscated -> gateway -> session -> APIFull -> payment provider -> PostgreSQL`, userId: userId.toString(), subscriptionId, observed }));
  } finally {
    client.destroy();
    cleanup();
  }
}

run().then(() => process.exit(0)).catch((error) => {
  console.error(`Stars subscription probe failed: ${error instanceof Error ? error.stack || error.message : String(error)}`);
  process.exit(1);
});
