import path from 'node:path';
import { createRequire } from 'node:module';

const root = process.env.TEAMGRAM_GRAMJS_DIR!;
const A_ID = BigInt(process.env.A_USER_ID!);
const B_ID = BigInt(process.env.B_USER_ID!);
const A_HASH = BigInt(process.env.A_ACCESS_HASH!);
const B_HASH = BigInt(process.env.B_ACCESS_HASH!);
const dcId = Number(process.env.TG_DC_ID || '2');
const requireModule = createRequire(import.meta.url);
const TelegramClient = requireModule(path.join(root, 'src/lib/gramjs/client/TelegramClient.ts')).default;
const CallbackSession = requireModule(path.join(root, 'src/lib/gramjs/sessions/CallbackSession.ts')).default;
const Api = requireModule(path.join(root, 'src/lib/gramjs/tl/index.ts')).Api;
const Connection = requireModule(path.join(root, 'src/lib/gramjs/network/connection/TCPObfuscated.ts')).ConnectionTCPObfuscated;
const quietLogger = { debug() {}, info() {}, warn() {}, error() {} };
const globalAny = globalThis as typeof globalThis & { self?: any; addEventListener?: () => void };
globalAny.self ??= globalThis; globalAny.addEventListener ??= () => {}; globalAny.self.addEventListener ??= globalAny.addEventListener;

function makeClient(auth: string): any {
  const session = new CallbackSession({ mainDcId: dcId, keys: { [dcId]: Buffer.from(auth, 'base64').toString('hex') } }, () => {});
  const load = session.load.bind(session);
  session.load = async () => { await load(); (session as any)._serverAddress = process.env.TG_DC_HOST || '127.0.0.1'; (session as any)._port = Number(process.env.TG_DC_PORT || '11443'); };
  return new TelegramClient(session, 1, '0123456789abcdef0123456789abcdef', {
    connection: Connection, fallbackConnection: Connection, shouldAllowHttpTransport: false, shouldForceHttpTransport: false,
    timeout: 20, requestRetries: 2, connectionRetries: 2, connectionRetriesToFallback: 0, retryDelay: 250, autoReconnect: false, baseLogger: quietLogger,
  });
}
function name(v: any): string { return v?.className || v?.constructor?.name || typeof v; }
function history(peer: any): any { return new Api.messages.GetHistory({ peer, offsetId: 0, offsetDate: 0, addOffset: 0, limit: 100, maxId: 0, minId: 0, hash: 0n }); }
function findMessage(value: any, text: string): any { return (value?.messages || []).find((m: any) => m?.message === text); }

async function main() {
  const a = makeClient(process.env.A_AUTH_B64!); const b = makeClient(process.env.B_AUTH_B64!);
  const tag = `prod-delete-history-${Date.now()}-${process.pid}`;
  let groupId: bigint | number | undefined; let senderId = 0; let receiverId = 0;
  try {
    await a.connect(); await b.connect();
    if (process.env.CLEANUP_CHAT_ID) {
      const chatId = BigInt(process.env.CLEANUP_CHAT_ID);
      let deleted = false;
      try { await a.invoke(new Api.messages.DeleteChat({ chatId })); deleted = true; } catch {}
      console.log(JSON.stringify({ cleanupChatId: process.env.CLEANUP_CHAT_ID, deleted }));
      return;
    }
    if (process.env.CLEANUP_TITLE) {
      const dialogs = await a.invoke(new Api.messages.GetDialogs({ offsetDate: 0, offsetId: 0, offsetPeer: new Api.InputPeerEmpty(), limit: 200, hash: 0n }));
      const stale = (dialogs?.chats || []).find((c: any) => c?.title === process.env.CLEANUP_TITLE);
      if (stale?.id !== undefined) await a.invoke(new Api.messages.DeleteChat({ chatId: stale.id }));
      console.log(JSON.stringify({ cleanupTitle: process.env.CLEANUP_TITLE, found: stale?.id !== undefined }));
      return;
    }
    const created = await a.invoke(new Api.messages.CreateChat({ users: [new Api.InputUser({ userId: B_ID, accessHash: B_HASH })], title: `${tag} group` }));
    let group = (created?.chats || []).find((c: any) => c?.title === `${tag} group`);
    for (let attempt = 0; !group && attempt < 8; attempt += 1) {
      const dialogs = await a.invoke(new Api.messages.GetDialogs({ offsetDate: 0, offsetId: 0, offsetPeer: new Api.InputPeerEmpty(), limit: 200, hash: 0n }));
      group = (dialogs?.chats || []).find((c: any) => c?.title === `${tag} group`);
      if (!group) await new Promise((resolve) => setTimeout(resolve, 250));
    }
    if (!group?.id) throw new Error(`createChat returned ${name(created)} without group`);
    groupId = group.id;
    const peer = new Api.InputPeerChat({ chatId: groupId });
    const sent = await a.invoke(new Api.messages.SendMessage({ peer, message: tag, randomId: BigInt(Date.now()) * 1000n + 1n }));
    const sender = findMessage(await a.invoke(history(peer)), tag); const receiver = findMessage(await b.invoke(history(peer)), tag);
    if (!sender?.id || !receiver?.id) throw new Error('temporary group message was not visible to both users');
    senderId = Number(sender.id); receiverId = Number(receiver.id);
    const deleted = await a.invoke(new Api.messages.DeleteHistory({ peer, justClear: true, revoke: true, maxId: senderId, minDate: 0, maxDate: 0 }));
    const aAfter = await a.invoke(history(peer)); const bAfter = await b.invoke(history(peer));
    if (findMessage(aAfter, tag) || findMessage(bAfter, tag)) throw new Error('deleteHistory left the tagged message visible');
    console.log(JSON.stringify({ tag, create: name(created), send: name(sent), deleteHistory: name(deleted), senderHistory: name(aAfter), receiverHistory: name(bAfter), senderId, receiverId }));
  } finally {
    if (groupId !== undefined) {
      try { await a.invoke(new Api.messages.DeleteChat({ chatId: groupId })); } catch {}
    }
    a.destroy(); b.destroy();
  }
}
main().then(() => process.exit(0)).catch((error) => { console.error(String(error)); process.exit(1); });
