import path from 'node:path';
import { createRequire } from 'node:module';
import process from 'node:process';

const API_ID = 1;
const API_HASH = '0123456789abcdef0123456789abcdef';
const FIXTURES = [
  { phone: '12025550101', username: 'isolated_test_01' },
  { phone: '12025550102', username: 'isolated_test_02' },
];

type Actor = {
  client: any;
  id: bigint | number;
  username: string;
};

const quietLogger = { debug() {}, info() {}, warn() {}, error() {} };
let randomIDCounter = 0n;

function requireCondition(condition: unknown, message: string): asserts condition {
  if (!condition) throw new Error(message);
}

function rpcName(value: any) {
  return value?.className || value?.constructor?.name || typeof value;
}

function randomID() {
  randomIDCounter += 1n;
  return BigInt(Date.now()) * 1_000_000n + randomIDCounter;
}

function messageList(value: any): any[] {
  return [
    ...(Array.isArray(value?.messages) ? value.messages : []),
    ...(Array.isArray(value?.newMessages) ? value.newMessages : []),
  ];
}

function inputPeerUser(Api: any, user: any) {
  requireCondition(user?.id !== undefined && user?.accessHash !== undefined, 'user has no usable access hash');
  return new Api.InputPeerUser({ userId: user.id, accessHash: user.accessHash });
}

async function loadGramJs(root: string) {
  const requireModule = createRequire(import.meta.url);
  const clientModule = requireModule(path.join(root, 'src/lib/gramjs/client/TelegramClient.ts'));
  const sessionModule = requireModule(path.join(root, 'src/lib/gramjs/sessions/Memory.ts'));
  const tlModule = requireModule(path.join(root, 'src/lib/gramjs/tl/index.ts'));
  const connectionModule = requireModule(path.join(root, 'src/lib/gramjs/network/connection/TCPObfuscated.ts'));
  return {
    TelegramClient: clientModule.default,
    MemorySession: sessionModule.default,
    Api: tlModule.Api,
    Connection: connectionModule.ConnectionTCPObfuscated,
  };
}

async function login(gram: any, fixture: typeof FIXTURES[number]): Promise<Actor> {
  const client = new gram.TelegramClient(new gram.MemorySession(), API_ID, API_HASH, {
    connection: gram.Connection,
    fallbackConnection: gram.Connection,
    shouldAllowHttpTransport: false,
    shouldForceHttpTransport: false,
    timeout: 15,
    requestRetries: 2,
    connectionRetries: 2,
    connectionRetriesToFallback: 0,
    retryDelay: 250,
    autoReconnect: false,
    baseLogger: quietLogger,
  });

  try {
    await client.start({
      phoneNumber: fixture.phone,
      phoneCode: async () => '12345',
      password: async () => '',
      firstAndLastNames: async () => ['Isolated', 'Scheduled'],
      qrCode: async () => {},
      onError: (error: Error) => { throw error; },
      webAuthTokenFailed: () => {},
      onPasskeyOption: () => {},
      initialMethod: 'phoneNumber',
    });
    const me = await client.getMe();
    requireCondition(me?.id !== undefined, `login failed for ${fixture.username}`);
    return { client, id: me.id, username: fixture.username };
  } catch (error) {
    client.destroy();
    throw error;
  }
}

async function searchUser(Api: any, actor: Actor, username: string) {
  const result = await actor.client.invoke(new Api.contacts.Search({ q: username, limit: 10 }));
  const user = result?.users?.find((candidate: any) => candidate?.username === username);
  requireCondition(user, `contacts.search did not find ${username}`);
  return user;
}

async function scheduledHistory(Api: any, actor: Actor, peer: any) {
  return actor.client.invoke(new Api.messages.GetScheduledHistory({ peer, hash: 0n }));
}

async function ordinaryHistory(Api: any, actor: Actor, peer: any) {
  return actor.client.invoke(new Api.messages.GetHistory({
    peer,
    offsetId: 0,
    offsetDate: 0,
    addOffset: 0,
    limit: 20,
    maxId: 0,
    minId: 0,
    hash: 0n,
  }));
}

function scheduledID(updates: any) {
  const update = Array.isArray(updates?.updates)
    ? updates.updates.find((candidate: any) => candidate?.message?.id !== undefined)
    : undefined;
  requireCondition(update?.message?.id !== undefined, 'messages.sendMessage did not return updateNewScheduledMessage');
  return update.message.id;
}

async function scheduleText(Api: any, actor: Actor, peer: any, text: string, scheduleDate: number) {
  const updates = await actor.client.invoke(new Api.messages.SendMessage({
    peer,
    message: text,
    randomId: randomID(),
    scheduleDate,
  }));
  return { id: scheduledID(updates), updates };
}

async function expectRpcError(name: string, code: string, action: () => Promise<unknown>) {
  try {
    await action();
  } catch (error) {
    requireCondition(String(error).includes(code), `${name} returned ${String(error)}, want ${code}`);
    console.log(`PASS ${name}: ${code}`);
    return;
  }
  throw new Error(`${name} unexpectedly succeeded`);
}

async function run() {
  requireCondition(process.env.TG_DC_HOST === '127.0.0.1', 'TG_DC_HOST must target the isolated gateway');
  requireCondition(process.env.TG_DC_PORT === '31443', 'TG_DC_PORT must target the isolated gateway');
  requireCondition(process.env.TEAMGRAM_GRAMJS_DIR, 'TEAMGRAM_GRAMJS_DIR is required');

  const globalAny = globalThis as typeof globalThis & { self?: any; addEventListener?: () => void };
  globalAny.self ??= globalThis;
  globalAny.addEventListener ??= () => {};
  globalAny.self.addEventListener ??= globalAny.addEventListener;

  const gram = await loadGramJs(process.env.TEAMGRAM_GRAMJS_DIR);
  const { Api } = gram;
  let alice: Actor | undefined;
  let bob: Actor | undefined;

  try {
    const [aliceActor, bobActor] = await Promise.all(FIXTURES.map((fixture) => login(gram, fixture)));
    alice = aliceActor;
    bob = bobActor;
    const bobPeer = inputPeerUser(Api, await searchUser(Api, aliceActor, bobActor.username));
    const alicePeer = inputPeerUser(Api, await searchUser(Api, bobActor, aliceActor.username));
    const tag = `scheduled_message_${Date.now()}_${process.pid}`;
    const when = Math.floor(Date.now() / 1000) + 3600;

    const firstText = `${tag} history`;
    const first = await scheduleText(Api, aliceActor, bobPeer, firstText, when);
    requireCondition(rpcName(first.updates).toLowerCase().includes('updates'), 'messages.sendMessage did not return updates');
    console.log(`PASS scheduled send: ${rpcName(first.updates)}`);

    const history = await scheduledHistory(Api, aliceActor, bobPeer);
    const pending = messageList(history).find((message: any) => message?.message === firstText);
    requireCondition(pending?.id === first.id, 'messages.getScheduledHistory did not return the scheduled message');
    console.log(`PASS scheduled history: ${rpcName(history)}`);

    const byID = await aliceActor.client.invoke(new Api.messages.GetScheduledMessages({ peer: bobPeer, id: [first.id] }));
    requireCondition(
      messageList(byID).some((message: any) => message?.id === first.id && message?.message === firstText),
      'messages.getScheduledMessages did not return the requested scheduled message',
    );
    console.log(`PASS scheduled lookup: ${rpcName(byID)}`);

    const cancelText = `${tag} cancel`;
    const cancelled = await scheduleText(Api, aliceActor, bobPeer, cancelText, when + 60);
    const deleted = await aliceActor.client.invoke(new Api.messages.DeleteScheduledMessages({ peer: bobPeer, id: [cancelled.id] }));
    requireCondition(rpcName(deleted).toLowerCase().includes('updates'), 'messages.deleteScheduledMessages did not return updates');
    const afterCancel = await scheduledHistory(Api, aliceActor, bobPeer);
    requireCondition(!messageList(afterCancel).some((message: any) => message?.id === cancelled.id), 'cancelled message remains scheduled');
    console.log(`PASS scheduled cancellation: ${rpcName(deleted)}`);

    const sendText = `${tag} deliver`;
    const ready = await scheduleText(Api, aliceActor, bobPeer, sendText, when + 120);
    const sent = await aliceActor.client.invoke(new Api.messages.SendScheduledMessages({ peer: bobPeer, id: [ready.id] }));
    requireCondition(rpcName(sent).toLowerCase().includes('updates'), 'messages.sendScheduledMessages did not return updates');
    const afterSend = await scheduledHistory(Api, aliceActor, bobPeer);
    requireCondition(!messageList(afterSend).some((message: any) => message?.id === ready.id), 'sent message remains scheduled');
    console.log(`PASS scheduled delivery consumption: ${rpcName(sent)}`);

    const bobHistory = await ordinaryHistory(Api, bobActor, alicePeer);
    requireCondition(
      messageList(bobHistory).some((message: any) => message?.message === sendText),
      'recipient history did not contain the delivered scheduled message',
    );
    console.log(`PASS scheduled recipient delivery: ${rpcName(bobHistory)}`);

    await expectRpcError('past schedule rejection', 'SCHEDULE_DATE_INVALID', () => aliceActor.client.invoke(
      new Api.messages.SendMessage({
        peer: bobPeer,
        message: `${tag} past`,
        randomId: randomID(),
        scheduleDate: Math.floor(Date.now() / 1000) - 60,
      }),
    ));
    await expectRpcError('invalid peer schedule rejection', 'PEER_ID_INVALID', () => aliceActor.client.invoke(
      new Api.messages.SendMessage({
        peer: new Api.InputPeerEmpty(),
        message: `${tag} invalid peer`,
        randomId: randomID(),
        scheduleDate: when,
      }),
    ));
  } finally {
    for (const actor of [alice, bob]) actor?.client.destroy();
  }
}

void run()
  .then(() => process.exit(0))
  .catch((error) => {
    console.error(`scheduled message probe fatal: ${error instanceof Error && error.stack ? error.stack : String(error)}`);
    process.exit(1);
  });
