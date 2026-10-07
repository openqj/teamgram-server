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

function inputChannel(Api: any, channel: any) {
  requireCondition(channel?.id !== undefined && channel?.accessHash !== undefined, 'channel has no usable access hash');
  return new Api.InputChannel({ channelId: channel.id, accessHash: channel.accessHash });
}

function inputPeerChannel(Api: any, channel: any) {
  requireCondition(channel?.id !== undefined && channel?.accessHash !== undefined, 'channel has no usable peer access hash');
  return new Api.InputPeerChannel({ channelId: channel.id, accessHash: channel.accessHash });
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
      firstAndLastNames: async () => ['Isolated', 'Channel'],
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

async function loginWithRetry(gram: any, fixture: typeof FIXTURES[number], attempts = 2): Promise<Actor> {
  let lastError: unknown;
  for (let attempt = 1; attempt <= attempts; attempt += 1) {
    try {
      return await login(gram, fixture);
    } catch (error) {
      lastError = error;
      if (attempt < attempts) {
        await new Promise((resolve) => setTimeout(resolve, 250));
      }
    }
  }
  throw lastError;
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
  requireCondition(channel, `contacts.resolveUsername did not return public channel ${username}`);
  return channel;
}

function wrongAccessHash(accessHash: bigint | number) {
  const value = BigInt(accessHash);
  return value === 0n ? 1n : value + 1n;
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

async function history(Api: any, actor: Actor, peer: any) {
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

async function search(Api: any, actor: Actor, peer: any, q: string) {
	return actor.client.invoke(new Api.messages.Search({
		peer,
		q,
		filter: new Api.InputMessagesFilterEmpty(),
		minDate: 0,
		maxDate: 0,
		offsetId: 0,
		addOffset: 0,
		limit: 20,
		maxId: 0,
		minId: 0,
		hash: 0n,
	}));
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
  let dave: Actor | undefined;
  let channelDeleted = false;
  let createdChannel: any;

  try {
    alice = await loginWithRetry(gram, FIXTURES[0]);
    bob = await loginWithRetry(gram, FIXTURES[1]);
    dave = await loginWithRetry(gram, FIXTURES[2]);
    const tag = `channel_message_${Date.now()}_${process.pid}`;
    const title = `${tag} public channel`;
    const created = await alice.client.invoke(new Api.channels.CreateChannel({
      broadcast: true,
      title,
      about: 'isolated channel message lifecycle probe',
    }));
    createdChannel = chatList(created).find((channel: any) => channel?.title === title);
    requireCondition(createdChannel, 'channels.createChannel did not return the new public channel');
    const ownerChannel = inputChannel(Api, createdChannel);
    const ownerPeer = inputPeerChannel(Api, createdChannel);
    console.log(`PASS channel creation: ${rpcName(created)}`);

    const username = `messages_${Date.now()}_${process.pid}`;
    const usernameUpdated = await alice.client.invoke(new Api.channels.UpdateUsername({
      channel: ownerChannel,
      username,
    }));
    requireCondition(usernameUpdated === true || rpcName(usernameUpdated).toLowerCase().includes('true'), 'channels.updateUsername did not return true');
    console.log(`PASS channel username: ${rpcName(usernameUpdated)}`);

    const bobChannel = await resolveChannel(Api, bob, username);
    const bobInputChannel = inputChannel(Api, bobChannel);
    const bobPeer = inputPeerChannel(Api, bobChannel);
    const bobUser = await searchUser(Api, alice, bob.username);
    const invited = await alice.client.invoke(new Api.channels.InviteToChannel({
      channel: ownerChannel,
      users: [new Api.InputUser({ userId: bobUser.id, accessHash: bobUser.accessHash })],
    }));
    requireCondition(rpcName(invited).toLowerCase().includes('invitedusers'), `channels.inviteToChannel returned ${rpcName(invited)}`);
    console.log(`PASS member invite: ${rpcName(invited)}`);

    await expectRpcError('duplicate join after invite rejection', 'USER_ALREADY_PARTICIPANT', () => bob.client.invoke(
      new Api.channels.JoinChannel({ channel: bobInputChannel }),
    ));

    const daveChannel = await resolveChannel(Api, dave, username);
	const daveInputChannel = inputChannel(Api, daveChannel);
    const davePeer = inputPeerChannel(Api, daveChannel);
    const text = `${tag} creator write`;
    const write = await alice.client.invoke(new Api.messages.SendMessage({
      peer: ownerPeer,
      message: text,
      randomId: randomID(),
    }));
    requireCondition(rpcName(write).toLowerCase().includes('updates'), 'messages.sendMessage did not return updates');
    console.log(`PASS creator member write: ${rpcName(write)}`);

	const memberHistory = await history(Api, bob, bobPeer);
	const message = messageList(memberHistory).find((candidate: any) => candidate?.message === text);
	requireCondition(message?.id !== undefined, 'member history did not return the stored channel message');
	console.log(`PASS member read: ${rpcName(memberHistory)}`);
	const memberSearch = await search(Api, bob, bobPeer, tag);
	requireCondition(messageList(memberSearch).some((candidate: any) => candidate?.message === text), 'member search did not return the stored channel message');
	console.log(`PASS member search: ${rpcName(memberSearch)}`);

	const byID = await bob.client.invoke(new Api.channels.GetMessages({
      channel: bobInputChannel,
      id: [new Api.InputMessageID({ id: message.id })],
    }));
    requireCondition(messageList(byID).some((candidate: any) => candidate?.message === text), 'channels.getMessages did not return the stored message');
    console.log(`PASS member read by ID: ${rpcName(byID)}`);

    const channelRead = await bob.client.invoke(new Api.channels.ReadHistory({
      channel: bobInputChannel,
      maxId: message.id,
    }));
    requireCondition(channelRead === true || rpcName(channelRead).toLowerCase().includes('true'), 'channels.readHistory did not return true');
    const readState = await bob.client.invoke(new Api.channels.GetFullChannel({ channel: bobInputChannel }));
    requireCondition(
      Number(readState?.fullChat?.readInboxMaxId) >= Number(message.id),
      'channels.readHistory did not advance the member read cursor',
    );
    console.log(`PASS member read receipt: ${rpcName(channelRead)}`);

    const contentRead = await bob.client.invoke(new Api.channels.ReadMessageContents({
      channel: bobInputChannel,
      id: [message.id],
    }));
    requireCondition(contentRead === true || rpcName(contentRead).toLowerCase().includes('true'), 'channels.readMessageContents did not return true');
    console.log(`PASS channel content read: ${rpcName(contentRead)}`);

	await expectRpcError('non-member history rejection', 'USER_NOT_PARTICIPANT', () => history(Api, dave, davePeer));
	await expectRpcError('non-member search rejection', 'USER_NOT_PARTICIPANT', () => search(Api, dave, davePeer, tag));
	await expectRpcError('non-member write rejection', 'USER_NOT_PARTICIPANT', () => dave.client.invoke(
      new Api.messages.SendMessage({ peer: davePeer, message: `${tag} outsider write`, randomId: randomID() }),
    ));
	await expectRpcError('non-member full channel rejection', 'USER_NOT_PARTICIPANT', () => dave.client.invoke(
		new Api.channels.GetFullChannel({ channel: daveInputChannel }),
	));
	await expectRpcError('non-member getMessages rejection', 'USER_NOT_PARTICIPANT', () => dave.client.invoke(
		new Api.channels.GetMessages({ channel: daveInputChannel, id: [new Api.InputMessageID({ id: message.id })] }),
	));
	await expectRpcError('non-member deleteHistory rejection', 'USER_NOT_PARTICIPANT', () => dave.client.invoke(
		new Api.channels.DeleteHistory({ channel: daveInputChannel, maxId: message.id }),
	));
	await expectRpcError('non-member readHistory rejection', 'USER_NOT_PARTICIPANT', () => dave.client.invoke(
		new Api.channels.ReadHistory({ channel: daveInputChannel, maxId: message.id }),
	));

    const badPeer = new Api.InputPeerChannel({
      channelId: createdChannel.id,
      accessHash: wrongAccessHash(createdChannel.accessHash),
    });
    await expectRpcError('wrong access-hash write rejection', 'CHANNEL_INVALID', () => alice.client.invoke(
      new Api.messages.SendMessage({ peer: badPeer, message: `${tag} bad hash write`, randomId: randomID() }),
    ));
	await expectRpcError('wrong access-hash history rejection', 'CHANNEL_INVALID', () => history(Api, bob, badPeer));
	await expectRpcError('wrong access-hash search rejection', 'CHANNEL_INVALID', () => search(Api, bob, badPeer, tag));
	const badChannel = new Api.InputChannel({
	  channelId: createdChannel.id,
	  accessHash: wrongAccessHash(createdChannel.accessHash),
	});
	await expectRpcError('wrong access-hash getMessages rejection', 'CHANNEL_INVALID', () => bob.client.invoke(
	  new Api.channels.GetMessages({ channel: badChannel, id: [new Api.InputMessageID({ id: message.id })] }),
	));
	await expectRpcError('wrong access-hash readHistory rejection', 'CHANNEL_INVALID', () => bob.client.invoke(
	  new Api.channels.ReadHistory({ channel: badChannel, maxId: message.id }),
	));

    const privateDelete = await bob.client.invoke(new Api.channels.DeleteHistory({
      channel: bobInputChannel,
      maxId: message.id,
    }));
    requireCondition(privateDelete, 'channels.deleteHistory returned no result');
    const hiddenHistory = await history(Api, bob, bobPeer);
    requireCondition(!messageList(hiddenHistory).some((candidate: any) => candidate?.message === text), 'member deleteHistory did not hide the stored message');
    const ownerHistory = await history(Api, alice, ownerPeer);
    requireCondition(messageList(ownerHistory).some((candidate: any) => candidate?.message === text), 'member deleteHistory removed the owner message');
	console.log(`PASS member private deleteHistory: ${rpcName(privateDelete)}`);

	await expectRpcError('wrong access-hash deleteHistory rejection', 'CHANNEL_INVALID', () => bob.client.invoke(
	  new Api.channels.DeleteHistory({ channel: badChannel, maxId: message.id }),
	));

	const deleted = await alice.client.invoke(new Api.channels.DeleteChannel({ channel: ownerChannel }));
	requireCondition(rpcName(deleted).toLowerCase().includes('updates'), 'channels.deleteChannel did not return updates');
	channelDeleted = true;
	console.log(`PASS channel cleanup: ${rpcName(deleted)}`);
	await expectRpcError('deleted channel rejection', 'CHANNEL_INVALID', () => alice.client.invoke(
	  new Api.channels.GetFullChannel({ channel: ownerChannel }),
	));
  } finally {
    if (!channelDeleted && alice && createdChannel) {
      try {
        await alice.client.invoke(new Api.channels.DeleteChannel({ channel: inputChannel(Api, createdChannel) }));
      } catch {
        // Preserve the original probe failure while making best-effort cleanup.
      }
    }
    for (const actor of [alice, bob, dave]) actor?.client.destroy();
  }
}

void run()
  .then(() => process.exit(0))
  .catch((error) => {
    console.error(`channel message probe fatal: ${error instanceof Error && error.stack ? error.stack : String(error)}`);
    process.exit(1);
  });
