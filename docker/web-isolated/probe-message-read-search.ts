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

type Actor = { client: any; id: bigint | number; username: string };

const quietLogger = { debug() {}, info() {}, warn() {}, error() {} };
let randomIdCounter = 0n;

function requireCondition(condition: unknown, message: string): asserts condition {
  if (!condition) throw new Error(message);
}

function rpcName(value: any) {
  return value?.className || value?.constructor?.name || typeof value;
}

function messages(value: any): any[] {
  return [
    ...(Array.isArray(value?.messages) ? value.messages : []),
    ...(Array.isArray(value?.newMessages) ? value.newMessages : []),
  ];
}

function chats(value: any): any[] {
  return [
    ...(Array.isArray(value?.chats) ? value.chats : []),
    ...(Array.isArray(value?.updates?.chats) ? value.updates.chats : []),
  ];
}

function randomId() {
  randomIdCounter += 1n;
  return BigInt(Date.now()) * 1_000_000n + randomIdCounter;
}

function inputUser(Api: any, user: any) {
  requireCondition(user?.id !== undefined && user?.accessHash !== undefined, 'user has no access hash');
  return new Api.InputUser({ userId: user.id, accessHash: user.accessHash });
}

function inputPeerUser(Api: any, user: any) {
  requireCondition(user?.id !== undefined && user?.accessHash !== undefined, 'user has no peer access hash');
  return new Api.InputPeerUser({ userId: user.id, accessHash: user.accessHash });
}

function inputPeerChat(Api: any, chat: any) {
  requireCondition(chat?.id !== undefined, 'chat has no id');
  return new Api.InputPeerChat({ chatId: chat.id });
}

function findMessage(value: any, text: string) {
  return messages(value).find((candidate) => candidate?.message === text);
}

async function waitForMessage(action: () => Promise<any>, text: string, attempts = 12) {
  let value: any;
  for (let attempt = 0; attempt < attempts; attempt += 1) {
    value = await action();
    if (findMessage(value, text)) return value;
    await new Promise((resolve) => setTimeout(resolve, 250));
  }
  return value;
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
      firstAndLastNames: async () => ['Message', 'Probe'],
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

function historyRequest(Api: any, peer: any) {
  return new Api.messages.GetHistory({
    peer,
    offsetId: 0,
    offsetDate: 0,
    addOffset: 0,
    limit: 20,
    maxId: 0,
    minId: 0,
    hash: 0n,
  });
}

function searchRequest(Api: any, peer: any, query: string) {
  return new Api.messages.Search({
    peer,
    q: query,
    filter: new Api.InputMessagesFilterEmpty(),
    minDate: 0,
    maxDate: 0,
    offsetId: 0,
    addOffset: 0,
    limit: 20,
    maxId: 0,
    minId: 0,
    hash: 0n,
  });
}

function unreadMentionsRequest(Api: any, peer: any) {
  return new Api.messages.GetUnreadMentions({
    peer,
    offsetId: 0,
    addOffset: 0,
    limit: 20,
    maxId: 0,
    minId: 0,
  });
}

async function expectError(name: string, code: string, action: () => Promise<unknown>) {
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
  const actors: Actor[] = [];
  let owner: Actor | undefined;
  let groupId: bigint | number | undefined;
  let directMessageId: number | undefined;
  let runError: unknown;

  try {
    actors.push(...await Promise.all(FIXTURES.map((fixture) => login(gram, fixture))));
    const [alice, bob, carol] = actors;
    owner = alice;
    const bobUser = await searchUser(Api, alice, bob.username);
    const aliceUser = await searchUser(Api, bob, alice.username);
    const tag = `message_read_search_${Date.now()}_${process.pid}`;

    const directText = `${tag} direct`;
    const sent = await alice.client.invoke(new Api.messages.SendMessage({
      peer: inputPeerUser(Api, bobUser),
      message: directText,
      randomId: randomId(),
    }));
    requireCondition(rpcName(sent).toLowerCase().includes('updates'), 'direct send did not return updates');
    console.log('PASS direct write');

    const bobPeer = inputPeerUser(Api, aliceUser);
    const directHistory = await waitForMessage(
      () => bob.client.invoke(historyRequest(Api, bobPeer)),
      directText,
    );
    const bobMessage = findMessage(directHistory, directText);
    requireCondition(bobMessage?.id > 0, 'messages.getHistory omitted the direct message');
    console.log(`PASS direct messages.getHistory: ${String(bobMessage.id)}`);

    const directById = await bob.client.invoke(new Api.messages.GetMessages({
      id: [new Api.InputMessageID({ id: bobMessage.id })],
    }));
    requireCondition(findMessage(directById, directText), 'messages.getMessages omitted the direct message');
    console.log('PASS direct messages.getMessages');

    const directSearch = await bob.client.invoke(searchRequest(Api, bobPeer, tag));
    requireCondition(findMessage(directSearch, directText), 'messages.search omitted the direct message');
    console.log('PASS direct messages.search');

    const globalSearch = await bob.client.invoke(new Api.messages.SearchGlobal({
      q: tag,
      filter: new Api.InputMessagesFilterEmpty(),
      minDate: 0,
      maxDate: 0,
      offsetRate: 0,
      offsetPeer: new Api.InputPeerEmpty(),
      offsetId: 0,
      limit: 20,
    }));
    requireCondition(findMessage(globalSearch, directText), 'messages.searchGlobal omitted the direct message');
    console.log('PASS direct messages.searchGlobal');

    const directRead = await bob.client.invoke(new Api.messages.ReadHistory({
      peer: bobPeer,
      maxId: bobMessage.id,
    }));
    requireCondition(Number.isInteger(directRead?.pts) && Number.isInteger(directRead?.ptsCount), 'messages.readHistory returned an untyped result');
    console.log(`PASS direct messages.readHistory: pts=${String(directRead.pts)} count=${String(directRead.ptsCount)}`);

    const directContents = await bob.client.invoke(new Api.messages.ReadMessageContents({ id: [bobMessage.id] }));
    requireCondition(Number.isInteger(directContents?.pts) && Number.isInteger(directContents?.ptsCount), 'messages.readMessageContents returned an untyped result');
    console.log('PASS direct messages.readMessageContents');

    const groupTitle = `${tag} group`;
    const created = await alice.client.invoke(new Api.messages.CreateChat({
      users: [inputUser(Api, bobUser)],
      title: groupTitle,
    }));
    const group = chats(created).find((candidate: any) => candidate?.title === groupTitle);
    requireCondition(group?.id !== undefined, 'messages.createChat did not return the temporary group');
    groupId = group.id;
    const groupPeer = inputPeerChat(Api, group);
    console.log(`PASS basic-group creation: ${String(groupId)}`);

    const groupText = `${tag} group`;
    const groupSend = await alice.client.invoke(new Api.messages.SendMessage({
      peer: groupPeer,
      message: groupText,
      randomId: randomId(),
    }));
    requireCondition(rpcName(groupSend).toLowerCase().includes('updates'), 'group send did not return updates');
    const groupHistory = await waitForMessage(() => bob.client.invoke(historyRequest(Api, groupPeer)), groupText);
    const groupMessage = findMessage(groupHistory, groupText);
    requireCondition(groupMessage?.id > 0, 'messages.getHistory omitted the group message');
    console.log(`PASS group messages.getHistory: ${String(groupMessage.id)}`);

    const groupById = await bob.client.invoke(new Api.messages.GetMessages({
      id: [new Api.InputMessageID({ id: groupMessage.id })],
    }));
    requireCondition(findMessage(groupById, groupText), 'messages.getMessages omitted the group message');
    console.log('PASS group messages.getMessages');

    const groupSearch = await bob.client.invoke(searchRequest(Api, groupPeer, tag));
    requireCondition(findMessage(groupSearch, groupText), 'messages.search omitted the group message');
    console.log('PASS group messages.search');

    const groupRead = await bob.client.invoke(new Api.messages.ReadHistory({
      peer: groupPeer,
      maxId: groupMessage.id,
    }));
    requireCondition(Number.isInteger(groupRead?.pts) && Number.isInteger(groupRead?.ptsCount), 'group readHistory returned an untyped result');
    console.log(`PASS group messages.readHistory: pts=${String(groupRead.pts)} count=${String(groupRead.ptsCount)}`);

    const groupContents = await bob.client.invoke(new Api.messages.ReadMessageContents({ id: [groupMessage.id] }));
    requireCondition(Number.isInteger(groupContents?.pts) && Number.isInteger(groupContents?.ptsCount), 'group readMessageContents returned an untyped result');
    console.log('PASS group messages.readMessageContents');

    const mentionText = `Bob ${tag} mention`;
    const mentionSend = await alice.client.invoke(new Api.messages.SendMessage({
      peer: groupPeer,
      message: mentionText,
      entities: [new Api.InputMessageEntityMentionName({
        offset: 0,
        length: 3,
        userId: inputUser(Api, bobUser),
      })],
      randomId: randomId(),
    }));
    requireCondition(rpcName(mentionSend).toLowerCase().includes('updates'), 'mention send did not return updates');
    const mentionHistory = await waitForMessage(() => bob.client.invoke(historyRequest(Api, groupPeer)), mentionText);
    const mentionMessage = findMessage(mentionHistory, mentionText);
    requireCondition(mentionMessage?.id > 0, 'mention message was not persisted');
    requireCondition(mentionMessage?.mentioned === true && mentionMessage?.mediaUnread === true, 'mention message did not carry unread mention flags');
    console.log(`PASS group mention flags: ${String(mentionMessage.id)}`);

    const unreadBefore = await bob.client.invoke(unreadMentionsRequest(Api, groupPeer));
    requireCondition(findMessage(unreadBefore, mentionText), 'messages.getUnreadMentions omitted the fresh mention');
    console.log('PASS messages.getUnreadMentions hit');
    await expectError('non-member unread mentions rejection', 'USER_NOT_PARTICIPANT', () => carol.client.invoke(unreadMentionsRequest(Api, groupPeer)));

    const mentionContents = await bob.client.invoke(new Api.messages.ReadMessageContents({ id: [mentionMessage.id] }));
    requireCondition(Number.isInteger(mentionContents?.pts) && Number(mentionContents?.ptsCount) >= 1, 'messages.readMessageContents did not clear a mention');
    const unreadAfter = await bob.client.invoke(unreadMentionsRequest(Api, groupPeer));
    requireCondition(!findMessage(unreadAfter, mentionText), 'messages.getUnreadMentions retained the cleared mention');
    console.log(`PASS messages.readMessageContents clears mention: pts_count=${String(mentionContents.ptsCount)}`);

    const outsiderPeer = groupPeer;
    const outsiderHistory = await carol.client.invoke(historyRequest(Api, outsiderPeer));
    requireCondition(!findMessage(outsiderHistory, groupText), 'non-member history leaked the group message');
    console.log(`PASS non-member history no leak: ${rpcName(outsiderHistory)}`);
    const outsiderSearch = await carol.client.invoke(searchRequest(Api, outsiderPeer, tag));
    requireCondition(!findMessage(outsiderSearch, groupText), 'non-member search leaked the group message');
    console.log(`PASS non-member search no leak: ${rpcName(outsiderSearch)}`);
    const outsiderById = await carol.client.invoke(new Api.messages.GetMessages({
      id: [new Api.InputMessageID({ id: groupMessage.id })],
    }));
    requireCondition(!findMessage(outsiderById, groupText), 'non-member getMessages leaked the group message');
    console.log(`PASS non-member getMessages no leak: ${rpcName(outsiderById)}`);

    await expectError('invalid getMessages ID', 'MESSAGE_ID_INVALID', () => bob.client.invoke(new Api.messages.GetMessages({
      id: [new Api.InputMessageID({ id: 0 })],
    })));
    await expectError('empty global search', 'SEARCH_QUERY_EMPTY', () => bob.client.invoke(new Api.messages.SearchGlobal({
      q: '',
      filter: new Api.InputMessagesFilterEmpty(),
      minDate: 0,
      maxDate: 0,
      offsetRate: 0,
      offsetPeer: new Api.InputPeerEmpty(),
      offsetId: 0,
      limit: 20,
    })));
    await expectError('readMentions fail-closed', 'METHOD_NOT_IMPL', () => bob.client.invoke(new Api.messages.ReadMentions({ peer: groupPeer })));

    directMessageId = Number((await alice.client.invoke(new Api.messages.GetHistory({
      peer: inputPeerUser(Api, bobUser),
      offsetId: 0,
      offsetDate: 0,
      addOffset: 0,
      limit: 20,
      maxId: 0,
      minId: 0,
      hash: 0n,
    }))).messages?.find((candidate: any) => candidate?.message === directText)?.id);
    requireCondition(Number.isInteger(directMessageId) && directMessageId > 0, 'owner message id unavailable for cleanup');
    console.log(`message-read-search probe passed: ${tag}`);
  } catch (error) {
    runError = error;
    throw error;
  } finally {
    const cleanupErrors: string[] = [];
    if (groupId !== undefined && owner) {
      try {
        await owner.client.invoke(new Api.messages.DeleteChat({ chatId: groupId }));
        console.log('PASS temporary group cleanup');
      } catch (error) {
        cleanupErrors.push(`group cleanup: ${String(error)}`);
      }
    }
    if (directMessageId !== undefined && owner) {
      try {
        await owner.client.invoke(new Api.messages.DeleteMessages({ revoke: true, id: [directMessageId] }));
        console.log('PASS temporary direct message cleanup');
      } catch (error) {
        cleanupErrors.push(`direct message cleanup: ${String(error)}`);
      }
    }
    for (const actor of actors) actor.client.destroy();
    if (cleanupErrors.length > 0 && !runError) throw new Error(cleanupErrors.join('; '));
    if (cleanupErrors.length > 0) console.error(`message-read-search cleanup warnings: ${cleanupErrors.join('; ')}`);
  }
}

void run()
  .then(() => process.exit(0))
  .catch((error) => {
    console.error(`message-read-search probe fatal: ${error instanceof Error && error.stack ? error.stack : String(error)}`);
    process.exit(1);
  });
