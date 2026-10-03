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

function requireCondition(condition: unknown, message: string): asserts condition {
  if (!condition) throw new Error(message);
}

function rpcName(value: any) {
  return value?.className || value?.constructor?.name || typeof value;
}

function inputUser(Api: any, user: any) {
  requireCondition(user?.id !== undefined && user?.accessHash !== undefined, 'user has no usable access hash');
  return new Api.InputUser({ userId: user.id, accessHash: user.accessHash });
}

function groupCallFromUpdates(value: any) {
  const call = Array.isArray(value?.updates)
    ? value.updates.map((update: any) => update?.call).find((candidate: any) => candidate?.id !== undefined && candidate?.accessHash !== undefined)
    : undefined;
  requireCondition(call, `phone.createGroupCall returned ${rpcName(value)} without a group call`);
  return call;
}

function wrongAccessHash(accessHash: bigint | number) {
  const value = BigInt(accessHash);
  return value === 0n ? 1n : value + 1n;
}

function joinRequest(Api: any, call: any) {
  return new Api.phone.JoinGroupCall({
    call,
    joinAs: new Api.InputPeerSelf(),
    muted: true,
    videoStopped: true,
    params: new Api.DataJSON({ data: '{}' }),
  });
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
      firstAndLastNames: async () => ['Isolated', 'GroupCall'],
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

  try {
    const actors = await Promise.all(FIXTURES.map((fixture) => login(gram, fixture)));
    const [owner, invitee, nonMember] = actors;
    requireCondition(owner && invitee && nonMember, 'fixture login returned an incomplete actor set');
    alice = owner;
    bob = invitee;
    carol = nonMember;
    const tag = `group_call_${Date.now()}_${process.pid}`;
    const created = await owner.client.invoke(new Api.phone.CreateGroupCall({
      peer: new Api.InputPeerSelf(),
      randomId: Number(BigInt(Date.now()) % 2_147_483_647n),
      title: `${tag} created`,
    }));
    const groupCall = groupCallFromUpdates(created);
    const input = new Api.InputGroupCall({ id: groupCall.id, accessHash: groupCall.accessHash });
    console.log(`PASS group call creation: ${rpcName(created)}`);

    const editedTitle = `${tag} edited`;
    const edited = await owner.client.invoke(new Api.phone.EditGroupCallTitle({ call: input, title: editedTitle }));
    requireCondition(rpcName(edited).toLowerCase().includes('updates'), 'phone.editGroupCallTitle did not return updates');
    const ownerReadback = await owner.client.invoke(new Api.phone.GetGroupCall({ call: input, limit: 20 }));
    requireCondition(ownerReadback?.call?.title === editedTitle, 'phone.getGroupCall did not return the edited title');
    console.log(`PASS title edit and readback: ${rpcName(ownerReadback)}`);

    const badCall = new Api.InputGroupCall({ id: groupCall.id, accessHash: wrongAccessHash(groupCall.accessHash) });
    await expectRpcError('wrong group-call access-hash rejection', 'GROUPCALL_INVALID', () => owner.client.invoke(
      new Api.phone.GetGroupCall({ call: badCall, limit: 20 }),
    ));
    await expectRpcError('non-member read rejection', 'GROUPCALL_FORBIDDEN', () => nonMember.client.invoke(
      new Api.phone.GetGroupCall({ call: input, limit: 20 }),
    ));

    const bobUser = await searchUser(Api, owner, invitee.username);
    const wrongBob = new Api.InputUser({ userId: bobUser.id, accessHash: wrongAccessHash(bobUser.accessHash) });
    await expectRpcError('wrong invitee access-hash rejection', 'USER_ID_INVALID', () => owner.client.invoke(
      new Api.phone.InviteToGroupCall({ call: input, users: [wrongBob] }),
    ));
    const invited = await owner.client.invoke(new Api.phone.InviteToGroupCall({ call: input, users: [inputUser(Api, bobUser)] }));
    requireCondition(rpcName(invited).toLowerCase().includes('updates'), 'phone.inviteToGroupCall did not return updates');
    const bobReadback = await invitee.client.invoke(new Api.phone.GetGroupCall({ call: input, limit: 20 }));
    requireCondition(String(bobReadback?.call?.id) === String(groupCall.id), 'invited user could not read the group call');
    const joined = await invitee.client.invoke(joinRequest(Api, input));
    requireCondition(rpcName(joined).toLowerCase().includes('updates'), 'invited user join did not return updates');
    requireCondition(
      Array.isArray(bobReadback?.participants)
        && bobReadback.participants.some((participant: any) => String(participant?.peer?.userId) === String(invitee.id)),
      'invited user is absent from the group call participants',
    );
    console.log(`PASS invite, read, and join: ${rpcName(joined)}`);

    await expectRpcError('non-creator title edit rejection', 'CHAT_ADMIN_REQUIRED', () => invitee.client.invoke(
      new Api.phone.EditGroupCallTitle({ call: input, title: `${tag} blocked` }),
    ));

    const discarded = await owner.client.invoke(new Api.phone.DiscardGroupCall({ call: input }));
    requireCondition(rpcName(discarded).toLowerCase().includes('updates'), 'phone.discardGroupCall did not return updates');
    console.log(`PASS group call discard: ${rpcName(discarded)}`);
    await expectRpcError('read after discard rejection', 'GROUPCALL_INVALID', () => owner.client.invoke(
      new Api.phone.GetGroupCall({ call: input, limit: 20 }),
    ));
    await expectRpcError('join after discard rejection', 'GROUPCALL_INVALID', () => invitee.client.invoke(joinRequest(Api, input)));
  } finally {
    for (const actor of [alice, bob, carol]) actor?.client.destroy();
  }
}

void run()
  .then(() => process.exit(0))
  .catch((error) => {
    console.error(`group call probe fatal: ${error instanceof Error && error.stack ? error.stack : String(error)}`);
    process.exit(1);
  });
