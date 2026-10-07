import path from 'node:path';
import { createRequire } from 'node:module';

type Client = any;

const required = ['TEAMGRAM_GRAMJS_DIR', 'A_AUTH_B64', 'B_AUTH_B64', 'A_USER_ID', 'B_USER_ID'];
for (const key of required) {
  if (!process.env[key]) throw new Error(`${key} is required`);
}

const root = process.env.TEAMGRAM_GRAMJS_DIR!;
const dcId = Number(process.env.TG_DC_ID || '2');
const apiId = Number(process.env.TG_API_ID || '1');
const apiHash = process.env.TG_API_HASH || '0123456789abcdef0123456789abcdef';
const aID = BigInt(process.env.A_USER_ID!);
const bID = BigInt(process.env.B_USER_ID!);

const globalAny = globalThis as typeof globalThis & { self?: any; addEventListener?: () => void };
globalAny.self ??= globalThis;
globalAny.addEventListener ??= () => {};
globalAny.self.addEventListener ??= globalAny.addEventListener;

const requireModule = createRequire(import.meta.url);
const TelegramClient = requireModule(path.join(root, 'src/lib/gramjs/client/TelegramClient.ts')).default;
const CallbackSession = requireModule(path.join(root, 'src/lib/gramjs/sessions/CallbackSession.ts')).default;
const Api = requireModule(path.join(root, 'src/lib/gramjs/tl/index.ts')).Api;
const ConnectionTCPObfuscated = requireModule(path.join(root, 'src/lib/gramjs/network/connection/TCPObfuscated.ts')).ConnectionTCPObfuscated;
const quietLogger = { debug() {}, info() {}, warn() {}, error() {} };

function rpcName(value: any): string {
  return value?.className || value?.constructor?.name || typeof value;
}

function makeClient(authKeyBase64: string): Client {
  const keyHex = Buffer.from(authKeyBase64, 'base64').toString('hex');
  const session = new CallbackSession({ mainDcId: dcId, keys: { [dcId]: keyHex } }, () => {});
  const load = session.load.bind(session);
  session.load = async () => {
    await load();
    (session as any)._serverAddress = process.env.TG_DC_HOST || '127.0.0.1';
    (session as any)._port = Number(process.env.TG_DC_PORT || '11443');
  };
  return new TelegramClient(session, apiId, apiHash, {
    connection: ConnectionTCPObfuscated,
    fallbackConnection: ConnectionTCPObfuscated,
    shouldAllowHttpTransport: false,
    shouldForceHttpTransport: false,
    timeout: 20,
    requestRetries: 2,
    connectionRetries: 2,
    connectionRetriesToFallback: 0,
    retryDelay: 250,
    autoReconnect: false,
    baseLogger: quietLogger,
  });
}

function callFromUpdates(value: any): any {
  const call = Array.isArray(value?.updates)
    ? value.updates.map((update: any) => update?.call).find((item: any) => item?.id !== undefined && item?.accessHash !== undefined)
    : undefined;
  if (!call) throw new Error(`phone.createGroupCall returned ${rpcName(value)} without a group call`);
  return call;
}

function inputCall(call: any): any {
  return new Api.InputGroupCall({ id: call.id, accessHash: call.accessHash });
}

async function expectError(name: string, code: string, action: () => Promise<unknown>) {
  try {
    await action();
  } catch (error) {
    if (!String(error).includes(code)) throw new Error(`${name} returned ${String(error)}, want ${code}`);
    return;
  }
  throw new Error(`${name} unexpectedly succeeded`);
}

async function main() {
  const a = makeClient(process.env.A_AUTH_B64!);
  const b = makeClient(process.env.B_AUTH_B64!);
  let input: any;
  let discarded = false;
  const tag = `prod-call-${Date.now()}-${process.pid}`;
  const results: Record<string, unknown> = { tag };
  try {
    await a.connect();
    await b.connect();
    const meA = await a.getMe();
    const meB = await b.getMe();
    if (String(meA?.id) !== String(aID) || String(meB?.id) !== String(bID)) {
      throw new Error(`auth key user mismatch: ${String(meA?.id)} ${String(meB?.id)}`);
    }

    const created = await a.invoke(new Api.phone.CreateGroupCall({
      peer: new Api.InputPeerSelf(),
      randomId: Number(BigInt(Date.now()) % 2_147_483_647n),
      title: `${tag} created`,
    }));
    const call = callFromUpdates(created);
    input = inputCall(call);
    const editedTitle = `${tag} edited`;
    const edited = await a.invoke(new Api.phone.EditGroupCallTitle({ call: input, title: editedTitle }));
    const ownerReadback = await a.invoke(new Api.phone.GetGroupCall({ call: input, limit: 20 }));
    if (ownerReadback?.call?.title !== editedTitle) throw new Error('phone.getGroupCall did not return edited title');

    const invited = await a.invoke(new Api.phone.InviteToGroupCall({
      call: input,
      users: [new Api.InputUser({ userId: bID, accessHash: BigInt(process.env.B_ACCESS_HASH || '0') })],
    }));
    const bobReadback = await b.invoke(new Api.phone.GetGroupCall({ call: input, limit: 20 }));
    const joined = await b.invoke(new Api.phone.JoinGroupCall({
      call: input,
      joinAs: new Api.InputPeerSelf(),
      muted: true,
      videoStopped: true,
      params: new Api.DataJSON({ data: '{}' }),
    }));
    if (String(bobReadback?.call?.id) !== String(call.id)) throw new Error('invited user could not read group call');

    await expectError('non-creator title edit', 'CHAT_ADMIN_REQUIRED', () => b.invoke(
      new Api.phone.EditGroupCallTitle({ call: input, title: `${tag} blocked` }),
    ));
    const left = await b.invoke(new Api.phone.LeaveGroupCall({ call: input, source: 0 }));
    const discardedResult = await a.invoke(new Api.phone.DiscardGroupCall({ call: input }));
    discarded = true;
    await expectError('read after discard', 'GROUPCALL_INVALID', () => a.invoke(
      new Api.phone.GetGroupCall({ call: input, limit: 20 }),
    ));
    results.groupCall = {
      create: rpcName(created),
      edit: rpcName(edited),
      ownerReadback: rpcName(ownerReadback),
      invite: rpcName(invited),
      memberReadback: rpcName(bobReadback),
      join: rpcName(joined),
      leave: rpcName(left),
      discard: rpcName(discardedResult),
    };
  } finally {
    if (input && !discarded) {
      try { await a.invoke(new Api.phone.DiscardGroupCall({ call: input })); } catch {}
    }
    a.destroy();
    b.destroy();
  }
  console.log(JSON.stringify(results));
  process.exit(0);
}

main().catch((error) => {
  console.error(`production control-plane probe failed: ${error instanceof Error ? error.stack || error.message : String(error)}`);
  process.exit(1);
});
