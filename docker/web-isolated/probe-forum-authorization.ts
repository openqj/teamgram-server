import path from 'node:path';
import { createRequire } from 'node:module';
import process from 'node:process';

const API_ID = 1;
const API_HASH = '0123456789abcdef0123456789abcdef';
const FIXTURES = [
  { phone: '12025550101', username: 'isolated_test_01' },
  { phone: '12025550102', username: 'isolated_test_02' },
  { phone: '12025550103', username: 'isolated_test_03' },
  { phone: '12025550104', username: 'isolated_test_04' },
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

function chatList(value: any): any[] {
  return [
    ...(Array.isArray(value?.chats) ? value.chats : []),
    ...(Array.isArray(value?.updates?.chats) ? value.updates.chats : []),
  ];
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

function wrongAccessHash(accessHash: bigint | number) {
  const value = BigInt(accessHash);
  return value === 0n ? 1n : value + 1n;
}

function randomID() {
  randomIDCounter += 1n;
  return BigInt(Date.now()) * 1_000_000n + randomIDCounter;
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
    const headerView = new DataView(header.buffer);
    const vectorView = new DataView(vector.buffer);
    headerView.setUint32(0, this.CONSTRUCTOR_ID, true);
    headerView.setUint32(4, 0, true);
    vectorView.setUint32(0, 0x1cb5c415, true);
    vectorView.setInt32(4, this.order.length, true);
    this.order.forEach((id, index) => vectorView.setInt32(8 + index * 4, id, true));

    const peerBytes = this.peer.getBytes();
    const bytes = new Uint8Array(header.length + peerBytes.length + vector.length);
    bytes.set(header);
    bytes.set(peerBytes, header.length);
    bytes.set(vector, header.length + peerBytes.length);
    return bytes;
  }

  readResult(reader: any) {
    return reader.tgReadObject();
  }
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
      firstAndLastNames: async () => ['Isolated', 'Forum'],
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

async function useLayer229(Api: any, actor: Actor) {
  const result = await actor.client.invoke(new Api.InvokeWithLayer({
    layer: 229,
    query: new Api.help.GetConfig(),
  }));
  requireCondition(result, `Layer 229 negotiation failed for ${actor.username}`);
}

async function resolveChannel(Api: any, actor: Actor, username: string) {
  const result = await actor.client.invoke(new Api.contacts.ResolveUsername({ username }));
  const channel = chatList(result).find((candidate: any) => rpcName(candidate).toLowerCase().includes('channel'));
  requireCondition(channel, `contacts.resolveUsername did not return ${username}`);
  return channel;
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
  let outsider: Actor | undefined;
  let moderator: Actor | undefined;
  let createdChannel: any;

  try {
    [alice, bob, outsider, moderator] = await Promise.all(FIXTURES.map((fixture) => login(gram, fixture)));
    await Promise.all([alice, bob, outsider, moderator].map((actor) => useLayer229(Api, actor)));
    const tag = `forum_auth_${Date.now()}_${process.pid}`;
    const title = `${tag} supergroup`;
    const created = await alice.client.invoke(new Api.channels.CreateChannel({
      broadcast: false,
      megagroup: true,
      title,
      about: 'isolated forum authorization probe',
    }));
    createdChannel = chatList(created).find((channel: any) => channel?.title === title);
    requireCondition(createdChannel, 'channels.createChannel did not return the temporary supergroup');
    const ownerChannel = inputChannel(Api, createdChannel);
    const ownerPeer = inputPeerChannel(Api, createdChannel);

    const username = `forumauth_${Date.now()}_${process.pid}`;
    const usernameUpdated = await alice.client.invoke(new Api.channels.UpdateUsername({ channel: ownerChannel, username }));
    requireCondition(usernameUpdated === true || rpcName(usernameUpdated).toLowerCase().includes('true'), 'channels.updateUsername did not return true');
    const bobUser = await searchUser(Api, alice, bob.username);
    const moderatorUser = await searchUser(Api, alice, moderator.username);
    await alice.client.invoke(new Api.channels.InviteToChannel({
      channel: ownerChannel,
      users: [inputUser(Api, bobUser), inputUser(Api, moderatorUser)],
    }));
    await alice.client.invoke(new Api.channels.EditAdmin({
      channel: ownerChannel,
      userId: inputUser(Api, moderatorUser),
      adminRights: new Api.ChatAdminRights({ manageTopics: true }),
      rank: 'forum moderator',
    }));
    const bobChannel = inputChannel(Api, await resolveChannel(Api, bob, username));
    const bobPeer = inputPeerChannel(Api, await resolveChannel(Api, bob, username));
    const moderatorPeer = inputPeerChannel(Api, await resolveChannel(Api, moderator, username));
    const outsiderChannel = inputChannel(Api, await resolveChannel(Api, outsider, username));
    const outsiderPeer = inputPeerChannel(Api, await resolveChannel(Api, outsider, username));
    console.log(`PASS supergroup creation, Alice -> Bob invitation, and ManageTopics promotion: ${rpcName(created)}`);

    await alice.client.invoke(new Api.channels.ToggleForum({ channel: ownerChannel, enabled: true, tabs: true }));
    const ownerTopics = await alice.client.invoke(new Api.messages.CreateForumTopic({
      peer: ownerPeer,
      title: `${tag} owner topic`,
      randomId: randomID(),
    }));
    requireCondition(ownerTopics, 'messages.createForumTopic returned no updates');
    const listed = await alice.client.invoke(new Api.messages.GetForumTopics({
      peer: ownerPeer,
      offsetDate: 0,
      offsetId: 0,
      offsetTopic: 0,
      limit: 20,
    }));
    const topic = listed?.topics?.find((candidate: any) => candidate?.title === `${tag} owner topic`);
    requireCondition(topic?.id !== undefined, 'owner topic was not listed');
    console.log(`PASS owner forum create and read: ${rpcName(listed)}`);

    const byID = await alice.client.invoke(new Api.messages.GetForumTopicsByID({
      peer: ownerPeer,
      topics: [topic.id],
    }));
    requireCondition(byID?.topics?.length === 1 && byID.topics[0]?.id === topic.id, 'messages.getForumTopicsByID did not return the owner topic');
    console.log(`PASS owner forum get-by-id: ${rpcName(byID)}`);

    await alice.client.invoke(new Api.messages.EditForumTopic({
      peer: ownerPeer,
      topicId: topic.id,
      title: `${tag} owner topic edited`,
      iconEmojiId: 42n,
      closed: true,
      hidden: true,
    }));
    const editedByID = await alice.client.invoke(new Api.messages.GetForumTopicsByID({
      peer: ownerPeer,
      topics: [topic.id],
    }));
    const editedTopic = editedByID?.topics?.[0];
    requireCondition(
      editedTopic?.title === `${tag} owner topic edited`
        && Number(editedTopic?.iconEmojiId) === 42
        && editedTopic?.closed === true
        && editedTopic?.hidden === true,
      'messages.editForumTopic did not persist the edited topic fields',
    );
    console.log(`PASS owner forum edit and readback: ${rpcName(editedByID)}`);

    const pinned = await alice.client.invoke(new Api.messages.UpdatePinnedForumTopic({
      peer: ownerPeer,
      topicId: topic.id,
      pinned: true,
    }));
    requireCondition(pinned, 'messages.updatePinnedForumTopic returned no updates');
    const pinnedByID = await alice.client.invoke(new Api.messages.GetForumTopicsByID({
      peer: ownerPeer,
      topics: [topic.id],
    }));
    requireCondition(pinnedByID?.topics?.[0]?.pinned === true, 'messages.updatePinnedForumTopic did not persist pinned state');
    console.log(`PASS owner forum pin and readback: ${rpcName(pinnedByID)}`);

    await alice.client.invoke(new Api.messages.CreateForumTopic({
      peer: ownerPeer,
      title: `${tag} reorder topic`,
      randomId: randomID(),
    }));
    const beforeReorder = await alice.client.invoke(new Api.messages.GetForumTopics({
      peer: ownerPeer,
      offsetDate: 0,
      offsetId: 0,
      offsetTopic: 0,
      limit: 20,
    }));
    const reorderTopic = beforeReorder?.topics?.find((candidate: any) => candidate?.title === `${tag} reorder topic`);
    requireCondition(reorderTopic?.id !== undefined, 'reorder topic was not listed');
    const reordered = await alice.client.invoke(new ReorderPinnedForumTopicsRequest(
      ownerPeer,
      [reorderTopic.id, topic.id],
    ));
    requireCondition(reordered, 'messages.reorderPinnedForumTopics returned no updates');
    const afterReorder = await alice.client.invoke(new Api.messages.GetForumTopics({
      peer: ownerPeer,
      offsetDate: 0,
      offsetId: 0,
      offsetTopic: 0,
      limit: 20,
    }));
    requireCondition(afterReorder?.topics?.[0]?.id === reorderTopic.id, 'messages.reorderPinnedForumTopics did not persist order');
    console.log(`PASS owner forum reorder and readback: ${rpcName(afterReorder)}`);

    const deleted = await alice.client.invoke(new Api.messages.DeleteTopicHistory({
      peer: ownerPeer,
      topMsgId: reorderTopic.id,
    }));
    requireCondition(Number(deleted?.ptsCount) === 1, 'messages.deleteTopicHistory did not report one deleted topic');
    const afterDelete = await alice.client.invoke(new Api.messages.GetForumTopicsByID({
      peer: ownerPeer,
      topics: [reorderTopic.id],
    }));
    requireCondition(!afterDelete?.topics?.some((candidate: any) => candidate?.id === reorderTopic.id), 'messages.deleteTopicHistory did not remove the topic');
    console.log(`PASS owner forum delete and readback: ${rpcName(afterDelete)}`);

    const memberTopics = await bob.client.invoke(new Api.messages.GetForumTopics({
      peer: bobPeer,
      offsetDate: 0,
      offsetId: 0,
      offsetTopic: 0,
      limit: 20,
    }));
    requireCondition(memberTopics?.topics?.some((candidate: any) => candidate?.id === topic.id), 'Bob could not read the forum topic');
    console.log(`PASS member forum read: ${rpcName(memberTopics)}`);

    await expectRpcError('member topic create rejection', 'CHAT_ADMIN_REQUIRED', () => bob.client.invoke(new Api.messages.CreateForumTopic({
      peer: bobPeer,
      title: `${tag} blocked member topic`,
      randomId: randomID(),
    })));
    await expectRpcError('member topic edit rejection', 'CHAT_ADMIN_REQUIRED', () => bob.client.invoke(new Api.messages.EditForumTopic({
      peer: bobPeer,
      topicId: topic.id,
      title: `${tag} blocked edit`,
    })));
    await expectRpcError('member topic pin rejection', 'CHAT_ADMIN_REQUIRED', () => bob.client.invoke(new Api.messages.UpdatePinnedForumTopic({
      peer: bobPeer,
      topicId: topic.id,
      pinned: true,
    })));
    await expectRpcError('member topic reorder rejection', 'CHAT_ADMIN_REQUIRED', () => bob.client.invoke(new ReorderPinnedForumTopicsRequest(
      bobPeer,
      [topic.id],
    )));
    await expectRpcError('member forum toggle rejection', 'CHAT_ADMIN_REQUIRED', () => bob.client.invoke(new Api.channels.ToggleForum({
      channel: bobChannel,
      enabled: false,
      tabs: false,
    })));
    await expectRpcError('member view-as-messages rejection', 'CHAT_ADMIN_REQUIRED', () => bob.client.invoke(new Api.channels.ToggleViewForumAsMessages({
      channel: bobChannel,
      enabled: true,
    })));

    const moderatorTopics = await moderator.client.invoke(new Api.messages.CreateForumTopic({
      peer: moderatorPeer,
      title: `${tag} moderator topic`,
      randomId: randomID(),
    }));
    requireCondition(moderatorTopics, 'ManageTopics admin topic creation returned no updates');
    const moderatorListed = await moderator.client.invoke(new Api.messages.GetForumTopics({
      peer: moderatorPeer,
      offsetDate: 0,
      offsetId: 0,
      offsetTopic: 0,
      limit: 20,
    }));
    requireCondition(moderatorListed?.topics?.some((candidate: any) => candidate?.title === `${tag} moderator topic`), 'ManageTopics admin topic was not listed');
    console.log(`PASS ManageTopics admin forum write: ${rpcName(moderatorListed)}`);

    await expectRpcError('outsider forum read rejection', 'USER_NOT_PARTICIPANT', () => outsider.client.invoke(new Api.messages.GetForumTopics({
      peer: outsiderPeer,
      offsetDate: 0,
      offsetId: 0,
      offsetTopic: 0,
      limit: 20,
    })));
    await expectRpcError('outsider topic create rejection', 'USER_NOT_PARTICIPANT', () => outsider.client.invoke(new Api.messages.CreateForumTopic({
      peer: outsiderPeer,
      title: `${tag} blocked outsider topic`,
      randomId: randomID(),
    })));
    await expectRpcError('outsider forum toggle rejection', 'USER_NOT_PARTICIPANT', () => outsider.client.invoke(new Api.channels.ToggleForum({
      channel: outsiderChannel,
      enabled: false,
      tabs: false,
    })));
    await expectRpcError('wrong access-hash rejection', 'CHANNEL_INVALID', () => bob.client.invoke(new Api.messages.GetForumTopics({
      peer: new Api.InputPeerChannel({
        channelId: createdChannel.id,
        accessHash: wrongAccessHash(createdChannel.accessHash),
      }),
      offsetDate: 0,
      offsetId: 0,
      offsetTopic: 0,
      limit: 20,
    })));
  } finally {
    if (createdChannel && alice) {
      try {
        await alice.client.invoke(new Api.channels.DeleteChannel({ channel: inputChannel(Api, createdChannel) }));
      } catch (error) {
        console.error(`forum probe cleanup failed: ${String(error)}`);
      }
    }
    for (const actor of [alice, bob, outsider, moderator]) actor?.client.destroy();
  }
}

void run()
  .then(() => process.exit(0))
  .catch((error) => {
    console.error(`forum authorization probe fatal: ${error instanceof Error && error.stack ? error.stack : String(error)}`);
    process.exit(1);
  });
