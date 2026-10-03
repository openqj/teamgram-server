import path from 'node:path';
import { createRequire } from 'node:module';
import process from 'node:process';

type GramJs = {
  Api: any;
  WebSocketConnection: any;
  MemorySession: new () => any;
  TelegramClient: new (session: any, apiId: number, apiHash: string, options: Record<string, unknown>) => any;
};

type Actor = {
  client: any;
  id: bigint | number;
  phone: string;
  username: string;
};

const API_ID = 1;
const API_HASH = '0123456789abcdef0123456789abcdef';
const LOGIN_TIMEOUT_MS = 45_000;
const FIXTURES = [
  { phone: '12025550101', username: 'isolated_test_01' },
  { phone: '12025550102', username: 'isolated_test_02' },
  { phone: '12025550103', username: 'isolated_test_03' },
  { phone: '12025550104', username: 'isolated_test_04' },
  { phone: '12025550105', username: 'isolated_test_05' },
  { phone: '12025550106', username: 'isolated_test_06' },
  { phone: '12025550107', username: 'isolated_test_07' },
  { phone: '12025550108', username: 'isolated_test_08' },
  { phone: '12025550109', username: 'isolated_test_09' },
  { phone: '12025550110', username: 'isolated_test_10' },
  { phone: '12025550111', username: 'isolated_test_11' },
  { phone: '12025550112', username: 'isolated_test_12' },
  { phone: '12025550113', username: 'isolated_test_13' },
  { phone: '12025550114', username: 'isolated_test_14' },
];

let randomIdCounter = 0n;

const quietLogger = {
  debug: () => {},
  info: () => {},
  warn: () => {},
  error: () => {},
};

function requireCondition(condition: unknown, message: string): asserts condition {
  if (!condition) throw new Error(message);
}

function rpcName(value: unknown) {
  return (value as { className?: string; constructor?: { name?: string } })?.className
    || (value as { constructor?: { name?: string } })?.constructor?.name
    || typeof value;
}

function messageList(value: any): any[] {
  const messages = Array.isArray(value?.messages) ? value.messages : [];
  const newMessages = Array.isArray(value?.newMessages) ? value.newMessages : [];
  return [...messages, ...newMessages];
}

function chatList(value: any): any[] {
  const direct = Array.isArray(value?.chats) ? value.chats : [];
  const nested = Array.isArray(value?.updates?.chats) ? value.updates.chats : [];
  return [...direct, ...nested];
}

function peerId(value: any): string | undefined {
  const id = value?.userId ?? value?.chatId ?? value?.channelId;
  return id === undefined ? undefined : String(id);
}

function reactionEmoticon(value: any): string | undefined {
  return typeof value?.reaction === 'string' ? value.reaction : value?.reaction?.emoticon;
}

function isBoolTrue(value: unknown): boolean {
  return value === true || rpcName(value).toLowerCase().includes('true');
}

function findMessage(value: any, text: string) {
  return messageList(value).find((message) => message?.message === text);
}

async function waitForMessage(action: () => Promise<any>, text: string, attempts = 10) {
  let value: any;
  for (let attempt = 0; attempt < attempts; attempt += 1) {
    value = await action();
    if (findMessage(value, text)) return value;
    await new Promise((resolve) => setTimeout(resolve, 200));
  }
  return value;
}

function randomId() {
  randomIdCounter += 1n;
  return BigInt(Date.now()) * 1_000_000n + randomIdCounter;
}

function inputUser(Api: any, user: any) {
  requireCondition(user?.id !== undefined && user?.accessHash !== undefined, 'search result lacks a usable input user');
  return new Api.InputUser({ userId: user.id, accessHash: user.accessHash });
}

function inputPeerUser(Api: any, user: any) {
  requireCondition(user?.id !== undefined && user?.accessHash !== undefined, 'search result lacks a usable input peer');
  return new Api.InputPeerUser({ userId: user.id, accessHash: user.accessHash });
}

function inputChannel(Api: any, channel: any) {
  requireCondition(channel?.id !== undefined && channel?.accessHash !== undefined, 'channel creation lacks a usable input channel');
  return new Api.InputChannel({ channelId: channel.id, accessHash: channel.accessHash });
}

function inputPeerChannel(Api: any, channel: any) {
  requireCondition(channel?.id !== undefined && channel?.accessHash !== undefined, 'channel creation lacks a usable input peer');
  return new Api.InputPeerChannel({ channelId: channel.id, accessHash: channel.accessHash });
}

async function loadGramJs(root: string): Promise<GramJs> {
  const requireModule = createRequire(import.meta.url);
  const clientModule = requireModule(path.join(root, 'src/lib/gramjs/client/TelegramClient.ts'));
  const sessionModule = requireModule(path.join(root, 'src/lib/gramjs/sessions/Memory.ts'));
  const tlModule = requireModule(path.join(root, 'src/lib/gramjs/tl/index.ts'));
  const connectionModule = requireModule(path.join(root, 'src/lib/gramjs/network/connection/TCPObfuscated.ts'));

  return {
    TelegramClient: clientModule.default,
    MemorySession: sessionModule.default,
    Api: tlModule.Api,
    WebSocketConnection: connectionModule.ConnectionTCPObfuscated,
  };
}

async function login(gram: GramJs, fixture: typeof FIXTURES[number]): Promise<Actor> {
  console.log(`login start: ${fixture.username}`);
  const client = new gram.TelegramClient(
    new gram.MemorySession(),
    API_ID,
    API_HASH,
    {
      connection: gram.WebSocketConnection,
      fallbackConnection: gram.WebSocketConnection,
      shouldAllowHttpTransport: false,
      shouldForceHttpTransport: false,
      timeout: 15,
      requestRetries: 2,
      connectionRetries: 2,
      connectionRetriesToFallback: 0,
      retryDelay: 250,
      autoReconnect: false,
      baseLogger: quietLogger,
    },
  );

  let timeoutId: ReturnType<typeof setTimeout> | undefined;
  try {
    const loginPromise = client.start({
      phoneNumber: fixture.phone,
      phoneCode: async () => '12345',
      password: async () => '',
      firstAndLastNames: async () => ['Isolated', 'Test'],
      qrCode: async () => {},
      onError: (error: Error) => { throw error; },
      webAuthTokenFailed: () => {},
      onPasskeyOption: () => {},
      initialMethod: 'phoneNumber',
    }).then(() => client.getMe());
    const me: any = await Promise.race([
      loginPromise,
      new Promise<never>((_resolve, reject) => {
        timeoutId = setTimeout(
          () => reject(new Error(`login timed out for ${fixture.username} after ${LOGIN_TIMEOUT_MS}ms`)),
          LOGIN_TIMEOUT_MS,
        );
      }),
    ]);
    requireCondition(me?.id !== undefined, `login did not return ${fixture.username}`);
    console.log(`login complete: ${fixture.username}`);
    return { client, id: me.id, ...fixture };
  } catch (error) {
    client.destroy();
    throw error;
  } finally {
    if (timeoutId !== undefined) clearTimeout(timeoutId);
  }
}

async function searchUser(Api: any, actor: Actor, username: string) {
  const found = await actor.client.invoke(new Api.contacts.Search({ q: username, limit: 10 }));
  requireCondition(Array.isArray(found?.users), `contacts.search returned ${rpcName(found)} without users`);
  const user = found.users.find((candidate: any) => candidate?.username === username);
  requireCondition(user, `contacts.search did not find ${username}`);
  return user;
}

async function expectChatAdminRequired(action: () => Promise<unknown>) {
  try {
    await action();
  } catch (error) {
    requireCondition(String(error).includes('CHAT_ADMIN_REQUIRED'), `expected CHAT_ADMIN_REQUIRED, got ${String(error)}`);
    return;
  }
  throw new Error('non-owner channel edit unexpectedly succeeded');
}

async function run() {
  requireCondition(process.env.TG_DC_HOST === '127.0.0.1', 'TG_DC_HOST must target the isolated gateway');
  requireCondition(process.env.TG_DC_PORT === '31443', 'TG_DC_PORT must target the isolated gateway');

  const globalAny = globalThis as typeof globalThis & { self?: any; addEventListener?: () => void };
  globalAny.self ??= globalThis;
  globalAny.addEventListener ??= () => {};
  globalAny.self.addEventListener ??= globalAny.addEventListener;

  const gramJsDir = process.env.TEAMGRAM_GRAMJS_DIR;
  requireCondition(gramJsDir, 'TEAMGRAM_GRAMJS_DIR is required');
  const gram = await loadGramJs(gramJsDir);
  const { Api } = gram;
  const tag = process.env.TEAMGRAM_FLOW_TAG || `isolated-e2e-${Date.now()}-${process.pid}`;
  const actors: Actor[] = [];
  let createdChannel: any;
  let createdGroupId: bigint | number | undefined;
  let directMessageId: number | undefined;
  let forwardedMessageId: number | undefined;
  let contactInput: any;
  let importedPhone: string | undefined;
  let importedSavedPhone: string | undefined;
  let unregisteredPhone: string | undefined;
  let unregisteredSavedPhone: string | undefined;
  let blockedPeer: any;
  let owner: Actor | undefined;
  let runError: unknown;
  let completed = false;

  try {
    for (const fixture of FIXTURES) {
      actors.push(await login(gram, fixture));
    }
    const [alice, bob, carol, dave, eve, frank, grace, heidi, ivan, julia, karl, lara, mike, nina] = actors;
    owner = alice;
    console.log('contacts and private-message flow');

    const bobUser = await searchUser(Api, alice, bob.username);
    const carolUser = await searchUser(Api, alice, carol.username);
    const daveUser = await searchUser(Api, alice, dave.username);
    const eveUser = await searchUser(Api, alice, eve.username);
    const frankUser = await searchUser(Api, alice, frank.username);
    const graceUser = await searchUser(Api, alice, grace.username);
    const heidiUser = await searchUser(Api, alice, heidi.username);
    const ivanUser = await searchUser(Api, alice, ivan.username);
    const juliaUser = await searchUser(Api, alice, julia.username);
    const karlUser = await searchUser(Api, alice, karl.username);
    const laraUser = await searchUser(Api, alice, lara.username);
    const mikeUser = await searchUser(Api, alice, mike.username);
    const ninaUser = await searchUser(Api, alice, nina.username);

    console.log('phone-contact import and saved-contact flow');
    // Use a registered fixture that has not been added through addContact so
    // this exercises phone lookup, contact creation and saved-phone storage.
    importedPhone = `+${carol.phone}`;
    importedSavedPhone = carol.phone;
    const importedClientId = randomId();
    const imported = await alice.client.invoke(new Api.contacts.ImportContacts({
      contacts: [new Api.InputPhoneContact({
        clientId: importedClientId,
        phone: importedPhone,
        firstName: 'Isolated Test Three',
        lastName: '',
      })],
    }));
    requireCondition(
      rpcName(imported).toLowerCase().includes('importedcontacts')
        && Array.isArray(imported?.imported)
        && imported.imported.some((entry: any) => String(entry?.userId) === String(carolUser.id)
          && String(entry?.clientId) === String(importedClientId)),
      'contacts.importContacts did not return the registered fixture user',
    );
    requireCondition(
      Array.isArray(imported?.users)
        && imported.users.some((user: any) => String(user?.id) === String(carolUser.id)),
      'contacts.importContacts did not return the imported user entity',
    );

    const savedContacts = await alice.client.invoke(new Api.contacts.GetSaved());
    requireCondition(
      Array.isArray(savedContacts)
        && savedContacts.some((saved: any) => saved?.phone === carol.phone),
      'contacts.getSaved did not return the normalized imported phone',
    );
    const importedContacts = await alice.client.invoke(new Api.contacts.GetContacts({ hash: 0n }));
    requireCondition(
      Array.isArray(importedContacts?.users)
        && importedContacts.users.some((user: any) => String(user?.id) === String(carolUser.id)),
      'contacts.getContacts did not include the imported registered user',
    );

    console.log('multi-user unregistered phone import ownership flow');
    unregisteredSavedPhone = `1202555${5000 + Math.floor(Math.random() * 5000)}`;
    unregisteredPhone = `+${unregisteredSavedPhone}`;
    const aliceUnregisteredClientId = randomId();
    const aliceUnregistered = await alice.client.invoke(new Api.contacts.ImportContacts({
      contacts: [new Api.InputPhoneContact({
        clientId: aliceUnregisteredClientId,
        phone: unregisteredPhone,
        firstName: 'Unregistered Contact',
        lastName: 'Alice',
      })],
    }));
    requireCondition(
      Array.isArray(aliceUnregistered?.popularInvites)
        && aliceUnregistered.popularInvites.some((entry: any) => String(entry?.clientId) === String(aliceUnregisteredClientId)
          && entry?.importers === 1),
      'contacts.importContacts did not count Alice as the unregistered phone importer',
    );
    const bobUnregisteredClientId = randomId();
    const bobUnregistered = await bob.client.invoke(new Api.contacts.ImportContacts({
      contacts: [new Api.InputPhoneContact({
        clientId: bobUnregisteredClientId,
        phone: unregisteredPhone,
        firstName: 'Unregistered Contact',
        lastName: 'Bob',
      })],
    }));
    requireCondition(
      Array.isArray(bobUnregistered?.popularInvites)
        && bobUnregistered.popularInvites.some((entry: any) => String(entry?.clientId) === String(bobUnregisteredClientId)
          && entry?.importers === 2),
      'contacts.importContacts did not count both users for the unregistered phone',
    );
    const unregisteredSavedByAlice = await alice.client.invoke(new Api.contacts.GetSaved());
    const unregisteredSavedByBob = await bob.client.invoke(new Api.contacts.GetSaved());
    requireCondition(
      Array.isArray(unregisteredSavedByAlice) && unregisteredSavedByAlice.some((saved: any) => saved?.phone === unregisteredSavedPhone),
      'contacts.getSaved did not return Alice’s unregistered phone import',
    );
    requireCondition(
      Array.isArray(unregisteredSavedByBob) && unregisteredSavedByBob.some((saved: any) => saved?.phone === unregisteredSavedPhone),
      'contacts.getSaved did not return Bob’s unregistered phone import',
    );
    const aliceDeleteUnregistered = await alice.client.invoke(new Api.contacts.DeleteByPhones({ phones: [unregisteredPhone] }));
    requireCondition(isBoolTrue(aliceDeleteUnregistered), 'Alice contacts.deleteByPhones did not return true');
    const aliceSavedAfterUnregisteredDelete = await alice.client.invoke(new Api.contacts.GetSaved());
    const bobSavedAfterAliceDelete = await bob.client.invoke(new Api.contacts.GetSaved());
    requireCondition(
      Array.isArray(aliceSavedAfterUnregisteredDelete)
        && !aliceSavedAfterUnregisteredDelete.some((saved: any) => saved?.phone === unregisteredSavedPhone),
      'contacts.deleteByPhones left Alice’s unregistered phone import in contacts.getSaved',
    );
    requireCondition(
      Array.isArray(bobSavedAfterAliceDelete)
        && bobSavedAfterAliceDelete.some((saved: any) => saved?.phone === unregisteredSavedPhone),
      'Alice contacts.deleteByPhones removed Bob’s saved phone import',
    );
    const bobRepeatClientId = randomId();
    const bobRepeatImport = await bob.client.invoke(new Api.contacts.ImportContacts({
      contacts: [new Api.InputPhoneContact({
        clientId: bobRepeatClientId,
        phone: unregisteredPhone,
        firstName: 'Unregistered Contact',
        lastName: 'Bob',
      })],
    }));
    requireCondition(
      Array.isArray(bobRepeatImport?.popularInvites)
        && bobRepeatImport.popularInvites.some((entry: any) => String(entry?.clientId) === String(bobRepeatClientId)
          && entry?.importers === 1),
      'contacts.deleteByPhones did not preserve Bob as the only remaining importer',
    );
    const bobDeleteUnregistered = await bob.client.invoke(new Api.contacts.DeleteByPhones({ phones: [unregisteredPhone] }));
    requireCondition(isBoolTrue(bobDeleteUnregistered), 'Bob contacts.deleteByPhones did not return true');
    const bobSavedAfterUnregisteredDelete = await bob.client.invoke(new Api.contacts.GetSaved());
    requireCondition(
      Array.isArray(bobSavedAfterUnregisteredDelete)
        && !bobSavedAfterUnregisteredDelete.some((saved: any) => saved?.phone === unregisteredSavedPhone),
      'contacts.deleteByPhones left Bob’s unregistered phone import in contacts.getSaved',
    );

    contactInput = inputUser(Api, bobUser);
    const contactUpdate = await alice.client.invoke(new Api.contacts.AddContact({
      id: contactInput,
      firstName: 'Isolated Test Two',
      lastName: '',
      phone: bob.phone,
    }));
    requireCondition(rpcName(contactUpdate).toLowerCase().includes('updates'), 'contacts.addContact did not return updates');

    const contacts = await alice.client.invoke(new Api.contacts.GetContacts({ hash: 0n }));
    requireCondition(
      Array.isArray(contacts?.users) && contacts.users.some((user: any) => user?.id === bobUser.id),
      'contacts.getContacts did not return the added contact',
    );
    const contactIDs = await alice.client.invoke(new Api.contacts.GetContactIDs({ hash: 0n }));
    requireCondition(
      Array.isArray(contactIDs)
        && contactIDs.some((id: any) => String(id) === String(bobUser.id))
        && contactIDs.some((id: any) => String(id) === String(carolUser.id)),
      'contacts.getContactIDs did not return the imported and added contacts',
    );
    const contactStatuses = await alice.client.invoke(new Api.contacts.GetStatuses());
    const contactStatusIDs = Array.isArray(contactStatuses)
      ? contactStatuses.map((status: any) => String(status?.userId))
      : [];
    requireCondition(
      Array.isArray(contactStatuses)
        && contactStatuses.some((status: any) => String(status?.userId) === String(bobUser.id))
        && contactStatuses.some((status: any) => String(status?.userId) === String(carolUser.id)),
      `contacts.getStatuses did not return statuses for the imported and added contacts: ${contactStatusIDs.join(',')}`,
    );
    const resolvedCarol = await alice.client.invoke(new Api.contacts.ResolveUsername({ username: carol.username }));
    requireCondition(
      rpcName(resolvedCarol).toLowerCase().includes('resolvedpeer')
        && Array.isArray(resolvedCarol?.users)
        && resolvedCarol.users.some((user: any) => String(user?.id) === String(carolUser.id)),
      'contacts.resolveUsername did not resolve Carol to the expected user',
    );
    const resolvedPhone = await alice.client.invoke(new Api.contacts.ResolvePhone({ phone: importedPhone }));
    requireCondition(
      rpcName(resolvedPhone).toLowerCase().includes('resolvedpeer')
        && String(resolvedPhone?.peer?.userId) === String(carolUser.id)
        && Array.isArray(resolvedPhone?.users)
        && resolvedPhone.users.some((user: any) => String(user?.id) === String(carolUser.id)),
      'contacts.resolvePhone did not resolve the imported E.164 phone to Carol',
    );

    console.log('blocked-user flow');
    blockedPeer = inputPeerUser(Api, graceUser);
    const blockResult = await alice.client.invoke(new Api.contacts.Block({ id: blockedPeer }));
    requireCondition(isBoolTrue(blockResult), 'contacts.block did not return true');
    const blockedList = await alice.client.invoke(new Api.contacts.GetBlocked({ offset: 0, limit: 50 }));
    requireCondition(
      Array.isArray(blockedList?.blocked)
        && blockedList.blocked.some((entry: any) => peerId(entry?.peerId) === String(graceUser.id)),
      'contacts.getBlocked did not include the blocked user',
    );
    const unblockResult = await alice.client.invoke(new Api.contacts.Unblock({ id: blockedPeer }));
    requireCondition(isBoolTrue(unblockResult), 'contacts.unblock did not return true');
    const unblockedList = await alice.client.invoke(new Api.contacts.GetBlocked({ offset: 0, limit: 50 }));
    requireCondition(
      !Array.isArray(unblockedList?.blocked)
        || !unblockedList.blocked.some((entry: any) => peerId(entry?.peerId) === String(graceUser.id)),
      'contacts.getBlocked still included the unblocked user',
    );

    const directText = `${tag} direct message`;
    const directUpdates = await alice.client.invoke(new Api.messages.SendMessage({
      peer: inputPeerUser(Api, bobUser),
      message: directText,
      randomId: randomId(),
    }));
    requireCondition(rpcName(directUpdates).toLowerCase().includes('updates'), 'messages.sendMessage did not return updates for a direct message');

    const bobToAlice = inputPeerUser(Api, await searchUser(Api, bob, alice.username));
    const directHistory = await waitForMessage(() => bob.client.invoke(new Api.messages.GetHistory({
      peer: bobToAlice,
      offsetId: 0,
      offsetDate: 0,
      addOffset: 0,
      limit: 20,
      maxId: 0,
      minId: 0,
      hash: 0n,
    })), directText);
    const bobDirectMessage = findMessage(directHistory, directText);
    requireCondition(bobDirectMessage, 'messages.getHistory did not return the direct message');
    // Mutating user messages uses the sender's outbox ID on the isolated backend.
    const aliceHistory = await waitForMessage(() => alice.client.invoke(new Api.messages.GetHistory({
      peer: inputPeerUser(Api, bobUser),
      offsetId: 0,
      offsetDate: 0,
      addOffset: 0,
      limit: 20,
      maxId: 0,
      minId: 0,
      hash: 0n,
    })), directText);
    const directMessage = findMessage(aliceHistory, directText);
    requireCondition(directMessage, 'messages.getHistory did not return the sent direct message for the sender');
    directMessageId = directMessage.id;

    const directSearch = await bob.client.invoke(new Api.messages.Search({
      peer: bobToAlice,
      q: tag,
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
    requireCondition(findMessage(directSearch, directText), 'messages.search did not return the direct message');

    const directRead = await bob.client.invoke(new Api.messages.ReadHistory({
      peer: bobToAlice,
      maxId: bobDirectMessage.id,
    }));
    requireCondition(Number.isInteger(directRead?.pts), 'messages.readHistory did not return pts');

    const markedUnread = await alice.client.invoke(new Api.messages.MarkDialogUnread({
      unread: true,
      peer: new Api.InputDialogPeer({ peer: inputPeerUser(Api, bobUser) }),
    }));
    requireCondition(isBoolTrue(markedUnread), 'messages.markDialogUnread did not mark the dialog');
    const markedRead = await alice.client.invoke(new Api.messages.MarkDialogUnread({
      peer: new Api.InputDialogPeer({ peer: inputPeerUser(Api, bobUser) }),
    }));
    requireCondition(isBoolTrue(markedRead), 'messages.markDialogUnread did not clear the dialog mark');

    const dialogs = await alice.client.invoke(new Api.messages.GetDialogs({
      offsetDate: 0,
      offsetId: 0,
      offsetPeer: new Api.InputPeerEmpty(),
      limit: 100,
      hash: 0n,
    }));
    requireCondition(Array.isArray(dialogs?.dialogs), 'messages.getDialogs did not return dialogs');

    const discussionRead = await alice.client.invoke(new Api.messages.ReadDiscussion({
      peer: inputPeerUser(Api, bobUser),
      msgId: directMessage.id,
      readMaxId: directMessage.id,
    }));
    requireCondition(
      discussionRead === true || rpcName(discussionRead).toLowerCase().includes('true'),
      'messages.readDiscussion did not return true',
    );

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
    requireCondition(findMessage(globalSearch, directText), 'messages.searchGlobal did not return the direct message');

    const editedDirectText = `${tag} edited direct message`;
    const editUpdates = await alice.client.invoke(new Api.messages.EditMessage({
      peer: inputPeerUser(Api, bobUser),
      id: directMessage.id,
      message: editedDirectText,
    }));
    requireCondition(rpcName(editUpdates).toLowerCase().includes('updates'), 'messages.editMessage did not return updates');
    const editedHistory = await waitForMessage(() => bob.client.invoke(new Api.messages.GetHistory({
      peer: bobToAlice,
      offsetId: 0,
      offsetDate: 0,
      addOffset: 0,
      limit: 20,
      maxId: 0,
      minId: 0,
      hash: 0n,
    })), editedDirectText);
    requireCondition(findMessage(editedHistory, editedDirectText), 'messages.editMessage did not update the direct message');

    const pinUpdates = await alice.client.invoke(new Api.messages.UpdatePinnedMessage({
      peer: inputPeerUser(Api, bobUser),
      id: directMessage.id,
      pmOneside: true,
      silent: true,
    }));
    requireCondition(rpcName(pinUpdates).toLowerCase().includes('updates'), 'messages.updatePinnedMessage did not return updates');
    const pinnedHistory = await alice.client.invoke(new Api.messages.GetHistory({
      peer: inputPeerUser(Api, bobUser),
      offsetId: 0,
      offsetDate: 0,
      addOffset: 0,
      limit: 20,
      maxId: 0,
      minId: 0,
      hash: 0n,
    }));
    const pinnedMessage = findMessage(pinnedHistory, editedDirectText);
    requireCondition(pinnedMessage?.pinned === true, 'messages.updatePinnedMessage did not persist the pin in history');
    const unpinUpdates = await alice.client.invoke(new Api.messages.UpdatePinnedMessage({
      peer: inputPeerUser(Api, bobUser),
      id: directMessage.id,
      pmOneside: true,
      unpin: true,
      silent: true,
    }));
    requireCondition(rpcName(unpinUpdates).toLowerCase().includes('updates'), 'messages.updatePinnedMessage unpin did not return updates');
    const unpinnedHistory = await alice.client.invoke(new Api.messages.GetHistory({
      peer: inputPeerUser(Api, bobUser),
      offsetId: 0,
      offsetDate: 0,
      addOffset: 0,
      limit: 20,
      maxId: 0,
      minId: 0,
      hash: 0n,
    }));
    const unpinnedMessage = findMessage(unpinnedHistory, editedDirectText);
    requireCondition(unpinnedMessage?.pinned !== true, 'messages.updatePinnedMessage unpin did not clear the pin in history');

    const forwardUpdates = await alice.client.invoke(new Api.messages.ForwardMessages({
      fromPeer: inputPeerUser(Api, bobUser),
      id: [directMessage.id],
      randomId: [randomId()],
      toPeer: inputPeerUser(Api, carolUser),
    }));
    requireCondition(rpcName(forwardUpdates).toLowerCase().includes('updates'), 'messages.forwardMessages did not return updates');
    const carolToAlice = inputPeerUser(Api, await searchUser(Api, carol, alice.username));
    const forwardedHistory = await waitForMessage(() => carol.client.invoke(new Api.messages.GetHistory({
      peer: carolToAlice,
      offsetId: 0,
      offsetDate: 0,
      addOffset: 0,
      limit: 20,
      maxId: 0,
      minId: 0,
      hash: 0n,
    })), editedDirectText);
    const forwardedMessage = findMessage(forwardedHistory, editedDirectText);
    requireCondition(forwardedMessage, 'messages.forwardMessages did not deliver the forwarded direct message');
    forwardedMessageId = forwardedMessage.id;

    const draftText = `${tag} private draft`;
    const draftPeer = inputPeerUser(Api, carolUser);
    const savedDraft = await alice.client.invoke(new Api.messages.SaveDraft({
      peer: draftPeer,
      message: draftText,
    }));
    requireCondition(savedDraft === true || rpcName(savedDraft).toLowerCase().includes('true'), 'messages.saveDraft did not return true');
    const draftDialogs = await alice.client.invoke(new Api.messages.GetPeerDialogs({
      peers: [new Api.InputDialogPeer({ peer: draftPeer })],
    }));
    const draftDialog = (Array.isArray(draftDialogs?.dialogs) ? draftDialogs.dialogs : [])[0];
    requireCondition(draftDialog?.draft?.message === draftText, 'messages.getPeerDialogs did not return the saved private draft');
    const clearedDraft = await alice.client.invoke(new Api.messages.SaveDraft({
      peer: draftPeer,
      message: '',
    }));
    requireCondition(clearedDraft === true || rpcName(clearedDraft).toLowerCase().includes('true'), 'messages.saveDraft did not clear the private draft');

    const groupTitle = `${tag} group`;
    console.log('group flow');
    const groupCreated = await alice.client.invoke(new Api.messages.CreateChat({
      users: [inputUser(Api, bobUser)],
      title: groupTitle,
    }));
    const group = chatList(groupCreated).find((chat) => chat?.title === groupTitle);
    requireCondition(group?.id !== undefined, 'messages.createChat did not return the new group');
    createdGroupId = group.id;

    for (const user of [
      carolUser,
      daveUser,
      eveUser,
      frankUser,
      graceUser,
      heidiUser,
      ivanUser,
      juliaUser,
      karlUser,
      laraUser,
      mikeUser,
      ninaUser,
    ]) {
      const update = await alice.client.invoke(new Api.messages.AddChatUser({
        chatId: group.id,
        userId: inputUser(Api, user),
        fwdLimit: 0,
      }));
      requireCondition(update?.updates, 'messages.addChatUser did not return invited-user updates');
    }

    const groupText = `${tag} group message`;
    await alice.client.invoke(new Api.messages.SendMessage({
      peer: new Api.InputPeerChat({ chatId: group.id }),
      message: groupText,
      randomId: randomId(),
    }));
    const bobGroupHistory = await waitForMessage(() => bob.client.invoke(new Api.messages.GetHistory({
        peer: new Api.InputPeerChat({ chatId: group.id }),
        offsetId: 0,
        offsetDate: 0,
        addOffset: 0,
        limit: 20,
        maxId: 0,
        minId: 0,
        hash: 0n,
      })), groupText);
    const groupMessage = findMessage(bobGroupHistory, groupText);
    requireCondition(groupMessage, 'group history did not return the group message');

    console.log('group reaction flow');
    const groupReaction = await bob.client.invoke(new Api.messages.SendReaction({
      peer: new Api.InputPeerChat({ chatId: group.id }),
      msgId: groupMessage.id,
      reaction: [new Api.ReactionEmoji({ emoticon: '👍' })],
    }));
    requireCondition(rpcName(groupReaction).toLowerCase().includes('updates'), 'messages.sendReaction did not return updates');
    const reactionUpdate = (Array.isArray(groupReaction?.updates) ? groupReaction.updates : []).find(
      (update: any) => rpcName(update).toLowerCase().includes('updatemessagereactions') && update?.msgId === groupMessage.id,
    );
    requireCondition(
      Array.isArray(reactionUpdate?.reactions?.results)
        && reactionUpdate.reactions.results.some((result: any) => reactionEmoticon(result) === '👍' && result?.count === 1),
      'messages.sendReaction did not return the selected group reaction',
    );
    const groupReactionList = await bob.client.invoke(new Api.messages.GetMessageReactionsList({
      peer: new Api.InputPeerChat({ chatId: group.id }),
      id: groupMessage.id,
      limit: 20,
    }));
    requireCondition(
      rpcName(groupReactionList).toLowerCase().includes('messagereactionslist')
        && groupReactionList?.count === 1
        && Array.isArray(groupReactionList?.reactions)
        && groupReactionList.reactions.some((reaction: any) => peerId(reaction?.peerId) === String(bob.id)
          && reactionEmoticon(reaction) === '👍'
          && reaction?.my === true),
      'messages.getMessageReactionsList did not return Bob’s group reaction',
    );
    const ownerGroupReactionList = await alice.client.invoke(new Api.messages.GetMessageReactionsList({
      peer: new Api.InputPeerChat({ chatId: group.id }),
      id: groupMessage.id,
      limit: 20,
    }));
    requireCondition(
      ownerGroupReactionList?.count === 1
        && Array.isArray(ownerGroupReactionList?.reactions)
        && ownerGroupReactionList.reactions.some((reaction: any) => peerId(reaction?.peerId) === String(bob.id)
          && reactionEmoticon(reaction) === '👍'
          && reaction?.my !== true),
      'messages.getMessageReactionsList did not expose Bob’s group reaction to Alice',
    );
    const clearedGroupReaction = await bob.client.invoke(new Api.messages.SendReaction({
      peer: new Api.InputPeerChat({ chatId: group.id }),
      msgId: groupMessage.id,
      reaction: [],
    }));
    requireCondition(
      rpcName(clearedGroupReaction).toLowerCase().includes('updates'),
      'messages.sendReaction did not clear Bob’s group reaction',
    );
    const clearedGroupReactionList = await bob.client.invoke(new Api.messages.GetMessageReactionsList({
      peer: new Api.InputPeerChat({ chatId: group.id }),
      id: groupMessage.id,
      limit: 20,
    }));
    requireCondition(
      clearedGroupReactionList?.count === 0 && Array.isArray(clearedGroupReactionList?.reactions)
        && clearedGroupReactionList.reactions.length === 0,
      'messages.getMessageReactionsList retained Bob’s cleared group reaction',
    );
    const ownerClearedGroupReactionList = await alice.client.invoke(new Api.messages.GetMessageReactionsList({
      peer: new Api.InputPeerChat({ chatId: group.id }),
      id: groupMessage.id,
      limit: 20,
    }));
    requireCondition(
      ownerClearedGroupReactionList?.count === 0
        && Array.isArray(ownerClearedGroupReactionList?.reactions)
        && ownerClearedGroupReactionList.reactions.length === 0,
      'messages.getMessageReactionsList retained Bob’s cleared reaction for Alice',
    );

    const memberGroupText = `${tag} group member message`;
    await carol.client.invoke(new Api.messages.SendMessage({
      peer: new Api.InputPeerChat({ chatId: group.id }),
      message: memberGroupText,
      randomId: randomId(),
    }));
    const aliceGroupHistory = await waitForMessage(() => alice.client.invoke(new Api.messages.GetHistory({
      peer: new Api.InputPeerChat({ chatId: group.id }),
      offsetId: 0,
      offsetDate: 0,
      addOffset: 0,
      limit: 20,
      maxId: 0,
      minId: 0,
      hash: 0n,
    })), memberGroupText);
    const memberGroupMessage = findMessage(aliceGroupHistory, memberGroupText);
    requireCondition(memberGroupMessage, 'group history did not return the member message');
    requireCondition(
      peerId(memberGroupMessage?.fromId) === String(carol.id),
      'group history did not identify Carol as the member-message sender',
    );

    const groupSearch = await waitForMessage(() => carol.client.invoke(new Api.messages.Search({
        peer: new Api.InputPeerChat({ chatId: group.id }),
        q: tag,
        filter: new Api.InputMessagesFilterEmpty(),
        minDate: 0,
        maxDate: 0,
        offsetId: 0,
        addOffset: 0,
        limit: 20,
        maxId: 0,
        minId: 0,
        hash: 0n,
      })), groupText);
    requireCondition(findMessage(groupSearch, groupText), 'group search did not return the group message');

    const groupRead = await bob.client.invoke(new Api.messages.ReadHistory({
      peer: new Api.InputPeerChat({ chatId: group.id }),
      maxId: groupMessage.id,
    }));
    requireCondition(Number.isInteger(groupRead?.pts), 'group read receipt did not return pts');

    const groupFull = await alice.client.invoke(new Api.messages.GetFullChat({ chatId: group.id }));
    requireCondition(groupFull?.fullChat && Array.isArray(groupFull?.users), 'messages.getFullChat did not return group details');
    for (const invitedUser of [
      bobUser,
      carolUser,
      daveUser,
      eveUser,
      frankUser,
      graceUser,
      heidiUser,
      ivanUser,
      juliaUser,
      karlUser,
      laraUser,
      mikeUser,
      ninaUser,
    ]) {
      requireCondition(
        groupFull.users.some((user: any) => String(user?.id) === String(invitedUser.id)),
        `messages.getFullChat did not include invited user ${String(invitedUser.id)}`,
      );
    }
    const commonGroupChats = await alice.client.invoke(new Api.messages.GetCommonChats({
      userId: inputUser(Api, bobUser),
      maxId: 0n,
      limit: 100,
    }));
    requireCondition(
      chatList(commonGroupChats).some((chat) => String(chat?.id) === String(group.id)),
      'messages.getCommonChats did not return the common basic group',
    );

    const channelTitle = `${tag} channel`;
    console.log('channel flow');
    const channelCreated = await alice.client.invoke(new Api.channels.CreateChannel({
      broadcast: true,
      title: channelTitle,
      about: `${tag} acceptance channel`,
    }));
    createdChannel = chatList(channelCreated).find((chat) => chat?.title === channelTitle);
    requireCondition(createdChannel, 'channels.createChannel did not return the new channel');

    const channelInput = inputChannel(Api, createdChannel);
    const channelPeer = inputPeerChannel(Api, createdChannel);
    const channelUsername = `isolated_${Date.now()}`;
    const usernameUpdated = await alice.client.invoke(new Api.channels.UpdateUsername({
      channel: channelInput,
      username: channelUsername,
    }));
    requireCondition(isBoolTrue(usernameUpdated), 'channels.updateUsername did not return true');
    const resolvedChannel = await bob.client.invoke(new Api.contacts.ResolveUsername({ username: channelUsername }));
    requireCondition(
      rpcName(resolvedChannel).toLowerCase().includes('resolvedpeer')
        && String(resolvedChannel?.peer?.channelId) === String(createdChannel.id)
        && Array.isArray(resolvedChannel?.chats)
        && resolvedChannel.chats.some((chat: any) => String(chat?.id) === String(createdChannel.id)
          && rpcName(chat).toLowerCase().includes('channel')),
      'contacts.resolveUsername did not hydrate the public channel for a non-member',
    );
    const invited = await alice.client.invoke(new Api.channels.InviteToChannel({
      channel: channelInput,
      users: [inputUser(Api, bobUser)],
    }));
    requireCondition(invited?.updates, 'channels.inviteToChannel did not return invited-user updates');
    const commonChannelChats = await alice.client.invoke(new Api.messages.GetCommonChats({
      userId: inputUser(Api, bobUser),
      maxId: 0n,
      limit: 100,
    }));
    requireCondition(
      chatList(commonChannelChats).some(
        (chat) => String(chat?.id) === String(createdChannel.id)
          && rpcName(chat).toLowerCase().includes('channel'),
      ),
      'messages.getCommonChats did not return the common channel',
    );

    const channelText = `${tag} channel message`;
    console.log('channel send');
    const channelUpdates = await alice.client.invoke(new Api.messages.SendMessage({
      peer: channelPeer,
      message: channelText,
      randomId: randomId(),
    }));
    requireCondition(rpcName(channelUpdates).toLowerCase().includes('updates'), 'channel send did not return updates');

    const bobChannelHistory = await bob.client.invoke(new Api.messages.GetHistory({
      peer: channelPeer,
      offsetId: 0,
      offsetDate: 0,
      addOffset: 0,
      limit: 20,
      maxId: 0,
      minId: 0,
      hash: 0n,
    }));
    const channelMessage = findMessage(bobChannelHistory, channelText);
    requireCondition(channelMessage, 'channel history did not return the channel message');

    console.log('channel getMessages');
    const channelMessages = await bob.client.invoke(new Api.channels.GetMessages({
      channel: channelInput,
      id: [new Api.InputMessageID({ id: channelMessage.id })],
    }));
    requireCondition(findMessage(channelMessages, channelText), 'channels.getMessages did not return the channel message');

    console.log('channel read');
    const channelRead = await bob.client.invoke(new Api.channels.ReadHistory({
      channel: channelInput,
      maxId: channelMessage.id,
    }));
    requireCondition(channelRead === true, 'channels.readHistory did not return true');

    console.log('channel views');
    const channelViews = await bob.client.invoke(new Api.messages.GetMessagesViews({
      peer: channelPeer,
      id: [channelMessage.id],
      increment: true,
    }));
    requireCondition(
      rpcName(channelViews).toLowerCase().includes('messageviews')
        && Array.isArray(channelViews?.views)
        && channelViews.views.length > 0
        && typeof channelViews.views[0]?.views === 'number',
      'messages.getMessagesViews did not return a numeric channel view count',
    );

    console.log('channel search');
    const channelSearch = await bob.client.invoke(new Api.messages.Search({
      peer: channelPeer,
      q: tag,
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
    requireCondition(findMessage(channelSearch, channelText), 'channel search did not return the channel message');

    console.log('channel difference');
    const channelDifference = await bob.client.invoke(new Api.updates.GetChannelDifference({
      force: true,
      channel: channelInput,
      filter: new Api.ChannelMessagesFilterEmpty(),
      pts: 0,
      limit: 100,
    }));
    requireCondition(
      rpcName(channelDifference).toLowerCase().includes('channeldifference')
        && typeof channelDifference?.final === 'boolean'
        && Number.isInteger(channelDifference?.pts),
      'updates.getChannelDifference did not return a typed channel difference',
    );
    requireCondition(
      findMessage(channelDifference, channelText),
      'updates.getChannelDifference did not include the posted channel message',
    );

    console.log('channel details');
    const fullChannel = await bob.client.invoke(new Api.channels.GetFullChannel({ channel: channelInput }));
    requireCondition(fullChannel?.fullChat && Array.isArray(fullChannel?.chats), 'channels.getFullChannel did not return channel details');

    const participants = await bob.client.invoke(new Api.channels.GetParticipants({
      channel: channelInput,
      filter: new Api.ChannelParticipantsRecent(),
      offset: 0,
      limit: 20,
      hash: 0n,
    }));
    requireCondition(Array.isArray(participants?.participants), 'channels.getParticipants did not return channel members');

    console.log('channel group-call control flow');
    const groupCallCreated = await alice.client.invoke(new Api.phone.CreateGroupCall({
      peer: channelPeer,
      randomId: Number(BigInt(Date.now()) % 2_147_483_647n),
      title: `${tag} group call`,
    }));
    const groupCall = Array.isArray(groupCallCreated?.updates)
      ? groupCallCreated.updates.map((update: any) => update?.call).find((call: any) => call?.id !== undefined && call?.accessHash !== undefined)
      : undefined;
    requireCondition(groupCall, 'phone.createGroupCall did not return a usable group call');
    const groupCallInput = new Api.InputGroupCall({ id: groupCall.id, accessHash: groupCall.accessHash });
    const bobGroupCall = await bob.client.invoke(new Api.phone.GetGroupCall({ call: groupCallInput, limit: 20 }));
    requireCondition(
      String(bobGroupCall?.call?.id) === String(groupCall.id),
      'phone.getGroupCall did not return the channel group call to a member',
    );
    const groupCallJoin = await bob.client.invoke(new Api.phone.JoinGroupCall({
      call: groupCallInput,
      joinAs: new Api.InputPeerSelf(),
      muted: true,
      videoStopped: true,
      params: new Api.DataJSON({ data: '{}' }),
    }));
    requireCondition(rpcName(groupCallJoin).toLowerCase().includes('updates'), 'phone.joinGroupCall did not return updates');
    const groupCallParticipants = await alice.client.invoke(new Api.phone.GetGroupParticipants({
      call: groupCallInput,
      ids: [],
      sources: [],
      offset: '',
      limit: 20,
    }));
    requireCondition(
      Array.isArray(groupCallParticipants?.participants)
        && groupCallParticipants.participants.some((participant: any) => String(participant?.peer?.userId) === String(bob.id)),
      'phone.getGroupParticipants did not contain the joining channel member',
    );
    const activeSources = await bob.client.invoke(new Api.phone.CheckGroupCall({
      call: groupCallInput,
      sources: [Number(alice.id), Number(bob.id)],
    }));
    requireCondition(
      Array.isArray(activeSources) && activeSources.length === 2,
      'phone.checkGroupCall did not return the active participant sources',
    );
    try {
      await carol.client.invoke(new Api.phone.GetGroupCall({ call: groupCallInput, limit: 20 }));
      throw new Error('non-member read of a channel group call unexpectedly succeeded');
    } catch (error) {
      requireCondition(String(error).includes('USER_NOT_PARTICIPANT'), `channel group-call outsider error: ${String(error)}`);
    }
    const groupCallLeave = await bob.client.invoke(new Api.phone.LeaveGroupCall({
      call: groupCallInput,
      source: Number(bob.id),
    }));
    requireCondition(rpcName(groupCallLeave).toLowerCase().includes('updates'), 'phone.leaveGroupCall did not return updates');
    const groupCallAfterLeave = await alice.client.invoke(new Api.phone.GetGroupParticipants({
      call: groupCallInput,
      ids: [],
      sources: [],
      offset: '',
      limit: 20,
    }));
    requireCondition(
      Array.isArray(groupCallAfterLeave?.participants) && groupCallAfterLeave.participants.length === 1,
      'phone.leaveGroupCall did not remove the member from participants',
    );

    console.log('channel statistics');
    const broadcastStats = await alice.client.invoke(new Api.stats.GetBroadcastStats({
      channel: channelInput,
    }));
    requireCondition(
      rpcName(broadcastStats).toLowerCase().includes('broadcaststats'),
      'stats.getBroadcastStats did not return typed statistics',
    );
    const megagroupStats = await alice.client.invoke(new Api.stats.GetMegagroupStats({
      channel: channelInput,
    }));
    requireCondition(
      rpcName(megagroupStats).toLowerCase().includes('megagroupstats'),
      'stats.getMegagroupStats did not return typed statistics',
    );
    await expectChatAdminRequired(() => bob.client.invoke(new Api.stats.GetBroadcastStats({
      channel: channelInput,
    })));
    await expectChatAdminRequired(() => bob.client.invoke(new Api.stats.GetMegagroupStats({
      channel: channelInput,
    })));

    await expectChatAdminRequired(() => bob.client.invoke(new Api.channels.EditTitle({
      channel: channelInput,
      title: `${channelTitle} forbidden`,
    })));

    await alice.client.invoke(new Api.channels.DeleteChannel({ channel: channelInput }));
    createdChannel = undefined;
    completed = true;
  } catch (error) {
    runError = error;
    throw error;
  } finally {
    const cleanupErrors: string[] = [];
    if (createdChannel && owner) {
      try {
        await owner.client.invoke(new Api.channels.DeleteChannel({ channel: inputChannel(Api, createdChannel) }));
      } catch (error) {
        cleanupErrors.push(`channel cleanup: ${String(error)}`);
      }
    }
    if (createdGroupId !== undefined && owner) {
      try {
        await owner.client.invoke(new Api.messages.DeleteChat({ chatId: createdGroupId }));
      } catch (error) {
        cleanupErrors.push(`group cleanup: ${String(error)}`);
      }
    }
    if (directMessageId !== undefined && owner) {
      try {
        await owner.client.invoke(new Api.messages.DeleteMessages({ revoke: true, id: [directMessageId] }));
      } catch (error) {
        cleanupErrors.push(`direct-message cleanup: ${String(error)}`);
      }
    }
    if (forwardedMessageId !== undefined && owner) {
      try {
        await owner.client.invoke(new Api.messages.DeleteMessages({ revoke: true, id: [forwardedMessageId] }));
      } catch (error) {
        cleanupErrors.push(`forwarded-message cleanup: ${String(error)}`);
      }
    }
    if (contactInput && owner) {
      try {
        await owner.client.invoke(new Api.contacts.DeleteContacts({ id: [contactInput] }));
      } catch (error) {
        cleanupErrors.push(`contact cleanup: ${String(error)}`);
      }
    }
    if (importedPhone && owner) {
      try {
        const deletedByPhone = await owner.client.invoke(new Api.contacts.DeleteByPhones({
          phones: [importedPhone],
        }));
        requireCondition(isBoolTrue(deletedByPhone), 'contacts.deleteByPhones did not return true');
        const savedAfterDelete = await owner.client.invoke(new Api.contacts.GetSaved());
        requireCondition(
          Array.isArray(savedAfterDelete)
            && !savedAfterDelete.some((saved: any) => saved?.phone === importedSavedPhone),
          'contacts.deleteByPhones left the imported phone in contacts.getSaved',
        );
      } catch (error) {
        cleanupErrors.push(`phone-contact cleanup: ${String(error)}`);
      }
    }
    if (unregisteredPhone) {
      for (const actor of actors.slice(0, 2)) {
        try {
          const deletedByPhone = await actor.client.invoke(new Api.contacts.DeleteByPhones({ phones: [unregisteredPhone] }));
          requireCondition(isBoolTrue(deletedByPhone), 'contacts.deleteByPhones did not return true during unregistered-phone cleanup');
          const savedAfterDelete = await actor.client.invoke(new Api.contacts.GetSaved());
          requireCondition(
            Array.isArray(savedAfterDelete)
              && !savedAfterDelete.some((saved: any) => saved?.phone === unregisteredSavedPhone),
            'contacts.deleteByPhones left an unregistered import during cleanup',
          );
        } catch (error) {
          cleanupErrors.push(`unregistered-phone cleanup for ${actor.username}: ${String(error)}`);
        }
      }
    }
    if (blockedPeer && owner) {
      try {
        await owner.client.invoke(new Api.contacts.Unblock({ id: blockedPeer }));
      } catch (error) {
        cleanupErrors.push(`blocked-user cleanup: ${String(error)}`);
      }
    }
    for (const actor of actors) actor.client.destroy();
    if (cleanupErrors.length > 0) {
      const message = `business-flow cleanup failed: ${cleanupErrors.join('; ')}`;
      if (runError) {
        console.error(message);
      } else {
        throw new Error(message);
      }
    }
    if (completed) console.log(`business-flow acceptance passed: ${tag}`);
  }
}

void run()
  .then(() => process.exit(0))
  .catch((error) => {
    const detail = error instanceof Error && error.stack ? error.stack : String(error);
    console.error(`business-flow acceptance failed: ${detail}`);
    process.exit(1);
  });
