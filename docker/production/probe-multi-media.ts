import path from 'node:path';
import { createRequire } from 'node:module';

type Client = any;

for (const key of ['TEAMGRAM_GRAMJS_DIR', 'A_AUTH_B64', 'B_AUTH_B64', 'A_USER_ID', 'B_USER_ID', 'A_ACCESS_HASH', 'B_ACCESS_HASH']) {
  if (!process.env[key]) throw new Error(`${key} is required`);
}

const root = process.env.TEAMGRAM_GRAMJS_DIR!;
const dcId = Number(process.env.TG_DC_ID || '2');
const aID = BigInt(process.env.A_USER_ID!);
const bID = BigInt(process.env.B_USER_ID!);
const aHash = BigInt(process.env.A_ACCESS_HASH!);
const bHash = BigInt(process.env.B_ACCESS_HASH!);
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

function makeClient(auth: string): Client {
  const session = new CallbackSession({
    mainDcId: dcId,
    keys: { [dcId]: Buffer.from(auth, 'base64').toString('hex') },
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

function history(peer: any): any {
  return new Api.messages.GetHistory({
    peer, offsetId: 0, offsetDate: 0, addOffset: 0, limit: 50,
    maxId: 0, minId: 0, hash: 0n,
  });
}

function search(peer: any, query: string): any {
  return new Api.messages.Search({
    peer,
    q: query,
    filter: new Api.InputMessagesFilterEmpty(),
    minDate: 0,
    maxDate: 0,
    offsetId: 0,
    addOffset: 0,
    limit: 50,
    maxId: 0,
    minId: 0,
    hash: 0n,
  });
}

async function main() {
  const alice = makeClient(process.env.A_AUTH_B64!);
  const bob = makeClient(process.env.B_AUTH_B64!);
  const tag = `prod-multi-media-${Date.now()}-${process.pid}`;
  const peerA = new Api.InputPeerUser({ userId: bID, accessHash: bHash });
  const peerB = new Api.InputPeerUser({ userId: aID, accessHash: aHash });
  const messageIds: number[] = [];
  const senderMessageIds: number[] = [];
  try {
    await alice.connect();
    await bob.connect();
    const meA = await alice.getMe();
    const meB = await bob.getMe();
    if (String(meA?.id) !== String(aID) || String(meB?.id) !== String(bID)) throw new Error('auth key user mismatch');

    const sent = await alice.invoke(new Api.messages.SendMultiMedia({
      peer: peerA,
      multiMedia: [
        new Api.InputSingleMedia({
          media: new Api.InputMediaContact({
            phoneNumber: '+14155550101', firstName: `${tag}-one`, lastName: 'Contact', vcard: '',
          }),
          message: `${tag} one`,
          randomId: BigInt(Date.now()) * 1000n + 1n,
        }),
        new Api.InputSingleMedia({
          media: new Api.InputMediaContact({
            phoneNumber: '+14155550102', firstName: `${tag}-two`, lastName: 'Contact', vcard: '',
          }),
          message: `${tag} two`,
          randomId: BigInt(Date.now()) * 1000n + 2n,
        }),
      ],
    }));
    if (!rpcName(sent).toLowerCase().includes('updates')) throw new Error(`sendMultiMedia returned ${rpcName(sent)}`);

    let read: any;
    let messages: any[] = [];
    for (let attempt = 0; attempt < 8; attempt += 1) {
      read = await bob.invoke(history(peerB));
      messages = Array.isArray(read?.messages) ? read.messages : [];
      if ([`${tag} one`, `${tag} two`].every((text) => messages.some((item: any) => item?.message === text))) break;
      const searched = await bob.invoke(search(peerB, tag));
      const searchedMessages = Array.isArray(searched?.messages) ? searched.messages : [];
      messages = [...messages, ...searchedMessages];
      if ([`${tag} one`, `${tag} two`].every((text) => messages.some((item: any) => item?.message === text))) break;
      await new Promise((resolve) => setTimeout(resolve, 250));
    }
    for (const text of [`${tag} one`, `${tag} two`]) {
      const message = messages.find((item: any) => item?.message === text);
      if (!message?.id) throw new Error(`recipient history missing ${text}`);
      messageIds.push(Number(message.id));
      if (message.media?.className !== 'MessageMediaContact') throw new Error(`media type for ${text} was ${rpcName(message.media)}`);
    }
    const senderRead = await alice.invoke(history(peerA));
    const senderMessages = Array.isArray(senderRead?.messages) ? senderRead.messages : [];
    for (const text of [`${tag} one`, `${tag} two`]) {
      const message = senderMessages.find((item: any) => item?.message === text);
      if (!message?.id) throw new Error(`sender history missing ${text}`);
      senderMessageIds.push(Number(message.id));
    }
    const grouped = messages.filter((item: any) => String(item?.message || '').startsWith(tag));
    const groupedIds = new Set(grouped.map((item: any) => String(item?.groupedId || '')));
    if (groupedIds.size !== 1 || groupedIds.has('')) throw new Error('album messages did not share one grouped_id');

    const deleted = await alice.invoke(new Api.messages.DeleteMessages({ revoke: true, id: senderMessageIds }));
    const recipientDeleted = await bob.invoke(new Api.messages.DeleteMessages({ revoke: false, id: messageIds }));
    console.log(JSON.stringify({
      tag,
      send: rpcName(sent),
      recipientHistory: rpcName(read),
      messageCount: messageIds.length,
      grouped: true,
      delete: rpcName(deleted),
      recipientDelete: rpcName(recipientDeleted),
    }));
  } finally {
    if (senderMessageIds.length > 0) {
      try { await alice.invoke(new Api.messages.DeleteMessages({ revoke: true, id: senderMessageIds })); } catch {}
    }
    if (messageIds.length > 0) {
      try { await bob.invoke(new Api.messages.DeleteMessages({ revoke: false, id: messageIds })); } catch {}
    }
    alice.destroy();
    bob.destroy();
  }
}

main().then(() => process.exit(0)).catch((error) => {
  console.error(`production multi-media probe failed: ${error instanceof Error ? error.stack || error.message : String(error)}`);
  process.exit(1);
});
