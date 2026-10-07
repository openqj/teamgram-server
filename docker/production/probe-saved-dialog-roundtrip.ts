import path from 'node:path';
import { createRequire } from 'node:module';

for (const key of ['TEAMGRAM_GRAMJS_DIR', 'AUTH_B64', 'USER_ID']) {
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

const rpcName = (value: any): string => value?.className || value?.constructor?.name || typeof value;

function tlInt(value: number): Uint8Array {
  const bytes = new Uint8Array(4);
  new DataView(bytes.buffer).setInt32(0, value, true);
  return bytes;
}

function rawRequest(constructorId: number, className: string, ...parts: Uint8Array[]): any {
  return {
    CONSTRUCTOR_ID: constructorId,
    SUBCLASS_OF_ID: 0,
    className,
    classType: 'request',
    getBytes: () => Uint8Array.from(Buffer.concat([
      Buffer.from(tlInt(constructorId)),
      ...parts.map((part) => Buffer.from(part)),
    ])),
    readResult: (reader: any) => reader.tgReadObject(),
  };
}

function makeClient(): any {
  const session = new CallbackSession({
    mainDcId: dcId,
    keys: { [dcId]: Buffer.from(process.env.AUTH_B64!, 'base64').toString('hex') },
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
    requestRetries: 1,
    connectionRetries: 1,
    connectionRetriesToFallback: 0,
    autoReconnect: false,
    baseLogger: quietLogger,
  });
}

function historyRequest(peer: any) {
  return new Api.messages.GetSavedHistory({
    peer,
    offsetId: 0,
    offsetDate: 0,
    addOffset: 0,
    limit: 50,
    maxId: 0,
    minId: 0,
    hash: 0n,
  });
}

function dialogsRequest() {
  return new Api.messages.GetSavedDialogs({
    offsetDate: 0,
    offsetId: 0,
    offsetPeer: new Api.InputPeerEmpty(),
    limit: 50,
    hash: 0n,
  });
}

async function main() {
  const client = makeClient();
  const text = `codex-saved-roundtrip-${Date.now()}-${process.pid}`;
  let messageId = Number(process.env.CLEANUP_ID || 0);
  try {
    (client as any)._borrowExportedSender = async () => undefined;
    (client as any)._updateLoop = async () => undefined;
    await client.connect();
    const me = await client.getMe();
    if (String(me?.id) !== String(process.env.USER_ID)) throw new Error('auth key user mismatch');
    if (process.env.CLEANUP_ONLY === '1') {
      if (messageId <= 0) throw new Error('CLEANUP_ID is required');
      const deleted = await client.invoke(new Api.messages.DeleteMessages({ revoke: true, id: [messageId] }));
      console.log(JSON.stringify({ userId: String(me.id), cleanup: rpcName(deleted), messageId }));
      return;
    }
    const historyPeer = process.env.SAVED_HISTORY_PEER === 'user'
      ? new Api.InputPeerUser({ userId: BigInt(process.env.USER_ID!), accessHash: 0n })
      : new Api.InputPeerSelf();
    const sent = await client.invoke(new Api.messages.SendMessage({
      peer: new Api.InputPeerSelf(),
      message: text,
      randomId: BigInt(Date.now()) * 1000n,
    }));
    if (!rpcName(sent).toLowerCase().includes('updates')) throw new Error(`send returned ${rpcName(sent)}`);
    const updateMessageID = (sent?.updates || []).find((item: any) => rpcName(item).toLowerCase().includes('messageid'));
    if (updateMessageID?.id) messageId = Number(updateMessageID.id);

    let history: any;
    let message: any;
    for (let attempt = 0; attempt < 12; attempt += 1) {
      history = await client.invoke(historyRequest(historyPeer));
      message = (history?.messages || []).find((item: any) => item?.message === text);
      if (message) break;
      await new Promise((resolve) => setTimeout(resolve, 250));
    }
    if (!message?.id) throw new Error('saved history did not return the probe message');
    messageId = Number(message.id);

    const dialogs = await client.invoke(dialogsRequest());
    const dialog = (dialogs?.dialogs || []).find((item: any) => Number(item?.topMessage) === messageId);
    if (!dialog) throw new Error('saved dialogs did not return the probe message dialog');
    const byID = await client.invoke(rawRequest(
      0x6f6f9c96,
      'messages.GetSavedDialogsByID',
      Buffer.from(tlInt(0)),
      Buffer.from(tlInt(0x1cb5c415)),
      Buffer.from(tlInt(1)),
      Buffer.from(new Api.InputPeerSelf().getBytes()),
    ));
    if (!(byID?.dialogs || []).some((item: any) => Number(item?.topMessage) === messageId)) {
      throw new Error('saved dialogs by ID did not return the probe dialog');
    }

    console.log(JSON.stringify({
      userId: String(me.id),
      send: rpcName(sent),
      history: rpcName(history),
      dialogs: rpcName(dialogs),
      dialogsByID: rpcName(byID),
      historyPeer: process.env.SAVED_HISTORY_PEER === 'user' ? 'InputPeerUser' : 'InputPeerSelf',
      messageId,
    }));
  } finally {
    if (messageId > 0) {
      try {
        await client.invoke(new Api.messages.DeleteMessages({ revoke: true, id: [messageId] }));
      } catch {}
    }
    client.destroy();
  }
}

main().then(() => process.exit(0)).catch((error) => {
  console.error(`saved dialog roundtrip failed: ${error instanceof Error ? error.stack || error.message : String(error)}`);
  process.exit(1);
});
