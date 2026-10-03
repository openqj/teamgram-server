import path from 'node:path';
import { createRequire } from 'node:module';
import process from 'node:process';

const API_ID = 1;
const API_HASH = '0123456789abcdef0123456789abcdef';
const FIXTURES = [
  { phone: '12025550101', username: 'isolated_test_01' },
  { phone: '12025550102', username: 'isolated_test_02' },
];
const DRAFT_ONLY_FIXTURES = [
  { username: 'isolated_test_14' },
  { username: 'isolated_test_13' },
  { username: 'isolated_test_12' },
  { username: 'isolated_test_11' },
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

class ZeroArgumentRequest {
  CONSTRUCTOR_ID: number;
  SUBCLASS_OF_ID = 0;
  className: string;
  classType = 'request';

  constructor(constructorId: number, className: string) {
    this.CONSTRUCTOR_ID = constructorId;
    this.className = className;
  }

  getBytes() {
    const bytes = new Uint8Array(4);
    new DataView(bytes.buffer).setUint32(0, this.CONSTRUCTOR_ID, true);
    return bytes;
  }

  readResult(reader: any) {
    return reader.tgReadObject();
  }
}

function getAllDraftsRequest() {
  return new ZeroArgumentRequest(0x6a3f8d65, 'messages.GetAllDrafts');
}

function clearAllDraftsRequest() {
  return new ZeroArgumentRequest(0x7e58ee9c, 'messages.ClearAllDrafts');
}

function inputUser(Api: any, user: any) {
  requireCondition(user?.id !== undefined && user?.accessHash !== undefined, 'user lacks an access hash');
  return new Api.InputUser({ userId: user.id, accessHash: user.accessHash });
}

function inputPeerUser(Api: any, user: any) {
  requireCondition(user?.id !== undefined && user?.accessHash !== undefined, 'peer lacks an access hash');
  return new Api.InputPeerUser({ userId: user.id, accessHash: user.accessHash });
}

function peerId(peer: any): string | undefined {
  const id = peer?.userId ?? peer?.chatId ?? peer?.channelId;
  return id === undefined ? undefined : String(id);
}

function draftUpdateForPeer(updates: any, userId: any) {
  return (updates?.updates || []).find((candidate: any) => (
    String(candidate?.className || candidate?.constructor?.name).includes('UpdateDraftMessage')
    && peerId(candidate?.peer) === String(userId)
  ));
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
    firstAndLastNames: async () => ['Isolated', 'Profile'],
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

function hasPrivacyRule(rules: any[], name: string) {
  return rules.some((rule) => String(rule?.className || rule?.constructor?.name).includes(name));
}

async function run() {
  requireCondition(process.env.TG_DC_HOST === '127.0.0.1', 'probe must target the isolated gateway');
  requireCondition(process.env.TG_DC_PORT === '31443', 'probe must target the isolated gateway');
  requireCondition(process.env.TEAMGRAM_GRAMJS_DIR, 'TEAMGRAM_GRAMJS_DIR is required');

  const globalAny = globalThis as typeof globalThis & { self?: any; addEventListener?: () => void };
  globalAny.self ??= globalThis;
  globalAny.addEventListener ??= () => {};
  globalAny.self.addEventListener ??= globalAny.addEventListener;

  const gram = await loadGramJs(process.env.TEAMGRAM_GRAMJS_DIR);
  const { Api } = gram;
  const alice = await login(gram, FIXTURES[0]);
  const bob = await login(gram, FIXTURES[1]);
  const bobUser = await searchUser(Api, alice, bob.username);
  const bobPeer = inputPeerUser(Api, bobUser);
  const tag = `profile-dialog-${Date.now()}-${process.pid}`;
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
    await check('profile update publishes a changed name', async () => {
      const firstName = `Profile${Date.now()}`;
      const updated = await alice.client.invoke(new Api.account.UpdateProfile({ firstName }));
      requireCondition(updated?.firstName === firstName, 'account.updateProfile did not return the changed name');
      const seenByBob = await searchUser(Api, bob, alice.username);
      requireCondition(seenByBob?.firstName === firstName, 'account.updateProfile did not persist the changed name');
    });

    await check('privacy allow-list round trip', async () => {
      const key = new Api.InputPrivacyKeyStatusTimestamp();
      const rules = [
        new Api.InputPrivacyValueAllowUsers({ users: [inputUser(Api, bobUser)] }),
        new Api.InputPrivacyValueDisallowAll(),
      ];
      const saved = await alice.client.invoke(new Api.account.SetPrivacy({ key, rules }));
      requireCondition(hasPrivacyRule(saved?.rules || [], 'PrivacyValueAllowUsers'), 'account.setPrivacy omitted the allow-list');
      const loaded = await alice.client.invoke(new Api.account.GetPrivacy({ key }));
      requireCondition(hasPrivacyRule(loaded?.rules || [], 'PrivacyValueAllowUsers'), 'account.getPrivacy did not return the allow-list');
      const allowRule = loaded.rules.find((rule: any) => String(rule?.className).includes('PrivacyValueAllowUsers'));
      requireCondition(Array.isArray(allowRule?.users) && allowRule.users.some((id: any) => String(id) === String(bobUser.id)), 'account.getPrivacy lost the allowed user');
    });

    await check('direct notification exception round trip', async () => {
      const settings = new Api.InputPeerNotifySettings({
        muteUntil: 2_147_483_647,
        showPreviews: false,
        silent: true,
        sound: new Api.NotificationSoundDefault(),
      });
      const updated = await alice.client.invoke(new Api.account.UpdateNotifySettings({
        peer: new Api.InputNotifyPeer({ peer: bobPeer }),
        settings,
      }));
      requireCondition(isTrue(updated), 'account.updateNotifySettings did not return true');
      const loaded = await alice.client.invoke(new Api.account.GetNotifySettings({
        peer: new Api.InputNotifyPeer({ peer: bobPeer }),
      }));
      requireCondition(Number(loaded?.muteUntil) === 2_147_483_647, 'account.getNotifySettings did not return the saved mute date');
      const exceptions = await alice.client.invoke(new Api.account.GetNotifyExceptions({}));
      const update = (exceptions?.updates || []).find((candidate: any) => (
        String(candidate?.className).includes('UpdateNotifySettings')
        && peerId(candidate?.peer?.peer) === String(bobUser.id)
      ));
      requireCondition(update, 'account.getNotifyExceptions did not return the direct-peer exception');
    });

    await check('getAllDrafts returns and clears a draft-only peer', async () => {
      const initialDialogs = await alice.client.invoke(new Api.messages.GetDialogs({
        offsetDate: 0,
        offsetId: 0,
        offsetPeer: new Api.InputPeerEmpty(),
        limit: 100,
        hash: 0n,
      }));
      const existingPeerIds = new Set((initialDialogs?.dialogs || []).map((dialog: any) => peerId(dialog?.peer)));
      let draftOnlyUser: any;
      for (const fixture of DRAFT_ONLY_FIXTURES) {
        const candidate = await searchUser(Api, alice, fixture.username);
        if (!existingPeerIds.has(String(candidate.id))) {
          draftOnlyUser = candidate;
          break;
        }
      }
      requireCondition(draftOnlyUser, 'no fixture without a direct dialog is available');
      const draftOnlyPeer = inputPeerUser(Api, draftOnlyUser);

      const beforeClear = await alice.client.invoke(clearAllDraftsRequest() as any);
      requireCondition(isTrue(beforeClear), 'messages.clearAllDrafts did not return true before saving the draft');

      const draftText = `${tag} draft-only`;
      const saved = await alice.client.invoke(new Api.messages.SaveDraft({
        peer: draftOnlyPeer,
        message: draftText,
      }));
      requireCondition(isTrue(saved), 'messages.saveDraft did not return true for the draft-only peer');

      const drafts = await alice.client.invoke(getAllDraftsRequest() as any);
      const savedDraft = draftUpdateForPeer(drafts, draftOnlyUser.id);
      requireCondition(savedDraft?.draft?.message === draftText, 'messages.getAllDrafts did not return the saved draft');
      const draftOnlyDialogs = await alice.client.invoke(new Api.messages.GetPeerDialogs({
        peers: [new Api.InputDialogPeer({ peer: draftOnlyPeer })],
      }));
      requireCondition(draftOnlyDialogs?.dialogs?.[0]?.draft?.message === draftText, 'messages.getPeerDialogs did not return the draft-only dialog draft');

      const cleared = await alice.client.invoke(clearAllDraftsRequest() as any);
      requireCondition(isTrue(cleared), 'messages.clearAllDrafts did not return true');

      const clearedDrafts = await alice.client.invoke(getAllDraftsRequest() as any);
      const clearedDraft = draftUpdateForPeer(clearedDrafts, draftOnlyUser.id);
      requireCondition(!clearedDraft?.draft?.message, 'messages.getAllDrafts returned a nonempty draft after clearing');
      const clearedDraftOnlyDialogs = await alice.client.invoke(new Api.messages.GetPeerDialogs({
        peers: [new Api.InputDialogPeer({ peer: draftOnlyPeer })],
      }));
      requireCondition(!clearedDraftOnlyDialogs?.dialogs?.[0]?.draft?.message, 'messages.getPeerDialogs returned a nonempty draft after clearAllDrafts');
    });

    await check('private draft overwrites the cached dialog draft', async () => {
      await alice.client.invoke(new Api.messages.SendMessage({
        peer: bobPeer,
        message: `${tag} draft setup`,
        randomId: randomId(),
      }));
      const firstDraft = `${tag} first draft`;
      const firstSaved = await alice.client.invoke(new Api.messages.SaveDraft({ peer: bobPeer, message: firstDraft }));
      requireCondition(isTrue(firstSaved), 'messages.saveDraft did not return true for the first draft');
      const firstDialogs = await alice.client.invoke(new Api.messages.GetPeerDialogs({
        peers: [new Api.InputDialogPeer({ peer: bobPeer })],
      }));
      requireCondition(firstDialogs?.dialogs?.[0]?.draft?.message === firstDraft, 'messages.getPeerDialogs did not return the first saved draft');
      const secondDraft = `${tag} second draft`;
      const secondSaved = await alice.client.invoke(new Api.messages.SaveDraft({ peer: bobPeer, message: secondDraft }));
      requireCondition(isTrue(secondSaved), 'messages.saveDraft did not return true for the replacement draft');
      const secondDialogs = await alice.client.invoke(new Api.messages.GetPeerDialogs({
        peers: [new Api.InputDialogPeer({ peer: bobPeer })],
      }));
      requireCondition(secondDialogs?.dialogs?.[0]?.draft?.message === secondDraft, 'messages.getPeerDialogs returned a stale draft after replacement');
      const secondAllDrafts = await alice.client.invoke(getAllDraftsRequest() as any);
      const secondAllDraft = draftUpdateForPeer(secondAllDrafts, bobUser.id);
      requireCondition(secondAllDraft?.draft?.message === secondDraft, 'messages.getAllDrafts returned a stale draft after replacement');
      const cleared = await alice.client.invoke(new Api.messages.SaveDraft({ peer: bobPeer, message: '' }));
      requireCondition(isTrue(cleared), 'messages.saveDraft did not clear the draft');
      const clearedDialogs = await alice.client.invoke(new Api.messages.GetPeerDialogs({
        peers: [new Api.InputDialogPeer({ peer: bobPeer })],
      }));
      requireCondition(!clearedDialogs?.dialogs?.[0]?.draft?.message, 'messages.getPeerDialogs returned a stale draft after clearing');
      const clearedAllDrafts = await alice.client.invoke(getAllDraftsRequest() as any);
      const clearedDraft = draftUpdateForPeer(clearedAllDrafts, bobUser.id);
      requireCondition(!clearedDraft?.draft?.message, 'messages.getAllDrafts returned a stale draft after saveDraft clear');
    });

    await check('pinned dialog can be retrieved and unpinned', async () => {
      const inputDialogPeer = new Api.InputDialogPeer({ peer: bobPeer });
      const pinned = await alice.client.invoke(new Api.messages.ToggleDialogPin({ pinned: true, peer: inputDialogPeer }));
      requireCondition(isTrue(pinned), 'messages.toggleDialogPin did not return true');
      const list = await alice.client.invoke(new Api.messages.GetPinnedDialogs({ folderId: 0 }));
      requireCondition((list?.dialogs || []).some((dialog: any) => peerId(dialog?.peer) === String(bobUser.id)), 'messages.getPinnedDialogs did not return the pinned user dialog');
      const unpinned = await alice.client.invoke(new Api.messages.ToggleDialogPin({ pinned: false, peer: inputDialogPeer }));
      requireCondition(isTrue(unpinned), 'messages.toggleDialogPin did not return true when unpinning');
      const afterUnpin = await alice.client.invoke(new Api.messages.GetPinnedDialogs({ folderId: 0 }));
      requireCondition(!(afterUnpin?.dialogs || []).some((dialog: any) => peerId(dialog?.peer) === String(bobUser.id)), 'messages.getPinnedDialogs retained the unpinned user dialog');
    });
  } finally {
    try {
      await alice.client.invoke(new Api.account.SetPrivacy({
        key: new Api.InputPrivacyKeyStatusTimestamp(),
        rules: [new Api.InputPrivacyValueAllowAll()],
      }));
      await alice.client.invoke(new Api.account.UpdateNotifySettings({
        peer: new Api.InputNotifyPeer({ peer: bobPeer }),
        settings: new Api.InputPeerNotifySettings({
          muteUntil: 0,
          showPreviews: true,
          silent: false,
          sound: new Api.NotificationSoundDefault(),
        }),
      }));
      await alice.client.invoke(new Api.messages.ToggleDialogPin({
        peer: new Api.InputDialogPeer({ peer: bobPeer }),
      }));
      await alice.client.invoke(clearAllDraftsRequest() as any);
      await alice.client.invoke(new Api.messages.SaveDraft({ peer: bobPeer, message: '' }));
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
