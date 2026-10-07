import path from 'node:path';
import { createRequire } from 'node:module';

const gramjsDir = process.env.TEAMGRAM_GRAMJS_DIR || '/Users/mac/open/test/telegram-tt-master';
const dcId = Number(process.env.TG_DC_ID || '2');
const dcHost = process.env.TG_DC_HOST || '127.0.0.1';
const dcPort = Number(process.env.TG_DC_PORT || '11443');
const authKeyB64 = process.env.CODEX_BROWSER_AUTH_B64;
const expectedUserID = process.env.CODEX_BROWSER_USER_ID;

if (!authKeyB64 || !expectedUserID) throw new Error('CODEX_BROWSER_AUTH_B64 and CODEX_BROWSER_USER_ID are required');

const globalAny = globalThis as typeof globalThis & { self?: any; addEventListener?: () => void };
globalAny.self ??= globalThis;
globalAny.addEventListener ??= () => {};
globalAny.self.addEventListener ??= globalAny.addEventListener;

const requireModule = createRequire(import.meta.url);
const TelegramClient = requireModule(path.join(gramjsDir, 'src/lib/gramjs/client/TelegramClient.ts')).default;
const CallbackSession = requireModule(path.join(gramjsDir, 'src/lib/gramjs/sessions/CallbackSession.ts')).default;
const Api = requireModule(path.join(gramjsDir, 'src/lib/gramjs/tl/index.ts')).Api;
const Connection = requireModule(path.join(gramjsDir, 'src/lib/gramjs/network/connection/TCPObfuscated.ts')).ConnectionTCPObfuscated;

function className(value: any): string {
  return value?.className || value?.constructor?.name || typeof value;
}

function requireCondition(condition: unknown, message: string): asserts condition {
  if (!condition) throw new Error(message);
}

function callFromUpdates(value: any): any {
  const call = Array.isArray(value?.updates)
    ? value.updates.map((update: any) => update?.call).find((candidate: any) => candidate?.id !== undefined && candidate?.accessHash !== undefined)
    : undefined;
  requireCondition(call, `phone.createGroupCall returned ${className(value)} without a group call`);
  return call;
}

function makeClient(authKeyHex: string): any {
  const session = new CallbackSession({ mainDcId: dcId, keys: { [dcId]: authKeyHex } }, () => {});
  const load = session.load.bind(session);
  session.load = async () => {
    await load();
    (session as any)._serverAddress = dcHost;
    (session as any)._port = dcPort;
  };
  return new TelegramClient(session, 1, '0123456789abcdef0123456789abcdef', {
    connection: Connection,
    fallbackConnection: Connection,
    shouldAllowHttpTransport: false,
    shouldForceHttpTransport: false,
    timeout: 20,
    requestRetries: 1,
    connectionRetries: 1,
    connectionRetriesToFallback: 0,
    autoReconnect: false,
    baseLogger: { debug() {}, info() {}, warn() {}, error() {} },
  });
}

async function main() {
  const client = makeClient(Buffer.from(authKeyB64, 'base64').toString('hex'));
  let inputCall: any;
  let discarded = false;
  const passed: string[] = [];
  try {
    (client as any)._borrowExportedSender = async () => undefined;
    (client as any)._updateLoop = async () => undefined;
    await client.connect();
    const me = await client.getMe();
    requireCondition(me?.id !== undefined && String(me.id) === expectedUserID, 'auth key user mismatch');

    console.log('STEP create');
    const created = await client.invoke(new Api.phone.CreateGroupCall({
      peer: new Api.InputPeerSelf(),
      randomId: Number(BigInt(Date.now()) % 2147483647n),
      title: `production control ${Date.now()}`,
    }));
    const call = callFromUpdates(created);
    inputCall = new Api.InputGroupCall({ id: call.id, accessHash: call.accessHash });
    passed.push(`create:${className(created)}`);

    console.log('STEP export invite');
    const invite = await client.invoke(new Api.phone.ExportGroupCallInvite({ call: inputCall, canSelfUnmute: true }));
    requireCondition(typeof invite?.link === 'string' && invite.link.includes('invite_hash='), 'group-call invite did not return a bearer link');
    passed.push(`exportInvite:${className(invite)}`);

    console.log('STEP subscribe');
    await client.invoke(new Api.phone.ToggleGroupCallStartSubscription({ call: inputCall, subscribed: true }));
    const subscribed = await client.invoke(new Api.phone.GetGroupCall({ call: inputCall, limit: 20 }));
    requireCondition(subscribed?.call?.scheduleStartSubscribed === true, 'group-call subscription was not persisted');
    passed.push('subscription:readback');

    const selfPeer = new Api.InputPeerSelf();
    console.log('STEP join');
    await client.invoke(new Api.phone.JoinGroupCall({
      call: inputCall,
      joinAs: selfPeer,
      muted: true,
      videoStopped: true,
      params: new Api.DataJSON({ data: '{}' }),
    }));
    console.log('STEP discard');
    await client.invoke(new Api.phone.DiscardGroupCall({ call: inputCall }));
    discarded = true;
    try {
      await client.invoke(new Api.phone.GetGroupCall({ call: inputCall, limit: 20 }));
      throw new Error('discarded group call remained readable');
    } catch (error) {
      requireCondition(String(error).includes('GROUPCALL_INVALID'), `discard readback returned ${String(error)}`);
    }
    passed.push('discard:readback');

    console.log(JSON.stringify({ userId: String(me.id), callId: String(call.id), passed, writes: 'ephemeral-and-cleaned' }));
  } finally {
    if (inputCall && !discarded) {
      try { await client.invoke(new Api.phone.DiscardGroupCall({ call: inputCall })); } catch {}
    }
    client.destroy();
  }
}

main().catch((error) => {
  console.error(error?.errorMessage || error?.message || String(error));
  process.exit(1);
}).then(() => process.exit(0));
