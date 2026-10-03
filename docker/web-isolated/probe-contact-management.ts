import path from 'node:path';
import { createRequire } from 'node:module';
import process from 'node:process';

const API_ID = 1;
const API_HASH = '0123456789abcdef0123456789abcdef';
const FIXTURES = [
  { phone: '12025550111', username: 'isolated_test_11' },
  { phone: '12025550112', username: 'isolated_test_12' },
  { phone: '12025550113', username: 'isolated_test_13' },
];

const quietLogger = { debug() {}, info() {}, warn() {}, error() {} };

function requireCondition(condition: unknown, message: string): asserts condition {
  if (!condition) throw new Error(message);
}

function rpcName(value: any): string {
  return value?.className || value?.constructor?.name || typeof value;
}

function isTrue(value: any): boolean {
  return value === true || rpcName(value).toLowerCase().includes('true');
}

function peerId(value: any): string | undefined {
  const id = value?.userId ?? value?.chatId ?? value?.channelId;
  return id === undefined ? undefined : String(id);
}

class RawRequest {
  CONSTRUCTOR_ID: number;
  SUBCLASS_OF_ID = 0;
  className: string;
  classType = 'request';
  private readonly body: Uint8Array;

  constructor(constructorId: number, className: string, body = new Uint8Array()) {
    this.CONSTRUCTOR_ID = constructorId;
    this.className = className;
    this.body = body;
  }

  getBytes() {
    return concatBytes(uint32(this.CONSTRUCTOR_ID), this.body);
  }

  readResult(reader: any) {
    return reader.tgReadObject();
  }
}

function uint32(value: number) {
  const bytes = new Uint8Array(4);
  new DataView(bytes.buffer).setUint32(0, value, true);
  return bytes;
}

function int32(value: number) {
  const bytes = new Uint8Array(4);
  new DataView(bytes.buffer).setInt32(0, value, true);
  return bytes;
}

function int64(value: bigint | number) {
  const bytes = new Uint8Array(8);
  new DataView(bytes.buffer).setBigInt64(0, BigInt(value), true);
  return bytes;
}

function concatBytes(...parts: Uint8Array[]) {
  const bytes = new Uint8Array(parts.reduce((size, part) => size + part.length, 0));
  let offset = 0;
  for (const part of parts) {
    bytes.set(part, offset);
    offset += part.length;
  }
  return bytes;
}

function objectBytes(value: any) {
  requireCondition(typeof value?.getBytes === 'function', 'raw request argument has no TL bytes');
  return value.getBytes();
}

function vectorBytes(values: Uint8Array[]) {
  return concatBytes(uint32(0x1cb5c415), int32(values.length), ...values);
}

function rawResetSaved() {
  return new RawRequest(0x879537f1, 'contacts.ResetSaved');
}

function rawAcceptContact(user: any) {
  return new RawRequest(0xf831a20f, 'contacts.AcceptContact', objectBytes(user));
}

function rawEditCloseFriends(ids: any[]) {
  return new RawRequest(0xba6705f0, 'contacts.EditCloseFriends', vectorBytes(ids.map(int64)));
}

function rawSetBlocked(peers: any[], limit = 100) {
  return new RawRequest(0x94c65c76, 'contacts.SetBlocked', concatBytes(
    int32(0),
    vectorBytes(peers.map(objectBytes)),
    int32(limit),
  ));
}

function rawUpdateContactNote(user: any, note: any) {
  return new RawRequest(0x139f63fb, 'contacts.UpdateContactNote', concatBytes(objectBytes(user), objectBytes(note)));
}

function rawToggleTopPeers(enabled: boolean) {
  return new RawRequest(0x8514bdda, 'contacts.ToggleTopPeers', uint32(enabled ? 0x997275b5 : 0xbc799737));
}

function rawResetTopPeerRating(peer: any) {
  return new RawRequest(0x1ae373ac, 'contacts.ResetTopPeerRating', concatBytes(
    uint32(0x0637b7ed),
    objectBytes(peer),
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
    firstAndLastNames: async () => ['Isolated', 'Contact'],
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

function inputUser(Api: any, user: any) {
  requireCondition(user?.id !== undefined && user?.accessHash !== undefined, 'user lacks access hash');
  return new Api.InputUser({ userId: user.id, accessHash: user.accessHash });
}

function inputPeerUser(Api: any, user: any) {
  requireCondition(user?.id !== undefined && user?.accessHash !== undefined, 'peer lacks access hash');
  return new Api.InputPeerUser({ userId: user.id, accessHash: user.accessHash });
}

async function contacts(Api: any, actor: any) {
  const result = await actor.client.invoke(new Api.contacts.GetContacts({ hash: 0n }));
  requireCondition(Array.isArray(result?.users), `contacts.getContacts returned ${rpcName(result)} without users`);
  return result;
}

async function blocked(Api: any, actor: any) {
  const result = await actor.client.invoke(new Api.contacts.GetBlocked({ offset: 0, limit: 100 }));
  requireCondition(Array.isArray(result?.blocked), `contacts.getBlocked returned ${rpcName(result)} without blocked peers`);
  return result;
}

function includesUser(result: any, userId: any) {
  return result.users.some((user: any) => String(user?.id) === String(userId));
}

function includesBlocked(result: any, userId: any) {
  return result.blocked.some((entry: any) => peerId(entry?.peerId) === String(userId));
}

function includesTopPeer(result: any, userId: any) {
  return (result?.categories || []).some((category: any) => (
    (category?.peers || []).some((entry: any) => peerId(entry?.peer) === String(userId))
  ));
}

async function addContact(Api: any, actor: any, user: any) {
  const result = await actor.client.invoke(new Api.contacts.AddContact({
    id: inputUser(Api, user),
    firstName: user.firstName || 'Isolated',
    lastName: user.lastName || 'Contact',
    phone: user.phone || '',
  }));
  requireCondition(rpcName(result).toLowerCase().includes('updates'), 'contacts.addContact did not return updates');
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
  const carol = await login(gram, FIXTURES[2]);
  const tag = `contact-note-${Date.now()}-${process.pid}`;

  try {
    const bobUser = await searchUser(Api, alice, bob.username);
    const carolUser = await searchUser(Api, alice, carol.username);
    const aliceUser = await searchUser(Api, bob, alice.username);
    const bobPeer = inputPeerUser(Api, bobUser);
    const carolPeer = inputPeerUser(Api, carolUser);

    await alice.client.invoke(rawResetSaved() as any);
    await bob.client.invoke(new Api.contacts.DeleteContacts({ id: [inputUser(Api, aliceUser)] })).catch(() => {});
    await alice.client.invoke(rawSetBlocked([]) as any);

    await addContact(Api, alice, bobUser);
    requireCondition(includesUser(await contacts(Api, alice), bobUser.id), 'contacts.addContact did not persist the contact');
    console.log('PASS addContact readback through getContacts');

    const closeFriends = await alice.client.invoke(rawEditCloseFriends([bobUser.id]) as any);
    requireCondition(isTrue(closeFriends), 'contacts.editCloseFriends did not return true');
    const closeFriend = (await contacts(Api, alice)).users.find((user: any) => String(user?.id) === String(bobUser.id));
    requireCondition(closeFriend?.closeFriend === true, 'contacts.editCloseFriends did not set closeFriend');
    const clearCloseFriends = await alice.client.invoke(rawEditCloseFriends([]) as any);
    requireCondition(isTrue(clearCloseFriends), 'contacts.editCloseFriends did not clear the list');
    const clearedCloseFriend = (await contacts(Api, alice)).users.find((user: any) => String(user?.id) === String(bobUser.id));
    requireCondition(clearedCloseFriend?.closeFriend !== true, 'contacts.editCloseFriends left closeFriend set');
    console.log('PASS editCloseFriends write and clear readback');

    const note = await alice.client.invoke(rawUpdateContactNote(
      inputUser(Api, bobUser),
      new Api.TextWithEntities({ text: tag, entities: [] }),
    ) as any);
    requireCondition(isTrue(note), 'contacts.updateContactNote did not return true');
    console.log(`CONTACT_NOTE_KEY=contact_note:${alice.me.id}:${bobUser.id}`);
    console.log(`CONTACT_NOTE_TEXT=${tag}`);
    console.log('PASS updateContactNote accepted for persisted-value inspection');

    await addContact(Api, bob, aliceUser);
    const resolved = await alice.client.invoke(new Api.contacts.ResolvePhone({ phone: bob.phone }));
    requireCondition(peerId(resolved?.peer) === String(bobUser.id), 'contacts.resolvePhone did not resolve Bob');
    console.log('PASS resolvePhone resolves a consented contact');

    const replaceWithBob = await alice.client.invoke(rawSetBlocked([bobPeer]) as any);
    requireCondition(isTrue(replaceWithBob), 'contacts.setBlocked did not block Bob');
    requireCondition(includesBlocked(await blocked(Api, alice), bobUser.id), 'contacts.setBlocked did not persist Bob');
    const replaceWithCarol = await alice.client.invoke(rawSetBlocked([carolPeer]) as any);
    requireCondition(isTrue(replaceWithCarol), 'contacts.setBlocked did not replace Bob with Carol');
    const afterReplacement = await blocked(Api, alice);
    requireCondition(!includesBlocked(afterReplacement, bobUser.id) && includesBlocked(afterReplacement, carolUser.id), 'contacts.setBlocked did not replace the blocked set');
    await alice.client.invoke(rawSetBlocked([]) as any);
    const afterUnblock = await blocked(Api, alice);
    requireCondition(!includesBlocked(afterUnblock, bobUser.id) && !includesBlocked(afterUnblock, carolUser.id), 'contacts.setBlocked did not clear the blocked set');
    console.log('PASS setBlocked replaces and clears the blocked set');

    const topPeersDisabled = await alice.client.invoke(rawToggleTopPeers(false) as any);
    requireCondition(isTrue(topPeersDisabled), 'contacts.toggleTopPeers(false) did not return true');
    const disabled = await alice.client.invoke(new Api.contacts.GetTopPeers({ correspondents: true, offset: 0, limit: 20, hash: 0n }));
    requireCondition(rpcName(disabled).includes('TopPeersDisabled'), 'contacts.getTopPeers did not reflect disabled state');
    const topPeersEnabled = await alice.client.invoke(rawToggleTopPeers(true) as any);
    requireCondition(isTrue(topPeersEnabled), 'contacts.toggleTopPeers(true) did not return true');
    const enabled = await alice.client.invoke(new Api.contacts.GetTopPeers({ correspondents: true, offset: 0, limit: 20, hash: 0n }));
    requireCondition(includesTopPeer(enabled, bobUser.id), 'contacts.getTopPeers did not return the contact after enable');
    const hidden = await alice.client.invoke(rawResetTopPeerRating(bobPeer) as any);
    requireCondition(isTrue(hidden), 'contacts.resetTopPeerRating did not return true');
    const afterHide = await alice.client.invoke(new Api.contacts.GetTopPeers({ correspondents: true, offset: 0, limit: 20, hash: 0n }));
    requireCondition(!includesTopPeer(afterHide, bobUser.id), 'contacts.resetTopPeerRating did not hide the contact');
    console.log(`TOP_PEERS_KEY=contacts:${alice.me.id}:top_peers`);
    console.log('PASS getTopPeers, toggleTopPeers, and resetTopPeerRating readback');

    const resetSaved = await alice.client.invoke(rawResetSaved() as any);
    requireCondition(isTrue(resetSaved), 'contacts.resetSaved did not return true');
    requireCondition(!includesUser(await contacts(Api, alice), bobUser.id), 'contacts.resetSaved left the contact in getContacts');
    const saved = await alice.client.invoke(new Api.contacts.GetSaved());
    requireCondition(Array.isArray(saved) && saved.length === 0, 'contacts.resetSaved left saved contacts');
    console.log('PASS resetSaved clears contact and saved-contact state');

    await addContact(Api, bob, aliceUser);
    const accepted = await alice.client.invoke(rawAcceptContact(inputUser(Api, bobUser)) as any);
    requireCondition(rpcName(accepted).toLowerCase().includes('updates'), 'contacts.acceptContact did not return updates');
    requireCondition(includesUser(await contacts(Api, alice), bobUser.id), 'contacts.acceptContact did not persist Alice contact state');
    console.log('PASS acceptContact persists a reciprocal contact');

    const deleted = await alice.client.invoke(new Api.contacts.DeleteContacts({ id: [inputUser(Api, bobUser)] }));
    requireCondition(rpcName(deleted).toLowerCase().includes('updates'), 'contacts.deleteContacts did not return updates');
    requireCondition(!includesUser(await contacts(Api, alice), bobUser.id), 'contacts.deleteContacts left the contact in getContacts');
    console.log('PASS deleteContacts clears the contact readback');
  } finally {
    try { await alice.client.invoke(rawSetBlocked([]) as any); } catch {}
    try { await alice.client.invoke(rawResetSaved() as any); } catch {}
    try { await bob.client.invoke(new Api.contacts.DeleteContacts({ id: [inputUser(Api, await searchUser(Api, bob, alice.username))] })); } catch {}
    alice.client.destroy();
    bob.client.destroy();
    carol.client.destroy();
  }
}

void run()
  .then(() => process.exit(0))
  .catch((error) => {
    console.error(error instanceof Error && error.stack ? error.stack : String(error));
    process.exit(1);
  });
