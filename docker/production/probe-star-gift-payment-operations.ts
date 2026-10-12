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
const giftId = 101n;
const giftSlug = 'probe-gift';
const auctionSlug = 'probe-auction';
const startedAt = Math.floor(Date.now() / 1000);

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

function sql(query: string): string {
  const container = process.env.TEAMGRAM_POSTGRES_CONTAINER || 'teamgram-postgres';
  return execFileSync('docker', [
    'exec', container, 'psql', '-v', 'ON_ERROR_STOP=1', '-At', '-q', '-U', 'teamgram', '-d', 'teamgram', '-c', query,
  ], { encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] }).trim();
}

function quote(value: string): string {
  return `'${value.replaceAll("'", "''")}'`;
}

function fingerprint(payload: string): string {
  return createHash('sha256').update(payload).digest('hex');
}

function makeClient(authB64: string): any {
  const keyHex = Buffer.from(authB64, 'base64').toString('hex');
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

function concatBytes(...parts: Uint8Array[]): Uint8Array {
  return Uint8Array.from(Buffer.concat(parts.map((part) => Buffer.from(part))));
}

function tlInt(value: number): Uint8Array {
  const bytes = Buffer.alloc(4);
  bytes.writeUInt32LE(value >>> 0);
  return bytes;
}

function tlLong(value: bigint): Uint8Array {
  const bytes = Buffer.alloc(8);
  bytes.writeBigInt64LE(BigInt.asIntN(64, value));
  return bytes;
}

function tlString(value: string): Uint8Array {
  return Uint8Array.from(serializeBytes(value));
}

function tlVector(...values: Uint8Array[]): Uint8Array {
  return concatBytes(tlInt(0x1cb5c415), tlInt(values.length), ...values);
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

function savedGift(): any {
  return new Api.InputSavedStarGiftSlug({ slug: giftSlug });
}

function auction(): any {
  return new Api.InputStarGiftAuctionSlug({ slug: auctionSlug });
}

function raw(name: string): RawRequest {
  switch (name) {
    case 'payments.getStarGiftUpgradePreview':
      return new RawRequest(0x9c9abcb1, tlLong(giftId), name);
    case 'payments.upgradeStarGift':
      return new RawRequest(0xaed6e4f5, concatBytes(tlInt(0), savedGift().getBytes()), name);
    case 'payments.getStarGiftWithdrawalUrl':
      return new RawRequest(0xd06e93a8, concatBytes(savedGift().getBytes(), new Api.InputCheckPasswordEmpty().getBytes()), name);
    case 'payments.getResaleStarGifts':
      return new RawRequest(0x7a5fa236, concatBytes(tlInt(0), tlLong(giftId), tlString(''), tlInt(10)), name);
    case 'payments.updateStarGiftPrice':
      return new RawRequest(0xedbe6ccb, concatBytes(savedGift().getBytes(), new Api.StarsAmount({ amount: 25n }).getBytes()), name);
    case 'payments.checkCanSendGift':
      return new RawRequest(0xc0c4edc9, tlLong(giftId), name);
    case 'payments.getStarGiftAuctionState':
      return new RawRequest(0x5c9ff4d6, concatBytes(auction().getBytes(), tlInt(0)), name);
    case 'payments.getStarGiftAuctionAcquiredGifts':
      return new RawRequest(0x6ba2cbec, tlLong(giftId), name);
    case 'payments.getStarGiftActiveAuctions':
      return new RawRequest(0xa5d0514d, tlLong(0n), name);
    case 'payments.resolveStarGiftOffer':
      return new RawRequest(0xe9ce781c, concatBytes(tlInt(0), tlInt(17)), name);
    case 'payments.sendStarGiftOffer':
      return new RawRequest(0x8fb86b41, concatBytes(
        tlInt(0), new Api.InputPeerSelf().getBytes(), tlString(giftSlug),
        new Api.StarsAmount({ amount: 25n }).getBytes(), tlInt(7), tlLong(9001n),
      ), name);
    case 'payments.getStarGiftUpgradeAttributes':
      return new RawRequest(0x6d038b58, tlLong(giftId), name);
    case 'payments.getCraftStarGifts':
      return new RawRequest(0xfd05dd00, concatBytes(tlLong(giftId), tlString(''), tlInt(10)), name);
    case 'payments.craftStarGift':
      return new RawRequest(0xb0f9684f, tlVector(savedGift().getBytes()), name);
    case 'payments.launchPrepaidGiveaway':
      return new RawRequest(0x5ff58f20, concatBytes(
        new Api.InputPeerSelf().getBytes(), tlLong(9001n),
        new Api.InputStorePaymentStarsGiveaway({
          stars: 25n,
          boostPeer: new Api.InputPeerSelf(),
          randomId: 9001n,
          untilDate: 1760000000,
          currency: 'XTR',
          amount: 25n,
          users: 1,
        }).getBytes(),
      ), name);
    default:
      throw new Error(`unsupported request ${name}`);
  }
}

const updates = JSON.stringify({ predicate_name: 'updates', updates: [], users: [], chats: [], date: 1760000000, seq: 0 });
const payloads: Record<string, string> = {
  'payments.launchPrepaidGiveaway': '{"giveaway_id":9001,"peer":{"predicate_name":"inputPeerSelf","constructor":2107670217},"purpose":{"predicate_name":"inputStorePaymentStarsGiveaway","constructor":1964968186,"currency":"XTR","amount":25,"boost_peer":{"predicate_name":"inputPeerSelf","constructor":2107670217},"random_id":9001,"until_date":1760000000,"stars":25,"users_INT32":1}}',
  'payments.getStarGiftUpgradePreview': '{"gift_id":101}',
  'payments.upgradeStarGift': '{"keep_original_details":false,"msg_id":0,"stargift":{"predicate_name":"inputSavedStarGiftSlug","constructor":545636920,"slug":"probe-gift"}}',
  'payments.getStarGiftWithdrawalUrl': '{"request":{"constructor":-798059608,"stargift":{"predicate_name":"inputSavedStarGiftSlug","constructor":545636920,"slug":"probe-gift"},"password":{"predicate_name":"inputCheckPasswordEmpty","constructor":-1736378792}}}',
  'payments.getResaleStarGifts': '{"request":{"constructor":2053087798,"gift_id":101,"limit":10}}',
  'payments.updateStarGiftPrice': '{"request":{"constructor":-306287413,"stargift":{"predicate_name":"inputSavedStarGiftSlug","constructor":545636920,"slug":"probe-gift"},"resell_amount":{"predicate_name":"starsAmount","constructor":-1145654109,"amount":25}}}',
  'payments.checkCanSendGift': '{"request":{"constructor":-1060835895,"gift_id":101}}',
  'payments.getStarGiftAuctionState': '{"request":{"constructor":1553986774,"auction":{"predicate_name":"inputStarGiftAuctionSlug","constructor":2058715912,"slug":"probe-auction"}}}',
  'payments.getStarGiftAuctionAcquiredGifts': '{"request":{"constructor":1805831148,"gift_id":101}}',
  'payments.getStarGiftActiveAuctions': '{"request":{"constructor":-1513074355}}',
  'payments.resolveStarGiftOffer': '{"request":{"constructor":-372344804,"offer_msg_id":17}}',
  'payments.sendStarGiftOffer': '{"request":{"constructor":-1883739327,"peer":{"predicate_name":"inputPeerSelf","constructor":2107670217},"slug":"probe-gift","price":{"predicate_name":"starsAmount","constructor":-1145654109,"amount":25},"duration":7,"random_id":9001}}',
  'payments.getStarGiftUpgradeAttributes': '{"request":{"constructor":1828948824,"gift_id":101}}',
  'payments.getCraftStarGifts': '{"request":{"constructor":-49947392,"gift_id":101,"limit":10}}',
  'payments.craftStarGift': '{"request":{"constructor":-1325832113,"stargift":[{"predicate_name":"inputSavedStarGiftSlug","constructor":545636920,"slug":"probe-gift"}]}}',
};

const results: Record<string, string> = {
  'payments.launchPrepaidGiveaway': updates,
  'payments.getStarGiftUpgradePreview': '{"predicate_name":"payments_starGiftUpgradePreview","sample_attributes":[],"prices":[],"next_prices":[]}',
  'payments.upgradeStarGift': updates,
  'payments.getStarGiftWithdrawalUrl': '{"predicate_name":"payments_starGiftWithdrawalUrl","url":"https://provider.example/gift/withdraw"}',
  'payments.getResaleStarGifts': '{"predicate_name":"payments_resaleStarGifts","count":0,"gifts":[],"chats":[],"users":[]}',
  'payments.updateStarGiftPrice': updates,
  'payments.checkCanSendGift': '{"predicate_name":"payments_checkCanSendGiftResultOk"}',
  'payments.getStarGiftAuctionState': '{"predicate_name":"payments_starGiftAuctionState","gift":{"predicate_name":"starGiftUnique","id":1,"gift_id":101,"title_STRING":"Probe","slug":"probe-unique","num":1,"attributes":[],"availability_issued":1,"availability_total_INT32":1},"state":{"predicate_name":"starGiftAuctionStateNotModified"},"user_state":{"predicate_name":"starGiftAuctionUserState","acquired_count":0},"timeout":0,"users":[],"chats":[]}',
  'payments.getStarGiftAuctionAcquiredGifts': '{"predicate_name":"payments_starGiftAuctionAcquiredGifts","gifts":[],"users":[],"chats":[]}',
  'payments.getStarGiftActiveAuctions': '{"predicate_name":"payments_starGiftActiveAuctionsNotModified","auctions":[],"users":[],"chats":[]}',
  'payments.resolveStarGiftOffer': updates,
  'payments.sendStarGiftOffer': updates,
  'payments.getStarGiftUpgradeAttributes': '{"predicate_name":"payments_starGiftUpgradeAttributes","attributes":[]}',
  'payments.getCraftStarGifts': '{"predicate_name":"payments_savedStarGifts","count":0,"gifts":[],"chats":[],"users":[]}',
  'payments.craftStarGift': updates,
};

type Seed = { operation: string; fingerprint: string };

function seed(): Seed[] {
  const entries = Object.keys(payloads).map((operation) => ({
    operation,
    fingerprint: fingerprint(payloads[operation]),
  }));
  const values = entries.map((entry) => `(${quote(entry.operation)}, ${userId}, decode(${quote(entry.fingerprint)}, 'hex'), ${quote(results[entry.operation])}::jsonb, CURRENT_TIMESTAMP + INTERVAL '10 minutes')`).join(',\n');
  sql(`INSERT INTO teamgram_business_payment_operation
    (operation, user_id, request_fingerprint, result, expires_at)
    VALUES ${values}
    ON CONFLICT (operation, user_id, request_fingerprint) DO UPDATE
      SET result=EXCLUDED.result, expires_at=EXCLUDED.expires_at, updated_at=CURRENT_TIMESTAMP`);
  return entries;
}

function cleanup(entries: Seed[]): number {
  if (process.env.KEEP_ROWS === '1') return 0;
  const clauses = entries.map((entry) => `(${quote(entry.operation)}, ${userId}, ${quote(entry.fingerprint)})`).join(', ');
  return Number(sql(`WITH deleted AS (
    DELETE FROM teamgram_business_payment_operation AS operation
     WHERE (operation.operation, operation.user_id, encode(operation.request_fingerprint, 'hex')) IN (${clauses})
     RETURNING operation.operation
  ) SELECT count(*) FROM deleted`));
}

function rpcName(value: any): string {
  return value?.className || value?.constructor?.name || typeof value;
}

function assertResult(operation: string, result: any): void {
  const name = rpcName(result);
  if (operation === 'payments.getStarGiftActiveAuctions' && name.includes('StarGiftActiveAuctions')) return;
  if (operation === 'payments.getStarGiftAuctionState' && name.includes('StarGiftAuctionState')) return;
  const expected = operation.split('.')[1].replace(/^get/, '').replace(/^upgrade/, '').replace(/^send/, '').replace(/^resolve/, '').replace(/^craft/, '').replace(/^update/, '').replace(/^check/, 'Check');
  if (!name || name === 'Object' || name === 'undefined') throw new Error(`${operation} returned ${name}; expected typed Layer 229 result`);
  if (result?.constructorId === undefined && result?.className === undefined && !name.includes(expected)) {
    throw new Error(`${operation} returned untyped result ${name}`);
  }
}

async function run() {
  const entries = seed();
  const client = makeClient(process.env.AUTH_B64!);
  const observed: Record<string, unknown> = {};
  let cleanupCount = 0;
  try {
    await client.connect();
    const me = await client.getMe();
    if (String(me?.id) !== userId.toString()) throw new Error(`auth mismatch: ${String(me?.id)}`);
    for (const operation of Object.keys(payloads)) {
      try {
        const result = await client.invoke(raw(operation));
        assertResult(operation, result);
        observed[operation] = {
          constructor: rpcName(result),
          stateConstructor: result?.state?.className || result?.state?.constructor?.name,
          count: result?.count,
          url: result?.url,
          date: result?.date,
          seq: result?.seq,
        };
      } catch (error) {
        observed[operation] = { error: error instanceof Error ? error.message : String(error) };
      }
    }
  } finally {
    client.disconnect();
    cleanupCount = cleanup(entries);
  }
  const failures = Object.entries(observed).filter(([, value]) => (value as any)?.error);
  const remaining = Number(sql(`SELECT count(*) FROM teamgram_business_payment_operation WHERE user_id=${userId} AND updated_at >= to_timestamp(${startedAt})`));
  console.log(JSON.stringify({
    transport: `DC${dcId} TCPObfuscated -> gateway -> session -> APIFull/business provider/PostgreSQL`,
    userId: userId.toString(),
    methods: Object.keys(payloads).length,
    seeded: entries.length,
    cleaned: cleanupCount,
    remaining,
    observed,
  }));
  if (failures.length > 0 || cleanupCount !== entries.length || remaining !== 0) process.exitCode = 1;
  setTimeout(() => process.exit(process.exitCode || 0), 250);
}

run().catch((error) => {
  try { cleanup(Object.keys(payloads).map((operation) => ({ operation, fingerprint: fingerprint(payloads[operation]) }))); } catch { /* best effort cleanup */ }
  console.error(`star gift payment probe failed: ${error instanceof Error ? error.stack || error.message : String(error)}`);
  process.exitCode = 1;
  setTimeout(() => process.exit(1), 250);
});
