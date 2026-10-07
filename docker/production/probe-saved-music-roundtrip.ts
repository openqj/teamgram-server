import path from 'node:path';
import { createRequire } from 'node:module';

for (const key of ['TEAMGRAM_GRAMJS_DIR', 'A_AUTH_B64', 'A_USER_ID', 'DOCUMENT_ID', 'DOCUMENT_ACCESS_HASH']) {
  if (!process.env[key]) throw new Error(`${key} is required`);
}

const root = process.env.TEAMGRAM_GRAMJS_DIR!;
const dcId = Number(process.env.TG_DC_ID || '2');
const requireModule = createRequire(import.meta.url);
const TelegramClient = requireModule(path.join(root, 'src/lib/gramjs/client/TelegramClient.ts')).default;
const CallbackSession = requireModule(path.join(root, 'src/lib/gramjs/sessions/CallbackSession.ts')).default;
const Api = requireModule(path.join(root, 'src/lib/gramjs/tl/index.ts')).Api;
const Connection = requireModule(path.join(root, 'src/lib/gramjs/network/connection/TCPObfuscated.ts')).ConnectionTCPObfuscated;
const quietLogger = { debug() {}, info() {}, warn() {}, error() {} };
const globalAny = globalThis as typeof globalThis & { self?: any; addEventListener?: () => void };
globalAny.self ??= globalThis;
globalAny.addEventListener ??= () => {};
globalAny.self.addEventListener ??= globalAny.addEventListener;

function rpcName(value: any): string {
  return value?.className || value?.constructor?.name || typeof value;
}

function isTrue(value: any): boolean {
  if (value === true) return true;
  if (value instanceof Boolean) return value.valueOf();
  return rpcName(value).toLowerCase().includes('true');
}

function makeClient(): any {
  const session = new CallbackSession({
    mainDcId: dcId,
    keys: { [dcId]: Buffer.from(process.env.A_AUTH_B64!, 'base64').toString('hex') },
  }, () => {});
  const load = session.load.bind(session);
  session.load = async () => {
    await load();
    (session as any)._serverAddress = process.env.TG_DC_HOST || '127.0.0.1';
    (session as any)._port = Number(process.env.TG_DC_PORT || '11443');
  };
  return new TelegramClient(session, 1, '0123456789abcdef0123456789abcdef', {
    connection: Connection,
    fallbackConnection: Connection,
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
}

function idsOf(value: any): string[] {
  return (Array.isArray(value?.ids) ? value.ids : []).map((id: any) => String(id));
}

function savedMusicHash(ids: string[]): bigint {
  const sorted = ids.map(BigInt).sort((a, b) => (a < b ? -1 : a > b ? 1 : 0));
  let hash = 0n;
  const mask = (1n << 64n) - 1n;
  for (const id of sorted) {
    hash ^= hash >> 21n;
    hash ^= (hash << 35n) & mask;
    hash ^= hash >> 4n;
    hash = (hash + (id & mask)) & mask;
  }
  return hash >= (1n << 63n) ? hash - (1n << 64n) : hash;
}

async function main() {
  const client = makeClient();
  const document = new Api.InputDocument({
    id: BigInt(process.env.DOCUMENT_ID!),
    accessHash: BigInt(process.env.DOCUMENT_ACCESS_HASH!),
    fileReference: new Uint8Array(),
  });
  let saved = false;
  try {
    await client.connect();
    const me = await client.getMe();
    if (String(me?.id) !== String(process.env.A_USER_ID)) throw new Error('auth key user mismatch');

    const before = await client.invoke(new Api.account.GetSavedMusicIds({ hash: 0n }));
    const save = await client.invoke(new Api.account.SaveMusic({ id: document }));
    if (!isTrue(save)) throw new Error(`account.saveMusic returned ${rpcName(save)}`);
    saved = true;
    const read = await client.invoke(new Api.account.GetSavedMusicIds({ hash: 0n }));
    const ids = idsOf(read);
    if (!ids.includes(String(process.env.DOCUMENT_ID))) {
      throw new Error(`saved document was not returned: ${ids.join(',')}`);
    }
    const savedMusic = await client.invoke(new Api.users.GetSavedMusic({
      id: new Api.InputUserSelf(),
      offset: 0,
      limit: 20,
      hash: 0n,
    }));
    const savedMusicDocuments = Array.isArray(savedMusic?.documents) ? savedMusic.documents : [];
    if (!rpcName(savedMusic).toLowerCase().includes('savedmusic')
      || !savedMusicDocuments.some((item: any) => String(item?.id) === String(process.env.DOCUMENT_ID))) {
      throw new Error(`users.getSavedMusic did not hydrate the saved document: ${rpcName(savedMusic)} ${savedMusicDocuments.map((item: any) => String(item?.id)).join(',')}`);
    }
    const savedMusicByID = await client.invoke(new Api.users.GetSavedMusicByID({
      id: new Api.InputUserSelf(),
      documents: [document],
    }));
    const byIDDocuments = Array.isArray(savedMusicByID?.documents) ? savedMusicByID.documents : [];
    if (!rpcName(savedMusicByID).toLowerCase().includes('savedmusic')
      || !byIDDocuments.some((item: any) => String(item?.id) === String(process.env.DOCUMENT_ID))) {
      throw new Error(`users.getSavedMusicByID did not return the saved document: ${rpcName(savedMusicByID)}`);
    }
    const notModified = await client.invoke(new Api.account.GetSavedMusicIds({ hash: savedMusicHash(ids) }));
    if (!rpcName(notModified).toLowerCase().includes('notmodified')) {
      throw new Error(`saved music hash was not honored: ${rpcName(notModified)}`);
    }
    const unsave = await client.invoke(new Api.account.SaveMusic({ id: document, unsave: true }));
    if (!isTrue(unsave)) throw new Error(`account.saveMusic unsave returned ${rpcName(unsave)}`);
    saved = false;
    const after = await client.invoke(new Api.account.GetSavedMusicIds({ hash: 0n }));
    const afterIds = idsOf(after);
    if (afterIds.includes(String(process.env.DOCUMENT_ID))) throw new Error('saved document remained after unsave');
    console.log(JSON.stringify({
      userId: String(me.id),
      beforeType: rpcName(before),
      beforeIds: idsOf(before),
      saveType: rpcName(save),
      readType: rpcName(read),
      readIds: ids,
      hashProbeType: rpcName(notModified),
      savedMusicType: rpcName(savedMusic),
      savedMusicDocumentCount: savedMusicDocuments.length,
      savedMusicByIDType: rpcName(savedMusicByID),
      savedMusicByIDDocumentCount: byIDDocuments.length,
      unsaveType: rpcName(unsave),
      afterType: rpcName(after),
      afterIds,
      documentId: String(process.env.DOCUMENT_ID),
    }));
  } finally {
    if (saved) {
      try { await client.invoke(new Api.account.SaveMusic({ id: document, unsave: true })); } catch {}
    }
    client.destroy();
  }
}

main().then(() => process.exit(0)).catch((error) => {
  console.error(`saved music roundtrip failed: ${error instanceof Error ? error.stack || error.message : String(error)}`);
  process.exit(1);
});
