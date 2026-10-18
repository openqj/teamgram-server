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
const userId = BigInt(process.env.USER_ID!);
const startedAt = Math.floor(Date.now() / 1000);
const suffix = `${Date.now()}-${process.pid}`;
const product = `probe-stars-catalog-${suffix}`;
const giftSlug = `probe-saved-gift-${suffix}`;
const starsIdem = `probe-stars-status-${suffix}`;
const starsDelta = 37;
let cleanupBeforeBalance = 0;
let cleanupHadBalance = false;
let provisioned = false;
const auctionPayload = '{"request":{"constructor":-1513074355}}';
const auctionResult = JSON.stringify({
  predicate_name: 'payments_starGiftActiveAuctions',
  auctions: [{
    predicate_name: 'starGiftActiveAuctionState',
    gift: {
      predicate_name: 'starGiftUnique',
      id: 900001,
      gift_id: 900002,
      title: 'Probe Auction Gift',
      slug: `probe-auction-${suffix}`,
      num: 1,
      attributes: [],
      availability_issued: 1,
      availability_total: 1,
    },
    state: { predicate_name: 'starGiftAuctionStateNotModified' },
    user_state: { predicate_name: 'starGiftAuctionUserState', acquired_count: 0 },
  }],
  users: [],
  chats: [],
});

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
  const container = process.env.TEAMGRAM_POSTGRES_CONTAINER || 'teamgram-postgres';
  return execFileSync('docker', [
    'exec', container, 'psql', '-v', 'ON_ERROR_STOP=1', '-At', '-q', '-F', '\t',
    '-U', 'teamgram', '-d', 'teamgram', '-c', query,
  ], { encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] }).trim();
}

function quote(value: string): string {
  return `'${value.replaceAll("'", "''")}'`;
}

function className(value: any): string {
  return value?.className || value?.constructor?.name || typeof value;
}

function isType(value: any, name: string): boolean {
  const actual = className(value);
  return actual === name || actual === `payments.${name}`;
}

function sha256(value: string): string {
  return createHash('sha256').update(value).digest('hex');
}

function tlInt(value: number): Uint8Array {
  const bytes = new Uint8Array(4);
  new DataView(bytes.buffer).setInt32(0, value, true);
  return bytes;
}

function tlLong(value: bigint): Uint8Array {
  const bytes = new Uint8Array(8);
  new DataView(bytes.buffer).setBigInt64(0, BigInt.asIntN(64, value), true);
  return bytes;
}

function rawRequest(constructorId: number, classNameValue: string, ...parts: Uint8Array[]): any {
  return {
    CONSTRUCTOR_ID: constructorId,
    SUBCLASS_OF_ID: 0,
    className: classNameValue,
    classType: 'request',
    getBytes: () => Uint8Array.from(Buffer.concat([
      Buffer.from(tlInt(constructorId)),
      ...parts.map((part) => Buffer.from(part)),
    ])),
    readResult: (reader: any) => reader.tgReadObject(),
  };
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
  const initWith = client._initWith.bind(client);
  client._initWith = (query: any) => {
    const original = initWith(query);
    if (className(original) !== 'InvokeWithLayer' || className(original.query) !== 'InitConnection') {
      throw new Error('INVALID_INIT_CONNECTION_WRAPPER');
    }
    return new Api.InvokeWithLayer({ layer: 229, query: original.query });
  };
  (client as any)._borrowExportedSender = async () => undefined;
  (client as any)._updateLoop = async () => undefined;
  return client;
}

function seed(beforeBalance: number): void {
  const providerFingerprint = sha256(auctionPayload);
  sql(`BEGIN;
    INSERT INTO apifull_stars_offer
      (kind, stars, store_product, currency, amount, extended, active)
      VALUES ('topup', 37, ${quote(product)}, 'USD', 370, 0, 1);
    INSERT INTO apifull_star_tx (user_id, amount, idem)
      VALUES (${userId}, ${starsDelta}, ${quote(starsIdem)});
    INSERT INTO apifull_stars (user_id, balance)
      VALUES (${userId}, ${beforeBalance + starsDelta})
      ON CONFLICT (user_id) DO UPDATE SET balance=EXCLUDED.balance;
    INSERT INTO apifull_gift (from_user, to_user, slug, stars, saved)
      VALUES (${userId}, ${userId}, ${quote(giftSlug)}, 37, 1);
    INSERT INTO teamgram_business_payment_operation
      (operation, user_id, request_fingerprint, result, expires_at)
      VALUES ('payments.getStarGiftActiveAuctions', ${userId}, decode(${quote(providerFingerprint)}, 'hex'),
              ${quote(auctionResult)}::jsonb, CURRENT_TIMESTAMP + INTERVAL '10 minutes');
    COMMIT;`);
}

function cleanup(beforeBalance: number, hadBalance: boolean): Record<string, number> {
  const providerFingerprint = sha256(auctionPayload);
  const hadGift = Number(sql(`SELECT count(*) FROM apifull_gift WHERE to_user=${userId} AND slug=${quote(giftSlug)}`));
  const result = sql(`BEGIN;
    DELETE FROM teamgram_business_payment_operation
      WHERE operation='payments.getStarGiftActiveAuctions' AND user_id=${userId}
        AND request_fingerprint=decode(${quote(providerFingerprint)}, 'hex');
    DELETE FROM apifull_gift WHERE to_user=${userId} AND slug=${quote(giftSlug)};
    DELETE FROM apifull_star_tx WHERE user_id=${userId} AND idem=${quote(starsIdem)};
    UPDATE apifull_stars SET balance=${beforeBalance} WHERE user_id=${userId};
    DELETE FROM apifull_stars WHERE user_id=${userId} AND ${hadBalance ? 'FALSE' : 'TRUE'};
    DELETE FROM apifull_stars_offer WHERE store_product=${quote(product)};
    COMMIT;`);
  void result;
  return {
    offers: Number(sql(`SELECT count(*) FROM apifull_stars_offer WHERE store_product=${quote(product)}`)),
    transactions: Number(sql(`SELECT count(*) FROM apifull_star_tx WHERE user_id=${userId} AND idem=${quote(starsIdem)}`)),
    gifts: Number(sql(`SELECT count(*) FROM apifull_gift WHERE to_user=${userId} AND slug=${quote(giftSlug)}`)),
    providerOperations: Number(sql(`SELECT count(*) FROM teamgram_business_payment_operation WHERE operation='payments.getStarGiftActiveAuctions' AND user_id=${userId} AND request_fingerprint=decode(${quote(providerFingerprint)}, 'hex')`)),
    balance: Number(sql(`SELECT COALESCE((SELECT balance FROM apifull_stars WHERE user_id=${userId}),0)`)),
    hadGift,
  };
}

async function run() {
  const beforeBalanceResult = sql(`SELECT balance FROM apifull_stars WHERE user_id=${userId}`);
  const hadBalance = beforeBalanceResult !== '';
  const beforeBalance = hadBalance ? Number(beforeBalanceResult) : 0;
  cleanupBeforeBalance = beforeBalance;
  cleanupHadBalance = hadBalance;
  seed(beforeBalance);
  provisioned = true;
  const seededGiftId = Number(sql(`SELECT id FROM apifull_gift WHERE to_user=${userId} AND slug=${quote(giftSlug)} ORDER BY id DESC LIMIT 1`));
  if (!Number.isSafeInteger(seededGiftId) || seededGiftId <= 0) throw new Error('saved-gift fixture did not return an id');
  const client = makeClient(process.env.AUTH_B64!);
  let observed: Record<string, unknown> = {};
  let cleanupEvidence: Record<string, number> | undefined;
  try {
    await client.connect();
    const me = await client.getMe();
    if (String(me?.id) !== userId.toString()) throw new Error(`auth mismatch: ${String(me?.id)}`);

    const topup = await client.invoke(new Api.payments.GetStarsTopupOptions({}));
    const option = Array.isArray(topup)
      ? topup.find((value: any) => value?.storeProduct?.value === product || value?.storeProduct === product)
      : null;
    if (!option || className(option) !== 'StarsTopupOption') {
      throw new Error(`topup catalog did not return ${product}: ${className(topup)}`);
    }

    const status = await client.invoke(new Api.payments.GetStarsStatus({ peer: new Api.InputPeerSelf() }));
    const statusBalance = Number(status?.balance?.amount ?? status?.balance?.amount_INT64 ?? status?.balance_INT64 ?? NaN);
    if (!isType(status, 'StarsStatus') || statusBalance !== beforeBalance + starsDelta) {
      throw new Error(`unexpected StarsStatus ${className(status)} balance=${statusBalance}`);
    }

    const history = await client.invoke(new Api.payments.GetStarsTransactions({
      peer: new Api.InputPeerSelf(),
      offset: '',
      limit: 10,
      ascending: true,
    }));
    const matchingHistory = Array.isArray(history?.history)
      ? history.history.filter((value: any) => String(value?.id) === starsIdem)
      : [];
    const historyAmount = matchingHistory.length === 1
      ? Number(matchingHistory[0]?.amount?.amount ?? matchingHistory[0]?.amount?.amount_INT64 ?? NaN)
      : NaN;
    const historyBalance = Number(history?.balance?.amount ?? history?.balance?.amount_INT64 ?? NaN);
    if (!isType(history, 'StarsStatus') || matchingHistory.length !== 1 || historyAmount !== starsDelta
      || historyBalance !== beforeBalance + starsDelta) {
      throw new Error(`unexpected StarsTransactions ${className(history)} balance=${historyBalance} matches=${matchingHistory.length} amount=${historyAmount}`);
    }

    const byId = await client.invoke(new Api.payments.GetStarsTransactionsByID({
      peer: new Api.InputPeerSelf(),
      id: [new Api.InputStarsTransaction({ id: starsIdem })],
    }));
    const byIdHistory = Array.isArray(byId?.history)
      ? byId.history.filter((value: any) => String(value?.id) === starsIdem)
      : [];
    const byIdAmount = byIdHistory.length === 1
      ? Number(byIdHistory[0]?.amount?.amount ?? byIdHistory[0]?.amount?.amount_INT64 ?? NaN)
      : NaN;
    const byIdBalance = Number(byId?.balance?.amount ?? byId?.balance?.amount_INT64 ?? NaN);
    if (!isType(byId, 'StarsStatus') || byIdHistory.length !== 1 || byIdAmount !== starsDelta
      || byIdBalance !== beforeBalance + starsDelta) {
      throw new Error(`unexpected StarsTransactionsByID ${className(byId)} balance=${byIdBalance} matches=${byIdHistory.length} amount=${byIdAmount}`);
    }

    const saved = await client.invoke(new Api.payments.GetSavedStarGifts({
      peer: new Api.InputPeerSelf(),
      offset: '',
      limit: 10,
    }));
    const savedGift = saved?.gifts?.find((value: any) => String(value?.savedId?.value ?? value?.savedId) === String(seededGiftId));
    if (!isType(saved, 'SavedStarGifts') || !savedGift || Number(saved.count) < 1) {
      const giftSummary = (saved?.gifts || []).map((value: any) => `${String(value?.gift?.slug)}:${className(value?.gift)}:${String(value?.savedId)}`).join(',');
      throw new Error(`saved gift not returned: ${className(saved)} count=${saved?.count} gifts=${giftSummary}`);
    }

    const auctions = await client.invoke(rawRequest(0xa5d0514d, 'payments.GetStarGiftActiveAuctions', tlLong(0n)));
    if (!isType(auctions, 'StarGiftActiveAuctions') || !Array.isArray(auctions?.auctions) || auctions.auctions.length !== 1) {
      throw new Error(`active auction fixture not returned: ${className(auctions)} count=${auctions?.auctions?.length}`);
    }
    observed = {
      'payments.getStarsTopupOptions': { constructor: className(topup), matchedProduct: option.storeProduct?.value ?? option.storeProduct, stars: Number(option.stars), amount: Number(option.amount) },
      'payments.getStarsStatus': { constructor: className(status), balance: statusBalance, historyCount: status.history?.length ?? 0 },
      'payments.getStarsTransactions': { constructor: className(history), balance: historyBalance, matchingHistoryCount: matchingHistory.length, id: String(matchingHistory[0].id), amount: historyAmount },
      'payments.getStarsTransactionsByID': { constructor: className(byId), balance: byIdBalance, matchingHistoryCount: byIdHistory.length, id: String(byIdHistory[0].id), amount: byIdAmount },
      'payments.getSavedStarGifts': { constructor: className(saved), count: Number(saved.count), giftCount: saved.gifts?.length ?? 0, savedId: String(savedGift.savedId?.value ?? savedGift.savedId ?? ''), expectedGiftId: seededGiftId },
      'payments.getStarGiftActiveAuctions': { constructor: className(auctions), auctionCount: auctions.auctions.length, giftSlug: auctions.auctions[0]?.gift?.slug },
    };
  } finally {
    try { client.destroy(); } catch { /* best effort */ }
    cleanupEvidence = cleanup(beforeBalance, hadBalance);
  }
  const remaining = ['offers', 'transactions', 'gifts', 'providerOperations']
    .some((key) => cleanupEvidence?.[key] !== 0) || cleanupEvidence.balance !== beforeBalance;
  console.log(JSON.stringify({
    transport: `DC${dcId} TCPObfuscated -> gateway -> session -> APIFull/PostgreSQL/business provider fixture`,
    userId: userId.toString(),
    fixture: { product, giftSlug, starsIdem, starsDelta },
    beforeBalance,
    observed,
    cleanup: cleanupEvidence,
  }));
  if (remaining) process.exitCode = 1;
}

run().then(() => {
  process.exit(process.exitCode || 0);
}).catch((error) => {
  console.error(`stars/gifts catalog probe failed: ${error instanceof Error ? error.stack || error.message : String(error)}`);
  if (provisioned) {
    try { cleanup(cleanupBeforeBalance, cleanupHadBalance); } catch { /* best effort cleanup */ }
  }
  process.exitCode = 1;
  setTimeout(() => process.exit(1), 250);
});
