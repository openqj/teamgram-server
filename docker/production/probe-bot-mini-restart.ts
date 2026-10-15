import fs from 'node:fs';
import path from 'node:path';
import { createRequire } from 'node:module';
import { execFileSync } from 'node:child_process';
import { authSession, loadFixtureAuthKey } from '../web-isolated/fixture-auth';

/**
 * Three phase Bot/Mini App restart probe.
 *
 * The before process creates durable inline queries and WebView requests for
 * two independent users. The after process creates new authenticated
 * sessions, reads the exact records back, retries the bot publication with
 * the same payload, and sends one idempotent cross-account message. Cleanup
 * only removes rows identified by this run's query ids/random id.
 */
const gramjsDir = process.env.TEAMGRAM_GRAMJS_DIR;
if (!gramjsDir) throw new Error('TEAMGRAM_GRAMJS_DIR is required');

const phase = process.env.PROBE_PHASE || '';
const tag = process.env.PROBE_TAG || `bot-restart-${Date.now()}-${process.pid}`;
const stateFile = process.env.PROBE_STATE_FILE || `/tmp/teamgram-bot-mini-restart-${tag}.json`;
if (!['before', 'after', 'cleanup'].includes(phase)) throw new Error(`unsupported PROBE_PHASE: ${phase}`);
if (!/^[A-Za-z0-9_-]{5,80}$/.test(tag)) throw new Error('PROBE_TAG must contain only letters, numbers, _ or -');

const dcId = Number(process.env.TG_DC_ID || '2');
const dcHost = process.env.TG_DC_HOST || '127.0.0.1';
const dcPort = Number(process.env.TG_DC_PORT || '11443');
const apiId = Number(process.env.TG_API_ID || '1');
const apiHash = process.env.TG_API_HASH || '0123456789abcdef0123456789abcdef';
const postgresContainer = process.env.TEAMGRAM_POSTGRES_CONTAINER || 'teamgram-postgres';
const aUsername = process.env.A_USERNAME || 'isolated_test_01';
const bUsername = process.env.B_USERNAME || 'isolated_test_02';
const botUsername = process.env.BOT_USERNAME || 'web_e2e_bot';
const quietLogger = { debug() {}, info() {}, warn() {}, error() {} };

const globalAny = globalThis as typeof globalThis & { self?: any; addEventListener?: () => void };
globalAny.self ??= globalThis;
globalAny.addEventListener ??= () => {};
globalAny.self.addEventListener ??= globalAny.addEventListener;

const requireModule = createRequire(import.meta.url);
const TelegramClient = requireModule(path.join(gramjsDir, 'src/lib/gramjs/client/TelegramClient.ts')).default;
const CallbackSession = requireModule(path.join(gramjsDir, 'src/lib/gramjs/sessions/CallbackSession.ts')).default;
const Api = requireModule(path.join(gramjsDir, 'src/lib/gramjs/tl/index.ts')).Api;
const serializeBytes = requireModule(path.join(gramjsDir, 'src/lib/gramjs/tl/index.ts')).serializeBytes;
const Connection = requireModule(path.join(gramjsDir, 'src/lib/gramjs/network/connection/TCPObfuscated.ts')).ConnectionTCPObfuscated;

function rpcName(value: any): string {
  return value?.className || value?.constructor?.name || typeof value;
}

function requireCondition(value: unknown, message: string): asserts value {
  if (!value) throw new Error(message);
}

function isTrue(value: any): boolean {
  return value === true || value?.value === true || rpcName(value).toLowerCase().includes('true');
}

function sql(query: string): string {
  return execFileSync('docker', [
    'exec', postgresContainer, 'psql', '-v', 'ON_ERROR_STOP=1', '-At', '-q', '-F', '\t',
    '-U', 'teamgram', '-d', 'teamgram', '-c', query,
  ], { encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] }).trim();
}

function quote(value: string): string {
  return `'${value.replaceAll("'", "''")}'`;
}

function sqlLines(query: string): string[] {
  const value = sql(query);
  return value === '' ? [] : value.split('\n').filter(Boolean);
}

function int32(value: number): Uint8Array {
  const bytes = Buffer.alloc(4);
  bytes.writeInt32LE(value);
  return bytes;
}

function uint32(value: number): Uint8Array {
  const bytes = Buffer.alloc(4);
  bytes.writeUInt32LE(value >>> 0);
  return bytes;
}

function int64(value: bigint): Uint8Array {
  const bytes = Buffer.alloc(8);
  bytes.writeBigInt64LE(BigInt.asIntN(64, value));
  return bytes;
}

function tlString(value: string): Uint8Array {
  return Uint8Array.from(serializeBytes(value));
}

function concatBytes(...parts: Uint8Array[]): Uint8Array {
  return Uint8Array.from(Buffer.concat(parts.map((part) => Buffer.from(part))));
}

function vector(items: Uint8Array[]): Uint8Array {
  return concatBytes(uint32(0x1cb5c415), int32(items.length), ...items);
}

class RawRequest {
  SUBCLASS_OF_ID = 0;
  classType = 'request';

  constructor(
    public readonly CONSTRUCTOR_ID: number,
    private readonly payload: Uint8Array,
    public readonly className: string,
  ) {}

  getBytes(): Uint8Array {
    return concatBytes(uint32(this.CONSTRUCTOR_ID), this.payload);
  }

  readResult(reader: any): any {
    return reader.tgReadObject();
  }
}

function makeClient(authB64: string, updates = false): any {
  const session = new CallbackSession({
    mainDcId: dcId,
    keys: { [dcId]: Buffer.from(authB64, 'base64').toString('hex') },
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
    requestRetries: 2,
    connectionRetries: 3,
    connectionRetriesToFallback: 0,
    retryDelay: 250,
    autoReconnect: false,
    sequentialUpdates: true,
    baseLogger: quietLogger,
  });
  (client as any)._borrowExportedSender = async () => undefined;
  if (!updates) (client as any)._updateLoop = async () => undefined;
  return client;
}

type Actor = {
  client: any;
  id: bigint;
  accessHash: bigint;
  username: string;
};

async function connectActor(role: 'A' | 'B' | 'BOT', updates = false, onUpdate?: (update: any) => void): Promise<Actor> {
  const username = role === 'A' ? aUsername : role === 'B' ? bUsername : botUsername;
  const supplied = process.env[`${role}_AUTH_B64`];
  const auth = supplied || loadFixtureAuthKey(username);
  if (!auth) throw new Error(`no auth key for ${role} (${username}); set ${role}_AUTH_B64`);
  let lastError: unknown;
  for (let attempt = 0; attempt < 4; attempt += 1) {
    const client = makeClient(auth, updates);
    try {
      if (onUpdate) client.addEventHandler(onUpdate, { build: (value: any) => value });
      await client.connect();
      const me = await client.getMe();
      const id = BigInt(me?.id ?? 0);
      const accessHash = BigInt(me?.accessHash ?? 0);
      requireCondition(id > 0n && accessHash !== 0n, `${role} getMe returned no identity/access hash`);
      return { client, id, accessHash, username };
    } catch (error) {
      lastError = error;
      try { await client.disconnect(); } catch {}
      if (attempt < 3) await new Promise((resolve) => setTimeout(resolve, 750));
    }
  }
  throw lastError || new Error(`${role} connection failed`);
}

function stateFields(value: any): { pts: number; qts: number; seq: number; date: number } {
  return {
    pts: Number(value?.pts ?? 0),
    qts: Number(value?.qts ?? 0),
    seq: Number(value?.seq ?? 0),
    date: Number(value?.date ?? 0),
  };
}

function assertState(label: string, value: any): { pts: number; qts: number; seq: number; date: number } {
  const state = stateFields(value);
  requireCondition(rpcName(value) === 'updates.State', `${label} returned ${rpcName(value)}`);
  requireCondition(Number.isInteger(state.pts) && state.pts >= 0, `${label} invalid pts=${state.pts}`);
  requireCondition(Number.isInteger(state.qts) && state.qts >= 0, `${label} invalid qts=${state.qts}`);
  // An account without a global sequence reports the canonical Telegram
  // sentinel seq=-1 until it receives one.
  requireCondition(Number.isInteger(state.seq) && state.seq >= -1, `${label} invalid seq=${state.seq}`);
  requireCondition(Number.isInteger(state.date) && state.date > 0, `${label} invalid date=${state.date}`);
  return state;
}

function assertMonotonic(before: { pts: number; qts: number; seq: number; date: number }, after: any, label: string): { pts: number; qts: number; seq: number; date: number } {
  const current = assertState(label, after);
  for (const field of ['pts', 'qts', 'seq', 'date'] as const) {
    requireCondition(current[field] >= before[field], `${label} ${field} regressed ${before[field]} -> ${current[field]}`);
  }
  return current;
}

function selfPeer(): any {
  return new Api.InputPeerSelf();
}

function inputBot(bot: Actor): any {
  return new Api.InputUser({ userId: bot.id, accessHash: bot.accessHash });
}

function inputPeer(user: Actor): any {
  return new Api.InputPeerUser({ userId: user.id, accessHash: user.accessHash });
}

function inlineResult(id: string, message: string): any {
  return new Api.InputBotInlineResult({
    id,
    type: 'article',
    sendMessage: new Api.InputBotInlineMessageText({ message }),
  });
}

function rawGetInlineBotResults(bot: Actor, query: string): RawRequest {
  return new RawRequest(
    0x514e999d,
    concatBytes(int32(0), inputBot(bot).getBytes(), selfPeer().getBytes(), tlString(query), tlString('')),
    'messages.getInlineBotResults',
  );
}

function rawSetInlineBotResults(queryId: bigint, result: any): RawRequest {
  return new RawRequest(
    0xbb12a419,
    concatBytes(int32(0), int64(queryId), vector([result.getBytes()]), int32(30)),
    'messages.setInlineBotResults',
  );
}

function rawRequestWebView(bot: Actor, url: string): RawRequest {
  return new RawRequest(
    0x269dc2c1,
    concatBytes(int32(1 << 1), selfPeer().getBytes(), inputBot(bot).getBytes(), tlString(url), tlString('web')),
    'messages.requestWebView',
  );
}

function rawProlongWebViewFor(bot: Actor, queryId: bigint): RawRequest {
  return new RawRequest(
    0xb0d81a83,
    concatBytes(int32(0), selfPeer().getBytes(), inputBot(bot).getBytes(), int64(queryId)),
    'messages.prolongWebView',
  );
}

function queryId(value: any): bigint {
  const candidate = value?.queryId ?? value?.queryId_FLAGINT64 ?? value?.queryId_INT64;
  const parsed = BigInt(candidate?.value ?? candidate ?? 0);
  requireCondition(parsed > 0n, `missing positive query_id in ${rpcName(value)}`);
  return parsed;
}

async function invoke(client: any, label: string, request: any): Promise<any> {
  try {
    return await client.invoke(request);
  } catch (error) {
    throw new Error(`${label}: ${error instanceof Error ? error.message : String(error)}`);
  }
}

function readState<T extends Record<string, any>>(): T {
  if (!fs.existsSync(stateFile)) {
    if (phase === 'cleanup') return { tag, phase } as T;
    throw new Error(`state file is missing: ${stateFile}`);
  }
  return JSON.parse(fs.readFileSync(stateFile, 'utf8')) as T;
}

function writeState(value: Record<string, any>): void {
  fs.mkdirSync(path.dirname(stateFile), { recursive: true });
  fs.writeFileSync(stateFile, `${JSON.stringify(value, null, 2)}\n`, { encoding: 'utf8', mode: 0o600 });
}

type InlineSeed = { query: string; resultId: string; message: string; queryId: string };
type WebViewSeed = { queryId: string; url: string; userId: string };
type ProbeState = {
  tag: string;
  botId: string;
  aId: string;
  bId: string;
  inline: InlineSeed[];
  webViews: WebViewSeed[];
  stateA: ReturnType<typeof stateFields>;
  stateB: ReturnType<typeof stateFields>;
  messageText: string;
  messageRandomId: string;
  messageId?: number;
  beforeInlineRow?: string;
  beforeStatusRow?: string;
  beforeWebViewRows: string[];
};

function captureRows(a: Actor, b: Actor, bot: Actor): Pick<ProbeState, 'beforeInlineRow' | 'beforeStatusRow' | 'beforeWebViewRows'> {
  const inlineKey = `inline-bot-query:${bot.id}`;
  const inline = sqlLines(`SELECT v FROM apifull_kv WHERE k=${quote(inlineKey)}`);
  const status = sqlLines(`SELECT pending_updates_count || E'\\t' || encode(convert_to(message,'UTF8'),'base64') FROM apifull_bot_updates_status WHERE bot_user_id=${bot.id}`);
  const webViews = sqlLines(`SELECT request_id FROM apifull_webview_request WHERE bot_id=${bot.id} AND user_id IN (${a.id},${b.id}) AND kind='requestWebView'`);
  return { beforeInlineRow: inline[0], beforeStatusRow: status[0], beforeWebViewRows: webViews };
}

function inlineItems(raw: string | undefined): any[] {
  if (!raw) return [];
  const parsed = JSON.parse(raw);
  return Array.isArray(parsed) ? parsed : [];
}

function assertInlineDurable(botId: bigint, seeds: InlineSeed[]): Record<string, number> {
  const raw = sqlLines(`SELECT v FROM apifull_kv WHERE k=${quote(`inline-bot-query:${botId}`)}`)[0];
  const items = inlineItems(raw);
  const counts: Record<string, number> = {};
  for (const seed of seeds) {
    const matches = items.filter((item) => item?.query === seed.query);
    counts[seed.query] = matches.length;
    requireCondition(matches.length === 1, `inline query ${seed.query} has ${matches.length} durable records`);
    const item = matches[0];
    requireCondition(item.ready === true, `inline query ${seed.query} is not ready after replay`);
    requireCondition(Array.isArray(item.results) && item.results.length === 1, `inline query ${seed.query} result count is not one`);
    requireCondition(item.results[0]?.id === seed.resultId, `inline query ${seed.query} result id mismatch`);
  }
  return counts;
}

function updateMessages(update: any, envelope: { date?: number; seq?: number; qts?: number } = {}): any[] {
  const nextEnvelope = {
    date: Number.isInteger(update?.date) ? Number(update.date) : envelope.date,
    seq: Number.isInteger(update?.seq) ? Number(update.seq) : envelope.seq,
    qts: Number.isInteger(update?.qts) ? Number(update.qts) : envelope.qts,
  };
  if (Array.isArray(update?.updates)) return update.updates.flatMap((item: any) => updateMessages(item, nextEnvelope));
  if (update?.update) return updateMessages(update.update, nextEnvelope);
  const message = update?.message;
  if (message?.message !== undefined) {
    return [{ text: String(message.message), id: Number(message.id || 0), pts: Number(update?.pts || message.pts || 0), ptsCount: Number(update?.ptsCount || message.ptsCount || 0), date: Number(message.date || nextEnvelope.date || 0), seq: nextEnvelope.seq, qts: nextEnvelope.qts }];
  }
  if (typeof update?.message === 'string') {
    return [{ text: String(update.message), id: Number(update.id || 0), pts: Number(update.pts || 0), ptsCount: Number(update.ptsCount || 0), date: Number(update.date || nextEnvelope.date || 0), seq: nextEnvelope.seq, qts: nextEnvelope.qts }];
  }
  return [];
}

function history(peer: any): any {
  return new Api.messages.GetHistory({ peer, offsetId: 0, offsetDate: 0, addOffset: 0, limit: 100, maxId: 0, minId: 0, hash: 0n });
}

function extractMessageId(value: any, text: string): number {
  if (!value || typeof value !== 'object') return 0;
  if (Array.isArray(value.updates)) {
    for (const item of value.updates) {
      const found = extractMessageId(item, text);
      if (found > 0) return found;
    }
  }
  if (value.message?.message === text) return Number(value.message.id || 0);
  if (value.message === text) return Number(value.id || 0);
  return 0;
}

async function disconnect(client: any): Promise<void> {
  try { await client.disconnect(); } catch {}
}

async function beforePhase(): Promise<void> {
  const a = await connectActor('A');
  const b = await connectActor('B');
  const bot = await connectActor('BOT');
  const owned = captureRows(a, b, bot);
  const clients = [a.client, b.client, bot.client];
  try {
    const [stateAResult, stateBResult] = await Promise.all([
      invoke(a.client, 'A updates.getState', new Api.updates.GetState()),
      invoke(b.client, 'B updates.getState', new Api.updates.GetState()),
    ]);
    const stateA = assertState('A before', stateAResult);
    const stateB = assertState('B before', stateBResult);
    const inline: InlineSeed[] = [];
    const webViews: WebViewSeed[] = [];
    for (const [owner, suffix] of [[a, 'a'], [b, 'b']] as const) {
      const query = `${tag}:inline:${suffix}`;
      const resultId = `${tag}-${suffix}`;
      const pending = await invoke(owner.client, `${suffix} messages.getInlineBotResults`, rawGetInlineBotResults(bot, query));
      inline.push({ query, resultId, message: `${tag}:inline-message:${suffix}`, queryId: queryId(pending).toString() });
      const web = await invoke(owner.client, `${suffix} messages.requestWebView`, rawRequestWebView(bot, `https://mini.example/restart/${tag}/${suffix}`));
      const webQueryId = queryId(web);
      webViews.push({ queryId: webQueryId.toString(), url: String(web?.url || ''), userId: owner.id.toString() });
      const prolonged = await invoke(owner.client, `${suffix} messages.prolongWebView`, rawProlongWebViewFor(bot, webQueryId));
      requireCondition(isTrue(prolonged), `${suffix} WebView did not prolong before restart (${rpcName(prolonged)} ${JSON.stringify(prolonged)})`);
      const published = await invoke(bot.client, `bot messages.setInlineBotResults ${suffix}`, rawSetInlineBotResults(BigInt(inline.at(-1)!.queryId), inlineResult(resultId, inline.at(-1)!.message)));
      requireCondition(isTrue(published), `bot publication ${suffix} failed before restart`);
    }
    const statusMessage = `${tag}:bot-status`;
    const status = await invoke(bot.client, 'help.setBotUpdatesStatus', new RawRequest(0xec22cfcd, concatBytes(int32(2), tlString(statusMessage)), 'help.setBotUpdatesStatus'));
    requireCondition(isTrue(status), 'help.setBotUpdatesStatus failed before restart');
    const messageText = `${tag}:exactly-once`;
    const messageRandomId = (BigInt(Date.now()) * 1000n + BigInt(process.pid % 1000)).toString();
    const state: ProbeState = {
      tag,
      botId: bot.id.toString(),
      aId: a.id.toString(),
      bId: b.id.toString(),
      inline,
      webViews,
      stateA,
      stateB,
      messageText,
      messageRandomId,
      ...owned,
    };
    writeState(state);
    console.log(JSON.stringify({ phase, tag, users: { a: a.id.toString(), b: b.id.toString() }, bot: bot.id.toString(), inline, webViews, state: { a: stateA, b: stateB } }));
  } finally {
    await Promise.all(clients.map(disconnect));
  }
}

async function afterPhase(): Promise<void> {
  const saved = readState<ProbeState>();
  const a = await connectActor('A', true);
  const eventsB: any[] = [];
  const b = await connectActor('B', true, (update: any) => eventsB.push(...updateMessages(update)));
  const bot = await connectActor('BOT');
  const clients = [a.client, b.client, bot.client];
  try {
    requireCondition(a.id.toString() === saved.aId && b.id.toString() === saved.bId && bot.id.toString() === saved.botId, 'fixture auth identity changed across restart');
    const [stateAResult, stateBResult] = await Promise.all([
      invoke(a.client, 'A updates.getState after restart', new Api.updates.GetState()),
      invoke(b.client, 'B updates.getState after restart', new Api.updates.GetState()),
    ]);
    const stateA = assertMonotonic(saved.stateA, stateAResult, 'A after restart');
    const stateB = assertMonotonic(saved.stateB, stateBResult, 'B after restart');
    await Promise.all([
      invoke(a.client, 'A account.updateStatus', new Api.account.UpdateStatus({ offline: false })),
      invoke(b.client, 'B account.updateStatus', new Api.account.UpdateStatus({ offline: false })),
    ]);
    const readbacks: Record<string, any> = {};
    for (const [owner, suffix, index] of [[a, 'a', 0], [b, 'b', 1]] as const) {
      const seed = saved.inline[index];
      const ready = await invoke(owner.client, `${suffix} inline readback`, rawGetInlineBotResults(bot, seed.query));
      requireCondition(queryId(ready).toString() === seed.queryId, `${suffix} query id changed across restart`);
      requireCondition(Array.isArray(ready?.results) && ready.results.length === 1 && ready.results[0]?.id === seed.resultId, `${suffix} inline result readback is not exactly one tagged result`);
      const result = inlineResult(seed.resultId, seed.message);
      const replayOne = await invoke(bot.client, `bot inline replay ${suffix} #1`, rawSetInlineBotResults(BigInt(seed.queryId), result));
      const replayTwo = await invoke(bot.client, `bot inline replay ${suffix} #2`, rawSetInlineBotResults(BigInt(seed.queryId), result));
      for (const [label, value] of [['first', replayOne], ['second', replayTwo]] as const) {
        requireCondition(isTrue(value), `${suffix} bot replay ${label} was not BoolTrue`);
      }
      const reread = await invoke(owner.client, `${suffix} inline reread`, rawGetInlineBotResults(bot, seed.query));
      requireCondition(Array.isArray(reread?.results) && reread.results.length === 1 && reread.results[0]?.id === seed.resultId, `${suffix} duplicate replay changed result cardinality`);
      readbacks[suffix] = { queryId: seed.queryId, resultId: seed.resultId, replay: [rpcName(replayOne), rpcName(replayTwo)] };
      const prolonged = await invoke(owner.client, `${suffix} WebView prolong after restart`, rawProlongWebViewFor(bot, BigInt(saved.webViews[index].queryId)));
      requireCondition(isTrue(prolonged), `${suffix} WebView query did not survive restart (${rpcName(prolonged)} ${JSON.stringify(prolonged)})`);
    }
    const durableInline = assertInlineDurable(bot.id, saved.inline);
    const sentOne = await invoke(a.client, 'A exactly-once send #1', new Api.messages.SendMessage({ peer: inputPeer(b), message: saved.messageText, randomId: BigInt(saved.messageRandomId) }));
    const sentTwo = await invoke(a.client, 'A exactly-once send #2', new Api.messages.SendMessage({ peer: inputPeer(b), message: saved.messageText, randomId: BigInt(saved.messageRandomId) }));
    let historyB: any;
    let messages: any[] = [];
    // The production-shaped MQ path can commit the recipient row after the
    // sender row. Wait long enough for the asynchronous delivery intent to
    // settle before asserting exactly-once semantics.
    for (let attempt = 0; attempt < 40; attempt += 1) {
      historyB = await invoke(b.client, 'B history after exactly-once replay', history(inputPeer(a)));
      const allMessages = [...(Array.isArray(historyB?.messages) ? historyB.messages : []), ...(Array.isArray(historyB?.newMessages) ? historyB.newMessages : [])].filter((item: any) => item?.message === saved.messageText);
      messages = [...new Map(allMessages.map((item: any) => [String(item?.id || ''), item])).values()].filter((item: any) => item?.id);
      if (messages.length > 0) break;
      await new Promise((resolve) => setTimeout(resolve, 200));
    }
    const recipientIds = sqlLines(`SELECT id FROM messages WHERE user_id=${b.id} AND sender_user_id=${a.id} AND peer_id=${a.id} AND message=${quote(saved.messageText)} AND deleted=false ORDER BY id DESC`);
    requireCondition(recipientIds.length === 1, `exactly-once recipient row count is ${recipientIds.length}: history=${messages.length}`);
    requireCondition(messages.length <= 1, `exactly-once message history count is ${messages.length}: ${JSON.stringify(historyB, (_key, value) => typeof value === 'bigint' ? value.toString() : value)}`);
    const messageId = Number(recipientIds[0] || messages[0]?.id || extractMessageId(sentOne, saved.messageText) || extractMessageId(sentTwo, saved.messageText));
    requireCondition(messageId > 0, 'exactly-once message has no durable id');
    const eventDeadline = Date.now() + 8_000;
    while (Date.now() < eventDeadline && !eventsB.some((event) => event?.text === saved.messageText)) {
      await new Promise((resolve) => setTimeout(resolve, 50));
    }
    const matchingEvents = eventsB.filter((event) => event?.text === saved.messageText);
    requireCondition(matchingEvents.length === 1, `recipient observed ${matchingEvents.length} live updates`);
    const finalStateA = assertMonotonic(stateA, await invoke(a.client, 'A updates.getState final', new Api.updates.GetState()), 'A final');
    const finalStateB = assertMonotonic(stateB, await invoke(b.client, 'B updates.getState final', new Api.updates.GetState()), 'B final');
    const event = matchingEvents[0];
    requireCondition(event.id > 0 && event.date > 0, 'recipient update has no id/date');
    requireCondition(event.pts > stateB.pts && event.pts <= finalStateB.pts, `recipient update pts is outside state window: ${JSON.stringify(event)}`);
    requireCondition(event.ptsCount > 0, `recipient update pts_count is invalid: ${JSON.stringify(event)}`);
    saved.messageId = messageId;
    writeState({ ...saved, after: { stateA: finalStateA, stateB: finalStateB, readbacks, durableInline, eventCount: matchingEvents.length, sendResults: [rpcName(sentOne), rpcName(sentTwo)] } });
    console.log(JSON.stringify({ phase, tag, readbacks, durableInline, message: { id: messageId, eventCount: matchingEvents.length }, state: { before: { a: saved.stateA, b: saved.stateB }, after: { a: finalStateA, b: finalStateB } } }));
  } finally {
    await Promise.all(clients.map(disconnect));
  }
}

function cleanupByTag(tagValue: string): Record<string, string> {
  const escapedTag = quote(`${tagValue}:inline:%`);
  const inlineBefore = sql(`SELECT count(*) FROM apifull_kv WHERE k LIKE 'inline-bot-query:%' AND v::jsonb @> jsonb_build_array(jsonb_build_object('query', '${tagValue}:inline:a'))`);
  sql(`UPDATE apifull_kv SET v=(SELECT COALESCE(jsonb_agg(item),'[]'::jsonb)::text FROM jsonb_array_elements(v::jsonb) AS item WHERE item->>'query' NOT LIKE ${escapedTag}) WHERE k LIKE 'inline-bot-query:%' AND EXISTS (SELECT 1 FROM jsonb_array_elements(v::jsonb) AS candidate WHERE candidate->>'query' LIKE ${escapedTag})`);
  const webViews = sql(`WITH deleted AS (DELETE FROM apifull_webview_request WHERE payload->>'url' LIKE ${quote(`https://mini.example/restart/${tagValue}/%`)} RETURNING request_id) SELECT count(*) FROM deleted`);
  sql(`DELETE FROM apifull_bot_updates_status WHERE message=${quote(`${tagValue}:bot-status`)}`);
  const messages = sql(`WITH changed AS (UPDATE messages SET deleted=true,updated_at=CURRENT_TIMESTAMP WHERE message=${quote(`${tagValue}:exactly-once`)} AND deleted=false RETURNING id) SELECT count(*) FROM changed`);
  return { inline: inlineBefore, webViews, messages };
}

async function cleanupPhase(): Promise<void> {
  const saved = readState<ProbeState>();
  if (!saved.botId || !saved.aId || !saved.bId) {
    const fallback = cleanupByTag(tag);
    try { fs.unlinkSync(stateFile); } catch {}
    console.log(JSON.stringify({ phase, tag, deleted: true, fallback }));
    return;
  }
  let a: Actor | undefined;
  try {
    a = await connectActor('A');
    if (saved.messageId && saved.messageId > 0) {
      try { await invoke(a.client, 'cleanup messages.deleteMessages', new Api.messages.DeleteMessages({ revoke: true, id: [saved.messageId] })); } catch {}
    }
  } finally {
    if (a) await disconnect(a.client);
  }
  const inlineKey = `inline-bot-query:${saved.botId}`;
  if (saved.beforeInlineRow === undefined) {
    sql(`DELETE FROM apifull_kv WHERE k=${quote(inlineKey)}`);
  } else {
    sql(`INSERT INTO apifull_kv(k,v) VALUES (${quote(inlineKey)},${quote(saved.beforeInlineRow)}) ON CONFLICT (k) DO UPDATE SET v=EXCLUDED.v`);
  }
  if (saved.beforeStatusRow === undefined) {
    sql(`DELETE FROM apifull_bot_updates_status WHERE bot_user_id=${saved.botId}`);
  } else {
    const [count, messageB64] = saved.beforeStatusRow.split('\t');
    const message = Buffer.from(messageB64 || '', 'base64').toString('utf8');
    sql(`INSERT INTO apifull_bot_updates_status(bot_user_id,pending_updates_count,message) VALUES (${saved.botId},${Number(count)},${quote(message)}) ON CONFLICT (bot_user_id) DO UPDATE SET pending_updates_count=EXCLUDED.pending_updates_count,message=EXCLUDED.message,updated_at=CURRENT_TIMESTAMP`);
  }
  const queryIds = saved.webViews.map((item) => item.queryId).map(quote).join(',');
  if (queryIds) {
    sql(`DELETE FROM apifull_webview_request WHERE bot_id=${saved.botId} AND user_id IN (${saved.aId},${saved.bId}) AND payload->>'query_id' IN (${queryIds})`);
  }
  // If the API deletion raced with a failed after phase, tombstone only this
  // run's random id and sender. The random id is unique and never broad.
  sql(`UPDATE messages SET deleted=true, updated_at=CURRENT_TIMESTAMP WHERE sender_user_id=${saved.aId} AND random_id=${saved.messageRandomId}`);
  try { fs.unlinkSync(stateFile); } catch {}
  const remaining = {
    inline: sql(`SELECT count(*) FROM apifull_kv WHERE k=${quote(inlineKey)} AND v LIKE ${quote(`%${tag}%`)}`),
    webViews: queryIds ? sql(`SELECT count(*) FROM apifull_webview_request WHERE bot_id=${saved.botId} AND user_id IN (${saved.aId},${saved.bId}) AND payload->>'query_id' IN (${queryIds})`) : '0',
    messages: sql(`SELECT count(*) FROM messages WHERE sender_user_id=${saved.aId} AND random_id=${saved.messageRandomId} AND deleted=false`),
  };
  requireCondition(Object.values(remaining).every((value) => value === '0'), `cleanup incomplete: ${JSON.stringify(remaining)}`);
  console.log(JSON.stringify({ phase, tag, deleted: true, remaining }));
}

async function main(): Promise<void> {
  if (phase === 'before') await beforePhase();
  else if (phase === 'after') await afterPhase();
  else await cleanupPhase();
}

main().then(() => process.exit(0)).catch((error) => {
  console.error(`bot/mini restart probe failed (${phase}): ${error instanceof Error ? error.stack || error.message : String(error)}`);
  process.exit(1);
});
