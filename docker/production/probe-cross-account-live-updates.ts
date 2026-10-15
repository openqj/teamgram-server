import fs from 'node:fs';
import path from 'node:path';
import { createRequire } from 'node:module';
import { randomBytes } from 'node:crypto';
import { monitorEventLoopDelay, performance } from 'node:perf_hooks';

// Local Layer 229 acceptance: SLO_SAMPLES counts sequential sends in EACH
// direction. Fresh clients reuse the auth keys; they do not register new keys.
// before/after/cleanup can be placed around an external backend restart barrier.
// fault uses ready/stopped/sent/restored milestones in PROBE_CONTROL_DIR.
const phase = process.env.PROBE_PHASE || 'single';
const tag = process.env.PROBE_TAG || `live-update-${Date.now()}-${randomBytes(4).toString('hex')}`;
const sampleCount = Number(process.env.SLO_SAMPLES || '1');
const timeoutMs = Number(process.env.LIVE_UPDATE_TIMEOUT_MS || '8000');
const dcId = Number(process.env.TG_DC_ID || '2');
const dcHost = process.env.TG_DC_HOST || '127.0.0.1';
const dcPort = Number(process.env.TG_DC_PORT || '11443');
const actorPorts = ['A', 'B'].map((name) => Number(process.env[`${name}_TG_DC_PORT`] || dcPort));
const stateFile = process.env.PROBE_STATE_FILE || `/tmp/teamgram-${tag}.json`;
const controlDir = process.env.PROBE_CONTROL_DIR || '';
const loadRate = Number(process.env.LOAD_RATE || '10');
const loadSeconds = Number(process.env.LOAD_SECONDS || '60');
const loadInflight = Number(process.env.LOAD_MAX_INFLIGHT || '32');
const loadLagMs = Number(process.env.LOAD_MAX_LAG_MS || '100');
const sloConnectionHoldMs = Number(process.env.SLO_CONNECTION_HOLD_MS || '10000');
const measureSlo = process.env.PROBE_SLO_METRICS === '1';
const apiLayer = 229;
const sendMessageConstructorId = 0xfef48f62;
let Api: any;
let TelegramClient: any;
let CallbackSession: any;
let Connection: any;
let stage = 'configuration';
const eventLoopDelay = monitorEventLoopDelay({ resolution: 20 });
eventLoopDelay.enable();
const runtimeStartedAt = performance.now();
let eventLoopUtilizationBase = performance.eventLoopUtilization();
const runtimeSamples: any[] = [];
const runtimeCpuStarted = process.cpuUsage();
const runtimeSampleTimer = setInterval(() => {
  const utilization = performance.eventLoopUtilization(eventLoopUtilizationBase);
  eventLoopUtilizationBase = performance.eventLoopUtilization();
  runtimeSamples.push({
    elapsedMs: performance.now() - runtimeStartedAt,
    stage,
    eventLoopUtilization: utilization.utilization,
    eventLoopDelayP95Ms: eventLoopDelay.percentile(95) / 1e6,
    eventLoopDelayMaxMs: eventLoopDelay.max / 1e6,
  });
  eventLoopDelay.reset();
}, 1000);
runtimeSampleTimer.unref();
type State = { pts: number; qts: number; seq: number; date: number };
type Actor = { name: 'A' | 'B'; id: bigint; hash: bigint; client: any; events: any[] };
type MessageSpec = { sender: 'A' | 'B'; text: string; randomId: string; ids?: Record<string, number> };
const integrity = { eligible: new Set<string>(), observed: new Set<string>(), lost: new Set<string>(), duplicates: new Set<string>() };
let stopConnectionObserver: (() => void) | undefined;
const report: any = {
  schemaVersion: 1, probe: 'messaging.cross-account-live-updates',
  evidenceClass: 'local_probe', productionSloEvidence: false, phase, tag,
  externalBarrierVerificationRequired: ['before', 'after', 'fault'].includes(phase),
  clock: 'performance.now', samplesPerDirection: phase === 'fault' ? 1 : sampleCount, ok: false,
  rpcSamples: [], deliverySamples: [], replaySamples: [], checks: {},
};
const quietLogger = { debug() {}, info() {}, warn() {}, error() {} };
const sleep = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms));

function check(condition: unknown, code: string): asserts condition {
  if (!condition) throw Object.assign(new Error(code), { probeCode: code });
}

function errorCode(error: any): string {
  if (typeof error?.probeCode === 'string') return error.probeCode;
  // SDK messages can contain requests or credentials. Emit only canonical RPC
  // identifiers and our own fixed codes, never stack traces or arbitrary text.
  return /^[A-Z][A-Z0-9_]{0,79}$/.test(error?.errorMessage || '')
    ? error.errorMessage : 'RPC_OR_TRANSPORT_ERROR';
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

function rpcName(value: any): string {
  return value?.className || value?.constructor?.name || typeof value;
}

function stateFields(value: any): State {
  const state = { pts: Number(value?.pts), qts: Number(value?.qts), seq: Number(value?.seq), date: Number(value?.date) };
  check(Object.values(state).every(Number.isInteger) && state.pts >= 0
    && state.qts >= 0 && state.seq >= -1 && state.date > 0, 'INVALID_UPDATE_STATE');
  return state;
}

function assertMonotonic(before: State, after: State): void {
  check(after.pts >= before.pts && after.qts === before.qts
    && after.seq >= before.seq && after.date >= before.date, 'UPDATE_STATE_REGRESSED');
}

function updateMessages(update: any, envelope: any = {}): any[] {
  const next = {
    date: Number.isInteger(update?.date) ? update.date : envelope.date,
    seq: Number.isInteger(update?.seq) ? update.seq : envelope.seq,
  };
  if (Array.isArray(update?.updates)) return update.updates.flatMap((item: any) => updateMessages(item, next));
  if (update?.update) return updateMessages(update.update, next);
  const message = typeof update?.message === 'string' ? update : update?.message;
  if (typeof message?.message !== 'string') return [];
  return [{
    text: message.message, id: Number(message.id || 0),
    pts: Number(update.pts ?? message.pts), ptsCount: Number(update.ptsCount ?? message.ptsCount),
    date: Number(message.date ?? next.date), seq: next.seq,
  }];
}

function peer(actor: Actor): any {
  return new Api.InputPeerUser({ userId: actor.id, accessHash: actor.hash });
}

async function disconnect(actor?: Actor): Promise<void> {
  if (actor) { try { await bounded(actor.client.disconnect()); } catch {} }
}

async function connectActor(name: 'A' | 'B'): Promise<Actor> {
  const actorHost = process.env[`${name}_TG_DC_HOST`] || dcHost;
  const actorPort = Number(process.env[`${name}_TG_DC_PORT`] || dcPort);
  const session = new CallbackSession({
    mainDcId: dcId, keys: { [dcId]: Buffer.from(process.env[`${name}_AUTH_B64`]!, 'base64').toString('hex') },
  }, () => {});
  const load = session.load.bind(session);
  session.load = async () => { await load(); session._serverAddress = actorHost; session._port = actorPort; };
  const client = new TelegramClient(session, Number(process.env.TG_API_ID || '1'),
    process.env.TG_API_HASH || '0123456789abcdef0123456789abcdef', {
      connection: Connection, fallbackConnection: Connection,
      shouldAllowHttpTransport: false, shouldForceHttpTransport: false,
      timeout: 20, requestRetries: 1, connectionRetries: 1, connectionRetriesToFallback: 0,
      retryDelay: 250, autoReconnect: false, sequentialUpdates: true, baseLogger: quietLogger,
    });
  const initWith = client._initWith.bind(client);
  client._initWith = (query: any) => {
    const original = initWith(query);
    check(rpcName(original) === 'InvokeWithLayer' && rpcName(original.query) === 'InitConnection'
      && original.query.query === query, 'INVALID_INIT_CONNECTION_WRAPPER');
    const initialized = new Api.InvokeWithLayer({ layer: apiLayer, query: original.query });
    report.apiLayer = initialized.layer;
    return initialized;
  };
  // Keep the primary update loop; media sender creation is unrelated to this probe.
  client._borrowExportedSender = async () => undefined;
  const actor: Actor = {
    name, id: BigInt(process.env[`${name}_USER_ID`]!), hash: BigInt(process.env[`${name}_ACCESS_HASH`]!), client, events: [],
  };
  client.addEventHandler((update: any) => {
    const receivedAtMs = performance.now();
    for (const event of updateMessages(update)) actor.events.push({ ...event, receivedAtMs, update: rpcName(update) });
  }, { build: (update: any) => update });
  try {
    await bounded(client.connect());
    const me: any = await bounded(client.getMe());
    check(String(me?.id) === actor.id.toString(), 'AUTH_KEY_USER_MISMATCH');
    return actor;
  } catch (error) {
    await disconnect(actor);
    throw error;
  }
}

async function invoke(actor: Actor, request: any, purpose: string): Promise<any> {
  if (rpcName(request) === 'messages.SendMessage') {
    check(Number.isInteger(request.CONSTRUCTOR_ID) && (request.CONSTRUCTOR_ID >>> 0) === sendMessageConstructorId,
      'LAYER229_SEND_CONSTRUCTOR_MISMATCH');
  }
  const started = performance.now();
  const sample: any = { actor: actor.name, method: rpcName(request), purpose, ok: false };
  report.rpcSamples.push(sample);
  try {
    const result = await bounded(actor.client.invoke(request));
    sample.ok = true;
    return result;
  } catch (error) {
    sample.error = errorCode(error);
    throw error;
  } finally {
    sample.elapsedMs = performance.now() - started;
  }
}

async function states(a: Actor, b: Actor): Promise<Record<string, State>> {
  const result: Record<string, State> = {};
  for (const actor of [a, b]) {
    const value = await invoke(actor, new Api.updates.GetState(), 'state');
    check(rpcName(value) === 'updates.State', 'INVALID_STATE_CONSTRUCTOR');
    result[actor.name] = stateFields(value);
  }
  return result;
}

async function activate(a: Actor, b: Actor): Promise<Record<string, State>> {
  // getState and updateStatus register a generic updates recipient session.
  await states(a, b);
  for (const actor of [a, b]) await invoke(actor, new Api.account.UpdateStatus({ offline: false }), 'online');
  return states(a, b);
}

async function readTagged(actor: Actor, other: Actor): Promise<any[]> {
  const tagged: any[] = [];
  let offsetId = 0;
  // Pagination also makes cleanup correct when SLO_SAMPLES exceeds one page.
  const pageLimit = phase === 'capacity' ? Math.ceil(loadRate * loadSeconds / 50) + 2 : 20;
  for (let page = 0; page < pageLimit; page += 1) {
    const value = await invoke(actor, new Api.messages.GetHistory({
      peer: peer(other), offsetId, offsetDate: 0, addOffset: 0, limit: 50, maxId: 0, minId: 0, hash: 0n,
    }), 'history');
    check(Array.isArray(value?.messages), 'INVALID_HISTORY_CONSTRUCTOR');
    tagged.push(...value.messages.filter((message: any) => typeof message?.message === 'string' && message.message.startsWith(`${tag}:`)));
    if (value.messages.length < 50) return tagged;
    const next = Number(value.messages[value.messages.length - 1]?.id);
    check(next > 0 && (!offsetId || next < offsetId), 'HISTORY_PAGINATION_STALLED');
    offsetId = next;
  }
  throw Object.assign(new Error('HISTORY_PAGE_LIMIT'), { probeCode: 'HISTORY_PAGE_LIMIT' });
}

async function verifyHistory(a: Actor, b: Actor, messages: MessageSpec[]): Promise<void> {
  let historyValid = true;
  for (const [actor, other] of [[a, b], [b, a]]) {
    const history = await readTagged(actor, other);
    if (measureSlo && ['capacity', 'slo'].includes(phase)) {
      for (const expected of messages.filter((message) => message.sender !== actor.name)) {
        const found = history.filter((message: any) => message.message === expected.text);
        if (actor.events.filter((value) => value.text === expected.text).length > 1) {
          integrity.duplicates.add(expected.text);
        }
        integrity.eligible.add(expected.text);
        integrity.observed.add(expected.text);
        if (!found.length) integrity.lost.add(expected.text);
        else integrity.lost.delete(expected.text);
        if (found.length > 1) integrity.duplicates.add(expected.text);
      }
      report.messageIntegrity = {
        eligible: integrity.eligible.size,
        lost: integrity.lost.size,
        duplicates: integrity.duplicates.size,
        complete: integrity.eligible.size > 0 && integrity.observed.size === integrity.eligible.size,
      };
    }
    if (history.length !== messages.length) {
      historyValid = false;
      if (phase !== 'slo') check(false, 'HISTORY_MESSAGE_COUNT_MISMATCH');
    }
    for (const expected of messages) {
      const found = history.filter((message: any) => message.message === expected.text);
      if (found.length !== 1 || Number(found[0]?.id) <= 0) {
        historyValid = false;
        if (phase !== 'slo') check(false, 'HISTORY_MISSING_OR_DUPLICATE');
        continue;
      }
      const id = Number(found[0].id);
      check(!expected.ids?.[actor.name] || expected.ids[actor.name] === id, 'HISTORY_MESSAGE_ID_CHANGED');
      const live = report.deliverySamples.find((sample: any) => sample.text === expected.text);
      if (actor.name !== expected.sender && live?.ok) check(live.messageId === id, 'LIVE_UPDATE_HISTORY_ID_MISMATCH');
      expected.ids = { ...expected.ids, [actor.name]: id };
    }
  }
  report.checks.historiesUnique = historyValid;
}

function observeConnections(a: Actor, b: Actor): () => void {
  let eligible = 0;
  let good = 0;
  let samples = 0;
  const sample = () => {
    samples += 1;
    eligible += 2;
    good += Number(a.client.isConnected()) + Number(b.client.isConnected());
  };
  sample();
  const timer = setInterval(sample, 1000);
  let stopped = false;
  return () => {
    if (stopped) return;
    stopped = true;
    clearInterval(timer);
    sample();
    report.connectionObservation = { eligible, good, samples, intervalMs: 1000 };
  };
}

async function reconnectSloSample(actor: Actor): Promise<{ eligible: number; good: number; elapsedMs: number }> {
  const started = performance.now();
  try {
    await bounded(Promise.resolve(actor.client.disconnect()));
    await bounded(actor.client.connect());
    const me: any = await bounded(actor.client.getMe());
    check(String(me?.id) === actor.id.toString() && actor.client.isConnected(), 'RECONNECT_AUTH_FAILED');
    return { eligible: 1, good: 1, elapsedMs: performance.now() - started };
  } catch {
    return { eligible: 1, good: 0, elapsedMs: performance.now() - started };
  }
}

async function sendMessage(sender: Actor, recipient: Actor, spec: MessageSpec, fresh = false): Promise<void> {
  const before = await states(sender, recipient);
  const started = performance.now();
  const sample: any = { direction: `${sender.name}-to-${recipient.name}`, text: spec.text, freshSession: fresh, ok: false };
  report.deliverySamples.push(sample);
  try {
    const response = await invoke(sender, new Api.messages.SendMessage({
      peer: peer(recipient), message: spec.text, randomId: BigInt(spec.randomId),
    }), 'send');
    sample.rpcMs = performance.now() - started;
    check(rpcName(response).toLowerCase().includes('update'), 'INVALID_SEND_CONSTRUCTOR');
    const deadline = performance.now() + timeoutMs;
    let event: any;
    while (!(event = recipient.events.find((value) => value.text === spec.text))) {
      check(performance.now() < deadline, 'LIVE_UPDATE_TIMEOUT');
      await sleep(10);
    }
    sample.deliveryMs = event.receivedAtMs - started;
    sample.messageId = event.id;
    sample.pts = event.pts;
    sample.ptsCount = event.ptsCount;
    sample.date = event.date;
    sample.seq = event.seq;
    const after = await states(sender, recipient);
    const previous = before[recipient.name];
    check(Number.isInteger(event.ptsCount) && event.ptsCount > 0 && event.id > 0
      && event.pts === previous.pts + event.ptsCount && after[recipient.name].pts === event.pts, 'PTS_CONTINUITY_GAP');
    check(after[sender.name].pts === before[sender.name].pts + 1, 'SENDER_PTS_CONTINUITY_GAP');
    assertMonotonic(before[sender.name], after[sender.name]);
    assertMonotonic(previous, after[recipient.name]);
    check(event.date >= previous.date, 'UPDATE_DATE_REGRESSED');
    if (Number.isInteger(event.seq) && event.seq > 0) {
      check(event.seq > previous.seq && event.seq <= after[recipient.name].seq, 'UPDATE_SEQ_OUT_OF_STATE');
    }
    sample.before = before;
    sample.after = after;
    sample.contiguous = true;
    sample.ok = true;
  } catch (error) {
    sample.error = errorCode(error);
    throw error;
  } finally {
    sample.elapsedMs = performance.now() - started;
  }
}

async function capacityMessages(a: Actor, b: Actor, messages: MessageSpec[]): Promise<void> {
  const before = await states(a, b);
  const durationMs = loadSeconds * 1000;
  const samples: any[] = Array.from({ length: loadRate * loadSeconds }, (_, index) => ({
    slot: index, direction: index % 2 ? 'B-to-A' : 'A-to-B', scheduledAtMs: index * 1000 / loadRate,
  }));
  const evidence: any = { rate: loadRate, durationMs, maxInflight: loadInflight, maxScheduleLagMs: loadLagMs,
    planned: samples.length, samples, before, after: {}, startedAtMs: performance.now() };
  report.capacity = evidence;
  const started = evidence.startedAtMs;
  const active = new Set<Promise<void>>();
  const send = async (row: any, sender: Actor, recipient: Actor, spec: MessageSpec) => {
    try {
      const response = await invoke(sender, new Api.messages.SendMessage({
        peer: peer(recipient), message: spec.text, randomId: BigInt(spec.randomId),
      }), 'capacity-send');
      row.ackAtMs = performance.now() - started;
      const constructor = rpcName(response);
      check(['Updates', 'UpdatesCombined', 'UpdateShortSentMessage'].includes(constructor), 'INVALID_SEND_CONSTRUCTOR');
      const events = updateMessages(response).filter((value) => value.text === spec.text);
      const ack = constructor === 'UpdateShortSentMessage'
        ? { id: Number(response.id), pts: Number(response.pts), ptsCount: Number(response.ptsCount) }
        : events.length === 1 ? events[0] : undefined;
      check(ack?.id > 0 && Number.isInteger(ack.pts) && ack.ptsCount === 1, 'CAPACITY_INVALID_ACK');
      row.ack = { id: ack.id, pts: ack.pts, ptsCount: ack.ptsCount };
      spec.ids = { [sender.name]: ack.id };
    } catch (error) {
      row.ackError = errorCode(error);
    }
    const deadline = started + row.dispatchedAtMs + timeoutMs;
    let events: any[];
    while (!(events = recipient.events.filter((event) => event.text === spec.text)).length && performance.now() < deadline) {
      await sleep(10);
    }
    if (events.length === 1) {
      const event = events[0];
      row.liveAtMs = event.receivedAtMs - started;
      row.live = { id: event.id, pts: event.pts, ptsCount: event.ptsCount, date: event.date };
    } else {
      row.liveError = events.length ? 'DUPLICATE_LIVE_UPDATE' : 'LIVE_UPDATE_TIMEOUT';
      if (measureSlo && events.length > 1) integrity.duplicates.add(spec.text);
    }
  };
  for (const row of samples) {
    const delay = started + row.scheduledAtMs - performance.now();
    if (delay > 0) await sleep(delay);
    row.observedScheduleLagMs = Math.max(0, performance.now() - started - row.scheduledAtMs);
    if (row.observedScheduleLagMs > loadLagMs || active.size >= loadInflight) {
      row.dropped = row.observedScheduleLagMs > loadLagMs ? 'GENERATOR_LATE' : 'MAX_INFLIGHT';
      continue;
    }
    const [sender, recipient] = row.slot % 2 ? [b, a] : [a, b];
    const spec: MessageSpec = { sender: sender.name, text: `${tag}:${sender.name}:load${row.slot}`,
      randomId: (BigInt(`0x${randomBytes(8).toString('hex')}`) & ((1n << 63n) - 1n)).toString() };
    messages.push(spec);
    Object.assign(row, { text: spec.text, randomId: spec.randomId, dispatchedAtMs: performance.now() - started });
    const task = send(row, sender, recipient, spec);
    active.add(task);
    void task.finally(() => active.delete(task));
  }
  const remaining = started + durationMs - performance.now();
  if (remaining > 0) await sleep(remaining);
  await Promise.all(active);
  report.loadDurationMs = performance.now() - started;
}

async function verifyCapacityAfterDrain(a: Actor, b: Actor, messages: MessageSpec[], barrierAt: number): Promise<void> {
  const evidence = report.capacity;
  const samples = evidence.samples.filter((row: any) => row.dispatchedAtMs !== undefined);
  const timedOut = samples.filter((row: any) => row.liveError === 'LIVE_UPDATE_TIMEOUT');
  const observationMs = 10000;
  const deadline = performance.now() + observationMs;
  const missing = () => timedOut.filter((row: any) => {
    const recipient = row.direction.endsWith('A') ? a : b;
    return !recipient.events.some((event) => event.text === row.text);
  });
  while (missing().length > 0 && performance.now() < deadline) await sleep(20);

  for (const row of samples) {
    const recipient = row.direction.endsWith('A') ? a : b;
    const events = recipient.events.filter((event) => event.text === row.text);
    if (events.length !== 1) {
      if (events.length > 1) row.liveError = 'DUPLICATE_LIVE_UPDATE';
      else if (row.liveError === 'LIVE_UPDATE_TIMEOUT') row.lateUnobserved = true;
      continue;
    }
    const event = events[0];
    if (row.liveError === 'LIVE_UPDATE_TIMEOUT') {
      row.liveDeadlineError = row.liveError;
      delete row.liveError;
      row.liveLate = true;
      row.lateObservedAfterDrainMs = Math.max(0, event.receivedAtMs - barrierAt);
    }
    row.liveAtMs = event.receivedAtMs - evidence.startedAtMs;
    row.live = { id: event.id, pts: event.pts, ptsCount: event.ptsCount, date: event.date };
  }
  evidence.lateDelivery = {
    observationMs,
    deadlineMissed: timedOut.length,
    observedLate: samples.filter((row: any) => row.liveLate).length,
    observedAfterDrain: samples.filter((row: any) => row.liveLate && row.lateObservedAfterDrainMs > 0).length,
    unobservedAfterWindow: samples.filter((row: any) => row.lateUnobserved).length,
  };

  evidence.after = await states(a, b);
  let ptsContinuity = true;
  for (const actor of [a, b]) {
    try {
      assertMonotonic(evidence.before[actor.name], evidence.after[actor.name]);
    } catch {
      ptsContinuity = false;
    }
    const intervals = samples.map((row: any) =>
      row.direction.startsWith(actor.name) ? row.ack : row.live).sort((x: any, y: any) => (x?.pts || 0) - (y?.pts || 0));
    let pts = evidence.before[actor.name].pts;
    for (const interval of intervals) {
      if (!(interval?.id > 0 && interval.ptsCount === 1 && interval.pts === pts + 1)) {
        ptsContinuity = false;
        continue;
      }
      pts = interval.pts;
    }
    if (evidence.after[actor.name].pts !== pts) ptsContinuity = false;
  }
  report.checks.ptsContinuity = ptsContinuity;
  let historiesUnique = true;
  try {
    await verifyHistory(a, b, messages);
  } catch {
    historiesUnique = false;
  }
  report.checks.historiesUnique = historiesUnique;
  let liveMatchesHistory = true;
  for (const row of samples) {
    const recipient = row.direction.endsWith('A') ? a : b;
    const spec = messages.find((value) => value.text === row.text)!;
    if (recipient.events.filter((value) => value.text === row.text).length !== 1
      || row.live?.id !== spec.ids?.[recipient.name]) liveMatchesHistory = false;
  }
  report.checks.liveMatchesHistory = liveMatchesHistory;
  if (!ptsContinuity) throw Object.assign(new Error('CAPACITY_PTS_GAP_OR_DUPLICATE'), { probeCode: 'CAPACITY_PTS_GAP_OR_DUPLICATE' });
  if (!historiesUnique) throw Object.assign(new Error('CAPACITY_HISTORY_MISMATCH'), { probeCode: 'CAPACITY_HISTORY_MISMATCH' });
  if (!liveMatchesHistory) throw Object.assign(new Error('CAPACITY_LIVE_HISTORY_MISMATCH'), { probeCode: 'CAPACITY_LIVE_HISTORY_MISMATCH' });
}

function writeMilestone(name: 'ready' | 'sent', payload: any): void {
  const target = path.join(controlDir, `${name}.json`);
  const temporary = `${target}.${process.pid}.tmp`;
  fs.writeFileSync(temporary, `${JSON.stringify({ schemaVersion: 1, tag, stage: name, ...payload })}\n`, { mode: 0o600 });
  fs.renameSync(temporary, target);
}

async function waitMilestone(name: 'stopped' | 'restored'): Promise<void> {
  const target = path.join(controlDir, `${name}.json`);
  const deadline = performance.now() + 120000;
  while (!fs.existsSync(target)) {
    check(performance.now() < deadline, `FAULT_${name.toUpperCase()}_BARRIER_TIMEOUT`);
    await sleep(50);
  }
  const payload = JSON.parse(fs.readFileSync(target, 'utf8'));
  check(payload?.schemaVersion === 1 && payload.tag === tag && payload.stage === name, 'INVALID_FAULT_MILESTONE');
}

async function faultMessages(a: Actor, b: Actor, messages: MessageSpec[]): Promise<void> {
  fs.mkdirSync(controlDir, { recursive: true, mode: 0o700 });
  for (const name of ['ready', 'stopped', 'sent', 'restored']) {
    check(!fs.existsSync(path.join(controlDir, `${name}.json`)), 'FAULT_CONTROL_DIR_NOT_EMPTY');
  }
  const before = await states(a, b);
  const sessions = [a, b].map((actor) => ({
    actor, sender: actor.client._sender, id: actor.client._sender?._state?.id,
  }));
  check(sessions.every((session) => typeof session.id === 'bigint'), 'FAULT_SESSION_ID_UNAVAILABLE');
  report.state.before = before;
  for (const sender of [a, b]) messages.push({
    sender: sender.name, text: `${tag}:${sender.name}:fault`,
    randomId: (BigInt(`0x${randomBytes(8).toString('hex')}`) & ((1n << 63n) - 1n)).toString(),
  });
  const fault: any = { preStates: before, messages: [...messages], acknowledgements: [], sameSessions: false };
  report.fault = fault;
  stage = 'fault-ready';
  writeMilestone('ready', { preStates: before, messages });
  await waitMilestone('stopped');
  const rpcBoundary = report.rpcSamples.length;
  const deliveries: { recipient: Actor; spec: MessageSpec; started: number; sample: any }[] = [];
  stage = 'fault-durable-send';
  for (const [sender, recipient] of [[a, b], [b, a]]) {
    const spec = messages.find((value) => value.sender === sender.name)!;
    const started = performance.now();
    const sample: any = {
      direction: `${sender.name}-to-${recipient.name}`, text: spec.text,
      freshSession: false, fault: true, ok: false,
    };
    report.deliverySamples.push(sample);
    deliveries.push({ recipient, spec, started, sample });
    const response = await invoke(sender, new Api.messages.SendMessage({
      peer: peer(recipient), message: spec.text, randomId: BigInt(spec.randomId),
    }), 'send');
    sample.rpcMs = performance.now() - started;
    const constructor = rpcName(response);
    check(['Updates', 'UpdatesCombined', 'UpdateShortSentMessage'].includes(constructor), 'INVALID_SEND_CONSTRUCTOR');
    const ackMessages = updateMessages(response).filter((value) => value.text === spec.text);
    const ack = constructor === 'UpdateShortSentMessage'
      ? { id: Number(response.id), pts: Number(response.pts), ptsCount: Number(response.ptsCount), date: Number(response.date) }
      : ackMessages.length === 1 ? ackMessages[0] : undefined;
    check(ack && ack.id > 0 && ack.ptsCount === 1 && ack.pts === before[sender.name].pts + 1
      && ack.date >= before[sender.name].date, 'FAULT_DURABLE_ACK_STATE_MISMATCH');
    spec.ids = { [sender.name]: ack.id };
    fault.acknowledgements.push({
      sender: sender.name, recipient: recipient.name, text: spec.text, constructor,
      rpcMs: sample.rpcMs, messageId: ack.id, pts: ack.pts, ptsCount: ack.ptsCount, date: ack.date,
    });
  }
  // Only send RPCs are allowed during the stopped-broker window. State,
  // difference or history requests could repair the gap and hide failed MQ
  // delivery. The same connected recipient sessions must receive both events.
  await sleep(1000);
  for (const { recipient, spec } of deliveries) {
    check(!recipient.events.some((value) => value.text === spec.text), 'LIVE_UPDATE_WHILE_BROKER_STOPPED');
  }
  fault.durableAckWhileStopped = true;
  fault.noLiveUpdateWhileStopped = true;
  stage = 'fault-durable-ack';
  writeMilestone('sent', { preStates: before, messages, acknowledgements: fault.acknowledgements });
  await waitMilestone('restored');
  const restoredAt = performance.now();
  stage = 'fault-live-recovery';
  const deadline = restoredAt + 120000;
  while (deliveries.some(({ recipient, spec }) => !recipient.events.some((value) => value.text === spec.text))) {
    check(performance.now() < deadline, 'FAULT_LIVE_RECOVERY_TIMEOUT');
    await sleep(10);
  }
  check(report.rpcSamples.slice(rpcBoundary).every((value: any) => value.purpose === 'send'), 'FAULT_RECOVERY_READ_BEFORE_LIVE_UPDATE');
  check(sessions.every(({ actor, sender, id }) => actor.client._sender === sender
    && sender._state.id === id && actor.client.isConnected()), 'FAULT_RECIPIENT_SESSION_CHANGED');
  fault.sameSessions = true;
  fault.noRecoveryReadBeforeLiveUpdate = true;
  fault.liveRecoveryBeforeStateRead = true;
  const after = await states(a, b);
  fault.afterStates = after;
  for (const actor of [a, b]) {
    assertMonotonic(before[actor.name], after[actor.name]);
    // Each actor has one sender commit and one recipient commit.
    check(after[actor.name].pts === before[actor.name].pts + 2, 'FAULT_FINAL_PTS_MISMATCH');
  }
  for (const { recipient, spec, started, sample } of deliveries) {
    const events = recipient.events.filter((value) => value.text === spec.text);
    check(events.length === 1, 'FAULT_DUPLICATE_LIVE_UPDATE');
    const event = events[0];
    check(event.id > 0 && event.ptsCount === 1 && event.pts === before[recipient.name].pts + 2, 'FAULT_RECIPIENT_PTS_MISMATCH');
    check(event.date >= before[recipient.name].date, 'UPDATE_DATE_REGRESSED');
    if (Number.isInteger(event.seq) && event.seq > 0) {
      check(event.seq > before[recipient.name].seq && event.seq <= after[recipient.name].seq, 'UPDATE_SEQ_OUT_OF_STATE');
    }
    Object.assign(sample, {
      deliveryMs: event.receivedAtMs - started,
      recoveryAfterBarrierMs: Math.max(0, event.receivedAtMs - restoredAt),
      messageId: event.id, pts: event.pts, ptsCount: event.ptsCount,
      date: event.date, seq: event.seq, before, after, contiguous: true, ok: true,
      elapsedMs: performance.now() - started,
    });
  }
  report.state.after = after;
  report.checks.brokerFaultRecovery = true;
}

async function replay(a: Actor, b: Actor, spec: MessageSpec): Promise<void> {
  const sender = spec.sender === 'A' ? a : b;
  const recipient = spec.sender === 'A' ? b : a;
  const before = await states(a, b);
  const eventCount = recipient.events.filter((value) => value.text === spec.text).length;
  await invoke(sender, new Api.messages.SendMessage({
    peer: peer(recipient), message: spec.text, randomId: BigInt(spec.randomId),
  }), 'replay');
  await sleep(500);
  const after = await states(a, b);
  for (const actor of [a, b]) {
    check(after[actor.name].pts === before[actor.name].pts, 'REPLAY_ADVANCED_PTS');
    assertMonotonic(before[actor.name], after[actor.name]);
  }
  check(recipient.events.filter((value) => value.text === spec.text).length === eventCount, 'REPLAY_CREATED_UPDATE');
  report.replaySamples.push({ direction: `${sender.name}-to-${recipient.name}`, before, after, noNewUpdate: true, ok: true });
}

async function cleanupTagged(): Promise<void> {
  let a: Actor | undefined;
  let b: Actor | undefined;
  try {
    a = await connectActor('A');
    b = await connectActor('B');
    const deleted: Record<string, number> = {};
    for (const [actor, other] of [[a, b], [b, a]]) {
      const found = await readTagged(actor, other);
      const ids = found.map((value) => Number(value.id));
      for (let offset = 0; offset < ids.length; offset += 100) {
        await invoke(actor, new Api.messages.DeleteMessages({ revoke: true, id: ids.slice(offset, offset + 100) }), 'cleanup');
      }
      deleted[actor.name] = ids.length;
    }
    const remaining = (await readTagged(a, b)).length + (await readTagged(b, a)).length;
    check(remaining === 0, 'CLEANUP_MESSAGES_REMAIN');
    report.cleanup = { ok: true, deleted, remaining };
  } finally {
    await disconnect(a);
    await disconnect(b);
  }
}

function distribution(values: number[]): any {
  const sorted = [...values].sort((a, b) => a - b);
  const percentile = (p: number) => sorted.length ? sorted[Math.ceil(sorted.length * p) - 1] : null;
  return { count: sorted.length, p50Ms: percentile(0.5), p95Ms: percentile(0.95), p99Ms: percentile(0.99), maxMs: sorted.at(-1) ?? null };
}

async function main(): Promise<void> {
  const started = performance.now();
  let a: Actor | undefined;
  let b: Actor | undefined;
  let configured = false;
  let ownsTag = false;
  let success = false;
  let messages: MessageSpec[] = [];
  const save = () => fs.writeFileSync(stateFile, `${JSON.stringify({
    schemaVersion: 1, tag, users: { A: process.env.A_USER_ID, B: process.env.B_USER_ID }, messages, state: report.state?.after,
  }, null, 2)}\n`, { mode: 0o600 });
  try {
    for (const key of ['TEAMGRAM_GRAMJS_DIR', 'A_AUTH_B64', 'B_AUTH_B64', 'A_USER_ID', 'B_USER_ID', 'A_ACCESS_HASH', 'B_ACCESS_HASH']) {
      check(Boolean(process.env[key]), `MISSING_${key}`);
    }
    check(Number.isInteger(sampleCount) && sampleCount >= 1 && sampleCount <= 200, 'INVALID_SLO_SAMPLES');
    check(Number.isInteger(timeoutMs) && timeoutMs >= 1000 && timeoutMs <= 60000, 'INVALID_TIMEOUT');
    check(Number.isInteger(dcId) && dcId > 0 && Number.isInteger(dcPort) && dcPort > 0 && dcPort < 65536, 'INVALID_DC_ENDPOINT');
    check(actorPorts.every((port) => Number.isInteger(port) && port > 0 && port < 65536), 'INVALID_ACTOR_DC_ENDPOINT');
    check(['single', 'before', 'after', 'fault', 'capacity', 'cleanup', 'slo'].includes(phase) && /^[A-Za-z0-9_-]{1,100}$/.test(tag), 'INVALID_PHASE_OR_TAG');
    check(phase !== 'slo' || (measureSlo && Number.isInteger(sloConnectionHoldMs)
      && sloConnectionHoldMs >= 1000 && sloConnectionHoldMs <= 30000), 'INVALID_SLO_PROFILE');
    if (phase === 'capacity') {
      check([loadRate, loadSeconds, loadInflight, loadLagMs].every(Number.isInteger)
        && loadRate > 0 && loadSeconds > 0 && loadRate * loadSeconds <= 20000
        && loadInflight > 0 && loadInflight <= 256 && loadLagMs > 0, 'INVALID_CAPACITY_PROFILE');
    }
    check(phase === 'single' || Boolean(process.env.PROBE_TAG), 'PHASE_REQUIRES_PROBE_TAG');
    check(phase !== 'fault' || Boolean(controlDir), 'FAULT_REQUIRES_CONTROL_DIR');
    check(phase !== 'capacity' || Boolean(controlDir), 'CAPACITY_REQUIRES_DRAIN_CONTROL_DIR');
    const authA = Buffer.from(process.env.A_AUTH_B64!, 'base64');
    const authB = Buffer.from(process.env.B_AUTH_B64!, 'base64');
    check(authA.length === 256 && authB.length === 256 && !authA.equals(authB), 'AUTH_KEYS_MUST_BE_DISTINCT_256_BYTES');
    check(process.env.A_USER_ID !== process.env.B_USER_ID, 'ACCOUNTS_MUST_DIFFER');
    const globalAny = globalThis as any;
    globalAny.self ??= globalThis;
    globalAny.addEventListener ??= () => {};
    globalAny.self.addEventListener ??= globalAny.addEventListener;
    const requireModule = createRequire(import.meta.url);
    const gramjsDir = process.env.TEAMGRAM_GRAMJS_DIR!;
    TelegramClient = requireModule(path.join(gramjsDir, 'src/lib/gramjs/client/TelegramClient.ts')).default;
    CallbackSession = requireModule(path.join(gramjsDir, 'src/lib/gramjs/sessions/CallbackSession.ts')).default;
    Api = requireModule(path.join(gramjsDir, 'src/lib/gramjs/tl/index.ts')).Api;
    const sendId = Api.messages.SendMessage.CONSTRUCTOR_ID;
    check(Number.isInteger(sendId) && (sendId >>> 0) === sendMessageConstructorId, 'LAYER229_SEND_CONSTRUCTOR_MISMATCH');
    report.sendConstructorId = (sendId >>> 0).toString(16);
    Connection = requireModule(path.join(gramjsDir, 'src/lib/gramjs/network/connection/TCPObfuscated.ts')).ConnectionTCPObfuscated;
    configured = true;
    stage = 'connect';
    a = await connectActor('A');
    b = await connectActor('B');
    check(a.id > 0n && b.id > 0n && a.id !== b.id && a.hash !== 0n && b.hash !== 0n, 'INVALID_IDENTITIES');
    report.users = { A: a.id.toString(), B: b.id.toString() };
    report.state = { before: await activate(a, b) };
    if (phase === 'cleanup') { ownsTag = true; success = true; return; }
    if (phase === 'after') {
      stage = 'restart-readback';
      const saved = JSON.parse(fs.readFileSync(stateFile, 'utf8'));
      check(saved.tag === tag && saved.users?.A === a.id.toString() && saved.users?.B === b.id.toString()
        && Array.isArray(saved.messages) && saved.messages.length > 0 && saved.state, 'INVALID_SAVED_STATE');
      messages = saved.messages;
      for (const message of messages) check(message.text.startsWith(`${tag}:`) && ['A', 'B'].includes(message.sender) && /^\d+$/.test(message.randomId), 'INVALID_SAVED_MESSAGE');
      ownsTag = true;
      for (const actor of [a, b]) assertMonotonic(stateFields(saved.state[actor.name]), report.state.before[actor.name]);
      await verifyHistory(a, b, messages);
      report.checks.savedStateReadback = true;
    } else if (phase === 'slo') {
      stage = 'minute-slo-canary';
      check((await readTagged(a, b)).length === 0 && (await readTagged(b, a)).length === 0, 'PROBE_TAG_ALREADY_EXISTS');
      ownsTag = true;
      stopConnectionObserver = observeConnections(a, b);
      await sleep(sloConnectionHoldMs);
      stopConnectionObserver();
      stopConnectionObserver = undefined;
      let canaryFailure = false;
      for (const [sender, recipient] of [[a, b], [b, a]]) {
        const spec: MessageSpec = {
          sender: sender.name, text: `${tag}:${sender.name}:0`,
          randomId: (BigInt(`0x${randomBytes(8).toString('hex')}`) & ((1n << 63n) - 1n)).toString(),
        };
        messages.push(spec);
        try {
          await sendMessage(sender, recipient, spec);
        } catch {
          canaryFailure = true;
        }
      }
      try {
        await verifyHistory(a, b, messages);
      } catch {
        canaryFailure = true;
      }
      try {
        report.state.after = await states(a, b);
      } catch {
        canaryFailure = true;
      }
      const reconnects = await Promise.all([reconnectSloSample(a), reconnectSloSample(b)]);
      report.reconnectSample = {
        eligible: reconnects.reduce((sum, value) => sum + value.eligible, 0),
        good: reconnects.reduce((sum, value) => sum + value.good, 0),
        durationsMs: reconnects.map((value) => value.elapsedMs),
      };
      report.checks.sloCanary = true;
      if (canaryFailure) report.error = { stage: 'minute-slo-canary', code: 'CANARY_CHECK_FAILED' };
      success = true;
      return;
    } else if (phase === 'capacity') {
      stage = 'fixed-rate-capacity';
      check((await readTagged(a, b)).length === 0 && (await readTagged(b, a)).length === 0, 'PROBE_TAG_ALREADY_EXISTS');
      ownsTag = true;
      if (measureSlo) stopConnectionObserver = observeConnections(a, b);
      await capacityMessages(a, b, messages);
      if (measureSlo) {
        stopConnectionObserver?.();
        stopConnectionObserver = undefined;
        const reconnects = await Promise.all([reconnectSloSample(a), reconnectSloSample(b)]);
        report.reconnectSample = {
          eligible: reconnects.reduce((sum, value) => sum + value.eligible, 0),
          good: reconnects.reduce((sum, value) => sum + value.good, 0),
          durationsMs: reconnects.map((value) => value.elapsedMs),
        };
      }
      success = true;
      return;
    } else if (phase === 'fault') {
      stage = 'fault-preflight';
      check((await readTagged(a, b)).length === 0 && (await readTagged(b, a)).length === 0, 'PROBE_TAG_ALREADY_EXISTS');
      ownsTag = true;
      await faultMessages(a, b, messages);
      await verifyHistory(a, b, messages);
    } else {
      stage = 'sequential-messages';
      check((await readTagged(a, b)).length === 0 && (await readTagged(b, a)).length === 0, 'PROBE_TAG_ALREADY_EXISTS');
      ownsTag = true;
      const loadStarted = performance.now();
      for (let index = 0; index < sampleCount; index += 1) {
        for (const [sender, recipient] of [[a, b], [b, a]]) {
          const spec: MessageSpec = {
            sender: sender.name, text: `${tag}:${sender.name}:${index}`,
            randomId: (BigInt(`0x${randomBytes(8).toString('hex')}`) & ((1n << 63n) - 1n)).toString(),
          };
          messages.push(spec);
          if (phase === 'before') save();
          await sendMessage(sender, recipient, spec);
        }
      }
      report.loadDurationMs = performance.now() - loadStarted;
      await verifyHistory(a, b, messages);
    }
    stage = 'random-id-replay';
    for (const sender of ['A', 'B']) {
      const first = messages.find((value) => value.sender === sender);
      check(first, 'MISSING_DIRECTION_SAMPLE');
      await replay(a, b, first);
    }
    await verifyHistory(a, b, messages);
    report.checks.randomIdReplay = true;
    report.state.after = await states(a, b);
    if (phase === 'before') {
      save();
      report.stateFile = stateFile;
      report.cleanup = { ok: true, retainedForRestart: true };
      success = true;
      return;
    }
    stage = 'fresh-session';
    const beforeFresh = report.state.after;
    await disconnect(a);
    await disconnect(b);
    a = await connectActor('A');
    b = await connectActor('B');
    const freshState = await activate(a, b);
    for (const actor of [a, b]) assertMonotonic(beforeFresh[actor.name], freshState[actor.name]);
    await verifyHistory(a, b, messages);
    // Replay an old random_id from a new session before continuing both ways.
    for (const sender of ['A', 'B']) await replay(a, b, messages.find((value) => value.sender === sender)!);
    for (const [sender, recipient] of [[a, b], [b, a]]) {
      const spec: MessageSpec = { sender: sender.name, text: `${tag}:${sender.name}:fresh`, randomId: (BigInt(Date.now()) * 1000n + (sender.name === 'A' ? 1n : 2n)).toString() };
      messages.push(spec);
      await sendMessage(sender, recipient, spec, true);
    }
    await verifyHistory(a, b, messages);
    report.state.after = await states(a, b);
    report.checks.freshSessionReadbackAndSend = true;
    report.freshSession = { sameAuthKeys: true, newClientObjects: true, newAuthKeys: false };
    success = true;
  } catch (error) {
    report.error = { stage, code: errorCode(error) };
  } finally {
    if (phase === 'capacity' && configured && ownsTag && controlDir) {
      try {
        writeMilestone('sent', { purpose: 'capacity-drain' });
        await waitMilestone('restored');
        report.checks.outboxDrainedBeforeCleanup = true;
        if (report.capacity && a && b) {
          stage = 'capacity-late-delivery-observation';
          await verifyCapacityAfterDrain(a, b, messages, performance.now());
        }
      } catch (error) {
        report.error = { stage, code: errorCode(error) };
        success = false;
      }
    }
    stopConnectionObserver?.();
    await disconnect(a);
    await disconnect(b);
    if (configured && ownsTag && (phase !== 'before' || !success)) {
      try { await cleanupTagged(); } catch (error) { report.cleanup = { ok: false, code: errorCode(error) }; success = false; }
    }
    if (phase === 'slo') {
      const directions = new Set(report.deliverySamples.map((sample: any) => sample.direction));
      report.collectionComplete = report.checks.sloCanary === true
        && directions.size === 2 && directions.has('A-to-B') && directions.has('B-to-A')
        && Number.isInteger(report.connectionObservation?.eligible) && report.connectionObservation.eligible > 0
        && report.reconnectSample?.eligible === 2
        && report.messageIntegrity?.complete === true
        && report.cleanup?.ok === true && report.cleanup?.remaining === 0;
    }
    report.durationMs = performance.now() - started;
    clearInterval(runtimeSampleTimer);
    eventLoopDelay.disable();
    const runtimeCPU = process.cpuUsage(runtimeCpuStarted);
    report.runtime = {
      samples: runtimeSamples,
      eventLoopDelayMs: {
        mean: eventLoopDelay.mean / 1e6,
        max: eventLoopDelay.max / 1e6,
        p95: eventLoopDelay.percentile(95) / 1e6,
        p99: eventLoopDelay.percentile(99) / 1e6,
      },
      cpuUsageMs: { user: runtimeCPU.user / 1000, system: runtimeCPU.system / 1000 },
      memory: process.memoryUsage(),
    };
    const deliveries = report.deliverySamples.filter((value: any) => value.ok);
    const load = deliveries.filter((value: any) => !value.freshSession);
    report.stats = {
      rpc: distribution(report.rpcSamples.filter((value: any) => value.ok).map((value: any) => value.elapsedMs)),
      delivery: distribution(deliveries.map((value: any) => value.deliveryMs)),
      throughput: { messages: load.length, durationMs: report.loadDurationMs || 0, messagesPerSecond: report.loadDurationMs ? load.length * 1000 / report.loadDurationMs : null },
    };
    if (phase !== 'capacity') report.checks.ptsContinuity = deliveries.length > 0 && deliveries.every((value: any) => value.contiguous);
    report.ok = success && !report.error && report.cleanup?.ok === true;
    await new Promise<void>((resolve, reject) => {
      process.stdout.write(`${JSON.stringify(report)}\n`, (error) => { if (error) reject(error); else resolve(); });
    });
  }
}

main().then(() => process.exit(report.ok ? 0 : 1)).catch(() => {
  process.stdout.write(`${JSON.stringify({ ...report, ok: false, error: { stage, code: 'UNEXPECTED_PROBE_ERROR' } })}\n`, () => process.exit(1));
});
