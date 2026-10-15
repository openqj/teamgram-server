import { execFileSync } from 'node:child_process';
import path from 'node:path';
import { createRequire } from 'node:module';

const root = process.env.TEAMGRAM_GRAMJS_DIR || '/Users/mac/open/test/telegram-tt-master';
const userId = process.env.A_USER_ID || '7';
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

function loadAuthKey(): string {
  const output = execFileSync('docker', [
    'exec', process.env.TEAMGRAM_POSTGRES_CONTAINER || 'teamgram-postgres', 'psql',
    '-Atq', '-U', 'teamgram', '-d', 'teamgram', '-c',
    `SELECT encode(k.body, 'base64') FROM auth_users u JOIN auth_keys k USING (auth_key_id)
     JOIN auth_key_infos i USING (auth_key_id) WHERE u.user_id=${Number(userId)}
     AND u.state=0 AND NOT u.deleted AND NOT k.deleted AND NOT i.deleted AND i.auth_key_type=0
     ORDER BY u.date_active DESC,u.id DESC LIMIT 1`,
  ], { encoding: 'utf8' }).trim();
  if (!output) throw new Error(`no active production auth key found for user ${userId}`);
  return output;
}

function makeClient(auth: string): any {
  const session = new CallbackSession({ mainDcId: dcId, keys: { [dcId]: Buffer.from(auth, 'base64').toString('hex') } }, () => {});
  const load = session.load.bind(session);
  session.load = async () => {
    await load();
    (session as any)._serverAddress = process.env.TG_DC_HOST || '127.0.0.1';
    (session as any)._port = Number(process.env.TG_DC_PORT || '11443');
  };
  const client = new TelegramClient(session, 1, '0123456789abcdef0123456789abcdef', {
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
  (client as any)._borrowExportedSender = async () => undefined;
  (client as any)._updateLoop = async () => undefined;
  return client;
}

const rpcName = (value: any) => value?.className || value?.constructor?.name || typeof value;
function chatList(value: any): any[] {
  return [...(Array.isArray(value?.chats) ? value.chats : []), ...(Array.isArray(value?.updates?.chats) ? value.updates.chats : [])];
}

class ReorderPinnedForumTopicsRequest {
  CONSTRUCTOR_ID = 0x0e7841f0;
  SUBCLASS_OF_ID = 0;
  className = 'messages.ReorderPinnedForumTopics';
  classType = 'request';
  constructor(private readonly peer: any, private readonly order: number[]) {}
  getBytes() {
    const header = new Uint8Array(8);
    const vector = new Uint8Array(8 + this.order.length * 4);
    new DataView(header.buffer).setUint32(0, this.CONSTRUCTOR_ID, true);
    new DataView(vector.buffer).setUint32(0, 0x1cb5c415, true);
    new DataView(vector.buffer).setInt32(4, this.order.length, true);
    this.order.forEach((id, index) => new DataView(vector.buffer).setInt32(8 + index * 4, id, true));
    const peerBytes = this.peer.getBytes();
    const bytes = new Uint8Array(header.length + peerBytes.length + vector.length);
    bytes.set(header); bytes.set(peerBytes, header.length); bytes.set(vector, header.length + peerBytes.length);
    return bytes;
  }
  readResult(reader: any) { return reader.tgReadObject(); }
}

async function main() {
  const client = makeClient(process.env.A_AUTH_B64 || loadAuthKey());
  let channel: any;
  const tag = `prod-forum-${Date.now()}-${process.pid}`;
  try {
    await client.connect();
    const me = await client.getMe();
    if (String(me?.id) !== String(userId)) throw new Error('auth key user mismatch');
    const created = await client.invoke(new Api.channels.CreateChannel({ broadcast: false, megagroup: true, title: tag, about: 'production forum probe' }));
    channel = chatList(created).find((item: any) => item?.title === tag);
    if (!channel?.id || channel.accessHash === undefined) throw new Error(`channel create returned ${rpcName(created)} without channel`);
    const peer = new Api.InputPeerChannel({ channelId: channel.id, accessHash: channel.accessHash });
    const input = new Api.InputChannel({ channelId: channel.id, accessHash: channel.accessHash });
    await client.invoke(new Api.channels.ToggleForum({ channel: input, enabled: true, tabs: true }));
    const firstTitle = `${tag}-first`;
    const createdFirst = await client.invoke(new Api.messages.CreateForumTopic({ peer, title: firstTitle, randomId: BigInt(Date.now()) }));
    const listed = await client.invoke(new Api.messages.GetForumTopics({ peer, offsetDate: 0, offsetId: 0, offsetTopic: 0, limit: 20 }));
    const first = listed?.topics?.find((item: any) => item?.title === firstTitle);
    if (!first?.id) throw new Error('first topic was not listed');
    const edited = await client.invoke(new Api.messages.EditForumTopic({ peer, topicId: first.id, title: `${tag}-edited`, iconEmojiId: 42n, closed: true, hidden: true }));
    const editedRead = await client.invoke(new Api.messages.GetForumTopicsByID({ peer, topics: [first.id] }));
    const editedTopic = editedRead?.topics?.[0];
    if (editedTopic?.title !== `${tag}-edited` || Number(editedTopic?.iconEmojiId) !== 42 || editedTopic?.closed !== true || editedTopic?.hidden !== true) throw new Error('edit readback mismatch');
    const pinned = await client.invoke(new Api.messages.UpdatePinnedForumTopic({ peer, topicId: first.id, pinned: true }));
    const pinnedRead = await client.invoke(new Api.messages.GetForumTopicsByID({ peer, topics: [first.id] }));
    if (pinnedRead?.topics?.[0]?.pinned !== true) throw new Error('pin readback mismatch');
    const secondTitle = `${tag}-second`;
    await client.invoke(new Api.messages.CreateForumTopic({ peer, title: secondTitle, randomId: BigInt(Date.now()) + 1n }));
    const before = await client.invoke(new Api.messages.GetForumTopics({ peer, offsetDate: 0, offsetId: 0, offsetTopic: 0, limit: 20 }));
    const second = before?.topics?.find((item: any) => item?.title === secondTitle);
    if (!second?.id) throw new Error('second topic was not listed');
    const reordered = await client.invoke(new ReorderPinnedForumTopicsRequest(peer, [second.id, first.id]));
    const after = await client.invoke(new Api.messages.GetForumTopics({ peer, offsetDate: 0, offsetId: 0, offsetTopic: 0, limit: 20 }));
    if (after?.topics?.[0]?.id !== second.id) throw new Error('reorder readback mismatch');
    console.log(JSON.stringify({ create: rpcName(created), createTopic: rpcName(createdFirst), edit: rpcName(edited), editRead: rpcName(editedRead), pin: rpcName(pinned), pinRead: rpcName(pinnedRead), reorder: rpcName(reordered), reorderRead: rpcName(after), channelId: String(channel.id), topicIds: [first.id, second.id] }));
  } finally {
    if (channel) {
      try { await client.invoke(new Api.channels.DeleteChannel({ channel: new Api.InputChannel({ channelId: channel.id, accessHash: channel.accessHash }) })); } catch {}
    }
    client.destroy();
  }
}

main().then(() => process.exit(0)).catch((error) => { console.error(`forum topics probe failed: ${error instanceof Error ? error.stack || error.message : String(error)}`); process.exit(1); });
