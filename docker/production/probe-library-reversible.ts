import path from 'node:path';
import { createRequire } from 'node:module';

for (const key of ['TEAMGRAM_GRAMJS_DIR', 'A_AUTH_B64']) {
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

function makeClient(auth: string): any {
  const session = new CallbackSession({ mainDcId: dcId, keys: { [dcId]: Buffer.from(auth, 'base64').toString('hex') } }, () => {});
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

async function main() {
  const client = makeClient(process.env.A_AUTH_B64!);
  const mode = process.env.GIF_MODE || 'roundtrip';
  const documentId = BigInt(process.env.GIF_DOCUMENT_ID || `9${Date.now()}${process.pid}`);
  const document = new Api.InputDocument({
    id: documentId,
    accessHash: 7788990011n,
    fileReference: new Uint8Array([1, 2, 3, 4]),
  });
  try {
    await client.connect();
    const before = await client.invoke(new Api.messages.GetSavedGifs({ hash: 0n }));
    if (mode === 'read') {
      const found = (before?.gifs || []).some((item: any) => String(item?.id) === String(documentId));
      if (!found) throw new Error('saved GIF was not preserved across the BFF restart');
      const unsaved = await client.invoke(new Api.messages.SaveGif({ id: document, unsave: true }));
      if (!isTrue(unsaved)) throw new Error(`messages.saveGif cleanup returned ${rpcName(unsaved)}`);
      const after = await client.invoke(new Api.messages.GetSavedGifs({ hash: 0n }));
      if ((after?.gifs || []).some((item: any) => String(item?.id) === String(documentId))) throw new Error('saved GIF remained after restart cleanup');
      console.log(JSON.stringify({ mode, before: rpcName(before), found: true, unsave: rpcName(unsaved), after: rpcName(after), documentId: String(documentId) }));
      return;
    }
    const saved = await client.invoke(new Api.messages.SaveGif({ id: document, unsave: false }));
    if (!isTrue(saved)) throw new Error(`messages.saveGif returned ${rpcName(saved)}`);
    const read = await client.invoke(new Api.messages.GetSavedGifs({ hash: 0n }));
    const found = (read?.gifs || []).some((item: any) => String(item?.id) === String(documentId));
    if (!found) throw new Error('saved GIF was not returned by messages.getSavedGifs');
    const effects = await client.invoke(new Api.messages.GetAvailableEffects({ hash: 0 }));
    const recent = await client.invoke(new Api.messages.GetRecentReactions({ limit: 20, hash: 0n }));
    if (mode === 'save') {
      console.log(JSON.stringify({ mode, before: rpcName(before), save: rpcName(saved), read: rpcName(read), found: true, documentId: String(documentId) }));
      return;
    }
    const unsaved = await client.invoke(new Api.messages.SaveGif({ id: document, unsave: true }));
    if (!isTrue(unsaved)) throw new Error(`messages.saveGif unsave returned ${rpcName(unsaved)}`);
    const after = await client.invoke(new Api.messages.GetSavedGifs({ hash: 0n }));
    if ((after?.gifs || []).some((item: any) => String(item?.id) === String(documentId))) throw new Error('saved GIF remained after unsave');
    console.log(JSON.stringify({
      before: rpcName(before), save: rpcName(saved), read: rpcName(read),
      effects: rpcName(effects), effectCount: effects?.effects?.length || 0,
      recentReactions: rpcName(recent), recentCount: recent?.reactions?.length || 0,
      unsave: rpcName(unsaved), after: rpcName(after), documentId: String(documentId),
    }));
  } finally {
    if (mode !== 'save') {
      try { await client.invoke(new Api.messages.SaveGif({ id: document, unsave: true })); } catch {}
    }
    client.destroy();
  }
}

main().then(() => process.exit(0)).catch((error) => {
  console.error(String(error));
  process.exit(1);
});
