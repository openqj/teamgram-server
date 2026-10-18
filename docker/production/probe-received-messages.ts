import path from 'node:path';
import { createRequire } from 'node:module';
import { randomBytes } from 'node:crypto';
import { authSession, loadFixtureAuthKey } from '../web-isolated/fixture-auth';

for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) {
  delete process.env[key];
}

const dcId = Number(process.env.TG_DC_ID || '2');
const dcHost = process.env.TG_DC_HOST || '127.0.0.1';
const dcPort = Number(process.env.TG_DC_PORT || '11443');
const timeoutMs = Number(process.env.RECEIVED_MESSAGES_TIMEOUT_MS || '15000');
const tag = process.env.RECEIVED_MESSAGES_TAG || `received-messages-${Date.now()}-${randomBytes(4).toString('hex')}`;
const gramjsDir = process.env.TEAMGRAM_GRAMJS_DIR || '/Users/mac/open/test/telegram-tt-master';
const quietLogger = { debug() {}, info() {}, warn() {}, error() {} };

const globalAny = globalThis as typeof globalThis & { self?: any; addEventListener?: () => void };
globalAny.self ??= globalThis;
globalAny.addEventListener ??= () => {};
globalAny.self.addEventListener ??= globalAny.addEventListener;

const requireModule = createRequire(import.meta.url);
const TelegramClient = requireModule(path.join(gramjsDir, 'src/lib/gramjs/client/TelegramClient.ts')).default;
const CallbackSession = requireModule(path.join(gramjsDir, 'src/lib/gramjs/sessions/CallbackSession.ts')).default;
const Api = requireModule(path.join(gramjsDir, 'src/lib/gramjs/tl/index.ts')).Api;
const Connection = requireModule(path.join(gramjsDir, 'src/lib/gramjs/network/connection/TCPObfuscated.ts')).ConnectionTCPObfuscated;

type Actor = { name: 'A' | 'B'; client: any; id: bigint; accessHash: bigint; events: any[] };

function rpcName(value: any): string {
  return value?.className || value?.constructor?.name || typeof value;
}

function check(value: unknown, message: string): asserts value {
  if (!value) throw new Error(message);
}

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

async function bounded<T>(promise: Promise<T>): Promise<T> {
  let timer: ReturnType<typeof setTimeout> | undefined;
  try {
    return await Promise.race([promise, new Promise<T>((_, reject) => {
      timer = setTimeout(() => reject(new Error('PROBE_TIMEOUT')), timeoutMs);
    })]);
  } finally {
    if (timer) clearTimeout(timer);
  }
}

function walkMessages(value: any, output: any[] = []): any[] {
  if (!value || typeof value !== 'object') return output;
  if (value.message && typeof value.message === 'object' && typeof value.message.message === 'string') {
    output.push(value.message);
  }
  if (Array.isArray(value.updates)) value.updates.forEach((item: any) => walkMessages(item, output));
  if (value.update) walkMessages(value.update, output);
  return output;
}

function makeActor(name: 'A' | 'B', authB64: string): Actor {
  check(Buffer.from(authB64, 'base64').length === 256, `${name}_AUTH_KEY_INVALID`);
  const session = authSession({ CallbackSession } as any, authB64, dcId, dcHost, dcPort);
  const client = new TelegramClient(session, Number(process.env.TG_API_ID || '1'),
    process.env.TG_API_HASH || '0123456789abcdef0123456789abcdef', {
      connection: Connection,
      fallbackConnection: Connection,
      shouldAllowHttpTransport: false,
      shouldForceHttpTransport: false,
      timeout: 20,
      requestRetries: 1,
      connectionRetries: 2,
      connectionRetriesToFallback: 0,
      retryDelay: 250,
      autoReconnect: false,
      sequentialUpdates: true,
      baseLogger: quietLogger,
    });
  (client as any)._borrowExportedSender = async () => undefined;
  const actor: Actor = { name, client, id: 0n, accessHash: 0n, events: [] };
  client.addEventHandler((update: any) => {
    actor.events.push(...walkMessages(update).map((message) => ({ message, at: Date.now() })));
  }, { build: (update: any) => update });
  return actor;
}

async function connect(actor: Actor): Promise<void> {
  await bounded(actor.client.connect());
  const me = await bounded(actor.client.getMe());
  actor.id = BigInt(me?.id || 0);
  actor.accessHash = BigInt(me?.accessHash || 0);
  check(actor.id > 0n && actor.accessHash !== 0n, `${actor.name}_IDENTITY_INVALID`);
  // Register the session as an active updates recipient before sending. The
  // server does not need a client-side polling loop for this, but getState and
  // updateStatus are the Layer 229 activation calls used by real clients.
  await bounded(actor.client.invoke(new Api.updates.GetState()));
  await bounded(actor.client.invoke(new Api.account.UpdateStatus({ offline: false })));
}

async function waitForMessage(actor: Actor, text: string): Promise<any> {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    const found = actor.events.find((event) => event.message?.message === text);
    if (found) return found.message;
    await sleep(25);
  }
  throw new Error(`${actor.name}_LIVE_UPDATE_TIMEOUT`);
}

async function deleteTagged(actor: Actor, peer: any): Promise<number> {
  const history = await bounded(actor.client.invoke(new Api.messages.GetHistory({
    peer, offsetId: 0, offsetDate: 0, addOffset: 0, limit: 100, maxId: 0, minId: 0, hash: 0n,
  })));
  const ids = (history?.messages || [])
    .filter((message: any) => typeof message?.message === 'string' && message.message.startsWith(`${tag}:`))
    .map((message: any) => Number(message.id)).filter((id: number) => id > 0);
  if (ids.length) await bounded(actor.client.invoke(new Api.messages.DeleteMessages({ revoke: true, id: ids })));
  return ids.length;
}

async function main(): Promise<void> {
  check(Number.isInteger(timeoutMs) && timeoutMs >= 1000 && timeoutMs <= 60000, 'INVALID_TIMEOUT');
  const authA = process.env.A_AUTH_B64 || loadFixtureAuthKey(process.env.A_USERNAME || 'isolated_test_01');
  const authB = process.env.B_AUTH_B64 || loadFixtureAuthKey(process.env.B_USERNAME || 'isolated_test_02');
  check(authA && authB, 'FIXTURE_AUTH_KEYS_UNAVAILABLE');
  const a = makeActor('A', authA);
  const b = makeActor('B', authB);
  const peerA = () => new Api.InputPeerUser({ userId: a.id, accessHash: a.accessHash });
  const peerB = () => new Api.InputPeerUser({ userId: b.id, accessHash: b.accessHash });
  let messageId = 0;
  let cleanup = { A: 0, B: 0 };
  try {
    await Promise.all([connect(a), connect(b)]);
    check(a.id !== b.id, 'FIXTURE_ACCOUNTS_NOT_DISTINCT');
    const text = `${tag}:A-to-B`;
    const sent = await bounded(a.client.invoke(new Api.messages.SendMessage({
      peer: peerB(), message: text, randomId: BigInt(`0x${randomBytes(8).toString('hex')}`),
    })));
    check(['Updates', 'UpdatesCombined', 'UpdateShortSentMessage'].includes(rpcName(sent)), 'SEND_CONSTRUCTOR_INVALID');
    const received = await waitForMessage(b, text);
    messageId = Number(received.id);
    check(messageId > 0, 'RECEIVED_MESSAGE_ID_INVALID');
    const acknowledged = await bounded(b.client.invoke(new Api.messages.ReceivedMessages({ maxId: messageId })));
    const rows = Array.isArray(acknowledged) ? acknowledged : (acknowledged?.messages || acknowledged?.datas || []);
    check(Array.isArray(rows), 'ACK_CONSTRUCTOR_INVALID');
    check(rows.some((row: any) => Number(row.id) === messageId), 'ACK_ROW_MISSING');
    const replay = await bounded(b.client.invoke(new Api.messages.ReceivedMessages({ maxId: messageId })));
    const replayRows = Array.isArray(replay) ? replay : (replay?.messages || replay?.datas || []);
    check(replayRows.length === 0, 'ACK_REPLAY_NOT_EMPTY');
    const senderAck = await bounded(a.client.invoke(new Api.messages.ReceivedMessages({ maxId: messageId })));
    const senderRows = Array.isArray(senderAck) ? senderAck : (senderAck?.messages || senderAck?.datas || []);
    check(senderRows.length === 0, 'SENDER_RECEIPT_LEAKED');
    console.log(JSON.stringify({ schemaVersion: 1, probe: 'messages.receivedMessages', layer: 229,
      transport: `DC${dcId} TCPObfuscated -> gateway -> session -> Sync -> Messages BFF`,
      users: { A: String(a.id), B: String(b.id) }, messageId, sent: rpcName(sent),
      acknowledged: rpcName(acknowledged), replay: rpcName(replay), ok: true }));
  } finally {
    try { cleanup.A = await deleteTagged(a, peerB()); } catch {}
    try { cleanup.B = await deleteTagged(b, peerA()); } catch {}
    a.client.destroy();
    b.client.destroy();
    if (messageId && cleanup.A + cleanup.B === 0) throw new Error('CLEANUP_FAILED');
  }
}

main().then(() => process.exit(0)).catch((error) => {
  console.error(error instanceof Error ? error.message : String(error));
  process.exit(1);
});
