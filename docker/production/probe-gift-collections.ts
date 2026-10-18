import { execFileSync } from 'node:child_process';
import path from 'node:path';
import { createRequire } from 'node:module';

import { loadFixtureAuthKey } from '../web-isolated/fixture-auth';

const gramjsDir = process.env.TEAMGRAM_GRAMJS_DIR || path.resolve(import.meta.dirname, '../../../telegram-tt-master');
const dcId = Number(process.env.TG_DC_ID || '2');
const dcHost = process.env.TG_DC_HOST || '127.0.0.1';
const dcPort = Number(process.env.TG_DC_PORT || '11443');
const apiId = Number(process.env.TG_API_ID || '1');
const apiHash = process.env.TG_API_HASH || '0123456789abcdef0123456789abcdef';
const userId = BigInt(process.env.USER_ID || '10');
const userUsername = process.env.USER_USERNAME || 'isolated_test_10';
const authB64 = process.env.AUTH_B64 || loadFixtureAuthKey(userUsername);
if (!authB64) throw new Error(`no active fixture auth key for ${userUsername}`);

for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) {
  delete process.env[key];
}

const tag = `probe-gift-collections-${Date.now()}-${process.pid}`;
const giftSlugs = [
  `${tag}-a`,
  `${tag}-b`,
  `${tag}-c`,
];
const firstTitle = `${tag}-first`;
const secondTitle = `${tag}-second`;
const updatedTitle = `${tag}-updated`;

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

function className(value: any): string {
  return value?.className || value?.constructor?.name || typeof value;
}

function requireType(value: any, expected: string, label: string): void {
  const actual = className(value);
  if (expected === 'BoolTrue' && ((typeof value === 'boolean' && value) || (actual === 'boolTrue' && value?.value === true))) return;
  if (actual !== expected && actual !== `payments.${expected}`) {
    throw new Error(`${label} returned ${actual}; expected ${expected}`);
  }
}

function concatBytes(...parts: Uint8Array[]): Uint8Array {
  return Uint8Array.from(Buffer.concat(parts.map((part) => Buffer.from(part))));
}

function tlInt(value: number): Uint8Array {
  const bytes = new Uint8Array(4);
  new DataView(bytes.buffer).setUint32(0, value >>> 0, true);
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

function rawCreate(peer: any, title: string, gifts: any[]): RawRequest {
  return new RawRequest(0x1f4a0e87, concatBytes(peer.getBytes(), tlString(title), tlVector(...gifts.map((gift) => gift.getBytes()))), 'payments.CreateStarGiftCollection');
}

function rawUpdate(peer: any, collectionId: number, title: string, deletes: any[], adds: any[], order: any[]): RawRequest {
  const flags = 1 | (deletes.length ? 2 : 0) | (adds.length ? 4 : 0) | (order.length ? 8 : 0);
  const parts = [tlInt(flags), peer.getBytes(), tlInt(collectionId), tlString(title)];
  if (deletes.length) parts.push(tlVector(...deletes.map((gift) => gift.getBytes())));
  if (adds.length) parts.push(tlVector(...adds.map((gift) => gift.getBytes())));
  if (order.length) parts.push(tlVector(...order.map((gift) => gift.getBytes())));
  return new RawRequest(0x4fddbee7, concatBytes(...parts), 'payments.UpdateStarGiftCollection');
}

function rawReorder(peer: any, order: number[]): RawRequest {
  return new RawRequest(0xc32af4cc, concatBytes(peer.getBytes(), tlVector(...order.map(tlInt))), 'payments.ReorderStarGiftCollections');
}

function rawDelete(peer: any, collectionId: number): RawRequest {
  return new RawRequest(0xad5648e8, concatBytes(peer.getBytes(), tlInt(collectionId)), 'payments.DeleteStarGiftCollection');
}

function makeClient(): any {
  const authKeyHex = Buffer.from(authB64!, 'base64').toString('hex');
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

function seed(): number[] {
  const values = giftSlugs.map((slug) => `(${userId},${userId},${quote(slug)},10,1)`).join(',');
  const ids = sql(`INSERT INTO apifull_gift (from_user,to_user,slug,stars,saved) VALUES ${values} RETURNING id`).split('\n').filter(Boolean).map(Number);
  if (ids.length !== giftSlugs.length || ids.some((id) => !Number.isSafeInteger(id) || id <= 0)) {
    throw new Error(`saved gift fixture insert returned ${ids.length} ids`);
  }
  return ids;
}

function cleanup(): Record<string, number> {
  const giftPredicate = giftSlugs.map((slug) => quote(slug)).join(',');
  const titlePredicate = [firstTitle, secondTitle, updatedTitle].map((title) => quote(title)).join(',');
  sql(`BEGIN;
    DELETE FROM apifull_gift_collection WHERE owner_user_id=${userId} AND title IN (${titlePredicate});
    DELETE FROM apifull_gift WHERE to_user=${userId} AND slug IN (${giftPredicate});
    COMMIT;`);
  return {
    collections: Number(sql(`SELECT count(*) FROM apifull_gift_collection WHERE owner_user_id=${userId} AND title IN (${titlePredicate})`)),
    collectionItems: Number(sql(`SELECT count(*) FROM apifull_gift_collection_item i JOIN apifull_gift_collection c ON c.id=i.collection_id WHERE c.owner_user_id=${userId} AND c.title IN (${titlePredicate})`)),
    gifts: Number(sql(`SELECT count(*) FROM apifull_gift WHERE to_user=${userId} AND slug IN (${giftPredicate})`)),
  };
}

function listHash(collections: any[]): bigint {
  let result = 0n;
  for (const collection of collections) {
    result = ((result * 20261n) + 0x80000000n + BigInt(collection.hash)) % 0x80000000n;
  }
  return result;
}

async function run(): Promise<void> {
  const giftIDs = seed();
  let client: any;
  let cleanupEvidence: Record<string, number> | undefined;
  const observed: Record<string, unknown> = {};
  try {
    client = makeClient();
    await client.connect();
    const me = await client.getMe();
    if (String(me?.id) !== userId.toString()) throw new Error(`auth mismatch: ${String(me?.id)}`);

    const peer = new Api.InputPeerSelf();
    const gifts = giftSlugs.map((slug) => new Api.InputSavedStarGiftSlug({ slug }));
    const first = await client.invoke(rawCreate(peer, firstTitle, [gifts[0]]));
    requireType(first, 'StarGiftCollection', 'payments.createStarGiftCollection');
    const collectionID = Number(first.collectionId);
    if (!Number.isSafeInteger(collectionID) || collectionID <= 0 || Number(first.giftsCount) !== 1) {
      throw new Error(`create returned invalid collection ${JSON.stringify({ id: first.collectionId, giftsCount: first.giftsCount })}`);
    }
    observed.create = { constructor: className(first), collectionId: collectionID, giftsCount: Number(first.giftsCount), hash: String(first.hash) };

    const initial = await client.invoke(new Api.payments.GetStarGiftCollections({ peer, hash: 0n }));
    requireType(initial, 'StarGiftCollections', 'payments.getStarGiftCollections');
    const initialCollection = initial.collections?.find((item: any) => Number(item.collectionId) === collectionID);
    if (!initialCollection || Number(initialCollection.giftsCount) !== 1) throw new Error('created collection missing from list readback');
    const initialHash = listHash(initial.collections || []);
    const notModified = await client.invoke(new Api.payments.GetStarGiftCollections({ peer, hash: initialHash }));
    requireType(notModified, 'StarGiftCollectionsNotModified', 'payments.getStarGiftCollections hash replay');
    observed.listReplay = { constructor: className(notModified), collectionCount: initial.collections?.length ?? 0, hash: initialHash.toString() };

    const updateRequest = rawUpdate(peer, collectionID, updatedTitle, [gifts[0]], [gifts[1], gifts[2]], [gifts[2], gifts[1]]);
    const updated = await client.invoke(updateRequest);
    requireType(updated, 'StarGiftCollection', 'payments.updateStarGiftCollection');
    if (updated.title !== updatedTitle || Number(updated.giftsCount) !== 2) throw new Error('update did not replace title/items');
    const updatedReplay = await client.invoke(updateRequest);
    requireType(updatedReplay, 'StarGiftCollection', 'payments.updateStarGiftCollection replay');
    if (updatedReplay.title !== updatedTitle || Number(updatedReplay.giftsCount) !== 2 || Number(updatedReplay.hash) !== Number(updated.hash)) {
      throw new Error('update replay changed the collection');
    }
    observed.update = { constructor: className(updated), replayConstructor: className(updatedReplay), collectionId: collectionID, giftsCount: Number(updated.giftsCount), hash: String(updated.hash) };

    const second = await client.invoke(rawCreate(peer, secondTitle, []));
    requireType(second, 'StarGiftCollection', 'payments.createStarGiftCollection second');
    const secondID = Number(second.collectionId);
    const reorderRequest = rawReorder(peer, [secondID, collectionID]);
    const reordered = await client.invoke(reorderRequest);
    requireType(reordered, 'BoolTrue', 'payments.reorderStarGiftCollections');
    const reorderedReplay = await client.invoke(reorderRequest);
    requireType(reorderedReplay, 'BoolTrue', 'payments.reorderStarGiftCollections replay');
    const afterReorder = await client.invoke(new Api.payments.GetStarGiftCollections({ peer, hash: 0n }));
    requireType(afterReorder, 'StarGiftCollections', 'payments.getStarGiftCollections after reorder');
    const orderIDs = (afterReorder.collections || []).filter((item: any) => [collectionID, secondID].includes(Number(item.collectionId))).map((item: any) => Number(item.collectionId));
    if (orderIDs[0] !== secondID || orderIDs[1] !== collectionID) throw new Error(`collection reorder readback mismatch: ${orderIDs.join(',')}`);
    observed.reorder = { constructor: className(reordered), replayConstructor: className(reorderedReplay), order: orderIDs };

    const deletedFirst = await client.invoke(rawDelete(peer, collectionID));
    requireType(deletedFirst, 'BoolTrue', 'payments.deleteStarGiftCollection');
    const deletedSecond = await client.invoke(rawDelete(peer, secondID));
    requireType(deletedSecond, 'BoolTrue', 'payments.deleteStarGiftCollection second');
    observed.delete = { constructor: className(deletedFirst), secondConstructor: className(deletedSecond), collectionIds: [collectionID, secondID] };
  } finally {
    try { client?.destroy(); } catch { /* best effort */ }
    cleanupEvidence = cleanup();
  }
  const remaining = Object.values(cleanupEvidence || {}).some((value) => value !== 0);
  console.log(JSON.stringify({
    transport: `DC${dcId} TCPObfuscated -> gateway -> session -> APIFull/PostgreSQL`,
    userId: userId.toString(),
    methods: [
      'payments.createStarGiftCollection',
      'payments.getStarGiftCollections',
      'payments.updateStarGiftCollection',
      'payments.reorderStarGiftCollections',
      'payments.deleteStarGiftCollection',
    ],
    fixture: { tag, giftIDs, giftSlugs },
    observed,
    updateEvidence: {
      updatesReturned: false,
      note: 'Layer 229 defines this method family as StarGiftCollection, payments.StarGiftCollections and Bool results; exact replay was asserted for list hash, update and reorder.',
    },
    cleanup: cleanupEvidence,
    providerLimitation: 'No external gift inventory/ownership provider was available; this run proves the signed PostgreSQL fixture boundary only.',
  }));
  if (remaining) process.exitCode = 1;
}

run().then(() => process.exit(process.exitCode || 0)).catch((error) => {
  try { cleanup(); } catch { /* best effort */ }
  console.error(`gift collections probe failed: ${error instanceof Error ? error.stack || error.message : String(error)}`);
  process.exitCode = 1;
  setTimeout(() => process.exit(1), 250);
});
