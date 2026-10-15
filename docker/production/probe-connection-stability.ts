import path from 'node:path';
import { createRequire } from 'node:module';
import { authSession, loadFixtureAuthKey } from '../web-isolated/fixture-auth';

for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) {
  delete process.env[key];
}

const gramjsDir = process.env.TEAMGRAM_GRAMJS_DIR || '/Users/mac/open/test/telegram-tt-master';
const dcId = Number(process.env.TG_DC_ID || '2');
const dcHost = process.env.TG_DC_HOST || '127.0.0.1';
const dcPort = Number(process.env.TG_DC_PORT || '11443');
const durationMs = Number(process.env.STABILITY_DURATION_MS || '15000');
const intervalMs = Number(process.env.STABILITY_INTERVAL_MS || '1000');
const reconnectTimeoutMs = Number(process.env.STABILITY_RECONNECT_TIMEOUT_MS || '10000');
const allowTransientRpcFailures = process.env.STABILITY_ALLOW_TRANSIENT_RPC_FAILURES === '1';
const aUsername = process.env.A_USERNAME || 'isolated_test_01';
const bUsername = process.env.B_USERNAME || 'isolated_test_02';
const apiId = Number(process.env.TG_API_ID || '1');
const apiHash = process.env.TG_API_HASH || '0123456789abcdef0123456789abcdef';
const quietLogger = { debug() {}, info() {}, warn() {}, error() {} };

if (!Number.isInteger(dcId) || dcId <= 0 || !Number.isInteger(dcPort) || dcPort <= 0) {
  throw new Error('TG_DC_ID and TG_DC_PORT must be positive integers');
}
if (!Number.isInteger(durationMs) || durationMs < 6000 || durationMs > 120000) {
  throw new Error('STABILITY_DURATION_MS must be between 6000 and 120000');
}
if (!Number.isInteger(intervalMs) || intervalMs < 250 || intervalMs > 10000) {
  throw new Error('STABILITY_INTERVAL_MS must be between 250 and 10000');
}

const globalAny = globalThis as typeof globalThis & { self?: any; addEventListener?: () => void };
globalAny.self ??= globalThis;
globalAny.addEventListener ??= () => {};
globalAny.self.addEventListener ??= globalAny.addEventListener;

const requireModule = createRequire(import.meta.url);
const TelegramClient = requireModule(path.join(gramjsDir, 'src/lib/gramjs/client/TelegramClient.ts')).default;
const CallbackSession = requireModule(path.join(gramjsDir, 'src/lib/gramjs/sessions/CallbackSession.ts')).default;
const Api = requireModule(path.join(gramjsDir, 'src/lib/gramjs/tl/index.ts')).Api;
const Connection = requireModule(path.join(gramjsDir, 'src/lib/gramjs/network/connection/TCPObfuscated.ts')).ConnectionTCPObfuscated;

type Actor = { name: string; client: any; id: bigint; accessHash: bigint; events: any[] };
type RpcSample = { actor: string; ok: boolean; elapsedMs: number; error?: string };

function rpcName(value: any): string {
  return value?.className || value?.constructor?.name || typeof value;
}

function requireCondition(value: unknown, message: string): asserts value {
  if (!value) throw new Error(message);
}

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

function stateFields(value: any) {
  return {
    pts: Number(value?.pts ?? 0),
    qts: Number(value?.qts ?? 0),
    seq: Number(value?.seq ?? 0),
    date: Number(value?.date ?? 0),
  };
}

function updateBuilder(): { build: (value: any) => any } {
  return { build: (value: any) => value };
}

function collectMessages(update: any, envelope: { date?: number; seq?: number; qts?: number } = {}): any[] {
  const nextEnvelope = {
    date: Number.isInteger(update?.date) ? Number(update.date) : envelope.date,
    seq: Number.isInteger(update?.seq) ? Number(update.seq) : envelope.seq,
    qts: Number.isInteger(update?.qts) ? Number(update.qts) : envelope.qts,
  };
  if (Array.isArray(update?.updates)) {
    return update.updates.flatMap((item: any) => collectMessages(item, nextEnvelope));
  }
  if (update?.update) return collectMessages(update.update, nextEnvelope);
  if (update?.message?.message !== undefined) {
    return [{
      text: String(update.message.message),
      id: Number(update.message.id || update.id || 0),
      pts: Number.isInteger(update.pts) ? Number(update.pts) : undefined,
      ptsCount: Number.isInteger(update.ptsCount) ? Number(update.ptsCount) : undefined,
      date: Number.isInteger(update.message.date) ? Number(update.message.date) : nextEnvelope.date,
      seq: nextEnvelope.seq,
      qts: nextEnvelope.qts,
    }];
  }
  if (typeof update?.message === 'string') {
    return [{
      text: update.message,
      id: Number(update.id || 0),
      pts: Number.isInteger(update.pts) ? Number(update.pts) : undefined,
      ptsCount: Number.isInteger(update.ptsCount) ? Number(update.ptsCount) : undefined,
      date: Number.isInteger(update.date) ? Number(update.date) : nextEnvelope.date,
      seq: nextEnvelope.seq,
      qts: nextEnvelope.qts,
    }];
  }
  return [];
}

function history(peer: any): any {
  return new Api.messages.GetHistory({
    peer, offsetId: 0, offsetDate: 0, addOffset: 0, limit: 100,
    maxId: 0, minId: 0, hash: 0n,
  });
}

function inputPeer(actor: Actor): any {
  return new Api.InputPeerUser({ userId: actor.id, accessHash: actor.accessHash });
}

function makeActor(name: string, username: string): Actor {
  const auth = process.env[`${name}_AUTH_B64`] || loadFixtureAuthKey(username);
  requireCondition(auth, `no auth key for ${name} (${username})`);
  const session = authSession({ CallbackSession } as any, auth, dcId, dcHost, dcPort);
  const client = new TelegramClient(session, apiId, apiHash, {
    connection: Connection,
    fallbackConnection: Connection,
    shouldAllowHttpTransport: false,
    shouldForceHttpTransport: false,
    timeout: 20,
    requestRetries: 2,
    connectionRetries: 3,
    connectionRetriesToFallback: 0,
    retryDelay: 250,
    autoReconnect: true,
    sequentialUpdates: true,
    baseLogger: quietLogger,
  });
  // The primary sender is the connection under test; exported media senders add noise
  (client as any)._borrowExportedSender = async () => undefined;
  const events: any[] = [];
  client.addEventHandler((update: any) => {
    if (rpcName(update) === 'UpdateConnectionState') {
      events.push({ kind: 'connection', state: Number(update.state), at: Date.now() });
      return;
    }
    for (const message of collectMessages(update)) events.push({ kind: 'message', ...message, at: Date.now() });
  }, updateBuilder());
  return { name, client, id: 0n, accessHash: 0n, events };
}

async function connectActor(actor: Actor): Promise<void> {
  await actor.client.connect();
  const me = await actor.client.getMe();
  actor.id = BigInt(me?.id || 0);
  actor.accessHash = BigInt(me?.accessHash || 0);
  requireCondition(actor.id > 0n && actor.accessHash !== 0n, `${actor.name} getMe returned no identity`);
}

async function waitForEvent(events: any[], text: string, timeoutMs: number): Promise<any> {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    const event = events.find((item) => item.kind === 'message' && item.text === text);
    if (event) return event;
    await sleep(50);
  }
  throw new Error(`message update timeout for ${text}`);
}

async function invokeState(actor: Actor, samples: RpcSample[]): Promise<any> {
  const startedAt = Date.now();
  try {
    const state = await actor.client.invoke(new Api.updates.GetState());
    const elapsedMs = Date.now() - startedAt;
    requireCondition(rpcName(state) === 'updates.State', `${actor.name} returned ${rpcName(state)}`);
    const fields = stateFields(state);
    requireCondition(fields.date > 0 && fields.pts >= 0 && fields.qts >= 0 && fields.seq >= -1, `${actor.name} returned invalid state`);
    samples.push({ actor: actor.name, ok: true, elapsedMs });
    return fields;
  } catch (error) {
    samples.push({ actor: actor.name, ok: false, elapsedMs: Date.now() - startedAt, error: String(error) });
    throw error;
  }
}

async function forceTransportDrop(actor: Actor): Promise<void> {
  const sender = (actor.client as any)._sender;
  const connection = sender?.getConnection?.();
  requireCondition(connection, `${actor.name} has no active transport`);
  connection.disconnect();
}

async function waitForReconnect(actor: Actor): Promise<void> {
  const deadline = Date.now() + reconnectTimeoutMs;
  while (Date.now() < deadline) {
    const sender = (actor.client as any)._sender;
    if (actor.client.isConnected?.() && !sender?.isReconnecting && !sender?.isConnecting) return;
    await sleep(100);
  }
  throw new Error(`${actor.name} did not reconnect within ${reconnectTimeoutMs}ms`);
}

async function cleanupMessages(a: Actor, b: Actor, texts: string[]): Promise<{ A: number[]; B: number[] }> {
  if (!texts.length || !a.id || !b.id) return { A: [], B: [] };
  const ids: { A: number[]; B: number[] } = { A: [], B: [] };
  try {
    const [historyA, historyB] = await Promise.all([
      a.client.invoke(history(inputPeer(b))),
      b.client.invoke(history(inputPeer(a))),
    ]);
    const collectIds = (value: any) => (Array.isArray(value?.messages) ? value.messages : [])
      .filter((message: any) => texts.includes(message?.message))
      .map((message: any) => Number(message.id || 0))
      .filter((id: number) => id > 0);
    ids.A = [...new Set(collectIds(historyA))];
    ids.B = [...new Set(collectIds(historyB))];
    await Promise.all([
      ids.A.length ? a.client.invoke(new Api.messages.DeleteMessages({ revoke: true, id: ids.A })) : undefined,
      ids.B.length ? b.client.invoke(new Api.messages.DeleteMessages({ revoke: true, id: ids.B })) : undefined,
    ]);
  } catch {
    // Cleanup is best effort when the transport itself is unavailable
  }
  return ids;
}

async function cleanup(actor: Actor): Promise<void> {
  try { await actor.client.disconnect(); } catch {}
}

async function main() {
  const startedAt = Date.now();
  const a = makeActor('A', aUsername);
  const b = makeActor('B', bUsername);
  const rpcSamples: RpcSample[] = [];
  const stateSamples: Record<string, any[]> = { A: [], B: [] };
  const forcedReconnects: string[] = [];
  const probeTexts: string[] = [];
  let aMessageId = 0;
  let bMessageId = 0;
  try {
    await Promise.all([connectActor(a), connectActor(b)]);
    const initialStates = await Promise.all([invokeState(a, rpcSamples), invokeState(b, rpcSamples)]);
    stateSamples.A.push(initialStates[0]);
    stateSamples.B.push(initialStates[1]);
    await Promise.all([
      a.client.invoke(new Api.account.UpdateStatus({ offline: false })),
      b.client.invoke(new Api.account.UpdateStatus({ offline: false })),
    ]);

    const tag = `stability-${Date.now()}-${process.pid}`;
    const aToB = `${tag}:a-to-b`;
    probeTexts.push(aToB);
    const sentA = await a.client.invoke(new Api.messages.SendMessage({
      peer: inputPeer(b), message: aToB, randomId: BigInt(Date.now()) * 1000n + 1n,
    }));
    const eventB = await waitForEvent(b.events, aToB, reconnectTimeoutMs);
    const startedLoopAt = Date.now();
    let dropped = false;
    let bToA = '';
    let eventA: any;
    while (Date.now() - startedLoopAt < durationMs) {
      const loopStates = await Promise.all([
        invokeState(a, rpcSamples).catch(() => undefined),
        invokeState(b, rpcSamples).catch(() => undefined),
      ]);
      if (loopStates[0]) stateSamples.A.push(loopStates[0]);
      if (loopStates[1]) stateSamples.B.push(loopStates[1]);
      if (!dropped && Date.now() - startedLoopAt >= Math.floor(durationMs / 3)) {
        dropped = true;
        await Promise.all([forceTransportDrop(a), forceTransportDrop(b)]);
        await Promise.all([waitForReconnect(a), waitForReconnect(b)]);
        forcedReconnects.push('A', 'B');
        // Re-register the generic updates session after a transport reconnect
        // before asserting live fan-out again.
        await Promise.all([
          a.client.invoke(new Api.updates.GetState()),
          b.client.invoke(new Api.updates.GetState()),
          a.client.invoke(new Api.account.UpdateStatus({ offline: false })),
          b.client.invoke(new Api.account.UpdateStatus({ offline: false })),
        ]);
        await sleep(250);
        bToA = `${tag}:b-to-a-after-reconnect`;
        probeTexts.push(bToA);
        const sentB = await b.client.invoke(new Api.messages.SendMessage({
          peer: inputPeer(a), message: bToA, randomId: BigInt(Date.now()) * 1000n + 2n,
        }));
        eventA = await waitForEvent(a.events, bToA, reconnectTimeoutMs);
        requireCondition(rpcName(sentB).startsWith('Updates'), `B send returned ${rpcName(sentB)}`);
      }
      await sleep(intervalMs);
    }

    const finalStates = await Promise.all([invokeState(a, rpcSamples), invokeState(b, rpcSamples)]);
    stateSamples.A.push(finalStates[0]);
    stateSamples.B.push(finalStates[1]);
    const historyA = await a.client.invoke(history(inputPeer(b)));
    const historyB = await b.client.invoke(history(inputPeer(a)));
    const findMessage = (value: any, text: string): any => (Array.isArray(value?.messages) ? value.messages : [])
      .find((message: any) => message?.message === text);
    aMessageId = Number(findMessage(historyA, aToB)?.id || 0);
    bMessageId = Number(findMessage(historyB, bToA)?.id || 0);
    requireCondition(eventB?.id > 0 && eventA?.id > 0, 'message fan-out did not include message IDs');
    requireCondition(aMessageId > 0 && bMessageId > 0, 'message history did not retain fan-out messages');
    requireCondition(forcedReconnects.length === 2, 'forced reconnect was not exercised for both accounts');
    requireCondition(rpcSamples.length >= 10, 'RPC stability probe collected too few samples');
    if (!allowTransientRpcFailures) {
      requireCondition(rpcSamples.every((sample) => sample.ok), 'RPC stability samples contained failures');
    }

    const cleanupIds = await cleanupMessages(a, b, probeTexts);
    console.log(JSON.stringify({
      ok: true,
      transport: `DC${dcId} ${dcHost}:${dcPort} TCPObfuscated`,
      durationMs: Date.now() - startedAt,
      actors: [aUsername, bUsername],
      autoReconnect: true,
      updateLoop: true,
      allowTransientRpcFailures,
      rpc: {
        total: rpcSamples.length,
        success: rpcSamples.filter((sample) => sample.ok).length,
        failures: rpcSamples.filter((sample) => !sample.ok),
      },
      forcedReconnects,
      connectionEvents: {
        A: a.events.filter((event) => event.kind === 'connection'),
        B: b.events.filter((event) => event.kind === 'connection'),
      },
      updateFanout: {
        aToB: { text: aToB, event: eventB },
        bToA: { text: bToA, event: eventA },
      },
      stateSamples,
      cleanup: { aMessageId, bMessageId, localIds: cleanupIds },
      sentConstructors: { aToB: rpcName(sentA) },
    }));
  } finally {
    await cleanupMessages(a, b, probeTexts);
    await Promise.all([cleanup(a), cleanup(b)]);
  }
}

main().then(() => process.exit(0)).catch((error) => {
  console.error(`connection stability probe failed: ${error instanceof Error ? error.stack || error.message : String(error)}`);
  process.exit(1);
});
