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
      firstAndLastNames: async () => ['Isolated', 'Link'],
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

async function resolveChannel(Api: any, actor: Actor, username: string) {
  const result = await actor.client.invoke(new Api.contacts.ResolveUsername({ username }));
  const channel = chatList(result).find((candidate: any) => rpcName(candidate).toLowerCase().includes('channel'));
  requireCondition(channel, `contacts.resolveUsername did not return public channel ${username}`);
  return channel;
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

async function createChannel(Api: any, actor: Actor, title: string, about: string) {
  const result = await actor.client.invoke(new Api.channels.CreateChannel({
    broadcast: true,
    title,
    about,
  }));
  const channel = chatList(result).find((candidate: any) => candidate?.title === title);
  requireCondition(channel, 'channels.createChannel did not return the new channel');
  return channel;
}

async function postAndFindMessage(Api: any, actor: Actor, peer: any, text: string) {
  const result = await actor.client.invoke(new Api.messages.SendMessage({
    peer,
    message: text,
    randomId: randomID(),
  }));
  requireCondition(rpcName(result).toLowerCase().includes('updates'), 'messages.sendMessage did not return updates');
  const stored = await history(Api, actor, peer);
  const message = messageList(stored).find((candidate: any) => candidate?.message === text);
  requireCondition(message?.id !== undefined, 'channel history did not return the stored message');
  return message;
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

  try {
    [alice, bob, dave] = await Promise.all(FIXTURES.map((fixture) => login(gram, fixture)));
    const tag = `channel_link_${Date.now()}_${process.pid}`;
    const publicChannel = await createChannel(Api, alice, `${tag} public`, 'isolated channel link probe');
    const ownerInput = inputChannel(Api, publicChannel);
    const ownerPeer = inputPeerChannel(Api, publicChannel);
    const username = `link_${Date.now()}_${process.pid}`;
    const usernameUpdated = await alice.client.invoke(new Api.channels.UpdateUsername({
      channel: ownerInput,
      username,
    }));
    requireCondition(usernameUpdated === true || rpcName(usernameUpdated).toLowerCase().includes('true'), 'channels.updateUsername did not return true');
    console.log(`PASS public channel username: ${rpcName(usernameUpdated)}`);

    const bobInput = inputChannel(Api, await resolveChannel(Api, bob, username));
    await bob.client.invoke(new Api.channels.JoinChannel({ channel: bobInput }));
    const outsiderInput = inputChannel(Api, await resolveChannel(Api, dave, username));
    const message = await postAndFindMessage(Api, alice, ownerPeer, `${tag} public message`);
    const want = `https://t.me/${username}/${message.id}`;

    const ownerLink = await alice.client.invoke(new Api.channels.ExportMessageLink({ channel: ownerInput, id: message.id }));
    requireCondition(ownerLink?.link === want && ownerLink?.html === want, `owner exported link = ${JSON.stringify(ownerLink)}, want ${want}`);
    console.log(`PASS creator exported link: ${ownerLink.link}`);

    const memberLink = await bob.client.invoke(new Api.channels.ExportMessageLink({ channel: bobInput, id: message.id }));
    requireCondition(memberLink?.link === want, `member exported link = ${JSON.stringify(memberLink)}, want ${want}`);
    console.log(`PASS member exported link: ${memberLink.link}`);

    await expectRpcError('non-member export rejection', 'USER_NOT_PARTICIPANT', () => dave.client.invoke(
      new Api.channels.ExportMessageLink({ channel: outsiderInput, id: message.id }),
    ));
    await expectRpcError('wrong access-hash export rejection', 'CHANNEL_INVALID', () => alice.client.invoke(
      new Api.channels.ExportMessageLink({
        channel: new Api.InputChannel({
          channelId: publicChannel.id,
          accessHash: wrongAccessHash(publicChannel.accessHash),
        }),
        id: message.id,
      }),
    ));

    const privateChannel = await createChannel(Api, alice, `${tag} private`, 'isolated private link probe');
    const privateInput = inputChannel(Api, privateChannel);
    const privatePeer = inputPeerChannel(Api, privateChannel);
    const privateMessage = await postAndFindMessage(Api, alice, privatePeer, `${tag} private message`);
    await expectRpcError('private channel export rejection', 'CHANNEL_PRIVATE', () => alice.client.invoke(
      new Api.channels.ExportMessageLink({ channel: privateInput, id: privateMessage.id }),
    ));
  } finally {
    for (const actor of [alice, bob, dave]) actor?.client.destroy();
  }
}

void run()
  .then(() => process.exit(0))
  .catch((error) => {
    console.error(`channel message link probe fatal: ${error instanceof Error && error.stack ? error.stack : String(error)}`);
    process.exit(1);
  });
