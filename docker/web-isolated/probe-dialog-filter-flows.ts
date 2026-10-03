import path from 'node:path';
import { createRequire } from 'node:module';
import process from 'node:process';

const API_ID = 1;
const API_HASH = '0123456789abcdef0123456789abcdef';
const FIXTURES = [
  { phone: '12025550101', username: 'isolated_test_01' },
  { phone: '12025550102', username: 'isolated_test_02' },
];

const quietLogger = { debug() {}, info() {}, warn() {}, error() {} };
let randomIdCounter = 0n;

function requireCondition(condition: unknown, message: string): asserts condition {
  if (!condition) throw new Error(message);
}

function isTrue(value: unknown) {
  const name = (value as { className?: string; constructor?: { name?: string } })?.className
    || (value as { constructor?: { name?: string } })?.constructor?.name
    || '';
  return value === true || name.toLowerCase().includes('true');
}

function randomId() {
  randomIdCounter += 1n;
  return BigInt(Date.now()) * 1_000_000n + randomIdCounter;
}

function peerId(peer: any): string | undefined {
  const id = peer?.userId ?? peer?.chatId ?? peer?.channelId;
  return id === undefined ? undefined : String(id);
}

function inputPeerUser(Api: any, user: any) {
  requireCondition(user?.id !== undefined && user?.accessHash !== undefined, 'peer lacks an access hash');
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

async function login(gram: any, fixture: typeof FIXTURES[number]) {
  const client = new gram.TelegramClient(new gram.MemorySession(), API_ID, API_HASH, {
    connection: gram.Connection,
    fallbackConnection: gram.Connection,
    shouldAllowHttpTransport: false,
    shouldForceHttpTransport: false,
    timeout: 15,
    requestRetries: 2,
    connectionRetries: 1,
    connectionRetriesToFallback: 0,
    retryDelay: 250,
    autoReconnect: false,
    baseLogger: quietLogger,
  });
  await client.start({
    phoneNumber: fixture.phone,
    phoneCode: async () => '12345',
    password: async () => '',
    firstAndLastNames: async () => ['Isolated', 'DialogFilter'],
    qrCode: async () => {},
    onError: (error: Error) => { throw error; },
    webAuthTokenFailed: () => {},
    onPasskeyOption: () => {},
    initialMethod: 'phoneNumber',
  });
  const me = await client.getMe();
  requireCondition(me?.id !== undefined, `login failed for ${fixture.username}`);
  return { client, me, ...fixture };
}

async function searchUser(Api: any, actor: any, username: string) {
  const result = await actor.client.invoke(new Api.contacts.Search({ q: username, limit: 10 }));
  const user = result?.users?.find((candidate: any) => candidate?.username === username);
  requireCondition(user, `contacts.search did not find ${username}`);
  return user;
}

function filterIDs(filters: any): number[] {
  return (filters?.filters || [])
    .map((filter: any) => Number(filter?.id))
    .filter((id: number) => Number.isInteger(id));
}

function filterByID(filters: any, id: number) {
  return (filters?.filters || []).find((filter: any) => Number(filter?.id) === id);
}

function allocateFilterIDs(aliceFilters: any, bobFilters: any): [number, number] {
  const used = new Set([...filterIDs(aliceFilters), ...filterIDs(bobFilters)]);
  const ids: number[] = [];
  for (let id = 2; id <= 255 && ids.length < 2; id += 1) {
    if (!used.has(id)) ids.push(id);
  }
  requireCondition(ids.length === 2, 'no two unused dialog-filter IDs available');
  return [ids[0], ids[1]];
}

function makeFilter(Api: any, id: number, title: string, includePeer: any) {
  return new Api.DialogFilter({
    id,
    title: new Api.TextWithEntities({ text: title, entities: [] }),
    pinnedPeers: [],
    includePeers: [includePeer],
    excludePeers: [],
  });
}

function errorText(error: unknown) {
  const value = error as { errorMessage?: unknown; message?: unknown };
  return String(value?.errorMessage || value?.message || error);
}

async function expectError(action: () => Promise<unknown>, text: string) {
  try {
    await action();
  } catch (error) {
    requireCondition(errorText(error).includes(text), `expected ${text}, got ${errorText(error)}`);
    return;
  }
  throw new Error(`expected ${text}, request succeeded`);
}

async function run() {
  requireCondition(process.env.TG_DC_HOST === '127.0.0.1', 'probe must target 127.0.0.1');
  requireCondition(process.env.TG_DC_PORT === '31443', 'probe must target the isolated gateway');
  requireCondition(process.env.TEAMGRAM_GRAMJS_DIR, 'TEAMGRAM_GRAMJS_DIR is required');

  const globalAny = globalThis as typeof globalThis & { self?: any; addEventListener?: () => void };
  globalAny.self ??= globalThis;
  globalAny.addEventListener ??= () => {};
  globalAny.self.addEventListener ??= globalAny.addEventListener;

  const gram = await loadGramJs(process.env.TEAMGRAM_GRAMJS_DIR);
  const { Api } = gram;
  requireCondition(Api.messages.GetDialogUnreadMarks, 'GramJS lacks messages.GetDialogUnreadMarks');

  const alice = await login(gram, FIXTURES[0]);
  const bob = await login(gram, FIXTURES[1]);
  const tag = `dialog-filter-${Date.now()}-${process.pid}`;
  const failures: string[] = [];
  let aliceFilterIDs: [number, number] | undefined;
  let bobPeer: any;
  let alicePeer: any;
  let reservedFilterCreated = false;

  async function check(name: string, action: () => Promise<void>) {
    try {
      await action();
      console.log(`PASS ${name}`);
    } catch (error) {
      const detail = errorText(error);
      failures.push(`${name}: ${detail}`);
      console.log(`FAIL ${name}: ${detail}`);
    }
  }

  try {
    const bobUser = await searchUser(Api, alice, bob.username);
    const aliceUser = await searchUser(Api, bob, alice.username);
    bobPeer = inputPeerUser(Api, bobUser);
    alicePeer = inputPeerUser(Api, aliceUser);

    await check('create a real direct dialog', async () => {
      const sent = await alice.client.invoke(new Api.messages.SendMessage({
        peer: bobPeer,
        message: `${tag} setup`,
        randomId: randomId(),
      }));
      requireCondition(sent, 'messages.sendMessage returned no updates');
    });

    await check('save, read, and isolate dialog filters', async () => {
      const aliceBefore = await alice.client.invoke(new Api.messages.GetDialogFilters());
      const bobBefore = await bob.client.invoke(new Api.messages.GetDialogFilters());
      aliceFilterIDs = allocateFilterIDs(aliceBefore, bobBefore);
      const [firstID, secondID] = aliceFilterIDs;
      const firstTitle = `${tag} one`;
      const secondTitle = `${tag} two`;
      requireCondition(isTrue(await alice.client.invoke(new Api.messages.UpdateDialogFilter({
        id: firstID,
        filter: makeFilter(Api, firstID, firstTitle, bobPeer),
      }))), 'first updateDialogFilter did not return true');
      requireCondition(isTrue(await alice.client.invoke(new Api.messages.UpdateDialogFilter({
        id: secondID,
        filter: makeFilter(Api, secondID, secondTitle, bobPeer),
      }))), 'second updateDialogFilter did not return true');

      const aliceAfter = await alice.client.invoke(new Api.messages.GetDialogFilters());
      requireCondition(filterByID(aliceAfter, firstID)?.title?.text === firstTitle, 'first filter was not read back');
      requireCondition(filterByID(aliceAfter, secondID)?.title?.text === secondTitle, 'second filter was not read back');
      const bobAfter = await bob.client.invoke(new Api.messages.GetDialogFilters());
      requireCondition(!filterByID(bobAfter, firstID) && !filterByID(bobAfter, secondID), 'another user can read Alice filters');
    });

    await check('reorder saved dialog filters', async () => {
      requireCondition(aliceFilterIDs, 'filter setup did not complete');
      const [firstID, secondID] = aliceFilterIDs;
      requireCondition(isTrue(await alice.client.invoke(new Api.messages.UpdateDialogFiltersOrder({
        order: [secondID, firstID],
      }))), 'updateDialogFiltersOrder did not return true');
      const filters = await alice.client.invoke(new Api.messages.GetDialogFilters());
      const positions = filterIDs(filters);
      requireCondition(positions.indexOf(secondID) < positions.indexOf(firstID), 'filter order was not persisted');
    });

    await check('toggle dialog-filter tags and read the flag', async () => {
      requireCondition(isTrue(await alice.client.invoke(new Api.messages.ToggleDialogFilterTags({ enabled: true }))), 'enable tags did not return true');
      requireCondition((await alice.client.invoke(new Api.messages.GetDialogFilters())).tagsEnabled === true, 'tagsEnabled was not persisted');
      requireCondition((await bob.client.invoke(new Api.messages.GetDialogFilters())).tagsEnabled !== true, 'tag setting leaked to another user');
      requireCondition(isTrue(await alice.client.invoke(new Api.messages.ToggleDialogFilterTags({ enabled: false }))), 'disable tags did not return true');
    });

    await check('move a real dialog into a folder and reject cross-user folder access', async () => {
      requireCondition(aliceFilterIDs, 'filter setup did not complete');
      const [firstID] = aliceFilterIDs;
      const moved = await alice.client.invoke(new Api.folders.EditPeerFolders({
        folderPeers: [new Api.InputFolderPeer({ peer: bobPeer, folderId: firstID })],
      }));
      requireCondition(moved?.updates?.some((update: any) => String(update?.className).includes('UpdateFolderPeers')), 'folders.editPeerFolders omitted updateFolderPeers');

      const folderDialogs = await alice.client.invoke(new Api.messages.GetDialogs({
        folderId: firstID,
        offsetDate: 0,
        offsetId: 0,
        offsetPeer: new Api.InputPeerEmpty(),
        limit: 100,
        hash: 0n,
      }));
      requireCondition((folderDialogs?.dialogs || []).some((dialog: any) => peerId(dialog?.peer) === String(bobUser.id)), 'folder does not contain the moved dialog');

      await expectError(() => bob.client.invoke(new Api.folders.EditPeerFolders({
        folderPeers: [new Api.InputFolderPeer({ peer: alicePeer, folderId: firstID })],
      })), 'FOLDER_ID_INVALID');
    });

    await check('move a real dialog into the archive', async () => {
      const archived = await alice.client.invoke(new Api.folders.EditPeerFolders({
        folderPeers: [new Api.InputFolderPeer({ peer: bobPeer, folderId: 1 })],
      }));
      requireCondition(archived?.updates?.some((update: any) => String(update?.className).includes('UpdateFolderPeers')), 'archiving omitted updateFolderPeers');
      const dialogs = await alice.client.invoke(new Api.messages.GetDialogs({
        folderId: 1,
        offsetDate: 0,
        offsetId: 0,
        offsetPeer: new Api.InputPeerEmpty(),
        limit: 100,
        hash: 0n,
      }));
      requireCondition((dialogs?.dialogs || []).some((dialog: any) => peerId(dialog?.peer) === String(bobUser.id)), 'archive does not contain the moved dialog');
    });

    await check('reject reserved dialog-filter IDs', async () => {
      await expectError(async () => {
        const result = await alice.client.invoke(new Api.messages.UpdateDialogFilter({
          id: 1,
          filter: makeFilter(Api, 1, `${tag} reserved`, bobPeer),
        }));
        reservedFilterCreated = true;
        return result;
      }, 'FILTER_ID_INVALID');
    });

    await check('reject an invalid folder ID', async () => {
      await expectError(() => alice.client.invoke(new Api.folders.EditPeerFolders({
        folderPeers: [new Api.InputFolderPeer({ peer: bobPeer, folderId: -1 })],
      })), 'FOLDER_ID_INVALID');
    });

    await check('mark and list unread dialogs', async () => {
      requireCondition(isTrue(await alice.client.invoke(new Api.messages.MarkDialogUnread({
        peer: new Api.InputDialogPeer({ peer: bobPeer }), unread: true,
      }))), 'messages.markDialogUnread did not return true');
      const marks = await alice.client.invoke(new Api.messages.GetDialogUnreadMarks({}));
      requireCondition((marks || []).some((mark: any) => peerId(mark?.peer) === String(bobUser.id)), 'messages.getDialogUnreadMarks omitted marked dialog');
      requireCondition(isTrue(await alice.client.invoke(new Api.messages.MarkDialogUnread({
        peer: new Api.InputDialogPeer({ peer: bobPeer }), unread: false,
      }))), 'clearing unread mark did not return true');
    });

    await check('delete saved dialog filters', async () => {
      requireCondition(aliceFilterIDs, 'filter setup did not complete');
      const [firstID, secondID] = aliceFilterIDs;
      const moved = await alice.client.invoke(new Api.folders.EditPeerFolders({
        folderPeers: [new Api.InputFolderPeer({ peer: bobPeer, folderId: 0 })],
      }));
      requireCondition(moved?.updates?.some((update: any) => String(update?.className).includes('UpdateFolderPeers')), 'moving dialog out of folder omitted updateFolderPeers');
      for (const id of [firstID, secondID]) {
        requireCondition(isTrue(await alice.client.invoke(new Api.messages.UpdateDialogFilter({ id }))), `deleteDialogFilter(${id}) did not return true`);
      }
      const filters = await alice.client.invoke(new Api.messages.GetDialogFilters());
      requireCondition(!filterByID(filters, firstID) && !filterByID(filters, secondID), 'deleted filters were still returned');
    });
  } finally {
    try {
      if (bobPeer) {
        await alice.client.invoke(new Api.messages.MarkDialogUnread({
          peer: new Api.InputDialogPeer({ peer: bobPeer }), unread: false,
        }));
        await alice.client.invoke(new Api.messages.ToggleDialogFilterTags({ enabled: false }));
        if (aliceFilterIDs) {
          const [firstID, secondID] = aliceFilterIDs;
          await alice.client.invoke(new Api.folders.EditPeerFolders({
            folderPeers: [new Api.InputFolderPeer({ peer: bobPeer, folderId: 0 })],
          }));
          await alice.client.invoke(new Api.messages.UpdateDialogFilter({ id: firstID }));
          await alice.client.invoke(new Api.messages.UpdateDialogFilter({ id: secondID }));
        }
        if (reservedFilterCreated) await alice.client.invoke(new Api.messages.UpdateDialogFilter({ id: 1 }));
      }
    } catch (error) {
      failures.push(`cleanup: ${errorText(error)}`);
    } finally {
      alice.client.destroy();
      bob.client.destroy();
    }
  }

  if (failures.length > 0) throw new Error(failures.join('\n'));
}

void run()
  .then(() => process.exit(0))
  .catch((error) => {
    console.error(error instanceof Error && error.stack ? error.stack : String(error));
    process.exit(1);
  });
