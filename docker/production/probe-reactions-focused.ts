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
async function main() {
  const a = makeClient(process.env.A_AUTH_B64!); const b = makeClient(process.env.B_AUTH_B64!); const tag = `prod-reaction-${Date.now()}-${process.pid}`;
  const peerA = new Api.InputPeerUser({ userId: B_ID, accessHash: B_HASH }); const peerB = new Api.InputPeerUser({ userId: A_ID, accessHash: A_HASH });
  let senderId = 0; let recipientId = 0;
  try {
    await a.connect(); await b.connect();
    const sent = await a.invoke(new Api.messages.SendMessage({ peer: peerA, message: tag, randomId: BigInt(Date.now()) * 1000n + 1n }));
    const senderMessage = ((await a.invoke(history(peerA)))?.messages || []).find((m: any) => m?.message === tag);
    const recipientMessage = ((await b.invoke(history(peerB)))?.messages || []).find((m: any) => m?.message === tag);
    if (!senderMessage?.id || !recipientMessage?.id) throw new Error('message not visible in both histories');
    senderId = Number(senderMessage.id); recipientId = Number(recipientMessage.id);
    const byId = await a.invoke(new Api.messages.GetMessages({ id: [new Api.InputMessageID({ id: senderId })] }));
    if (!((byId?.messages || []).some((m: any) => m?.message === tag))) throw new Error('messages.getMessages did not return the sent message');
    const contents = await b.invoke(new Api.messages.ReadMessageContents({ id: [recipientId] }));
    const set = await b.invoke(new Api.messages.SendReaction({ peer: peerB, msgId: recipientId, reaction: [new Api.ReactionEmoji({ emoticon: '👍' })] }));
    const unread = await b.invoke(new Api.messages.GetUnreadReactions({ peer: peerB, offsetId: 0, addOffset: 0, limit: 20, maxId: 0, minId: 0 }));
    const read = await b.invoke(new Api.messages.ReadReactions({ peer: peerB }));
    const clearRecent = await b.invoke(new Api.messages.ClearRecentReactions());
    const clear = await b.invoke(new Api.messages.SendReaction({ peer: peerB, msgId: recipientId, reaction: [] }));
    console.log(JSON.stringify({ tag, send: name(sent), byId: name(byId), contents: name(contents), set: name(set), unread: name(unread), read: name(read), clearRecent: name(clearRecent), clear: name(clear), senderId, recipientId }));
  } finally {
    if (senderId) { try { await a.invoke(new Api.messages.DeleteMessages({ revoke: true, id: [senderId] })); } catch {} }
    if (recipientId) { try { await b.invoke(new Api.messages.DeleteMessages({ revoke: false, id: [recipientId] })); } catch {} }
    a.destroy(); b.destroy();
  }
}
main().then(() => process.exit(0)).catch((error) => { console.error(String(error)); process.exit(1); });
