import path from 'node:path';
import { createRequire } from 'node:module';
import process from 'node:process';

const API_ID = 1;
const API_HASH = '0123456789abcdef0123456789abcdef';
const FIXTURES = [
  { phone: '12025550101', username: 'isolated_test_01' },
  { phone: '12025550102', username: 'isolated_test_02' },
  { phone: '12025550103', username: 'isolated_test_03' },
];

type Actor = {
  client: any;
  id: bigint | number;
  username: string;
};

const quietLogger = { debug() {}, info() {}, warn() {}, error() {} };
let randomIdCounter = 0n;

function requireCondition(condition: unknown, message: string): asserts condition {
  if (!condition) throw new Error(message);
}

function rpcName(value: any) {
  return value?.className || value?.constructor?.name || typeof value;
}

function chatList(value: any): any[] {
  return [
    ...(Array.isArray(value?.chats) ? value.chats : []),
    ...(Array.isArray(value?.updates?.chats) ? value.updates.chats : []),
  ];
}

function messageList(value: any): any[] {
  return [
    ...(Array.isArray(value?.messages) ? value.messages : []),
    ...(Array.isArray(value?.newMessages) ? value.newMessages : []),
  ];
}

function randomId() {
  randomIdCounter += 1n;
  return BigInt(Date.now()) * 1_000_000n + randomIdCounter;
}

function inputChannel(Api: any, channel: any) {
  requireCondition(channel?.id !== undefined && channel?.accessHash !== undefined, 'channel has no usable access hash');
  return new Api.InputChannel({ channelId: channel.id, accessHash: channel.accessHash });
}

function inputPeerChannel(Api: any, channel: any) {
  requireCondition(channel?.id !== undefined && channel?.accessHash !== undefined, 'channel has no usable peer access hash');
  return new Api.InputPeerChannel({ channelId: channel.id, accessHash: channel.accessHash });
}

function inputUser(Api: any, user: any) {
  requireCondition(user?.id !== undefined && user?.accessHash !== undefined, 'user has no usable access hash');
  return new Api.InputUser({ userId: user.id, accessHash: user.accessHash });
}

function reactionEmoticon(value: any): string | undefined {
  if (typeof value?.emoticon === 'string') return value.emoticon;
  return typeof value?.reaction === 'string' ? value.reaction : value?.reaction?.emoticon;
}

function reactionCounts(value: any, messageID: number) {
  const update = (Array.isArray(value?.updates) ? value.updates : []).find(
    (candidate: any) => rpcName(candidate).toLowerCase().includes('updatemessagereactions')
      && Number(candidate?.msgId) === Number(messageID),
  );
  requireCondition(Array.isArray(update?.reactions?.results), `messages.getMessagesReactions returned ${rpcName(value)}`);
  const counts = new Map<string, number>();
  for (const result of update.reactions.results) {
    const emoticon = reactionEmoticon(result);
    if (emoticon) counts.set(emoticon, Number(result.count));
  }
  return counts;
}

function assertCounts(counts: Map<string, number>, expected: Record<string, number>, label: string) {
  requireCondition(counts.size === Object.keys(expected).length, `${label} returned ${JSON.stringify(Object.fromEntries(counts))}`);
  for (const [emoticon, count] of Object.entries(expected)) {
    requireCondition(counts.get(emoticon) === count, `${label} returned ${JSON.stringify(Object.fromEntries(counts))}`);
  }
}

function wrongAccessHash(accessHash: bigint | number) {
  const value = BigInt(accessHash);
  return value === 0n ? 1n : value + 1n;
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
      firstAndLastNames: async () => ['Isolated', 'Reaction'],
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

async function resolveChannel(Api: any, actor: Actor, username: string) {
  const result = await actor.client.invoke(new Api.contacts.ResolveUsername({ username }));
  const channel = chatList(result).find((candidate: any) => rpcName(candidate).toLowerCase().includes('channel'));
  requireCondition(channel, `contacts.resolveUsername did not return ${username}`);
  return channel;
}

async function waitForMessage(action: () => Promise<any>, text: string, attempts = 10) {
  let value: any;
  for (let attempt = 0; attempt < attempts; attempt += 1) {
    value = await action();
    const message = messageList(value).find((candidate: any) => candidate?.message === text);
    if (message) return message;
    await new Promise((resolve) => setTimeout(resolve, 200));
  }
  throw new Error(`channel history did not return ${text}`);
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
  let carol: Actor | undefined;
  let createdChannel: any;

  try {
    [alice, bob, carol] = await Promise.all(FIXTURES.map((fixture) => login(gram, fixture)));
    const tag = `reaction_flow_${Date.now()}_${process.pid}`;
    const title = `${tag} channel`;
    const created = await alice.client.invoke(new Api.channels.CreateChannel({
      broadcast: true,
      title,
      about: 'isolated reaction flow probe',
    }));
    createdChannel = chatList(created).find((channel: any) => channel?.title === title);
    requireCondition(createdChannel, 'channels.createChannel did not return the temporary channel');
    const ownerChannel = inputChannel(Api, createdChannel);
    const ownerPeer = inputPeerChannel(Api, createdChannel);

    const username = `reaction_${Date.now()}_${process.pid}`;
    const usernameUpdated = await alice.client.invoke(new Api.channels.UpdateUsername({ channel: ownerChannel, username }));
    requireCondition(usernameUpdated === true || rpcName(usernameUpdated).toLowerCase().includes('true'), 'channels.updateUsername did not return true');
    const bobChannel = await resolveChannel(Api, bob, username);
    const carolChannel = await resolveChannel(Api, carol, username);
    const bobUser = await searchUser(Api, alice, bob.username);
    const invited = await alice.client.invoke(new Api.channels.InviteToChannel({
      channel: ownerChannel,
      users: [inputUser(Api, bobUser)],
    }));
    requireCondition(invited?.updates, 'channels.inviteToChannel returned no updates');
    const bobPeer = inputPeerChannel(Api, bobChannel);
    const carolPeer = inputPeerChannel(Api, carolChannel);
    console.log('PASS channel creation and invitation');

    const text = `${tag} message`;
    await alice.client.invoke(new Api.messages.SendMessage({
      peer: ownerPeer,
      message: text,
      randomId: randomId(),
    }));
    const message = await waitForMessage(() => bob.client.invoke(new Api.messages.GetHistory({
      peer: bobPeer,
      offsetId: 0,
      offsetDate: 0,
      addOffset: 0,
      limit: 20,
      maxId: 0,
      minId: 0,
      hash: 0n,
    })), text);
    requireCondition(typeof message.id === 'number', 'channel message did not have an ID');

    const emoji = (emoticon: string) => new Api.ReactionEmoji({ emoticon });
    await alice.client.invoke(new Api.messages.SendReaction({
      peer: ownerPeer,
      msgId: message.id,
      reaction: [emoji('👍')],
    }));
    await bob.client.invoke(new Api.messages.SendReaction({
      peer: bobPeer,
      msgId: message.id,
      reaction: [emoji('👍')],
    }));
    let reactionUpdate = await alice.client.invoke(new Api.messages.GetMessagesReactions({
      peer: ownerPeer,
      id: [message.id],
    }));
    assertCounts(reactionCounts(reactionUpdate, message.id), { '👍': 2 }, 'two-user read after write');
    console.log('PASS two-user write and read');

    await bob.client.invoke(new Api.messages.SendReaction({
      peer: bobPeer,
      msgId: message.id,
      reaction: [emoji('👍')],
    }));
    reactionUpdate = await alice.client.invoke(new Api.messages.GetMessagesReactions({ peer: ownerPeer, id: [message.id] }));
    assertCounts(reactionCounts(reactionUpdate, message.id), { '👍': 2 }, 'duplicate reaction');

    await bob.client.invoke(new Api.messages.SendReaction({
      peer: bobPeer,
      msgId: message.id,
      reaction: [emoji('🔥')],
    }));
    reactionUpdate = await alice.client.invoke(new Api.messages.GetMessagesReactions({ peer: ownerPeer, id: [message.id] }));
    assertCounts(reactionCounts(reactionUpdate, message.id), { '👍': 1, '🔥': 1 }, 'switched reaction');
    const reactionList = await alice.client.invoke(new Api.messages.GetMessageReactionsList({
      peer: ownerPeer,
      id: message.id,
      limit: 20,
    }));
    const reactionsByUser = new Map<string, string>();
    for (const item of reactionList?.reactions || []) {
      reactionsByUser.set(String(item?.peerId?.userId), reactionEmoticon(item) || '');
    }
    requireCondition(
      reactionList?.count === 2 && reactionsByUser.get(String(alice.id)) === '👍' && reactionsByUser.get(String(bob.id)) === '🔥',
      `messages.getMessageReactionsList returned ${rpcName(reactionList)}`,
    );
    console.log('PASS duplicate, switch, and list');

    await alice.client.invoke(new Api.messages.SetDefaultReaction({ reaction: emoji('👍') }));
    await bob.client.invoke(new Api.messages.SetDefaultReaction({ reaction: emoji('🔥') }));
    const aliceTop = await alice.client.invoke(new Api.messages.GetTopReactions({ limit: 10, hash: 0n }));
    const bobTop = await bob.client.invoke(new Api.messages.GetTopReactions({ limit: 10, hash: 0n }));
    requireCondition(reactionEmoticon(aliceTop?.reactions?.[0]) === '👍', `Alice top reactions returned ${rpcName(aliceTop)}`);
    requireCondition(reactionEmoticon(bobTop?.reactions?.[0]) === '🔥', `Bob top reactions returned ${rpcName(bobTop)}`);
    console.log('PASS user-scoped top reactions');

    await expectRpcError('invalid peer rejection', 'PEER_ID_INVALID', () => alice.client.invoke(new Api.messages.SendReaction({
      peer: new Api.InputPeerEmpty(),
      msgId: message.id,
      reaction: [emoji('👎')],
    })));
    await expectRpcError('wrong access-hash rejection', 'CHANNEL_INVALID', () => bob.client.invoke(new Api.messages.GetMessagesReactions({
      peer: new Api.InputPeerChannel({
        channelId: createdChannel.id,
        accessHash: wrongAccessHash(createdChannel.accessHash),
      }),
      id: [message.id],
    })));
    await expectRpcError('outsider reaction rejection', 'USER_NOT_PARTICIPANT', () => carol.client.invoke(new Api.messages.SendReaction({
      peer: carolPeer,
      msgId: message.id,
      reaction: [emoji('👎')],
    })));
    await expectRpcError('outsider reaction read rejection', 'USER_NOT_PARTICIPANT', () => carol.client.invoke(new Api.messages.GetMessagesReactions({
      peer: carolPeer,
      id: [message.id],
    })));
  } finally {
    if (createdChannel && alice) {
      try {
        await alice.client.invoke(new Api.channels.DeleteChannel({ channel: inputChannel(Api, createdChannel) }));
      } catch (error) {
        console.error(`channel cleanup failed: ${String(error)}`);
      }
    }
    for (const actor of [alice, bob, carol]) actor?.client.destroy();
  }
}

void run()
  .then(() => process.exit(0))
  .catch((error) => {
    console.error(`reaction flow probe fatal: ${error instanceof Error && error.stack ? error.stack : String(error)}`);
    process.exit(1);
  });
