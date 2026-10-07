import path from 'node:path';
import { createRequire } from 'node:module';

for (const key of ['TEAMGRAM_GRAMJS_DIR', 'A_AUTH_B64', 'B_USER_ID', 'B_ACCESS_HASH']) {
  if (!process.env[key]) throw new Error(`${key} is required`);
}

const root = process.env.TEAMGRAM_GRAMJS_DIR!;
const dcId = Number(process.env.TG_DC_ID || '2');
const bId = BigInt(process.env.B_USER_ID!);
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

function chatNoForwards(full: any, chatId: bigint | number): boolean | undefined {
  const chat = (full?.chats || []).find((item: any) => String(item?.id) === String(chatId));
  return chat?.noforwards;
}

async function main() {
  const client = makeClient(process.env.A_AUTH_B64!);
  const tag = `prod-no-forwards-${Date.now()}-${process.pid}`;
  let chatId: bigint | undefined;
  try {
    await client.connect();
    const created = await client.invoke(new Api.messages.CreateChat({
      users: [new Api.InputUser({ userId: bId, accessHash: bHash })],
      title: `${tag} group`,
    }));
    let chat = (created?.chats || []).find((item: any) => item?.title === `${tag} group`);
    for (let attempt = 0; !chat && attempt < 8; attempt += 1) {
      const dialogs = await client.invoke(new Api.messages.GetDialogs({
        offsetDate: 0,
        offsetId: 0,
        offsetPeer: new Api.InputPeerEmpty(),
        limit: 200,
        hash: 0n,
      }));
      chat = (dialogs?.chats || []).find((item: any) => item?.title === `${tag} group`);
      if (!chat) await new Promise((resolve) => setTimeout(resolve, 250));
    }
    if (!chat?.id) throw new Error(`messages.createChat returned ${rpcName(created)} without a chat`);
    chatId = chat.id;
    const peer = new Api.InputPeerChat({ chatId });
    const enabled = await client.invoke(new Api.messages.ToggleNoForwards({ peer, enabled: true }));
    const fullEnabled = await client.invoke(new Api.messages.GetFullChat({ chatId }));
    if (chatNoForwards(fullEnabled, chatId) !== true) throw new Error('noforwards=true did not read back');
    const disabled = await client.invoke(new Api.messages.ToggleNoForwards({ peer, enabled: false }));
    const fullDisabled = await client.invoke(new Api.messages.GetFullChat({ chatId }));
    // The canonical Chat object omits the default false field; only a true
    // value is meaningful for this flag.
    if (chatNoForwards(fullDisabled, chatId) === true) throw new Error('noforwards=false did not read back');
    console.log(JSON.stringify({
      tag,
      enable: rpcName(enabled),
      enabledRead: rpcName(fullEnabled),
      enabledValue: chatNoForwards(fullEnabled, chatId),
      disable: rpcName(disabled),
      disabledRead: rpcName(fullDisabled),
      disabledValue: chatNoForwards(fullDisabled, chatId),
    }));
  } finally {
    if (chatId !== undefined) {
      try { await client.invoke(new Api.messages.DeleteChat({ chatId })); } catch {}
    }
    client.destroy();
  }
}

main().then(() => process.exit(0)).catch((error) => {
  console.error(String(error));
  process.exit(1);
});
