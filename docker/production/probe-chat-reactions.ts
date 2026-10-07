import { createRequire } from 'node:module';
import { execFileSync } from 'node:child_process';
import path from 'node:path';

const globalAny = globalThis as typeof globalThis & { self?: any; addEventListener?: () => void };
globalAny.self ??= globalThis; globalAny.addEventListener ??= () => {}; globalAny.self.addEventListener ??= globalAny.addEventListener;
for (const key of ['TEAMGRAM_GRAMJS_DIR', 'A_AUTH_B64', 'B_AUTH_B64', 'A_USER_ID', 'B_USER_ID', 'B_ACCESS_HASH']) if (!process.env[key]) throw new Error(`${key} is required`);
const root = process.env.TEAMGRAM_GRAMJS_DIR!; const requireModule = createRequire(import.meta.url);
const TelegramClient = requireModule(path.join(root, 'src/lib/gramjs/client/TelegramClient.ts')).default;
const CallbackSession = requireModule(path.join(root, 'src/lib/gramjs/sessions/CallbackSession.ts')).default;
const Api = requireModule(path.join(root, 'src/lib/gramjs/tl/index.ts')).Api;
const Connection = requireModule(path.join(root, 'src/lib/gramjs/network/connection/TCPObfuscated.ts')).ConnectionTCPObfuscated;
const dcId = Number(process.env.TG_DC_ID || '2'); const aId = BigInt(process.env.A_USER_ID!); const bId = BigInt(process.env.B_USER_ID!); const bHash = BigInt(process.env.B_ACCESS_HASH!);
const quietLogger = { debug() {}, info() {}, warn() {}, error() {} };
const rpcName = (value: any) => value?.className || value?.constructor?.name || typeof value;
const isTrue = (value: any) => value === true || rpcName(value).toLowerCase().includes('true');
function makeClient(auth: string): any {
  const session = new CallbackSession({ mainDcId: dcId, keys: { [dcId]: Buffer.from(auth, 'base64').toString('hex') } }, () => {});
  const load = session.load.bind(session); session.load = async () => { await load(); (session as any)._serverAddress = process.env.TG_DC_HOST || '127.0.0.1'; (session as any)._port = Number(process.env.TG_DC_PORT || '11443'); };
  return new TelegramClient(session, 1, '0123456789abcdef0123456789abcdef', { connection: Connection, fallbackConnection: Connection, shouldAllowHttpTransport: false, shouldForceHttpTransport: false, timeout: 20, requestRetries: 2, connectionRetries: 2, connectionRetriesToFallback: 0, retryDelay: 250, autoReconnect: false, baseLogger: quietLogger });
}
function chatDb(chatId: bigint | number): { type: number; reactions: string; hex: string } {
  const sql = `SELECT available_reactions_type,available_reactions,HEX(available_reactions) FROM chats WHERE id=${chatId}`;
  const output = execFileSync('docker', ['exec', 'mysql', 'mysql', '-N', '-s', '-uteamgram', '-pteamgram', '-Dteamgram', '-e', sql], { encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'] }).trim();
  const [type, reactions, hex] = output.split('\t'); return { type: Number(type || 0), reactions: reactions || '', hex: hex || '' };
}
async function main() {
  const a = makeClient(process.env.A_AUTH_B64!); const b = makeClient(process.env.B_AUTH_B64!); let chatId: bigint | number | undefined;
  try {
    await a.connect(); await b.connect();
    if (String((await a.getMe())?.id) !== String(aId) || String((await b.getMe())?.id) !== String(bId)) throw new Error('auth key user mismatch');
    const title = `prod-chat-reactions-${Date.now()}-${process.pid}`;
    const created = await a.invoke(new Api.messages.CreateChat({ users: [new Api.InputUser({ userId: bId, accessHash: bHash })], title }));
    let group = Array.isArray(created?.chats) ? created.chats.find((item: any) => item?.title === title) : undefined;
    for (let attempt = 0; !group && attempt < 10; attempt += 1) {
      const dialogs = await a.invoke(new Api.messages.GetDialogs({ offsetDate: 0, offsetId: 0, offsetPeer: new Api.InputPeerEmpty(), limit: 100, hash: 0n }));
      group = Array.isArray(dialogs?.chats) ? dialogs.chats.find((item: any) => item?.title === title) : undefined;
      if (!group) await new Promise((resolve) => setTimeout(resolve, 250));
    }
    if (!group?.id) throw new Error(`messages.createChat returned ${rpcName(created)} without group`);
    chatId = group.id; const peer = new Api.InputPeerChat({ chatId });
    const before = b.invoke(new Api.messages.GetFullChat({ chatId }));
    const set = await a.invoke(new Api.messages.SetChatAvailableReactions({ peer, availableReactions: new Api.ChatReactionsSome({ reactions: [new Api.ReactionEmoji({ emoticon: '👍' })] }), reactionsLimit: 3 }));
    if (!rpcName(set).toLowerCase().includes('updates')) throw new Error(`setChatAvailableReactions returned ${rpcName(set)}`);
    const full = await b.invoke(new Api.messages.GetFullChat({ chatId })); const stored = chatDb(chatId);
    const clear = await a.invoke(new Api.messages.SetChatAvailableReactions({ peer, availableReactions: new Api.ChatReactionsNone() }));
    if (!rpcName(clear).toLowerCase().includes('updates')) throw new Error(`clear reactions returned ${rpcName(clear)}`);
    const cleared = chatDb(chatId); const fullAfter = await b.invoke(new Api.messages.GetFullChat({ chatId }));
    if (cleared.type !== 1 || cleared.reactions !== '' || cleared.hex !== '') throw new Error(`reaction cleanup failed: ${JSON.stringify(cleared)}`);
    console.log(JSON.stringify({ create: rpcName(created), before: rpcName(await before), set: rpcName(set), read: rpcName(full), stored, clear: rpcName(clear), cleared, readAfter: rpcName(fullAfter), chatId: String(chatId) }));
    const deleted = await a.invoke(new Api.messages.DeleteChat({ chatId })); if (!isTrue(deleted)) throw new Error(`messages.deleteChat returned ${rpcName(deleted)}`); chatId = undefined;
  } finally {
    if (chatId !== undefined) { try { await a.invoke(new Api.messages.DeleteChat({ chatId })); } catch {} }
    a.destroy(); b.destroy();
  }
}
main().then(() => process.exit(0)).catch((error) => { console.error(`chat reactions probe failed: ${error instanceof Error ? error.stack || error.message : String(error)}`); process.exit(1); });
