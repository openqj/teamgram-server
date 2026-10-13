import path from 'node:path';
import { createRequire } from 'node:module';

// This probe deliberately leaves GramJS' update transport enabled. The other
// acceptance probes disable it while polling history; this one verifies the
// recipient fan-out path on two independent authenticated sessions.
const required = [
  'TEAMGRAM_GRAMJS_DIR', 'A_AUTH_B64', 'B_AUTH_B64', 'A_USER_ID', 'B_USER_ID',
  'A_ACCESS_HASH', 'B_ACCESS_HASH',
];
for (const key of required) {
  if (!process.env[key]) throw new Error(`${key} is required`);
}

const gramjsDir = process.env.TEAMGRAM_GRAMJS_DIR!;
const dcId = Number(process.env.TG_DC_ID || '2');
const dcHost = process.env.TG_DC_HOST || '127.0.0.1';
const dcPort = Number(process.env.TG_DC_PORT || '11443');
const apiId = Number(process.env.TG_API_ID || '1');
const apiHash = process.env.TG_API_HASH || '0123456789abcdef0123456789abcdef';
const aID = BigInt(process.env.A_USER_ID!);
const bID = BigInt(process.env.B_USER_ID!);
const aHash = BigInt(process.env.A_ACCESS_HASH!);
const bHash = BigInt(process.env.B_ACCESS_HASH!);
const timeoutMs = Number(process.env.LIVE_UPDATE_TIMEOUT_MS || '8000');

const globalAny = globalThis as typeof globalThis & { self?: any; addEventListener?: () => void };
globalAny.self ??= globalThis;
globalAny.addEventListener ??= () => {};
globalAny.self.addEventListener ??= globalAny.addEventListener;

const requireModule = createRequire(import.meta.url);
const TelegramClient = requireModule(path.join(gramjsDir, 'src/lib/gramjs/client/TelegramClient.ts')).default;
const CallbackSession = requireModule(path.join(gramjsDir, 'src/lib/gramjs/sessions/CallbackSession.ts')).default;
const Api = requireModule(path.join(gramjsDir, 'src/lib/gramjs/tl/index.ts')).Api;
const Connection = requireModule(path.join(gramjsDir, 'src/lib/gramjs/network/connection/TCPObfuscated.ts')).ConnectionTCPObfuscated;
const quietLogger = { debug() {}, info() {}, warn() {}, error() {} };

function rpcName(value: any): string {
  return value?.className || value?.constructor?.name || typeof value;
}

function stateFields(value: any): { pts: number; qts: number; seq: number; date: number } {
  return {
    pts: Number(value?.pts ?? 0),
    qts: Number(value?.qts ?? 0),
    seq: Number(value?.seq ?? 0),
    date: Number(value?.date ?? 0),
  };
}

function assertState(label: string, state: { pts: number; qts: number; seq: number; date: number }): void {
  for (const [field, value] of Object.entries(state)) {
    if (!Number.isInteger(value)) throw new Error(`${label} state had non-integer ${field}=${value}`);
  }
  if (state.pts < 0 || state.qts < 0 || state.date <= 0) {
    throw new Error(`${label} state had invalid pts/qts/date=${JSON.stringify(state)}`);
  }
}

function makeClient(authBase64: string): any {
  const session = new CallbackSession({
    mainDcId: dcId,
    keys: { [dcId]: Buffer.from(authBase64, 'base64').toString('hex') },
  }, () => {});
  const load = session.load.bind(session);
  session.load = async () => {
    await load();
    (session as any)._serverAddress = dcHost;
    (session as any)._port = dcPort;
  };
  const client = new TelegramClient(session, apiId, apiHash, {
    connection: Connection,
    fallbackConnection: Connection,
    shouldAllowHttpTransport: false,
    shouldForceHttpTransport: false,
    timeout: 20,
    requestRetries: 1,
    connectionRetries: 1,
    connectionRetriesToFallback: 0,
    retryDelay: 250,
    autoReconnect: false,
    sequentialUpdates: true,
    baseLogger: quietLogger,
  });
  // The probe only uses the authenticated primary DC. Avoid an unrelated
  // exported media sender, while retaining the primary sender's update loop.
  (client as any)._borrowExportedSender = async () => undefined;
  return client;
}

function inputPeer(id: bigint, accessHash: bigint): any {
  return new Api.InputPeerUser({ userId: id, accessHash });
}

type LiveMessage = {
  text: string;
  id: number;
  pts?: number;
  ptsCount?: number;
  date?: number;
  seq?: number;
  qts?: number;
};

function updateMessage(update: any, envelope: { date?: number; seq?: number; qts?: number } = {}): LiveMessage | undefined {
  const message = update?.message;
  if (message?.message !== undefined) {
    return {
      text: String(message.message),
      id: Number(message.id || 0),
      pts: Number.isInteger(update.pts) ? Number(update.pts) : (Number.isInteger(message.pts) ? Number(message.pts) : undefined),
      ptsCount: Number.isInteger(update.ptsCount) ? Number(update.ptsCount) : (Number.isInteger(message.ptsCount) ? Number(message.ptsCount) : undefined),
      date: Number.isInteger(message.date) ? Number(message.date) : envelope.date,
      seq: envelope.seq,
      qts: Number.isInteger(update.qts) ? Number(update.qts) : envelope.qts,
    };
  }
  // UpdateShortMessage is used by some gateways for private messages and
  // carries the text directly on the update rather than on `message`.
  if (update?.message !== undefined && typeof update.message === 'string') {
    return {
      text: String(update.message),
      id: Number(update.id || 0),
      pts: Number.isInteger(update.pts) ? Number(update.pts) : undefined,
      ptsCount: Number.isInteger(update.ptsCount) ? Number(update.ptsCount) : undefined,
      date: Number.isInteger(update.date) ? Number(update.date) : envelope.date,
      seq: envelope.seq,
      qts: Number.isInteger(update.qts) ? Number(update.qts) : envelope.qts,
    };
  }
  return undefined;
}

function updateMessages(update: any, envelope: { date?: number; seq?: number; qts?: number } = {}): LiveMessage[] {
  const nextEnvelope = {
    date: Number.isInteger(update?.date) ? Number(update.date) : envelope.date,
    seq: Number.isInteger(update?.seq) ? Number(update.seq) : envelope.seq,
    qts: Number.isInteger(update?.qts) ? Number(update.qts) : envelope.qts,
  };
  if (Array.isArray(update?.updates)) {
    return update.updates.flatMap((item: any) => updateMessages(item, nextEnvelope));
  }
  if (update?.update) {
    return updateMessages(update.update, nextEnvelope);
  }
  const event = updateMessage(update, nextEnvelope);
  return event ? [event] : [];
}

function updateBuilder(): { build: (update: any) => any } {
  return { build: (update: any) => update };
}

function waitForText(events: any[], text: string, deadline: number): Promise<any> {
  return new Promise((resolve, reject) => {
    const poll = () => {
      const match = events.find((event) => event?.text === text);
      if (match) {
        resolve(match);
        return;
      }
      if (Date.now() >= deadline) {
        reject(new Error(`live update timeout for ${text}; observed=${JSON.stringify(events.slice(-8))}`));
        return;
      }
      setTimeout(poll, 25).unref?.();
    };
    poll();
  });
}

function history(peer: any): any {
  return new Api.messages.GetHistory({
    peer, offsetId: 0, offsetDate: 0, addOffset: 0, limit: 100,
    maxId: 0, minId: 0, hash: 0n,
  });
}

function findHistoryMessage(result: any, text: string): any {
  return (Array.isArray(result?.messages) ? result.messages : [])
    .find((message: any) => message?.message === text);
}

async function disconnect(client: any): Promise<void> {
  try {
    await client.disconnect();
  } catch {
    // Cleanup must not hide a failed assertion.
  }
}

async function main() {
  if (!Number.isSafeInteger(timeoutMs) || timeoutMs < 1000 || timeoutMs > 60000) {
    throw new Error('LIVE_UPDATE_TIMEOUT_MS must be between 1000 and 60000');
  }
  const a = makeClient(process.env.A_AUTH_B64!);
  const b = makeClient(process.env.B_AUTH_B64!);
  const eventsA: any[] = [];
  const eventsB: any[] = [];
  const tag = `live-update-${Date.now()}-${process.pid}`;
  const aToB = `${tag}:a-to-b`;
  const bToA = `${tag}:b-to-a`;
  let aMessageId = 0;
  let bMessageId = 0;
  try {
    a.addEventHandler((update: any) => {
      for (const event of updateMessages(update)) {
        eventsA.push({ ...event, update: rpcName(update) });
      }
    }, updateBuilder());
    b.addEventHandler((update: any) => {
      for (const event of updateMessages(update)) {
        eventsB.push({ ...event, update: rpcName(update) });
      }
    }, updateBuilder());
    await Promise.all([a.connect(), b.connect()]);
    const [meA, meB] = await Promise.all([a.getMe(), b.getMe()]);
    if (String(meA?.id) !== aID.toString() || String(meB?.id) !== bID.toString()) {
      throw new Error(`auth key user mismatch: ${String(meA?.id)} ${String(meB?.id)}`);
    }

    // Telegram marks a generic updates session online when updates.getState
    // (or getDifference) is requested. Without this handshake the status
    // service correctly treats these auth keys as history-only sessions and
    // recipient fan-out has nowhere to deliver a live update.
    const [stateA, stateB] = await Promise.all([
      a.invoke(new Api.updates.GetState()),
      b.invoke(new Api.updates.GetState()),
    ]);
    if (rpcName(stateA) !== 'updates.State' || rpcName(stateB) !== 'updates.State') {
      throw new Error(`updates.getState returned ${rpcName(stateA)} / ${rpcName(stateB)}`);
    }
    const beforeStateA = stateFields(stateA);
    const beforeStateB = stateFields(stateB);
    assertState('A before', beforeStateA);
    assertState('B before', beforeStateB);
    // Some gateways only mark the generic session online after the explicit
    // account.updateStatus handshake. Keep it in the acceptance path so the
    // check exercises the same producer/recipient registration used by the
    // Web client.
    await Promise.all([
      a.invoke(new Api.account.UpdateStatus({ offline: false })),
      b.invoke(new Api.account.UpdateStatus({ offline: false })),
    ]);

    const peerB = inputPeer(bID, bHash);
    const peerA = inputPeer(aID, aHash);
    const sentA = await a.invoke(new Api.messages.SendMessage({
      peer: peerB, message: aToB, randomId: BigInt(Date.now()) * 1000n + 1n,
    }));
    const eventB = await waitForText(eventsB, aToB, Date.now() + timeoutMs);
    const sentB = await b.invoke(new Api.messages.SendMessage({
      peer: peerA, message: bToA, randomId: BigInt(Date.now()) * 1000n + 2n,
    }));
    const eventA = await waitForText(eventsA, bToA, Date.now() + timeoutMs);

    const [historyA, historyB] = await Promise.all([a.invoke(history(peerB)), b.invoke(history(peerA))]);
    const [stateAfterA, stateAfterB] = await Promise.all([
      a.invoke(new Api.updates.GetState()),
      b.invoke(new Api.updates.GetState()),
    ]);
    if (rpcName(stateAfterA) !== 'updates.State' || rpcName(stateAfterB) !== 'updates.State') {
      throw new Error(`updates.getState after delivery returned ${rpcName(stateAfterA)} / ${rpcName(stateAfterB)}`);
    }
    const afterStateA = stateFields(stateAfterA);
    const afterStateB = stateFields(stateAfterB);
    assertState('A after', afterStateA);
    assertState('B after', afterStateB);
    const storedA = findHistoryMessage(historyA, bToA);
    const storedB = findHistoryMessage(historyB, aToB);
    if (!storedA?.id || !storedB?.id) throw new Error('live messages were not visible in recipient histories');
    aMessageId = Number(storedB.id);
    bMessageId = Number(storedA.id);
    if (eventB.id <= 0 || eventA.id <= 0) throw new Error('recipient live update did not include a message id');
    for (const [label, event] of [['A', eventA], ['B', eventB]] as const) {
      const before = label === 'A' ? beforeStateA : beforeStateB;
      const after = label === 'A' ? afterStateA : afterStateB;
      if (!Number.isInteger(event.pts) || event.pts <= before.pts || event.pts > after.pts) {
        throw new Error(`${label} recipient update had invalid pts=${event.pts}`);
      }
      if (!Number.isInteger(event.ptsCount) || event.ptsCount <= 0) {
        throw new Error(`${label} recipient update had invalid pts_count=${event.ptsCount}`);
      }
      if (!Number.isInteger(event.date) || event.date <= 0) {
        throw new Error(`${label} recipient update had invalid date=${event.date}`);
      }
    }
    console.log(JSON.stringify({
      ok: true,
      transport: `DC${dcId} TCPObfuscated -> gateway -> session -> recipient update producer`,
      users: { sender: aID.toString(), recipient: bID.toString() },
      aToB: { send: rpcName(sentA), event: eventB },
      bToA: { send: rpcName(sentB), event: eventA },
      histories: { sender: rpcName(historyA), recipient: rpcName(historyB), aToB: bMessageId, bToA: aMessageId },
      state: { before: { a: beforeStateA, b: beforeStateB }, after: { a: afterStateA, b: afterStateB } },
      eventCounts: { a: eventsA.length, b: eventsB.length },
    }));
  } finally {
    if (aMessageId > 0) {
      try { await a.invoke(new Api.messages.DeleteMessages({ revoke: true, id: [aMessageId] })); } catch {}
    }
    if (bMessageId > 0) {
      try { await b.invoke(new Api.messages.DeleteMessages({ revoke: true, id: [bMessageId] })); } catch {}
    }
    await Promise.all([disconnect(a), disconnect(b)]);
  }
}

main().then(() => process.exit(0)).catch((error) => {
  console.error(`cross-account live-update probe failed: ${error instanceof Error ? error.stack || error.message : String(error)}`);
  process.exit(1);
});
