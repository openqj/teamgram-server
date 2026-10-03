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

function inputUser(Api: any, user: any) {
  requireCondition(user?.id !== undefined && user?.accessHash !== undefined, 'user has no usable access hash');
  return new Api.InputUser({ userId: user.id, accessHash: user.accessHash });
}

function participantIDs(value: any) {
  requireCondition(Array.isArray(value?.participants), `channels.getParticipants returned ${rpcName(value)}`);
  return new Set(value.participants.map((participant: any) => String(participant?.userId)));
}

function requireMembers(value: any, expected: Actor[], absent: Actor[], label: string) {
  const ids = participantIDs(value);
  for (const actor of expected) {
    requireCondition(ids.has(String(actor.id)), `${label} omitted ${actor.username}`);
  }
  for (const actor of absent) {
    requireCondition(!ids.has(String(actor.id)), `${label} still included ${actor.username}`);
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
  let dave: Actor | undefined;

  try {
    [alice, bob, carol, dave] = await Promise.all(FIXTURES.map((fixture) => login(gram, fixture)));
    const tag = `channel_members_${Date.now()}_${process.pid}`;
    const title = `${tag} public channel`;

    const created = await alice.client.invoke(new Api.channels.CreateChannel({
      broadcast: true,
      title,
      about: 'isolated channel membership lifecycle probe',
    }));
    const createdChannel = chatList(created).find((channel: any) => channel?.title === title);
    requireCondition(createdChannel, 'channels.createChannel did not return the new public channel');
    const ownerChannel = inputChannel(Api, createdChannel);
    console.log(`PASS public channel creation: ${rpcName(created)}`);

    const username = `members_${Date.now()}_${process.pid}`;
    const usernameUpdated = await alice.client.invoke(new Api.channels.UpdateUsername({
      channel: ownerChannel,
      username,
    }));
    requireCondition(usernameUpdated === true || rpcName(usernameUpdated).toLowerCase().includes('true'), 'channels.updateUsername did not return true');
    console.log(`PASS public channel username: ${rpcName(usernameUpdated)}`);

    const bobChannel = inputChannel(Api, await resolveChannel(Api, bob, username));
    const joined = await bob.client.invoke(new Api.channels.JoinChannel({ channel: bobChannel }));
    requireCondition(joined, 'channels.joinChannel returned no result');
    console.log(`PASS non-member join: ${rpcName(joined)}`);

    const joinedMembers = await bob.client.invoke(new Api.channels.GetParticipants({
      channel: bobChannel,
      filter: new Api.ChannelParticipantsRecent(),
      offset: 0,
      limit: 20,
      hash: 0n,
    }));
    requireMembers(joinedMembers, [alice, bob], [carol, dave], 'joined-member roster');
    console.log(`PASS joined member read: ${rpcName(joinedMembers)}`);

    await expectRpcError('duplicate join rejection', 'USER_ALREADY_PARTICIPANT', () => bob.client.invoke(
      new Api.channels.JoinChannel({ channel: bobChannel }),
    ));

    const carolUser = await searchUser(Api, alice, carol.username);
    const invited = await alice.client.invoke(new Api.channels.InviteToChannel({
      channel: ownerChannel,
      users: [inputUser(Api, carolUser)],
    }));
    requireCondition(invited, 'channels.inviteToChannel returned no result');
    console.log(`PASS creator invite: ${rpcName(invited)}`);

    const carolChannel = inputChannel(Api, await resolveChannel(Api, carol, username));
    const invitedMembers = await carol.client.invoke(new Api.channels.GetParticipants({
      channel: carolChannel,
      filter: new Api.ChannelParticipantsRecent(),
      offset: 0,
      limit: 20,
      hash: 0n,
    }));
    requireMembers(invitedMembers, [alice, bob, carol], [dave], 'invited-member roster');
    console.log(`PASS invited member read: ${rpcName(invitedMembers)}`);

    const left = await bob.client.invoke(new Api.channels.LeaveChannel({ channel: bobChannel }));
    requireCondition(left, 'channels.leaveChannel returned no result');
    console.log(`PASS member leave: ${rpcName(left)}`);

    await expectRpcError('former member roster rejection', 'USER_NOT_PARTICIPANT', () => bob.client.invoke(
      new Api.channels.GetParticipants({
        channel: bobChannel,
        filter: new Api.ChannelParticipantsRecent(),
        offset: 0,
        limit: 20,
        hash: 0n,
      }),
    ));

    const outsiderChannel = inputChannel(Api, await resolveChannel(Api, dave, username));
    await expectRpcError('non-member roster rejection', 'USER_NOT_PARTICIPANT', () => dave.client.invoke(
      new Api.channels.GetParticipants({
        channel: outsiderChannel,
        filter: new Api.ChannelParticipantsRecent(),
        offset: 0,
        limit: 20,
        hash: 0n,
      }),
    ));

    const badHashChannel = new Api.InputChannel({
      channelId: createdChannel.id,
      accessHash: wrongAccessHash(createdChannel.accessHash),
    });
    await expectRpcError('wrong access-hash rejection', 'CHANNEL_INVALID', () => carol.client.invoke(
      new Api.channels.GetParticipants({
        channel: badHashChannel,
        filter: new Api.ChannelParticipantsRecent(),
        offset: 0,
        limit: 20,
        hash: 0n,
      }),
    ));

    carol.client.destroy();
    carol = await login(gram, FIXTURES[2]);
    const restartedCarolChannel = inputChannel(Api, await resolveChannel(Api, carol, username));
    const restartedMembers = await carol.client.invoke(new Api.channels.GetParticipants({
      channel: restartedCarolChannel,
      filter: new Api.ChannelParticipantsRecent(),
      offset: 0,
      limit: 20,
      hash: 0n,
    }));
    requireMembers(restartedMembers, [alice, carol], [bob, dave], 'restarted member roster');
    console.log(`PASS restarted member read: ${rpcName(restartedMembers)}`);
  } finally {
    for (const actor of [alice, bob, carol, dave]) actor?.client.destroy();
  }
}

void run()
  .then(() => process.exit(0))
  .catch((error) => {
    console.error(`channel member probe fatal: ${error instanceof Error && error.stack ? error.stack : String(error)}`);
    process.exit(1);
  });
