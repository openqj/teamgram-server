import { randomBytes } from 'node:crypto';
import path from 'node:path';
import { createRequire } from 'node:module';

const required = ['TEAMGRAM_GRAMJS_DIR', 'A_AUTH_B64', 'B_AUTH_B64', 'A_USER_ID', 'B_USER_ID', 'A_ACCESS_HASH', 'B_ACCESS_HASH'];
for (const key of required) {
  if (!process.env[key]) throw new Error(`${key} is required`);
}

const gramjsDir = process.env.TEAMGRAM_GRAMJS_DIR!;
const dcId = Number(process.env.TG_DC_ID || '2');
const dcHost = process.env.TG_DC_HOST || '127.0.0.1';
const dcPort = Number(process.env.TG_DC_PORT || '21543');
const A_ID = BigInt(process.env.A_USER_ID!);
const B_ID = BigInt(process.env.B_USER_ID!);
const B_HASH = BigInt(process.env.B_ACCESS_HASH!);
const pageLimit = 2;
const messageCount = 5;
const timeoutMs = Number(process.env.CHANNEL_RECOVERY_TIMEOUT_MS || '30000');
const tag = `channel-gap-${Date.now()}-${randomBytes(4).toString('hex')}`;

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

function check(condition: unknown, code: string): asserts condition {
  if (!condition) throw Object.assign(new Error(code), { probeCode: code });
}

function finish(evidence: Record<string, unknown>, exitCode: number): void {
  process.stdout.write(`${JSON.stringify(evidence)}\n`, () => process.exit(exitCode));
}

function errorCode(error: any): string {
  if (typeof error?.probeCode === 'string') return error.probeCode;
  const value = error?.errorMessage || error?.message || '';
  return typeof value === 'string' && /^[A-Z][A-Z0-9_]{0,79}$/.test(value) ? value : 'RPC_OR_TRANSPORT_ERROR';
}

function rpcName(value: any): string {
  return value?.className || value?.constructor?.name || typeof value;
}

function updatesChats(value: any): any[] {
  return [
    ...(Array.isArray(value?.chats) ? value.chats : []),
    ...(Array.isArray(value?.updates?.chats) ? value.updates.chats : []),
  ];
}

function messageEvents(value: any, seen = new Set<any>()): Array<{ text: string; pts: number; ptsCount: number }> {
  if (!value || typeof value !== 'object' || seen.has(value)) return [];
  seen.add(value);
  if (Array.isArray(value)) return value.flatMap((item) => messageEvents(item, seen));
  if (Array.isArray(value.updates)) return value.updates.flatMap((item: any) => messageEvents(item, seen));
  if (value.update) return messageEvents(value.update, seen);
  const message = typeof value.message === 'string' ? value : value.message;
  if (typeof message?.message !== 'string') return [];
  return [{ text: message.message, pts: Number(value.pts ?? message.pts ?? 0), ptsCount: Number(value.ptsCount ?? message.ptsCount ?? 0) }];
}

function makeClient(authKeyBase64: string): any {
  const session = new CallbackSession({
    mainDcId: dcId,
    keys: { [dcId]: Buffer.from(authKeyBase64, 'base64').toString('hex') },
  }, () => {});
  const load = session.load.bind(session);
  session.load = async () => {
    await load();
    session._serverAddress = dcHost;
    session._port = dcPort;
  };
  const client = new TelegramClient(session, Number(process.env.TG_API_ID || '1'),
    process.env.TG_API_HASH || '0123456789abcdef0123456789abcdef', {
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
  const initWith = client._initWith.bind(client);
  client._initWith = (query: any) => {
    const original = initWith(query);
    check(rpcName(original) === 'InvokeWithLayer' && rpcName(original.query) === 'InitConnection', 'INVALID_INIT_CONNECTION_WRAPPER');
    return new Api.InvokeWithLayer({ layer: 229, query: original.query });
  };
  client._borrowExportedSender = async () => undefined;
  return client;
}

async function invoke(client: any, request: any): Promise<any> {
  let timer: ReturnType<typeof setTimeout> | undefined;
  try {
    return await Promise.race([
      client.invoke(request),
      new Promise((_, reject) => { timer = setTimeout(() => reject(Object.assign(new Error('PROBE_TIMEOUT'), { probeCode: 'PROBE_TIMEOUT' })), timeoutMs); }),
    ]);
  } finally {
    if (timer) clearTimeout(timer);
  }
}

async function expectRpcError(code: string, action: () => Promise<unknown>): Promise<void> {
  try {
    await action();
  } catch (error) {
    check(errorCode(error) === code, `EXPECTED_${code}_GOT_${errorCode(error)}`);
    return;
  }
  check(false, `EXPECTED_${code}_BUT_SUCCEEDED`);
}

function inputChannel(channelId: bigint, accessHash: bigint): any {
  return new Api.InputChannel({ channelId, accessHash });
}

function differenceRequest(channelId: bigint, accessHash: bigint, pts: number): any {
  return new Api.updates.GetChannelDifference({
    force: true,
    channel: inputChannel(channelId, accessHash),
    filter: new Api.ChannelMessagesFilterEmpty(),
    pts,
    limit: pageLimit,
  });
}

async function connect(client: any, expectedUserID: bigint): Promise<void> {
  await client.connect();
  const me = await client.getMe();
  check(String(me?.id) === expectedUserID.toString(), 'AUTH_KEY_USER_MISMATCH');
  await invoke(client, new Api.updates.GetState());
  await invoke(client, new Api.account.UpdateStatus({ offline: false }));
}

async function waitForDroppedMessages(dropped: Set<string>, expected: string[]): Promise<void> {
  const deadline = performance.now() + timeoutMs;
  while (dropped.size < expected.length && performance.now() < deadline) {
    await new Promise((resolve) => setTimeout(resolve, 25));
  }
  check(expected.every((text) => dropped.has(text)), 'REALTIME_DROP_CAPTURE_INCOMPLETE');
}

async function run(): Promise<void> {
  const a = makeClient(process.env.A_AUTH_B64!);
  const b = makeClient(process.env.B_AUTH_B64!);
  const droppedRealtime = new Set<string>();
  const receivedRealtime = new Set<string>();
  const expectedMessages = Array.from({ length: messageCount }, (_, index) => `${tag}:${index + 1}`);
  let channelId = 0n;
  let accessHash = 0n;
  let failureCode = '';
  let cleanup = false;
  let baselinePts = -1;
  const pages: Array<{ pts: number; final: boolean; messages: string[] }> = [];

  b.addEventHandler((update: any) => {
    for (const event of messageEvents(update)) {
      if (!event.text.startsWith(`${tag}:`)) continue;
      receivedRealtime.add(event.text);
    }
  }, { build: (update: any) => update });

  try {
    await Promise.all([connect(a, A_ID), connect(b, B_ID)]);

    const title = `${tag} isolated recovery`;
    const created = await invoke(a, new Api.channels.CreateChannel({ broadcast: true, title, about: 'isolated channel difference recovery probe' }));
    const channel = updatesChats(created).find((item: any) => item?.title === title);
    check(channel?.id !== undefined && channel?.accessHash !== undefined, 'CHANNEL_CREATE_FAILED');
    channelId = BigInt(String(channel.id));
    accessHash = BigInt(String(channel.accessHash));

    await expectRpcError('USER_NOT_PARTICIPANT', () => invoke(b, differenceRequest(channelId, accessHash, 0)));

    const invited = await invoke(a, new Api.channels.InviteToChannel({
      channel: inputChannel(channelId, accessHash),
      users: [new Api.InputUser({ userId: B_ID, accessHash: B_HASH })],
    }));
    check(rpcName(invited).toLowerCase().includes('invitedusers'), 'CHANNEL_INVITE_FAILED');

    await expectRpcError('CHANNEL_INVALID', () => invoke(b, differenceRequest(channelId, accessHash + 1n, 0)));

    const initial = await invoke(b, differenceRequest(channelId, accessHash, 0));
    check(rpcName(initial) === 'updates.ChannelDifferenceEmpty' && initial.final === true, 'EMPTY_RESPONSE_PREDICATE_MISMATCH');
    baselinePts = Number(initial.pts);
    b.disconnect();
    check(!b.isConnected(), 'CLIENT_DID_NOT_DISCONNECT');

    for (let index = 0; index < expectedMessages.length; index += 1) {
      const sent = await invoke(a, new Api.messages.SendMessage({
        peer: new Api.InputPeerChannel({ channelId, accessHash }),
        message: expectedMessages[index],
        randomId: BigInt(Date.now()) * 1000n + BigInt(index + 1),
      }));
      check(rpcName(sent).toLowerCase().includes('updates'), 'CHANNEL_SEND_FAILED');
      if (!b.isConnected()) droppedRealtime.add(expectedMessages[index]);
    }
    await waitForDroppedMessages(droppedRealtime, expectedMessages);
    check(droppedRealtime.size === messageCount, 'REALTIME_DROP_COUNT_MISMATCH');
    await b.connect();
    check(b.isConnected(), 'CLIENT_RECONNECT_FAILED');
    await invoke(b, new Api.updates.GetState());
    check(receivedRealtime.size === 0, 'REALTIME_RECEIVED_DURING_OFFLINE_GAP');

    let cursor = baselinePts;
    let final = false;
    for (let attempt = 0; attempt < messageCount; attempt += 1) {
      const page = await invoke(b, differenceRequest(channelId, accessHash, cursor));
      check(rpcName(page) === 'updates.ChannelDifference' && Array.isArray(page.newMessages), 'INVALID_CHANNEL_DIFFERENCE_PAGE');
      const texts = page.newMessages.map((message: any) => String(message.message));
      check(texts.length > 0 && texts.length <= pageLimit, 'CHANNEL_DIFFERENCE_PAGE_SIZE_INVALID');
      check(texts.every((text: string) => expectedMessages.includes(text)), 'CHANNEL_DIFFERENCE_RETURNED_UNEXPECTED_MESSAGE');
      const pagePts = Number(page.pts);
      check(pagePts === cursor + texts.length, 'CHANNEL_DIFFERENCE_PTS_NOT_ADVANCING');
      pages.push({ pts: Number(page.pts), final: Boolean(page.final), messages: texts });
      cursor = Number(page.pts);
      if (page.final) {
        final = true;
        break;
      }
    }
    check(final && cursor === baselinePts + messageCount, 'CHANNEL_DIFFERENCE_DID_NOT_REACH_FINAL_STATE');
    const recovered = pages.flatMap((page) => page.messages);
    check(JSON.stringify(recovered) === JSON.stringify(expectedMessages), 'CHANNEL_DIFFERENCE_ORDER_OR_CONTENT_MISMATCH');
    check(new Set(recovered).size === messageCount, 'CHANNEL_DIFFERENCE_DUPLICATE_MESSAGE');

    const caughtUp = await invoke(b, differenceRequest(channelId, accessHash, cursor));
    check(rpcName(caughtUp) === 'updates.ChannelDifferenceEmpty' && caughtUp.final === true && Number(caughtUp.pts) === cursor,
      'CAUGHT_UP_RESPONSE_PREDICATE_MISMATCH');
    check(receivedRealtime.size === 0, 'REALTIME_RECEIVED_DURING_GAP_RECOVERY');

    const history = await invoke(b, new Api.messages.GetHistory({
      peer: new Api.InputPeerChannel({ channelId, accessHash }),
      offsetId: 0, offsetDate: 0, addOffset: 0, limit: 100, maxId: 0, minId: 0, hash: 0n,
    }));
    const historyTexts = (history?.messages || []).map((message: any) => String(message.message)).filter((text: string) => expectedMessages.includes(text));
    const historySet = new Set(historyTexts);
    check(historyTexts.length === messageCount && historySet.size === messageCount && expectedMessages.every((text) => historySet.has(text)),
      'HISTORY_AND_DIFFERENCE_MISMATCH');

    await expectRpcError('CHANNEL_INVALID', () => invoke(b, differenceRequest(channelId, accessHash + 1n, baselinePts)));
  } catch (error) {
    failureCode = errorCode(error);
  } finally {
    if (channelId > 0n) {
      try {
        const deleted = await invoke(a, new Api.channels.DeleteChannel({ channel: inputChannel(channelId, accessHash) }));
        cleanup = rpcName(deleted).toLowerCase().includes('updates');
      } catch {
        cleanup = false;
      }
    }
    try { a.destroy(); } catch {}
    try { b.destroy(); } catch {}
  }

  const evidence = {
    schemaVersion: 1,
    probe: 'messaging.channel-difference-recovery',
    evidenceClass: 'local_probe',
    productionSloEvidence: false,
    ok: !failureCode && cleanup,
    errorCode: failureCode || undefined,
    channelId: channelId > 0n ? channelId.toString() : undefined,
    baselinePts: baselinePts >= 0 ? baselinePts : undefined,
    droppedRealtimeMessages: droppedRealtime.size,
    realtimeDropMode: 'client_disconnected',
    pageLimit,
    pages,
    recoveredMessages: pages.flatMap((page) => page.messages),
    caughtUpPredicate: pages.length ? 'updates.ChannelDifferenceEmpty' : undefined,
    permissionChecks: { nonMember: 'USER_NOT_PARTICIPANT', wrongAccessHash: 'CHANNEL_INVALID' },
    cleanup,
  };
  finish(evidence, evidence.ok ? 0 : 1);
}

void run().catch((error) => {
  finish({
    schemaVersion: 1,
    probe: 'messaging.channel-difference-recovery',
    evidenceClass: 'local_probe',
    productionSloEvidence: false,
    ok: false,
    errorCode: errorCode(error),
    cleanup: false,
  }, 1);
});
