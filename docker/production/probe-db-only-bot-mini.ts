import { createRequire } from 'node:module';
import { execFileSync } from 'node:child_process';
import path from 'node:path';

for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) {
  delete process.env[key];
}

for (const key of ['TEAMGRAM_GRAMJS_DIR', 'USER_AUTH_B64', 'BOT_AUTH_B64', 'USER_ID', 'BOT_ID', 'USER_ACCESS_HASH', 'BOT_ACCESS_HASH']) {
  if (!process.env[key]) throw new Error(`${key} is required`);
}

const gramjsDir = process.env.TEAMGRAM_GRAMJS_DIR!;
const dcId = Number(process.env.TG_DC_ID || '2');
const dcHost = process.env.TG_DC_HOST || '127.0.0.1';
const dcPort = Number(process.env.TG_DC_PORT || '11443');
const userId = BigInt(process.env.USER_ID!);
const botId = BigInt(process.env.BOT_ID!);
const userAccessHash = BigInt(process.env.USER_ACCESS_HASH!);
const botAccessHash = BigInt(process.env.BOT_ACCESS_HASH!);
const postgresContainer = process.env.TEAMGRAM_POSTGRES_CONTAINER || 'teamgram-postgres';

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
const quietLogger = { debug() {}, info() {}, warn() {}, error() {} };

function sql(query: string): string {
  return execFileSync('docker', [
    'exec', postgresContainer, 'psql', '-v', 'ON_ERROR_STOP=1', '-At', '-q', '-F', '\t',
    '-U', 'teamgram', '-d', 'teamgram', '-c', query,
  ], { encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] }).trim();
}

function sqlLines(query: string): string[] {
  const value = sql(query);
  return value === '' ? [] : value.split('\n').filter(Boolean);
}

function quote(value: string): string {
  return `'${value.replaceAll("'", "''")}'`;
}

function rpcName(value: any): string {
  return value?.className || value?.constructor?.name || typeof value;
}

function concatBytes(...parts: Uint8Array[]): Uint8Array {
  return Uint8Array.from(Buffer.concat(parts.map((part) => Buffer.from(part))));
}

function tlInt(value: number): Uint8Array {
  const bytes = Buffer.alloc(4);
  bytes.writeInt32LE(value);
  return bytes;
}

function tlUInt(value: number): Uint8Array {
  const bytes = Buffer.alloc(4);
  bytes.writeUInt32LE(value >>> 0);
  return bytes;
}

function tlLong(value: bigint | number): Uint8Array {
  const bytes = Buffer.alloc(8);
  bytes.writeBigInt64LE(BigInt.asIntN(64, BigInt(value)));
  return bytes;
}

function tlString(value: string): Uint8Array {
  return serializeBytes(value);
}

function tlVector(items: Uint8Array[]): Uint8Array {
  return concatBytes(tlUInt(0x1cb5c415), tlInt(items.length), ...items);
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
    return concatBytes(tlUInt(this.CONSTRUCTOR_ID), this.payload);
  }

  readResult(reader: any): any {
    return reader.tgReadObject();
  }
}

function makeClient(authB64: string): any {
  const keyHex = Buffer.from(authB64, 'base64').toString('hex');
  const session = new CallbackSession({ mainDcId: dcId, keys: { [dcId]: keyHex } }, () => {});
  const load = session.load.bind(session);
  session.load = async () => {
    await load();
    (session as any)._serverAddress = dcHost;
    (session as any)._port = dcPort;
  };
  const client = new TelegramClient(session, 1, '0123456789abcdef0123456789abcdef', {
    connection: Connection,
    fallbackConnection: Connection,
    shouldAllowHttpTransport: false,
    shouldForceHttpTransport: false,
    timeout: 20,
    requestRetries: 1,
    connectionRetries: 1,
    connectionRetriesToFallback: 0,
    autoReconnect: false,
    baseLogger: quietLogger,
  });
  (client as any)._borrowExportedSender = async () => undefined;
  (client as any)._updateLoop = async () => undefined;
  return client;
}

function selfPeer(): any {
  return new Api.InputPeerSelf();
}

function inputUser(): any {
  return new Api.InputUser({ userId, accessHash: userAccessHash });
}

function inputBot(): any {
  return new Api.InputUser({ userId: botId, accessHash: botAccessHash });
}

function inlineResult(id: string, message: string): any {
  return new Api.InputBotInlineResult({
    id,
    type: 'article',
    sendMessage: new Api.InputBotInlineMessageText({ message }),
  });
}

function rawGetInlineBotResults(): RawRequest {
  return new RawRequest(
    0x514e999d,
    concatBytes(tlInt(0), inputBot().getBytes(), selfPeer().getBytes(), tlString('db-only-inline'), tlString('')),
    'messages.getInlineBotResults',
  );
}

function rawSetInlineBotResults(queryId: bigint, result: any): RawRequest {
  return new RawRequest(
    0xbb12a419,
    concatBytes(tlInt(0), tlLong(queryId), tlVector([result.getBytes()]), tlInt(30)),
    'messages.setInlineBotResults',
  );
}

function rawSendInlineBotResult(queryId: bigint, randomId: bigint, id: string): RawRequest {
  return new RawRequest(
    0xc0cf7646,
    concatBytes(tlInt(0), selfPeer().getBytes(), tlLong(randomId), tlLong(queryId), tlString(id)),
    'messages.sendInlineBotResult',
  );
}

function rawRequestWebView(): RawRequest {
  return new RawRequest(
    0x269dc2c1,
    concatBytes(tlInt(1 << 1), selfPeer().getBytes(), inputBot().getBytes(), tlString('https://mini.example/db-only'), tlString('web')),
    'messages.requestWebView',
  );
}

function rawProlongWebView(queryId: bigint): RawRequest {
  return new RawRequest(
    0xb0d81a83,
    concatBytes(tlInt(0), selfPeer().getBytes(), inputBot().getBytes(), tlLong(queryId)),
    'messages.prolongWebView',
  );
}

function rawRequestSimpleWebView(): RawRequest {
  return new RawRequest(
    0x413a3e73,
    concatBytes(tlInt(1 << 3), inputBot().getBytes(), tlString('https://mini.example/db-only-simple'), tlString('web')),
    'messages.requestSimpleWebView',
  );
}

function rawRequestWebViewButton(): RawRequest {
  const button = new Api.KeyboardButtonWebView({ text: 'Open', url: 'https://mini.example/db-only-button' });
  return new RawRequest(
    0x31a2a35e,
    concatBytes(inputBot().getBytes(), button.getBytes()),
    'bots.requestWebViewButton',
  );
}

function rawGetRequestedWebViewButton(requestId: string): RawRequest {
  return new RawRequest(
    0xbf25b7f3,
    concatBytes(inputBot().getBytes(), tlString(requestId)),
    'bots.getRequestedWebViewButton',
  );
}

function rawSetBotUpdatesStatus(): RawRequest {
  return new RawRequest(
    0xec22cfcd,
    concatBytes(tlInt(2), tlString('db-only-status')),
    'help.setBotUpdatesStatus',
  );
}

function rawSavePreparedInlineMessage(result: any): RawRequest {
  return new RawRequest(
    0xf21f7f2f,
    concatBytes(tlInt(0), result.getBytes(), inputUser().getBytes()),
    'messages.savePreparedInlineMessage',
  );
}

function rawGetPreparedInlineMessage(id: string): RawRequest {
  return new RawRequest(
    0x857ebdb8,
    concatBytes(inputBot().getBytes(), tlString(id)),
    'messages.getPreparedInlineMessage',
  );
}

function queryId(result: any): bigint {
  const candidate = result?.queryId ?? result?.queryId_FLAGINT64 ?? result?.queryId_INT64;
  const value = candidate?.value ?? candidate;
  const parsed = BigInt(value ?? 0);
  if (parsed <= 0n) throw new Error(`missing positive query_id in ${rpcName(result)}`);
  return parsed;
}

async function invoke(client: any, label: string, request: any): Promise<any> {
  try {
    return await client.invoke(request);
  } catch (error) {
    throw new Error(`${label}: ${error instanceof Error ? error.message : String(error)}`);
  }
}

const inlineKey = `inline-bot-query:${botId}`;
const preparedId = `db-only-prepared-${Date.now()}-${process.pid}`;
const preparedKey = `b7:${userId}:${preparedId}`;
const sendRandomId = BigInt(Date.now()) * 1000n + BigInt(process.pid % 1000);

let inlineBefore: string | undefined;
let statusBefore: string | undefined;
let webViewBefore: string[] = [];
let sentMessageIds: number[] = [];
let messagesDeletedViaApi = false;
let userClient: any;
let botClient: any;

function captureState(): void {
  const inline = sqlLines(`SELECT v FROM apifull_kv WHERE k=${quote(inlineKey)}`);
  inlineBefore = inline[0];
  statusBefore = sqlLines(`SELECT pending_updates_count || E'\\t' || encode(convert_to(message,'UTF8'),'base64') FROM apifull_bot_updates_status WHERE bot_user_id=${botId}`)[0];
  webViewBefore = sqlLines(`SELECT request_id FROM apifull_webview_request WHERE user_id=${userId} AND bot_id=${botId} AND kind IN ('requestWebView','requestSimpleWebView','requestWebViewButton')`);
}

function cleanupState(): void {
  try {
    if (sentMessageIds.length > 0 && !messagesDeletedViaApi) {
      const ids = sentMessageIds.join(',');
      sql(`UPDATE messages SET deleted=true WHERE user_id=${userId} AND sender_user_id=${userId} AND user_message_box_id IN (${ids})`);
    }
    sql(`DELETE FROM apifull_kv WHERE k=${quote(preparedKey)}`);
    if (inlineBefore === undefined) {
      sql(`DELETE FROM apifull_kv WHERE k=${quote(inlineKey)}`);
    } else {
      sql(`INSERT INTO apifull_kv(k,v) VALUES (${quote(inlineKey)},${quote(inlineBefore)}) ON CONFLICT (k) DO UPDATE SET v=EXCLUDED.v`);
    }
    if (statusBefore === undefined) {
      sql(`DELETE FROM apifull_bot_updates_status WHERE bot_user_id=${botId}`);
    } else {
      const [count, messageB64] = statusBefore.split('\t');
      const message = Buffer.from(messageB64 || '', 'base64').toString('utf8');
      sql(`INSERT INTO apifull_bot_updates_status(bot_user_id,pending_updates_count,message) VALUES (${botId},${Number(count)},${quote(message)}) ON CONFLICT (bot_user_id) DO UPDATE SET pending_updates_count=EXCLUDED.pending_updates_count,message=EXCLUDED.message,updated_at=CURRENT_TIMESTAMP`);
    }
    const current = sqlLines(`SELECT request_id FROM apifull_webview_request WHERE user_id=${userId} AND bot_id=${botId} AND kind IN ('requestWebView','requestSimpleWebView','requestWebViewButton')`);
    const before = new Set(webViewBefore);
    const added = current.filter((id) => !before.has(id));
    for (const id of added) sql(`DELETE FROM apifull_webview_request WHERE request_id=${quote(id)}`);
  } catch (error) {
    console.error(`cleanup warning: ${error instanceof Error ? error.message : String(error)}`);
  }
}

async function main(): Promise<void> {
  captureState();
  const results: Record<string, any> = {};
  try {
    userClient = makeClient(process.env.USER_AUTH_B64!);
    botClient = makeClient(process.env.BOT_AUTH_B64!);
    await userClient.connect();
    const userMe = await userClient.getMe();
    if (String(userMe?.id) !== userId.toString()) throw new Error(`user auth mismatch: ${String(userMe?.id)}`);
    await botClient.connect();
    const botMe = await botClient.getMe();
    if (String(botMe?.id) !== botId.toString()) throw new Error(`bot auth mismatch: ${String(botMe?.id)}`);

    const webView = await invoke(userClient, 'messages.requestWebView', rawRequestWebView());
    const webQueryId = queryId(webView);
    results.requestWebView = { constructor: rpcName(webView), queryId: webQueryId.toString(), url: webView?.url };
    const prolonged = await invoke(userClient, 'messages.prolongWebView', rawProlongWebView(webQueryId));
    results.prolongWebView = { constructor: rpcName(prolonged), value: prolonged?.value };
    const simple = await invoke(userClient, 'messages.requestSimpleWebView', rawRequestSimpleWebView());
    results.requestSimpleWebView = { constructor: rpcName(simple), url: simple?.url };

    const requestedButton = await invoke(userClient, 'bots.requestWebViewButton', rawRequestWebViewButton());
    const requestId = requestedButton?.webappReqId;
    if (!requestId) throw new Error(`bots.requestWebViewButton returned no webapp_req_id (${rpcName(requestedButton)})`);
    results.requestWebViewButton = { constructor: rpcName(requestedButton), requestId };
    const button = await invoke(userClient, 'bots.getRequestedWebViewButton', rawGetRequestedWebViewButton(requestId));
    results.getRequestedWebViewButton = { constructor: rpcName(button), url: button?.url, text: button?.text };

    // Layer 229 requires the bot account to save a prepared inline result;
    // the user session only reads it back after publication.
    const prepared = await invoke(botClient, 'messages.savePreparedInlineMessage', rawSavePreparedInlineMessage(inlineResult(preparedId, 'prepared')));
    results.savePreparedInlineMessage = { constructor: rpcName(prepared), id: prepared?.id };
    const preparedRead = await invoke(userClient, 'messages.getPreparedInlineMessage', rawGetPreparedInlineMessage(preparedId));
    results.getPreparedInlineMessage = { constructor: rpcName(preparedRead), id: preparedRead?.result?.id };

    const pending = await invoke(userClient, 'messages.getInlineBotResults', rawGetInlineBotResults());
    const inlineQueryId = queryId(pending);
    results.getInlineBotResults = { constructor: rpcName(pending), queryId: inlineQueryId.toString(), resultCount: pending?.results?.length ?? 0 };

    const set = await invoke(botClient, 'messages.setInlineBotResults', rawSetInlineBotResults(inlineQueryId, inlineResult('db-only-result', 'inline')));
    results.setInlineBotResults = { constructor: rpcName(set), value: set?.value };
    const status = await invoke(botClient, 'help.setBotUpdatesStatus', rawSetBotUpdatesStatus());
    results.setBotUpdatesStatus = { constructor: rpcName(status), value: status?.value };

    const ready = await invoke(userClient, 'messages.getInlineBotResults/readback', rawGetInlineBotResults());
    if (queryId(ready) !== inlineQueryId || ready?.results?.length !== 1 || ready.results[0]?.id !== 'db-only-result') {
      throw new Error(`messages.getInlineBotResults readback mismatch: ${JSON.stringify({ queryId: ready?.queryId, count: ready?.results?.length, id: ready?.results?.[0]?.id })}`);
    }
    results.inlineReadback = { constructor: rpcName(ready), queryId: inlineQueryId.toString(), resultCount: ready.results.length };
    const sent = await invoke(userClient, 'messages.sendInlineBotResult', rawSendInlineBotResult(inlineQueryId, sendRandomId, 'db-only-result'));
    results.sendInlineBotResult = { constructor: rpcName(sent), date: sent?.date, seq: sent?.seq };
    sentMessageIds = sqlLines(`SELECT user_message_box_id FROM messages WHERE user_id=${userId} AND sender_user_id=${userId} AND random_id=${sendRandomId}`).map(Number).filter(Number.isFinite);
    if (sentMessageIds.length === 0) throw new Error('messages.sendInlineBotResult returned without a durable message row');
    const deleted = await invoke(userClient, 'messages.deleteMessages', new Api.messages.DeleteMessages({ revoke: true, id: sentMessageIds }));
    messagesDeletedViaApi = true;
    results.deleteMessages = { constructor: rpcName(deleted), ids: sentMessageIds };
  } finally {
    try { userClient?.destroy(); } catch {}
    try { botClient?.destroy(); } catch {}
    cleanupState();
  }

  const remaining = {
    prepared: sql(`SELECT count(*) FROM apifull_kv WHERE k=${quote(preparedKey)}`),
    status: sql(`SELECT count(*) FROM apifull_bot_updates_status WHERE bot_user_id=${botId}`),
    webview: sql(`SELECT count(*) FROM apifull_webview_request WHERE user_id=${userId} AND bot_id=${botId} AND kind IN ('requestWebView','requestSimpleWebView','requestWebViewButton')`),
    messages: sql(`SELECT count(*) FROM messages WHERE user_id=${userId} AND sender_user_id=${userId} AND random_id=${sendRandomId} AND deleted=false`),
  };
  if (Object.values(remaining).some((value) => value !== '0')) throw new Error(`cleanup incomplete: ${JSON.stringify(remaining)}`);
  console.log(JSON.stringify({
    transport: `DC${dcId} TCPObfuscated -> gateway -> session -> APIFull/PostgreSQL`,
    userId: userId.toString(), botId: botId.toString(), results, remaining,
  }));
}

main().then(() => {
  // The authenticated GramJS senders can keep an idle timer alive after the
  // final cleanup assertion. Exit only after the successful JSON evidence has
  // been printed so the acceptance runner cannot turn a passed probe into a timeout.
  process.exit(0);
}).catch((error) => {
  cleanupState();
  console.error(`db-only bot/mini probe failed: ${error instanceof Error ? error.stack || error.message : String(error)}`);
  process.exitCode = 1;
});
