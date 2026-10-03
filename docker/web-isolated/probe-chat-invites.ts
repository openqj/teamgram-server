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

function inviteHash(link: string) {
  const marker = '/+';
  const index = link.lastIndexOf(marker);
  requireCondition(index >= 0, `invite link has no hash: ${link}`);
  const hash = link.slice(index + marker.length);
  requireCondition(hash.length === 20, `invite hash has length ${hash.length}, want 20`);
  return hash;
}

function inputUser(Api: any, user: any) {
  requireCondition(user?.id !== undefined && user?.accessHash !== undefined, 'user has no usable access hash');
  return new Api.InputUser({ userId: user.id, accessHash: user.accessHash });
}

function inputPeerChat(Api: any, chat: any) {
  requireCondition(chat?.id !== undefined, 'chat has no id');
  return new Api.InputPeerChat({ chatId: chat.id });
}

function containsInvite(value: any, link: string) {
  return Array.isArray(value?.invites) && value.invites.some((invite: any) => invite?.link === link);
}

function containsImporter(value: any, userID: bigint | number) {
  return Array.isArray(value?.importers)
    && value.importers.some((importer: any) => String(importer?.userId) === String(userID));
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
      firstAndLastNames: async () => ['Isolated', 'Invite'],
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

async function getInviteImporters(Api: any, actor: Actor, peer: any, link: string) {
  return actor.client.invoke(new Api.messages.GetChatInviteImporters({
    peer,
    requested: true,
    link,
    offsetDate: 0,
    offsetUser: new Api.InputUserEmpty(),
    limit: 20,
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
  let actors: Actor[] = [];

  try {
    actors = await Promise.all(FIXTURES.map((fixture) => login(gram, fixture)));
    const [owner, member, joiner, requester] = actors;
    requireCondition(owner && member && joiner && requester, 'fixture login returned an incomplete actor set');

    const memberUser = await searchUser(Api, owner, member.username);
    const requesterUser = await searchUser(Api, owner, requester.username);
    const tag = `chat_invites_${Date.now()}_${process.pid}`;
    const title = `${tag} group`;
    const created = await owner.client.invoke(new Api.messages.CreateChat({
      users: [inputUser(Api, memberUser)],
      title,
    }));
    const group = chatList(created).find((chat) => chat?.title === title);
    requireCondition(group?.id !== undefined, 'messages.createChat did not return the new group');
    const peer = inputPeerChat(Api, group);
    console.log(`PASS group creation: ${rpcName(created)}`);

    const normalInvite = await owner.client.invoke(new Api.messages.ExportChatInvite({
      peer,
      title: `${tag} normal`,
    }));
    requireCondition(typeof normalInvite?.link === 'string', 'messages.exportChatInvite did not return a link');
    const normalHash = inviteHash(normalInvite.link);
    const checkedNormal = await joiner.client.invoke(new Api.messages.CheckChatInvite({ hash: normalHash }));
    requireCondition(!checkedNormal?.requestNeeded, 'normal invite check unexpectedly requires approval');
    console.log(`PASS normal invite export and check: ${rpcName(checkedNormal)}`);

    const joined = await joiner.client.invoke(new Api.messages.ImportChatInvite({ hash: normalHash }));
    requireCondition(rpcName(joined).toLowerCase().includes('chatinvitejoinresult'), 'normal invite import did not return join result');
    console.log(`PASS normal invite join: ${rpcName(joined)}`);

    const exported = await owner.client.invoke(new Api.messages.GetExportedChatInvites({
      peer,
      adminId: new Api.InputUserSelf(),
      limit: 20,
    }));
    requireCondition(containsInvite(exported, normalInvite.link), 'exported invite list did not include normal invite');
    console.log(`PASS exported invite readback: ${rpcName(exported)}`);

    await expectRpcError('member export rejection', 'CHAT_ADMIN_REQUIRED', () => member.client.invoke(
      new Api.messages.ExportChatInvite({ peer, title: `${tag} denied` }),
    ));
    await expectRpcError('member exported-list rejection', 'CHAT_ADMIN_REQUIRED', () => member.client.invoke(
      new Api.messages.GetExportedChatInvites({ peer, adminId: new Api.InputUserSelf(), limit: 20 }),
    ));

    const requestInvite = await owner.client.invoke(new Api.messages.ExportChatInvite({
      peer,
      requestNeeded: true,
      title: `${tag} request`,
    }));
    requireCondition(typeof requestInvite?.link === 'string', 'request invite export did not return a link');
    const requestHash = inviteHash(requestInvite.link);
    const checkedRequest = await requester.client.invoke(new Api.messages.CheckChatInvite({ hash: requestHash }));
    requireCondition(checkedRequest?.requestNeeded === true, 'request invite check did not preserve request_needed');
    console.log(`PASS request invite export and check: ${rpcName(checkedRequest)}`);

    await expectRpcError('request invite import acknowledgement', 'INVITE_REQUEST_SENT', () => requester.client.invoke(
      new Api.messages.ImportChatInvite({ hash: requestHash }),
    ));
    let importers = await getInviteImporters(Api, owner, peer, requestInvite.link);
    requireCondition(containsImporter(importers, requester.id), 'request invite importer list omitted requester');
    console.log(`PASS request importer readback: ${rpcName(importers)}`);

    await expectRpcError('member request rejection', 'CHAT_ADMIN_REQUIRED', () => member.client.invoke(
      new Api.messages.HideChatJoinRequest({ peer, userId: inputUser(Api, requesterUser) }),
    ));
    const rejected = await owner.client.invoke(new Api.messages.HideChatJoinRequest({
      peer,
      userId: inputUser(Api, requesterUser),
    }));
    requireCondition(rpcName(rejected).toLowerCase().includes('updates'), 'single request rejection did not return updates');
    importers = await getInviteImporters(Api, owner, peer, requestInvite.link);
    requireCondition(!containsImporter(importers, requester.id), 'single rejection left requester in importer list');
    console.log(`PASS single request rejection: ${rpcName(rejected)}`);

    await expectRpcError('second request invite acknowledgement', 'INVITE_REQUEST_SENT', () => requester.client.invoke(
      new Api.messages.ImportChatInvite({ hash: requestHash }),
    ));
    const rejectedAll = await owner.client.invoke(new Api.messages.HideAllChatJoinRequests({
      peer,
      link: requestInvite.link,
    }));
    requireCondition(rpcName(rejectedAll).toLowerCase().includes('updates'), 'bulk request rejection did not return updates');
    importers = await getInviteImporters(Api, owner, peer, requestInvite.link);
    requireCondition(!containsImporter(importers, requester.id), 'bulk rejection left requester in importer list');
    console.log(`PASS bulk request rejection: ${rpcName(rejectedAll)}`);

    await expectRpcError('invalid invite check', 'INVITE_HASH_INVALID', () => owner.client.invoke(
      new Api.messages.CheckChatInvite({ hash: 'abcdefghijklmnopqrst' }),
    ));
  } finally {
    for (const actor of actors) actor.client.destroy();
  }
}

void run()
  .then(() => process.exit(0))
  .catch((error) => {
    console.error(`chat invite probe fatal: ${error instanceof Error && error.stack ? error.stack : String(error)}`);
    process.exit(1);
  });
