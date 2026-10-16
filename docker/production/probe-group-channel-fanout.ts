import fs from 'node:fs';
import path from 'node:path';
import { createRequire } from 'node:module';
import { randomBytes } from 'node:crypto';
import { monitorEventLoopDelay, performance } from 'node:perf_hooks';
import { spawn } from 'node:child_process';
import { fileURLToPath } from 'node:url';

const phase = process.env.FANOUT_PHASE || 'setup';
const users = Number(process.env.FANOUT_USERS || '5000');
const batchSize = Number(process.env.FANOUT_BATCH_SIZE || '50');
const recoveryUsers = Number(process.env.FANOUT_RECOVERY_USERS || '100');
const timeoutMs = Number(process.env.FANOUT_TIMEOUT_MS || '60000');
const waitMs = Number(process.env.FANOUT_WAIT_MS || '180000');
const dcId = 2;
const defaultPort = Number(process.env.TG_DC_PORT || '11443');
const endpoints: { host: string; port: number; replica: string }[] = JSON.parse(
  process.env.FANOUT_ENDPOINTS || JSON.stringify([{ host: '127.0.0.1', port: defaultPort, replica: 'backend-1' }]),
);
const tag = process.env.FANOUT_TAG || `group-channel-${Date.now()}-${randomBytes(4).toString('hex')}`;
let Api: any;
let TelegramClient: any;
let CallbackSession: any;
let Connection: any;
const eventLoopDelay = monitorEventLoopDelay({ resolution: 20 });
eventLoopDelay.enable();
const runtimeStartedAt = performance.now();
const runtimeCpuStarted = process.cpuUsage();
const quietLogger = { debug() {}, info() {}, warn() {}, error() {} };
const sleep = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms));

type Seed = { userId: string; accessHash: string; authB64: string };
type ChannelFixture = { kind: 'channel' | 'megagroup'; id: string; accessHash: string; title: string };
type ClientRow = { seed: Seed; index: number; client: any; activated: boolean };
type SeenEvent = { latencyMs: number; id: number; pts: number; update: string; observedAtMs: number };
type RecoveryOutcome =
  | { ok: true; index: number; recoveryMs: number; pts: number }
  | { ok: false; index: number; code: string };

const received = new Map<string, Map<number, SeenEvent>>();
const duplicates = new Map<string, number>();
const recoveryCohorts = new Map<string, Set<number>>();
const unexpectedRecoveryUpdates = new Map<string, Set<number>>();
const report: any = {
  schemaVersion: 1,
  probe: 'messaging.group-channel-fanout',
  evidenceClass: 'local_probe',
  productionCapacityAccepted: false,
  phase,
  tag,
  requestedUsers: users,
  batchSize,
  ok: false,
};

function check(condition: unknown, code: string): asserts condition {
  if (!condition) throw Object.assign(new Error(code), { probeCode: code });
}

function errorCode(error: any): string {
  if (typeof error?.probeCode === 'string') return error.probeCode;
  return rpcErrorMessage(error) || safeErrorType(error) || 'RPC_OR_TRANSPORT_ERROR';
}

function rpcErrorMessage(error: any): string | undefined {
  return typeof error?.errorMessage === 'string' && /^[A-Z][A-Z0-9_]{0,79}$/.test(error.errorMessage)
    ? error.errorMessage : undefined;
}

function safeErrorType(error: any): string | undefined {
  const name = error?.constructor?.name || error?.name;
  return typeof name === 'string' && /^[A-Za-z][A-Za-z0-9_]{0,79}$/.test(name) ? name : undefined;
}

function rpcName(value: any): string {
  return value?.className || value?.constructor?.name || typeof value;
}

async function bounded<T>(promise: Promise<T>): Promise<T> {
  let timer: ReturnType<typeof setTimeout> | undefined;
  try {
    return await Promise.race([promise, new Promise<T>((_, reject) => {
      timer = setTimeout(() => reject(Object.assign(new Error('PROBE_TIMEOUT'), { probeCode: 'PROBE_TIMEOUT' })), timeoutMs);
    })]);
  } finally {
    if (timer) clearTimeout(timer);
  }
}

function configure(): void {
  const globalAny = globalThis as any;
  globalAny.self ??= globalThis;
  globalAny.addEventListener ??= () => {};
  globalAny.self.addEventListener ??= globalAny.addEventListener;
  const gramjsDir = process.env.TEAMGRAM_GRAMJS_DIR;
  check(Boolean(gramjsDir), 'MISSING_TEAMGRAM_GRAMJS_DIR');
  const requireModule = createRequire(import.meta.url);
  TelegramClient = requireModule(path.join(gramjsDir!, 'src/lib/gramjs/client/TelegramClient.ts')).default;
  CallbackSession = requireModule(path.join(gramjsDir!, 'src/lib/gramjs/sessions/CallbackSession.ts')).default;
  Api = requireModule(path.join(gramjsDir!, 'src/lib/gramjs/tl/index.ts')).Api;
  Connection = requireModule(path.join(gramjsDir!, 'src/lib/gramjs/network/connection/TCPObfuscated.ts')).ConnectionTCPObfuscated;
}

function makeClient(authB64: string, index: number): any {
  const endpoint = endpoints[index % endpoints.length];
  const session = new CallbackSession({
    mainDcId: dcId,
    keys: { [dcId]: Buffer.from(authB64, 'base64').toString('hex') },
  }, () => {});
  const load = session.load.bind(session);
  session.load = async () => {
    await load();
    session._serverAddress = endpoint.host;
    session._port = endpoint.port;
  };
  const client = new TelegramClient(session, 1, '0123456789abcdef0123456789abcdef', {
    connection: Connection,
    fallbackConnection: Connection,
    shouldAllowHttpTransport: false,
    shouldForceHttpTransport: false,
    timeout: Math.ceil(timeoutMs / 1000),
    requestRetries: 1,
    connectionRetries: 1,
    connectionRetriesToFallback: 0,
    retryDelay: 250,
    autoReconnect: false,
    sequentialUpdates: true,
    baseLogger: quietLogger,
    deviceModel: `fanout-${index}`,
  });
  const initWith = client._initWith.bind(client);
  client._initWith = (query: any) => {
    const original = initWith(query);
    check(rpcName(original) === 'InvokeWithLayer' && rpcName(original.query) === 'InitConnection'
      && original.query.query === query, 'INVALID_INIT_CONNECTION_WRAPPER');
    return new Api.InvokeWithLayer({ layer: 229, query: original.query });
  };
  client._borrowExportedSender = async () => undefined;
  return client;
}

function messageEvents(update: any, envelope: any = {}): any[] {
  const next = {
    date: Number.isInteger(update?.date) ? update.date : envelope.date,
    seq: Number.isInteger(update?.seq) ? update.seq : envelope.seq,
  };
  if (Array.isArray(update?.updates)) return update.updates.flatMap((item: any) => messageEvents(item, next));
  if (Array.isArray(update)) return update.flatMap((item: any) => messageEvents(item, next));
  if (update?.update) return messageEvents(update.update, next);
  const message = typeof update?.message === 'string' ? update : update?.message;
  if (typeof message?.message !== 'string') return [];
  return [{
    text: message.message,
    id: Number(message.id || 0),
    pts: Number(update?.pts ?? message.pts ?? 0),
    update: rpcName(update),
  }];
}

function registerListener(row: ClientRow): void {
  row.client.addEventHandler((update: any) => {
    const observedAt = performance.now();
    for (const event of messageEvents(update)) {
      const marker = event.text;
      const bucket = received.get(marker);
      if (!bucket) continue;
      if (recoveryCohorts.get(marker)?.has(row.index)) {
        const unexpected = unexpectedRecoveryUpdates.get(marker) || new Set<number>();
        unexpected.add(row.index);
        unexpectedRecoveryUpdates.set(marker, unexpected);
        continue;
      }
      if (bucket.has(row.index)) {
        duplicates.set(marker, (duplicates.get(marker) || 0) + 1);
        continue;
      }
      const dispatchAt = (globalThis as any).__fanoutDispatchAt?.get(marker);
      bucket.set(row.index, {
        latencyMs: typeof dispatchAt === 'number' ? observedAt - dispatchAt : 0,
        id: event.id,
        pts: event.pts,
        update: event.update,
        observedAtMs: Date.now(),
      });
    }
  }, { build: (update: any) => update });
}

async function connectOwner(): Promise<any> {
  const authB64 = process.env.FANOUT_OWNER_AUTH_B64 || '';
  const userId = process.env.FANOUT_OWNER_USER_ID || '';
  check(Buffer.from(authB64, 'base64').length === 256 && /^\d+$/.test(userId), 'INVALID_OWNER_SESSION');
  const client = makeClient(authB64, 0);
  await bounded(client.connect());
  const me: any = await bounded(client.getMe());
  check(String(me?.id) === userId, 'OWNER_AUTH_KEY_USER_MISMATCH');
  return client;
}

async function createFixtures(): Promise<void> {
  const owner = await connectOwner();
  try {
    const fixtures: ChannelFixture[] = [];
    for (const kind of ['channel', 'megagroup'] as const) {
      const title = `fanout-${kind}-${tag}`;
      const result: any = await bounded(owner.invoke(new Api.channels.CreateChannel({
        title,
        about: `isolated reliability probe ${tag}`,
        broadcast: kind === 'channel',
        megagroup: kind === 'megagroup',
      })));
      const channel = (result?.chats || []).find((item: any) => item?.title === title);
      check(channel && String(channel.id) && String(channel.accessHash), `CHANNEL_CREATE_FAILED_${kind.toUpperCase()}`);
      fixtures.push({ kind, id: String(channel.id), accessHash: String(channel.accessHash), title });
    }
    report.channelFixtures = fixtures;
    report.channels = fixtures.map(({ kind, id, title }) => ({ kind, id, title }));
    report.ok = fixtures.length === 2;
  } finally {
    try { await bounded(Promise.resolve(owner.disconnect())); } catch {}
  }
}

async function connectOne(seed: Seed, index: number): Promise<ClientRow> {
  const row: ClientRow = { seed, index, client: makeClient(seed.authB64, index), activated: false };
  registerListener(row);
  try {
    await bounded(row.client.connect());
    const me: any = await bounded(row.client.getMe());
    check(String(me?.id) === seed.userId && row.client.isConnected(), 'AUTH_KEY_USER_MISMATCH');
    const state = await bounded(row.client.invoke(new Api.updates.GetState()));
    check(rpcName(state).toLowerCase().includes('state'), 'INVALID_UPDATE_STATE');
    await bounded(row.client.invoke(new Api.account.UpdateStatus({ offline: false })));
    row.activated = true;
    return row;
  } catch (error) {
    try { row.client.disconnect(); } catch {}
    throw error;
  }
}

async function connectAll(seeds: Seed[]): Promise<{ clients: ClientRow[]; failures: number }> {
  return connectIndexed(seeds.map((seed, index) => ({ seed, index })));
}

async function connectIndexed(items: Array<{ seed: Seed; index: number }>): Promise<{ clients: ClientRow[]; failures: number }> {
  const clients: ClientRow[] = [];
  let failures = 0;
  for (let offset = 0; offset < items.length; offset += batchSize) {
    const batch = items.slice(offset, offset + batchSize);
    let pending = batch;
    for (let attempt = 0; pending.length && attempt < 3; attempt += 1) {
      const results = await Promise.allSettled(pending.map((item) => connectOne(item.seed, item.index)));
      const retry: typeof pending = [];
      results.forEach((result, index) => {
        if (result.status === 'fulfilled') clients.push(result.value);
        else if (attempt < 2) retry.push(pending[index]);
        else failures += 1;
      });
      pending = retry;
      if (pending.length) await sleep(250);
    }
  }
  return { clients, failures };
}

async function queryActive(): Promise<number | null> {
  const port = Number(process.env.METRICS_PORT || '19090');
  try {
    const query = encodeURIComponent('sum(gnetway_conn_active{job="interface.gateway"})');
    const response = await fetch(`http://127.0.0.1:${port}/api/v1/query?query=${query}`);
    if (!response.ok) return null;
    const body: any = await response.json();
    const value = Number(body?.data?.result?.[0]?.value?.[1]);
    return Number.isFinite(value) ? value : null;
  } catch {
    return null;
  }
}

async function waitForGauge(expected: number, timeout: number): Promise<number | null> {
  const deadline = performance.now() + timeout;
  let latest: number | null = null;
  while (performance.now() < deadline) {
    latest = await queryActive();
    if (latest !== null && latest >= expected) return latest;
    await sleep(500);
  }
  return latest;
}

async function waitForGaugeAtMost(maximum: number, timeout: number): Promise<number | null> {
  const deadline = performance.now() + timeout;
  let latest: number | null = null;
  while (performance.now() < deadline) {
    latest = await queryActive();
    if (latest !== null && latest <= maximum) return latest;
    await sleep(500);
  }
  return latest;
}

function percentile(values: number[], fraction: number): number | null {
  if (!values.length) return null;
  const sorted = [...values].sort((left, right) => left - right);
  return sorted[Math.max(0, Math.ceil(fraction * sorted.length) - 1)];
}

function inputPeer(channel: ChannelFixture): any {
  return new Api.InputPeerChannel({ channelId: BigInt(channel.id), accessHash: BigInt(channel.accessHash) });
}

function differenceRequest(channel: ChannelFixture, pts: number): any {
  return new Api.updates.GetChannelDifference({
    force: true,
    channel: new Api.InputChannel({ channelId: BigInt(channel.id), accessHash: BigInt(channel.accessHash) }),
    filter: new Api.ChannelMessagesFilterEmpty(),
    pts,
    limit: 10,
  });
}

async function runDelivery(owner: any, clients: ClientRow[], channel: ChannelFixture, recoveryCount: number): Promise<any> {
  const marker = `${tag}:${channel.kind}:message`;
  const bucket = new Map<number, SeenEvent>();
  received.set(marker, bucket);
  duplicates.set(marker, 0);
  unexpectedRecoveryUpdates.set(marker, new Set<number>());
  const recoveryClients = clients.filter((row) => row.index < recoveryCount && row.activated);
  const liveRecipientsExpected = users - recoveryClients.length;
  const baselines = new Map<number, number>();
  for (let offset = 0; offset < recoveryClients.length; offset += batchSize) {
    const batch = recoveryClients.slice(offset, offset + batchSize);
    const states = await Promise.all(batch.map(async (row) => {
      const initial: any = await bounded(row.client.invoke(differenceRequest(channel, 0)));
      check(rpcName(initial) === 'updates.ChannelDifferenceEmpty' && initial.final === true,
        'RECOVERY_BASELINE_NOT_EMPTY');
      return [row.index, Number(initial.pts)] as const;
    }));
    for (const [index, pts] of states) baselines.set(index, pts);
  }
  recoveryCohorts.set(marker, new Set(recoveryClients.map((row) => row.index)));
  recoveryClients.forEach((row) => row.client.disconnect());
  check(recoveryClients.every((row) => !row.client.isConnected()), 'RECOVERY_CLIENT_DISCONNECT_FAILED');
  const ownerConnections = Number(owner.isConnected());
  const expectedGaugeMaximum = liveRecipientsExpected + ownerConnections;
  const disconnectedGauge = recoveryClients.length
    ? await waitForGaugeAtMost(expectedGaugeMaximum, Math.min(waitMs, 30000)) : await queryActive();
  report.recoveryDisconnectChecks ??= [];
  report.recoveryDisconnectChecks.push({
    kind: channel.kind,
    liveRecipientsExpected,
    ownerConnections,
    expectedGaugeMaximum,
    observedGauge: disconnectedGauge,
  });
  if (recoveryClients.length) {
    check(disconnectedGauge !== null && disconnectedGauge <= expectedGaugeMaximum,
      'RECOVERY_CLIENTS_REMAINED_ONLINE_AT_GATEWAY');
  }

  const dispatchTimes = new Map<string, number>();
  (globalThis as any).__fanoutDispatchAt = dispatchTimes;
  const started = performance.now();
  dispatchTimes.set(marker, started);
  const randomId = BigInt(`0x${randomBytes(8).toString('hex')}`);
  let ackMs: number | null = null;
  let ackError = '';
  let ackErrorType: string | undefined;
  let ackErrorCode: number | undefined;
  let ackRpcErrorMessage: string | undefined;
  try {
    const sent = await bounded(owner.invoke(new Api.messages.SendMessage({
      peer: inputPeer(channel),
      message: marker,
      randomId,
    })));
    check(rpcName(sent).toLowerCase().includes('updates'), 'CHANNEL_SEND_INVALID_RESULT');
  } catch (error) {
    ackError = errorCode(error);
    ackErrorType = safeErrorType(error);
    ackErrorCode = Number.isInteger((error as any)?.code) ? (error as any).code : undefined;
    ackRpcErrorMessage = rpcErrorMessage(error);
  } finally {
    ackMs = performance.now() - started;
  }
  // The disconnected recovery cohort is accounted for through channel difference.
  const deadline = performance.now() + waitMs;
  while (bucket.size < liveRecipientsExpected && performance.now() < deadline) await sleep(50);
  const latencies = [...bucket.values()].map((item) => item.latencyMs).filter((value) => value >= 0);
  const onTime = latencies.filter((value) => value <= 2000).length;
  let historyRead = false;
  const sample = clients.find((row) => row.index >= recoveryCount && row.activated);
  if (sample) {
    try {
      const history: any = await bounded(sample.client.invoke(new Api.messages.GetHistory({
        peer: inputPeer(channel), offsetId: 0, offsetDate: 0, addOffset: 0,
        limit: 20, maxId: 0, minId: 0, hash: 0n,
      })));
      const messages = history?.messages || history?.messages_MESSAGES || [];
      historyRead = Array.isArray(messages) && messages.some((item: any) => item?.message === marker);
    } catch {
      historyRead = false;
    }
  }
  const recoverySamples: Array<{ index: number; recoveryMs: number; pts: number }> = [];
  const recoveryErrors: string[] = [];
  for (let offset = 0; offset < recoveryClients.length; offset += batchSize) {
    const batch = recoveryClients.slice(offset, offset + batchSize);
    const outcomes: RecoveryOutcome[] = await Promise.all(batch.map(async (row): Promise<RecoveryOutcome> => {
      const recoveryStarted = performance.now();
      try {
        await bounded(row.client.connect());
        check(row.client.isConnected(), 'RECOVERY_CLIENT_RECONNECT_FAILED');
        await bounded(row.client.invoke(new Api.updates.GetState()));
        const baselinePts = baselines.get(row.index);
        check(baselinePts !== undefined, 'RECOVERY_BASELINE_MISSING');
        const difference: any = await bounded(row.client.invoke(differenceRequest(channel, baselinePts)));
        check(rpcName(difference) === 'updates.ChannelDifference' && difference.final === true,
          'RECOVERY_DIFFERENCE_NOT_FINAL');
        const texts = (difference.newMessages || []).map((message: any) => String(message.message));
        check(texts.length === 1 && texts[0] === marker, 'RECOVERY_MESSAGE_CONTENT_MISMATCH');
        const pts = Number(difference.pts);
        check(pts === baselinePts + 1, 'RECOVERY_PTS_NOT_CONTIGUOUS');
        return { ok: true, index: row.index, recoveryMs: performance.now() - recoveryStarted, pts };
      } catch (error) {
        return { ok: false, index: row.index, code: errorCode(error) };
      }
    }));
    for (const outcome of outcomes) {
      if (outcome.ok === true) recoverySamples.push(outcome);
      else recoveryErrors.push(outcome.code);
    }
  }
  const recoveryHistorySample = recoveryClients.find((row) => recoverySamples.some((item) => item.index === row.index));
  let recoveryHistoryRead = recoveryClients.length === 0;
  if (recoveryHistorySample) {
    try {
      const history: any = await bounded(recoveryHistorySample.client.invoke(new Api.messages.GetHistory({
        peer: inputPeer(channel), offsetId: 0, offsetDate: 0, addOffset: 0,
        limit: 20, maxId: 0, minId: 0, hash: 0n,
      })));
      const messages = history?.messages || history?.messages_MESSAGES || [];
      recoveryHistoryRead = Array.isArray(messages) && messages.some((item: any) => item?.message === marker);
    } catch {
      recoveryHistoryRead = false;
    }
  }
  const unexpectedRealtimeRecipients = unexpectedRecoveryUpdates.get(marker)?.size || 0;
  return {
    kind: channel.kind,
    channelId: channel.id,
    plannedRecipients: users,
    liveRecipientsExpected,
    liveRecipients: bucket.size,
    uniqueRecipients: bucket.size + recoverySamples.length,
    missingRecipients: users - bucket.size - recoverySamples.length,
    duplicateUpdates: duplicates.get(marker) || 0,
    ackMs,
    ackError: ackError || undefined,
    ackErrorType,
    ackErrorCode,
    ackRpcErrorMessage,
    ackWithin1s: ackMs !== null && ackMs <= 1000,
    receiveWithin2s: onTime,
    receiveLate: latencies.length - onTime,
    liveP50Ms: percentile(latencies, 0.50),
    liveP95Ms: percentile(latencies, 0.95),
    liveP99Ms: percentile(latencies, 0.99),
    liveMaxMs: latencies.length ? Math.max(...latencies) : null,
    channelHistoryReadByListener: historyRead,
    ptsValid: [...bucket.values()].every((item) => item.id > 0 && item.pts > 0),
    recovery: {
      requestedRecipients: recoveryClients.length,
      disconnectedRecipients: recoveryClients.length,
      reconnectedRecipients: recoveryClients.filter((row) => row.client.isConnected()).length,
      recoveredRecipients: recoverySamples.length,
      missingRecipients: recoveryClients.length - recoverySamples.length,
      duplicateRecoveredMessages: 0,
      unexpectedRealtimeRecipients,
      ptsContinuous: recoveryErrors.every((code) => code !== 'RECOVERY_PTS_NOT_CONTIGUOUS'),
      allFinal: recoveryErrors.length === 0,
      historyReadable: recoveryHistoryRead,
      recoveryP50Ms: percentile(recoverySamples.map((item) => item.recoveryMs), 0.50),
      recoveryP95Ms: percentile(recoverySamples.map((item) => item.recoveryMs), 0.95),
      recoveryP99Ms: percentile(recoverySamples.map((item) => item.recoveryMs), 0.99),
      recoveryMaxMs: recoverySamples.length ? Math.max(...recoverySamples.map((item) => item.recoveryMs)) : null,
      disconnectGauge: disconnectedGauge,
      errors: recoveryErrors.slice(0, 20),
    },
  };
}

async function runFanout(): Promise<void> {
  check(Number.isInteger(users) && users >= 1 && users <= 5000, 'INVALID_FANOUT_USERS');
  check(Number.isInteger(batchSize) && batchSize >= 1 && batchSize <= 200, 'INVALID_FANOUT_BATCH_SIZE');
  check(Number.isInteger(recoveryUsers) && recoveryUsers >= 0 && recoveryUsers < users, 'INVALID_FANOUT_RECOVERY_USERS');
  check(Number.isInteger(timeoutMs) && timeoutMs >= 1000 && timeoutMs <= 60000, 'INVALID_FANOUT_RPC_TIMEOUT');
  check(Number.isInteger(waitMs) && waitMs >= 5000 && waitMs <= 600000, 'INVALID_FANOUT_WAIT_TIMEOUT');
  const sessionFile = process.env.ONLINE_SESSIONS_FILE || '';
  const channelsFile = process.env.FANOUT_CHANNELS_FILE || '';
  check(sessionFile && fs.existsSync(sessionFile), 'MISSING_ONLINE_SESSION_FILE');
  check(channelsFile && fs.existsSync(channelsFile), 'MISSING_FANOUT_CHANNELS_FILE');
  const seeds: Seed[] = JSON.parse(fs.readFileSync(sessionFile, 'utf8'));
  const channels: ChannelFixture[] = JSON.parse(fs.readFileSync(channelsFile, 'utf8'));
  check(Array.isArray(seeds) && seeds.length === users, 'ONLINE_SESSION_COUNT_MISMATCH');
  check(channels.length === 2 && channels.some((item) => item.kind === 'channel')
    && channels.some((item) => item.kind === 'megagroup'), 'CHANNEL_FIXTURES_INVALID');
  report.channels = channels.map(({ kind, id }) => ({ kind, id }));
  report.recoveryPlan = { requestedPerTarget: recoveryUsers, mode: 'disconnect_and_channel_difference' };
  const owner = await connectOwner();
  const { clients, failures } = await connectAll(seeds);
  report.connections = {
    requested: users,
    authenticated: clients.length,
    activated: clients.filter((row) => row.activated).length,
    failures,
    connectedAtStart: clients.filter((row) => row.client.isConnected()).length,
    endpointDistribution: Object.fromEntries(endpoints.map((endpoint, replicaIndex) => [
      endpoint.replica, clients.filter((row) => row.index % endpoints.length === replicaIndex).length,
    ])),
  };
  report.serverGaugeAtStart = await waitForGauge(users, Math.min(waitMs, 90000));
  check(clients.length === users && clients.every((row) => row.activated && row.client.isConnected()),
    'NOT_ALL_CLIENTS_CONNECTED_BEFORE_FANOUT');
  report.deliveries = [];
  try {
    for (const channel of channels) {
      report.deliveries.push(await runDelivery(owner, clients, channel, recoveryUsers));
    }
    report.connections.connectedAtEnd = clients.filter((row) => row.client.isConnected()).length;
    report.serverGaugeAtEnd = await queryActive();
    report.ok = clients.length === users && clients.every((row) => row.activated)
      && report.deliveries.length === 2;
  } finally {
    for (const row of clients) {
      try { await bounded(Promise.resolve(row.client.disconnect())); } catch {}
    }
    try { await bounded(Promise.resolve(owner.disconnect())); } catch {}
    report.serverGaugeAfterDisconnect = await waitForGaugeAtMost(0, 15000);
    report.connectionCleanupObserved = report.serverGaugeAfterDisconnect === 0;
  }
}

type DiagnosticDispatch = Record<string, number>;

function writeJsonFile(filename: string, value: unknown): void {
  const temporary = `${filename}.tmp-${process.pid}`;
  fs.writeFileSync(temporary, `${JSON.stringify(value)}\n`, { mode: 0o600 });
  fs.renameSync(temporary, filename);
}

async function waitForFiles(filenames: string[], timeout: number): Promise<void> {
  const deadline = performance.now() + timeout;
  while (performance.now() < deadline) {
    if (filenames.every((filename) => fs.existsSync(filename))) return;
    await sleep(100);
  }
  check(false, 'MULTIPROCESS_DIAGNOSTIC_CHILD_TIMEOUT');
}

function diagnosticMarkers(channels: ChannelFixture[]): Map<string, ChannelFixture> {
  const markers = new Map<string, ChannelFixture>();
  for (const channel of channels) markers.set(`${tag}:${channel.kind}:message`, channel);
  return markers;
}

function diagnosticDelivery(marker: string, expected: number, dispatchAt: number | undefined): any {
  const bucket = received.get(marker) || new Map<number, SeenEvent>();
  const latencies = [...bucket.values()]
    .map((event) => typeof dispatchAt === 'number' ? event.observedAtMs - dispatchAt : event.latencyMs)
    .filter((value) => Number.isFinite(value) && value >= 0);
  const onTime = latencies.filter((value) => value <= 2000).length;
  return {
    marker,
    expectedRecipients: expected,
    uniqueRecipients: bucket.size,
    missingRecipients: expected - bucket.size,
    duplicateUpdates: duplicates.get(marker) || 0,
    receiveWithin2s: onTime,
    allWithin2s: bucket.size === expected && onTime === expected,
    liveP50Ms: percentile(latencies, 0.50),
    liveP95Ms: percentile(latencies, 0.95),
    liveP99Ms: percentile(latencies, 0.99),
    liveMaxMs: latencies.length ? Math.max(...latencies) : null,
  };
}

async function runListenerShard(): Promise<void> {
  const sessionFile = process.env.ONLINE_SESSIONS_FILE || '';
  const channelsFile = process.env.FANOUT_CHANNELS_FILE || '';
  const readyFile = process.env.FANOUT_SHARD_READY_FILE || '';
  const resultFile = process.env.FANOUT_SHARD_RESULT_FILE || '';
  const shardIndex = Number(process.env.FANOUT_SHARD_INDEX || '-1');
  const shardCount = Number(process.env.FANOUT_SHARD_COUNT || '0');
  const diagnosticWaitMs = Number(process.env.FANOUT_DIAGNOSTIC_WAIT_MS || String(waitMs));
  check(sessionFile && fs.existsSync(sessionFile), 'MISSING_ONLINE_SESSION_FILE');
  check(channelsFile && fs.existsSync(channelsFile), 'MISSING_FANOUT_CHANNELS_FILE');
  check(readyFile && resultFile && shardIndex >= 0 && shardIndex < shardCount && shardCount >= 2,
    'INVALID_MULTIPROCESS_SHARD');
  const seeds: Seed[] = JSON.parse(fs.readFileSync(sessionFile, 'utf8'));
  const channels: ChannelFixture[] = JSON.parse(fs.readFileSync(channelsFile, 'utf8'));
  check(Array.isArray(seeds) && seeds.length === users, 'ONLINE_SESSION_COUNT_MISMATCH');
  check(channels.length === 2, 'CHANNEL_FIXTURES_INVALID');
  const markers = diagnosticMarkers(channels);
  for (const marker of markers.keys()) {
    received.set(marker, new Map<number, SeenEvent>());
    duplicates.set(marker, 0);
  }
  const selected = seeds
    .map((seed, index) => ({ seed, index }))
    .filter((item) => item.index % shardCount === shardIndex);
  const { clients, failures } = await connectIndexed(selected);
  const ready = {
    shardIndex,
    shardCount,
    requested: selected.length,
    authenticated: clients.length,
    activated: clients.filter((row) => row.activated).length,
    failures,
    endpointDistribution: Object.fromEntries(endpoints.map((endpoint, endpointIndex) => [
      endpoint.replica, clients.filter((row) => row.index % endpoints.length === endpointIndex).length,
    ])),
  };
  writeJsonFile(readyFile, ready);
  let dispatch: DiagnosticDispatch = {};
  try {
    await sleep(diagnosticWaitMs);
    const dispatchFile = process.env.FANOUT_DISPATCH_FILE || '';
    if (dispatchFile && fs.existsSync(dispatchFile)) {
      dispatch = JSON.parse(fs.readFileSync(dispatchFile, 'utf8')) as DiagnosticDispatch;
    }
    const deliveries = [...markers.keys()].map((marker) => diagnosticDelivery(marker, selected.length, dispatch[marker]));
    const cpu = process.cpuUsage(runtimeCpuStarted);
    writeJsonFile(resultFile, {
      ...ready,
      deliveries,
      runtime: {
        memory: process.memoryUsage(),
        eventLoopDelayMs: {
          mean: eventLoopDelay.mean / 1e6,
          max: eventLoopDelay.max / 1e6,
          p95: eventLoopDelay.percentile(95) / 1e6,
          p99: eventLoopDelay.percentile(99) / 1e6,
        },
        cpuUsageMs: { user: cpu.user / 1000, system: cpu.system / 1000 },
      },
    });
  } finally {
    for (const row of clients) {
      try { await bounded(Promise.resolve(row.client.disconnect())); } catch {}
    }
  }
}

async function runMultiprocessDiagnostic(): Promise<void> {
  const processCount = Number(process.env.FANOUT_CLIENT_PROCESSES || '0');
  check(Number.isInteger(processCount) && processCount >= 2 && processCount <= 16,
    'INVALID_MULTIPROCESS_COUNT');
  const sessionFile = process.env.ONLINE_SESSIONS_FILE || '';
  const channelsFile = process.env.FANOUT_CHANNELS_FILE || '';
  const diagnosticDir = process.env.FANOUT_DIAGNOSTIC_DIR || '';
  check(sessionFile && fs.existsSync(sessionFile), 'MISSING_ONLINE_SESSION_FILE');
  check(channelsFile && fs.existsSync(channelsFile), 'MISSING_FANOUT_CHANNELS_FILE');
  check(diagnosticDir, 'MISSING_MULTIPROCESS_DIAGNOSTIC_DIR');
  fs.mkdirSync(diagnosticDir, { recursive: true, mode: 0o700 });
  const readyFiles = Array.from({ length: processCount }, (_, index) => path.join(diagnosticDir, `ready-${index}.json`));
  const resultFiles = Array.from({ length: processCount }, (_, index) => path.join(diagnosticDir, `result-${index}.json`));
  const dispatchFile = path.join(diagnosticDir, 'dispatch.json');
  for (const filename of [...readyFiles, ...resultFiles, dispatchFile]) fs.rmSync(filename, { force: true });

  const script = fileURLToPath(import.meta.url);
  const gramjsDir = process.env.TEAMGRAM_GRAMJS_DIR || '';
  const tsx = path.join(gramjsDir, 'node_modules/.bin/tsx');
  check(fs.existsSync(tsx), 'MISSING_GRAMJS_TSX');
  const children = readyFiles.map((readyFile, index) => {
    const childEnv = {
      ...process.env,
      FANOUT_PHASE: 'listen-shard',
      FANOUT_SHARD_INDEX: String(index),
      FANOUT_SHARD_COUNT: String(processCount),
      FANOUT_SHARD_READY_FILE: readyFile,
      FANOUT_SHARD_RESULT_FILE: resultFiles[index],
      FANOUT_DISPATCH_FILE: dispatchFile,
      FANOUT_DIAGNOSTIC_WAIT_MS: String(waitMs),
    };
    return spawn(tsx, [script], { env: childEnv, stdio: ['ignore', 'ignore', 'ignore'] });
  });
  const stopChildren = () => {
    for (const child of children) {
      if (!child.killed) child.kill('SIGTERM');
    }
  };
  let owner: any;
  try {
    await waitForFiles(readyFiles, Math.min(waitMs, 120000));
    const ready = readyFiles.map((filename) => JSON.parse(fs.readFileSync(filename, 'utf8')));
    check(ready.every((item) => item.authenticated === item.requested && item.activated === item.requested),
      'MULTIPROCESS_CLIENT_AUTHENTICATION_FAILED');
    owner = await connectOwner();
    const dispatch: DiagnosticDispatch = {};
    const channels: ChannelFixture[] = JSON.parse(fs.readFileSync(channelsFile, 'utf8'));
    const markers = diagnosticMarkers(channels);
    const acknowledgements: any[] = [];
    for (const [marker, channel] of markers) {
      dispatch[marker] = Date.now();
      writeJsonFile(dispatchFile, dispatch);
      const started = performance.now();
      let error: string | undefined;
      try {
        const sent = await bounded(owner.invoke(new Api.messages.SendMessage({
          peer: inputPeer(channel), message: marker, randomId: BigInt(`0x${randomBytes(8).toString('hex')}`),
        })));
        check(rpcName(sent).toLowerCase().includes('updates'), 'CHANNEL_SEND_INVALID_RESULT');
      } catch (sendError) {
        error = errorCode(sendError);
      }
      acknowledgements.push({ marker, ackMs: performance.now() - started, error });
    }
    writeJsonFile(dispatchFile, dispatch);
    await waitForFiles(resultFiles, waitMs + 30000);
    const shardResults = resultFiles.map((filename) => JSON.parse(fs.readFileSync(filename, 'utf8')));
    const deliveries = [...markers.keys()].map((marker) => {
      const shards = shardResults.map((item) => item.deliveries.find((delivery: any) => delivery.marker === marker));
      const uniqueRecipients = shards.reduce((sum, item) => sum + Number(item?.uniqueRecipients || 0), 0);
      const expectedRecipients = users;
      return {
        marker,
        expectedRecipients,
        uniqueRecipients,
        missingRecipients: expectedRecipients - uniqueRecipients,
        duplicateUpdates: shards.reduce((sum, item) => sum + Number(item?.duplicateUpdates || 0), 0),
        receiveWithin2s: shards.reduce((sum, item) => sum + Number(item?.receiveWithin2s || 0), 0),
        allWithin2s: uniqueRecipients === expectedRecipients && shards.every((item) => item?.allWithin2s === true),
        shardP95Ms: shards.map((item) => item?.liveP95Ms ?? null),
        shardP99Ms: shards.map((item) => item?.liveP99Ms ?? null),
      };
    });
    const connections = {
      requested: users,
      authenticated: ready.reduce((sum, item) => sum + item.authenticated, 0),
      activated: ready.reduce((sum, item) => sum + item.activated, 0),
      failures: ready.reduce((sum, item) => sum + item.failures, 0),
      processCount,
      endpointDistribution: Object.fromEntries(endpoints.map((endpoint) => [
        endpoint.replica, ready.reduce((sum, item) => sum + Number(item.endpointDistribution?.[endpoint.replica] || 0), 0),
      ])),
    };
    const complete = connections.authenticated === users && connections.activated === users
      && deliveries.every((item) => item.uniqueRecipients === users && item.missingRecipients === 0 && item.duplicateUpdates === 0);
    report.processMode = 'multi_process_listener_diagnostic';
    report.processCount = processCount;
    report.connections = connections;
    report.acknowledgements = acknowledgements;
    report.deliveries = deliveries;
    report.shards = shardResults.map((item) => ({ shardIndex: item.shardIndex, requested: item.requested, runtime: item.runtime }));
    report.status = complete && deliveries.every((item) => item.allWithin2s) ? 'PASS' : (complete ? 'BUDGET_EXCEEDED' : 'FAIL');
    report.ok = complete;
  } finally {
    try { if (owner) await bounded(Promise.resolve(owner.disconnect())); } catch {}
    stopChildren();
    await Promise.all(children.map((child) => new Promise<void>((resolve) => {
      if (child.exitCode !== null || child.signalCode !== null) return resolve();
      child.once('exit', () => resolve());
      setTimeout(() => resolve(), 5000);
    })));
  }
}

async function main(): Promise<void> {
  try {
    configure();
    check(endpoints.length >= 1 && endpoints.length <= 2
      && endpoints.every((endpoint) => typeof endpoint.host === 'string' && endpoint.host.length > 0
        && Number.isInteger(endpoint.port) && endpoint.port > 0 && endpoint.port < 65536
        && /^backend-[1-2]$/.test(endpoint.replica)), 'INVALID_FANOUT_ENDPOINTS');
    if (phase === 'setup') await createFixtures();
    else if (phase === 'fanout') await runFanout();
    else if (phase === 'multiprocess') await runMultiprocessDiagnostic();
    else if (phase === 'listen-shard') await runListenerShard();
    else check(false, 'INVALID_FANOUT_PHASE');
  } catch (error) {
    report.error = { code: errorCode(error) };
  } finally {
    eventLoopDelay.disable();
    const cpu = process.cpuUsage(runtimeCpuStarted);
    report.durationMs = performance.now() - runtimeStartedAt;
    report.runtime = {
      memory: process.memoryUsage(),
      eventLoopDelayMs: {
        mean: eventLoopDelay.mean / 1e6,
        max: eventLoopDelay.max / 1e6,
        p95: eventLoopDelay.percentile(95) / 1e6,
        p99: eventLoopDelay.percentile(99) / 1e6,
      },
      cpuUsageMs: { user: cpu.user / 1000, system: cpu.system / 1000 },
    };
    process.stdout.write(`${JSON.stringify(report)}\n`);
  }
}

main().then(() => process.exit(report.error ? 1 : 0)).catch(() => {
  process.stdout.write(`${JSON.stringify({ ...report, ok: false, error: { code: 'UNEXPECTED_PROBE_ERROR' } })}\n`);
  process.exit(1);
});
