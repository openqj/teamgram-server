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

class EditChatAdminRequest {
  CONSTRUCTOR_ID = 0xa85bd1c2;
  SUBCLASS_OF_ID = 0;
  className = 'messages.EditChatAdmin';
  classType = 'request';

  constructor(
    private readonly chatId: bigint | number,
    private readonly userId: any,
    private readonly isAdmin: boolean,
  ) {}

  getBytes() {
    const inputUser = this.userId.getBytes();
    requireCondition(inputUser.length === 20, 'messages.editChatAdmin requires inputUser bytes');
    const header = new Uint8Array(12);
    const bool = this.isAdmin
      ? new Uint8Array([0xb5, 0x75, 0x72, 0x99])
      : new Uint8Array([0x37, 0x97, 0x79, 0xbc]);
    const view = new DataView(header.buffer);
    view.setUint32(0, this.CONSTRUCTOR_ID, true);
    view.setBigInt64(4, BigInt(this.chatId), true);
    const bytes = new Uint8Array(header.length + inputUser.length + bool.length);
    bytes.set(header);
    bytes.set(inputUser, header.length);
    bytes.set(bool, header.length + inputUser.length);
    return bytes;
  }

  readResult(reader: any) {
    return reader.tgReadObject();
  }
}

function requireCondition(condition: unknown, message: string): asserts condition {
  if (!condition) throw new Error(message);
}

function rpcName(value: any) {
  return value?.className || value?.constructor?.name || typeof value;
}

function isTrue(value: unknown) {
  return value === true || rpcName(value).toLowerCase().includes('true');
}

function chatList(value: any): any[] {
  return [
    ...(Array.isArray(value?.chats) ? value.chats : []),
    ...(Array.isArray(value?.updates?.chats) ? value.updates.chats : []),
  ];
}

function inputUser(Api: any, user: any) {
  requireCondition(user?.id !== undefined && user?.accessHash !== undefined, 'user has no usable access hash');
  return new Api.InputUser({ userId: user.id, accessHash: user.accessHash });
}

function inputPeerChat(Api: any, chatId: bigint | number) {
  return new Api.InputPeerChat({ chatId });
}

function findChat(value: any, chatId: bigint | number) {
  return chatList(value).find((chat) => String(chat?.id) === String(chatId));
}

function participantForUser(full: any, userId: bigint | number) {
  return full?.fullChat?.participants?.participants?.find(
    (participant: any) => String(participant?.userId) === String(userId),
  );
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
      firstAndLastNames: async () => ['Isolated', 'GroupManagement'],
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

async function expectRpcError(name: string, code: string, action: () => Promise<unknown>) {
  try {
    await action();
  } catch (error) {
    requireCondition(String(error).includes(code), `${name} returned ${String(error)}, want ${code}`);
    return;
  }
  throw new Error(`${name} unexpectedly succeeded`);
}

async function eventually<T>(description: string, action: () => Promise<T>, matches: (value: T) => boolean) {
  let last: T | undefined;
  for (let attempt = 0; attempt < 8; attempt += 1) {
    last = await action();
    if (matches(last)) return last;
    await new Promise((resolve) => setTimeout(resolve, 250));
  }
  throw new Error(`${description} did not become visible; last response was ${rpcName(last)}`);
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
  let groupId: bigint | number | undefined;
  let carolUser: any;
  const failures: string[] = [];

  async function check(name: string, action: () => Promise<void>) {
    try {
      await action();
      console.log(`PASS ${name}`);
    } catch (error) {
      const detail = String(error);
      failures.push(`${name}: ${detail}`);
      console.log(`FAIL ${name}: ${detail}`);
    }
  }

  try {
    actors = await Promise.all(FIXTURES.map((fixture) => login(gram, fixture)));
    const [owner, member, removableMember, outsider] = actors;
    requireCondition(owner && member && removableMember && outsider, 'fixture login returned an incomplete actor set');

    const memberUser = await searchUser(Api, owner, member.username);
    carolUser = await searchUser(Api, owner, removableMember.username);
    const outsiderUser = await searchUser(Api, owner, outsider.username);
    const tag = `group_management_${Date.now()}_${process.pid}`;
    const initialTitle = `${tag} initial`;
    const created = await owner.client.invoke(new Api.messages.CreateChat({
      users: [inputUser(Api, memberUser)],
      title: initialTitle,
    }));
    const group = chatList(created).find((chat) => chat?.title === initialTitle);
    requireCondition(group?.id !== undefined, 'messages.createChat did not return the new group');
    groupId = group.id;
    const peer = inputPeerChat(Api, groupId);
    console.log(`PASS group creation: ${rpcName(created)} (${String(groupId)})`);

    await check('messages.getChats returns the created group to its owner', async () => {
      const chats = await owner.client.invoke(new Api.messages.GetChats({ id: [groupId!] }));
      const found = findChat(chats, groupId!);
      requireCondition(found?.title === initialTitle, 'messages.getChats omitted the created group or its title');
    });

    await check('messages.getChats does not disclose a basic group to a non-member', async () => {
      const chats = await outsider.client.invoke(new Api.messages.GetChats({ id: [groupId!] }));
      const found = findChat(chats, groupId!);
      requireCondition(found?.title !== initialTitle, 'messages.getChats disclosed the group title to a non-member');
    });

    await check('messages.getChats ignores an unknown group id', async () => {
      const unknownId = BigInt(groupId!) + 9_000_000_000n;
      const chats = await owner.client.invoke(new Api.messages.GetChats({ id: [groupId!, unknownId] }));
      requireCondition(findChat(chats, groupId!), 'messages.getChats lost the known group when an unknown id was included');
      requireCondition(!findChat(chats, unknownId), 'messages.getChats returned an entity for an unknown group id');
    });

    await check('messages.getFullChat returns initial members', async () => {
      const full = await owner.client.invoke(new Api.messages.GetFullChat({ chatId: groupId! }));
      requireCondition(participantForUser(full, owner.id), 'messages.getFullChat omitted the owner');
      requireCondition(participantForUser(full, member.id), 'messages.getFullChat omitted the initial member');
      requireCondition(Array.isArray(full?.users) && full.users.some((user: any) => String(user?.id) === String(member.id)), 'messages.getFullChat did not hydrate the initial member');
    });

    await check('messages.getFullChat rejects a non-member', async () => {
      await expectRpcError('non-member full-chat read', 'PEER_ID_INVALID', () => outsider.client.invoke(
        new Api.messages.GetFullChat({ chatId: groupId! }),
      ));
    });

    await check('messages.editChatTitle rejects an empty title', async () => {
      await expectRpcError('empty title', 'CHAT_TITLE_EMPTY', () => owner.client.invoke(
        new Api.messages.EditChatTitle({ chatId: groupId!, title: '' }),
      ));
    });

    await check('messages.editChatTitle rejects a normal member', async () => {
      await expectRpcError('normal member title edit', 'CHAT_ADMIN_REQUIRED', () => member.client.invoke(
        new Api.messages.EditChatTitle({ chatId: groupId!, title: `${tag} denied` }),
      ));
    });

    const ownerTitle = `${tag} owner title`;
    await check('messages.editChatTitle writes and messages.getChats reads it back', async () => {
      const updated = await owner.client.invoke(new Api.messages.EditChatTitle({ chatId: groupId!, title: ownerTitle }));
      requireCondition(rpcName(updated).toLowerCase().includes('updates'), 'messages.editChatTitle did not return updates');
      await eventually('edited title', () => owner.client.invoke(new Api.messages.GetChats({ id: [groupId!] })), (chats) => (
        findChat(chats, groupId!)?.title === ownerTitle
      ));
    });

    await check('messages.addChatUser writes a member visible through messages.getFullChat', async () => {
      const added = await owner.client.invoke(new Api.messages.AddChatUser({
        chatId: groupId!,
        userId: inputUser(Api, carolUser),
        fwdLimit: 0,
      }));
      requireCondition(added?.updates, 'messages.addChatUser did not return updates');
      await eventually('added member', () => owner.client.invoke(new Api.messages.GetFullChat({ chatId: groupId! })), (full) => (
        Boolean(participantForUser(full, removableMember.id))
          && Array.isArray(full?.users)
          && full.users.some((user: any) => String(user?.id) === String(removableMember.id))
      ));
    });

    await check('messages.addChatUser rejects a duplicate member', async () => {
      await expectRpcError('duplicate group member', 'USER_ALREADY_PARTICIPANT', () => owner.client.invoke(
        new Api.messages.AddChatUser({
          chatId: groupId!,
          userId: inputUser(Api, carolUser),
          fwdLimit: 0,
        }),
      ));
    });

    await check('messages.editChatAbout rejects a normal member', async () => {
      await expectRpcError('normal member about edit', 'CHAT_ADMIN_REQUIRED', () => member.client.invoke(
        new Api.messages.EditChatAbout({ peer, about: `${tag} denied about` }),
      ));
    });

    const about = `${tag} persisted about`;
    await check('messages.editChatAbout writes and messages.getFullChat reads it back', async () => {
      const updated = await owner.client.invoke(new Api.messages.EditChatAbout({ peer, about }));
      requireCondition(isTrue(updated), 'messages.editChatAbout did not return true');
      await eventually('edited about', () => owner.client.invoke(new Api.messages.GetFullChat({ chatId: groupId! })), (full) => (
        full?.fullChat?.about === about
      ));
    });

    await check('messages.editChatDefaultBannedRights rejects a normal member', async () => {
      await expectRpcError('normal member default-rights edit', 'CHAT_ADMIN_REQUIRED', () => member.client.invoke(
        new Api.messages.EditChatDefaultBannedRights({
          peer,
          bannedRights: new Api.ChatBannedRights({ inviteUsers: true, untilDate: 0 }),
        }),
      ));
    });

    await check('messages.editChatAdmin promotes a member and grants title-edit permission', async () => {
      const promoted = await owner.client.invoke(new EditChatAdminRequest(
        groupId!,
        inputUser(Api, memberUser),
        true,
      ));
      requireCondition(isTrue(promoted), 'messages.editChatAdmin did not return true while promoting');
      await eventually('promoted participant', () => owner.client.invoke(new Api.messages.GetFullChat({ chatId: groupId! })), (full) => (
        rpcName(participantForUser(full, member.id)).toLowerCase().includes('admin')
      ));

      const adminTitle = `${tag} admin title`;
      const updated = await member.client.invoke(new Api.messages.EditChatTitle({ chatId: groupId!, title: adminTitle }));
      requireCondition(rpcName(updated).toLowerCase().includes('updates'), 'promoted member could not edit the title');
      await eventually('admin-edited title', () => owner.client.invoke(new Api.messages.GetChats({ id: [groupId!] })), (chats) => (
        findChat(chats, groupId!)?.title === adminTitle
      ));
    });

    await check('messages.editChatAdmin demotes the member and restores the restriction', async () => {
      const demoted = await owner.client.invoke(new EditChatAdminRequest(
        groupId!,
        inputUser(Api, memberUser),
        false,
      ));
      requireCondition(isTrue(demoted), 'messages.editChatAdmin did not return true while demoting');
      await eventually('demoted participant', () => owner.client.invoke(new Api.messages.GetFullChat({ chatId: groupId! })), (full) => (
        !rpcName(participantForUser(full, member.id)).toLowerCase().includes('admin')
      ));
      await expectRpcError('demoted member title edit', 'CHAT_ADMIN_REQUIRED', () => member.client.invoke(
        new Api.messages.EditChatTitle({ chatId: groupId!, title: `${tag} denied after demotion` }),
      ));
    });

    await check('messages.editChatDefaultBannedRights writes and prevents member invitations', async () => {
      const restricted = await owner.client.invoke(new Api.messages.EditChatDefaultBannedRights({
        peer,
        bannedRights: new Api.ChatBannedRights({ inviteUsers: true, untilDate: 0 }),
      }));
      requireCondition(rpcName(restricted).toLowerCase().includes('updates'), 'messages.editChatDefaultBannedRights did not return updates');
      await eventually('default invite restriction', () => owner.client.invoke(new Api.messages.GetChats({ id: [groupId!] })), (chats) => (
        findChat(chats, groupId!)?.defaultBannedRights?.inviteUsers === true
      ));
      await expectRpcError('restricted member invitation', 'CHAT_ADMIN_REQUIRED', () => member.client.invoke(
        new Api.messages.AddChatUser({
          chatId: groupId!,
          userId: inputUser(Api, outsiderUser),
          fwdLimit: 0,
        }),
      ));
    });

    await check('messages.editChatDefaultBannedRights restores the default rights', async () => {
      const restored = await owner.client.invoke(new Api.messages.EditChatDefaultBannedRights({
        peer,
        bannedRights: new Api.ChatBannedRights({ untilDate: 0 }),
      }));
      requireCondition(rpcName(restored).toLowerCase().includes('updates'), 'default-rights restore did not return updates');
      await eventually('restored default rights', () => owner.client.invoke(new Api.messages.GetChats({ id: [groupId!] })), (chats) => (
        findChat(chats, groupId!)?.defaultBannedRights?.inviteUsers !== true
      ));
    });

    await check('messages.deleteChatUser removes the member and blocks their full-chat read', async () => {
      const deleted = await owner.client.invoke(new Api.messages.DeleteChatUser({
        chatId: groupId!,
        userId: inputUser(Api, carolUser),
        revokeHistory: true,
      }));
      requireCondition(rpcName(deleted).toLowerCase().includes('updates'), 'messages.deleteChatUser did not return updates');
      await eventually('removed member', () => owner.client.invoke(new Api.messages.GetFullChat({ chatId: groupId! })), (full) => (
        !participantForUser(full, removableMember.id)
      ));
      await expectRpcError('removed member full-chat read', 'PEER_ID_INVALID', () => removableMember.client.invoke(
        new Api.messages.GetFullChat({ chatId: groupId! }),
      ));
    });
  } finally {
    if (groupId !== undefined && carolUser && actors[0] && actors[2]) {
      try {
        const owner = actors[0];
        const removableMember = actors[2];
        const full = await owner.client.invoke(new gram.Api.messages.GetFullChat({ chatId: groupId }));
        if (participantForUser(full, removableMember.id)) {
          await owner.client.invoke(new gram.Api.messages.DeleteChatUser({
            chatId: groupId,
            userId: inputUser(gram.Api, carolUser),
          }));
          console.log('CLEANUP removed temporary member');
        }
        const deleted = await owner.client.invoke(new gram.Api.messages.DeleteChat({ chatId: groupId }));
        requireCondition(isTrue(deleted), 'messages.deleteChat did not return true during cleanup');
        console.log('CLEANUP deleted temporary group');
      } catch (error) {
        failures.push(`cleanup temporary group: ${String(error)}`);
        console.log(`FAIL cleanup temporary group: ${String(error)}`);
      }
    }
    for (const actor of actors) actor.client.destroy();
  }

  if (failures.length > 0) {
    throw new Error(`group-management probe failed:\n${failures.join('\n')}`);
  }
}

void run()
  .then(() => process.exit(0))
  .catch((error) => {
    console.error(`group-management probe fatal: ${error instanceof Error && error.stack ? error.stack : String(error)}`);
    process.exit(1);
  });
