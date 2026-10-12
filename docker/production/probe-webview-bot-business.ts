import { createHash } from 'node:crypto';
import { execFileSync } from 'node:child_process';
import path from 'node:path';
import { createRequire } from 'node:module';

for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) {
  delete process.env[key];
}

const gramjsDir = process.env.TEAMGRAM_GRAMJS_DIR || '/Users/mac/open/test/telegram-tt-master';
const dcId = Number(process.env.TG_DC_ID || '2');
const dcHost = process.env.TG_DC_HOST || '127.0.0.1';
const dcPort = Number(process.env.TG_DC_PORT || '11443');
const userId = BigInt(process.env.WEBVIEW_USER_ID || '10');
const botId = BigInt(process.env.WEBVIEW_BOT_ID || '900000000001');
const botAccessHash = BigInt(process.env.WEBVIEW_BOT_ACCESS_HASH || '910000000001');
const userAccessHash = BigInt(process.env.WEBVIEW_USER_ACCESS_HASH || '900000000000000010');
const botToken = process.env.WEBVIEW_BOT_TOKEN || 'web-e2e-token';
const apiId = Number(process.env.TG_API_ID || '1');
const apiHash = process.env.TG_API_HASH || '0123456789abcdef0123456789abcdef';

const globalAny = globalThis as typeof globalThis & { self?: any; addEventListener?: () => void };
globalAny.self ??= globalThis;
globalAny.addEventListener ??= () => {};
globalAny.self.addEventListener ??= globalAny.addEventListener;

const requireModule = createRequire(import.meta.url);
const TelegramClient = requireModule(path.join(gramjsDir, 'src/lib/gramjs/client/TelegramClient.ts')).default;
const CallbackSession = requireModule(path.join(gramjsDir, 'src/lib/gramjs/sessions/CallbackSession.ts')).default;
const MemorySession = requireModule(path.join(gramjsDir, 'src/lib/gramjs/sessions/Memory.ts')).default;
const Api = requireModule(path.join(gramjsDir, 'src/lib/gramjs/tl/index.ts')).Api;
const serializeBytes = requireModule(path.join(gramjsDir, 'src/lib/gramjs/tl/index.ts')).serializeBytes;
const Connection = requireModule(path.join(gramjsDir, 'src/lib/gramjs/network/connection/TCPObfuscated.ts')).ConnectionTCPObfuscated;
const quietLogger = { debug() {}, info() {}, warn() {}, error() {} };

// The checked-in GramJS runtime uses a reduced schema and may omit this
// Layer 229 method even though its declaration file contains it. Keep the
// probe able to exercise the real wire method without widening the client
// bundle used by the Web UI.
class SendBotRequestedPeerCompat {
  static CONSTRUCTOR_ID = 0x6c5cf2a7;
  static className = 'messages.sendBotRequestedPeer';
  static classType = 'request';
  CONSTRUCTOR_ID = SendBotRequestedPeerCompat.CONSTRUCTOR_ID;
  className = SendBotRequestedPeerCompat.className;
  classType = SendBotRequestedPeerCompat.classType;
  peer: any;
  msgId?: number;
  webappReqId?: string;
  buttonId: number;
  requestedPeers: any[];

  constructor(args: any) {
    Object.assign(this, args);
  }

  getBytes(): Uint8Array {
    const flags = (this.msgId === undefined ? 0 : 1) | (this.webappReqId === undefined ? 0 : 2);
    const int32 = (value: number) => {
      const bytes = Buffer.alloc(4);
      bytes.writeInt32LE(value);
      return bytes;
    };
    const uint32 = (value: number) => {
      const bytes = Buffer.alloc(4);
      bytes.writeUInt32LE(value >>> 0);
      return bytes;
    };
    const parts: Uint8Array[] = [uint32(this.CONSTRUCTOR_ID), int32(flags), this.peer.getBytes()];
    if ((flags & 1) !== 0) parts.push(int32(this.msgId!));
    if ((flags & 2) !== 0) parts.push(serializeBytes(this.webappReqId!));
    parts.push(int32(this.buttonId), uint32(0x1cb5c415), int32(this.requestedPeers.length));
    for (const peer of this.requestedPeers) parts.push(peer.getBytes());
    return Buffer.concat(parts.map((part) => Buffer.from(part)));
  }

  readResult(reader: any): any {
    return reader.tgReadObject();
  }
}

class ImportBotAuthorizationCompat {
  static CONSTRUCTOR_ID = 0x67a3ff2c;
  static className = 'auth.importBotAuthorization';
  static classType = 'request';
  CONSTRUCTOR_ID = ImportBotAuthorizationCompat.CONSTRUCTOR_ID;
  className = ImportBotAuthorizationCompat.className;
  classType = ImportBotAuthorizationCompat.classType;
  apiId: number;
  apiHash: string;
  botAuthToken: string;

  constructor(args: any) {
    Object.assign(this, args);
  }

  getBytes(): Uint8Array {
    const int32 = (value: number) => {
      const bytes = Buffer.alloc(4);
      bytes.writeInt32LE(value);
      return bytes;
    };
    return Buffer.concat([
      Buffer.from(new Uint8Array([0x2c, 0xff, 0xa3, 0x67])),
      int32(0),
      int32(this.apiId),
      Buffer.from(serializeBytes(this.apiHash)),
      Buffer.from(serializeBytes(this.botAuthToken)),
    ]);
  }

  readResult(reader: any): any {
    return reader.tgReadObject();
  }
}

class SetBotCallbackAnswerCompat {
  static CONSTRUCTOR_ID = 0xd58f130a;
  static className = 'messages.setBotCallbackAnswer';
  static classType = 'request';
  CONSTRUCTOR_ID = SetBotCallbackAnswerCompat.CONSTRUCTOR_ID;
  className = SetBotCallbackAnswerCompat.className;
  classType = SetBotCallbackAnswerCompat.classType;
  queryId: bigint;
  alert?: boolean;
  message?: string;
  url?: string;
  cacheTime: number;

  constructor(args: any) {
    Object.assign(this, args);
  }

  getBytes(): Uint8Array {
    const flags = (this.alert ? 1 << 1 : 0) | (this.message === undefined ? 0 : 1) | (this.url === undefined ? 0 : 1 << 2);
    const int32 = (value: number) => {
      const bytes = Buffer.alloc(4);
      bytes.writeInt32LE(value);
      return bytes;
    };
    const long = (value: bigint) => {
      const bytes = Buffer.alloc(8);
      bytes.writeBigInt64LE(BigInt.asIntN(64, value));
      return bytes;
    };
    const parts: Uint8Array[] = [
      new Uint8Array([0x0a, 0x13, 0x8f, 0xd5]),
      int32(flags),
      long(this.queryId),
    ];
    if (this.message !== undefined) parts.push(serializeBytes(this.message));
    if (this.url !== undefined) parts.push(serializeBytes(this.url));
    parts.push(int32(this.cacheTime));
    return Buffer.concat(parts.map((part) => Buffer.from(part)));
  }

  readResult(reader: any): any {
    return reader.tgReadObject();
  }
}

const SendBotRequestedPeer = Api.messages.SendBotRequestedPeer ?? SendBotRequestedPeerCompat;
const ImportBotAuthorization = Api.auth.ImportBotAuthorization ?? ImportBotAuthorizationCompat;
const SetBotCallbackAnswer = Api.messages.SetBotCallbackAnswer ?? SetBotCallbackAnswerCompat;

function sql(query: string): string {
  return execFileSync('docker', [
    'exec', 'teamgram-postgres', 'psql', '-v', 'ON_ERROR_STOP=1', '-At', '-q', '-F', '\t',
    '-U', 'teamgram', '-d', 'teamgram', '-c', query,
  ], { encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] }).trim();
}

function rpcName(value: any): string {
  return value?.className || value?.constructor?.name || typeof value;
}

function quote(value: string): string {
  return `'${value.replaceAll("'", "''")}'`;
}

function loadAuthKey(id: bigint): string {
  const body = sql(`SELECT encode(k.body, 'base64') FROM auth_users u
    JOIN auth_keys k USING (auth_key_id)
    JOIN auth_key_infos i USING (auth_key_id)
    WHERE u.user_id=${id} AND u.state=0 AND NOT u.deleted AND NOT k.deleted AND NOT i.deleted
      AND i.auth_key_type=0 ORDER BY u.date_active DESC,u.id DESC LIMIT 1`);
  if (!body) throw new Error(`no active auth key for ${id}`);
  const key = Buffer.from(body, 'base64');
  if (key.length !== 256) throw new Error(`invalid auth key length for ${id}: ${key.length}`);
  return key.toString('hex');
}

function makePermanentClient(authKeyHex: string): any {
  const session = new CallbackSession({ mainDcId: dcId, keys: { [dcId]: authKeyHex } }, () => {});
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
    autoReconnect: false,
    baseLogger: quietLogger,
  });
  (client as any)._borrowExportedSender = async () => undefined;
  (client as any)._updateLoop = async () => undefined;
  return client;
}

function makeBotClient(): any {
  const session = new MemorySession();
  session.setDC(dcId, dcHost, dcPort);
  const client = new TelegramClient(session, apiId, apiHash, {
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

function inputBot(): any {
  return new Api.InputUser({ userId: botId, accessHash: botAccessHash });
}

function inputUser(): any {
  return new Api.InputUser({ userId, accessHash: userAccessHash });
}

function digestData(data: string): string {
  // Go's businessControlHash uses encoding/json on []byte, which encodes the
  // bytes as a base64 JSON string before hashing. Keep the probe's scope key
  // identical to the handler's canonical Layer 229 lookup.
  const encoded = Buffer.from(data).toString('base64');
  return createHash('sha256').update(JSON.stringify(encoded)).digest('hex');
}

function updatesJson(date = Math.floor(Date.now() / 1000)): string {
  return JSON.stringify({ predicate_name: 'updates', updates: [], users: [], chats: [], date, seq: 0 });
}

function insert(operation: string, scope: string, caller: bigint, result: string): void {
  sql(`INSERT INTO apifull_business_control_plane
    (operation, scope_key, caller_user_id, state, result, updated_at)
    VALUES (${quote(operation)}, ${quote(scope)}, ${caller}, 'published', ${quote(result)}::jsonb, CURRENT_TIMESTAMP)
    ON CONFLICT (operation, scope_key, caller_user_id) DO UPDATE SET state='published',result=EXCLUDED.result,updated_at=CURRENT_TIMESTAMP`);
}

function cleanup(): void {
  if (process.env.KEEP_ROWS === '1') return;
  sql(`DELETE FROM apifull_business_control_plane WHERE caller_user_id IN (0,${userId},${botId})
    AND operation IN ('messages.getBotCallbackAnswer','messages.setBotCallbackAnswer','messages.sendWebViewResultMessage',
      'messages.sendWebViewData','messages.getBotApp','messages.requestAppWebView','messages.sendBotRequestedPeer',
      'messages.sendPaidReaction','messages.toggleSuggestedPostApproval')`);
  sql(`DELETE FROM apifull_webview_request WHERE user_id=${userId} AND bot_id=${botId} AND kind='requestAppWebView'`);
}

async function main() {
  const callbackData = 'approve';
  const callbackMsgId = 17;
  const callbackScope = `peer:1:${userId}:msg:${callbackMsgId}:data:sha256:${digestData(callbackData)}`;
  const webDataRandomId = 880001n;
  const requestedScope = `peer:1:${userId}:msg:${callbackMsgId}:button:7`;
  const paidRandomId = 880002n;
  const paidPeerScope = `peer:2:${userId}:msg:3:random:${paidRandomId}:count:2`;
  const suggestedScope = `peer:1:${userId}:msg:44`;
  const appKey = `bot:${botId}:short:demo`;
  const appWebViewScope = `${appKey}:peer:1:${userId}:start:start:platform:web`;
  const botQuery = 'webview-query-1';

  cleanup();
  insert('messages.getBotCallbackAnswer', callbackScope, userId,
    JSON.stringify({ predicate_name: 'messages_botCallbackAnswer', message: { value: 'approved' }, cache_time: 30 }));
  insert('messages.sendWebViewData', `bot:${botId}:random:${webDataRandomId}`, userId, updatesJson());
  insert('messages.getBotApp', appKey, 0n, JSON.stringify({
    predicate_name: 'messages_botApp', app: {
      predicate_name: 'botApp', id: 7001, access_hash: 7001001, short_name: 'demo',
      title: 'Demo', description: 'Demo app', hash: 1, photo: { predicate_name: 'photoEmpty', id: 1 },
    },
  }));
  insert('messages.requestAppWebView', appWebViewScope, userId,
    JSON.stringify({ predicate_name: 'webViewResultUrl', constructor: 202659196, query_id_INT64: 880003, url: 'https://mini.example/app' }));
  insert('messages.sendBotRequestedPeer', requestedScope, userId, updatesJson());
  insert('messages.sendPaidReaction', paidPeerScope, userId, updatesJson());
  insert('messages.toggleSuggestedPostApproval', suggestedScope, userId, updatesJson());
  insert('messages.sendWebViewResultMessage', `bot-query:${botQuery}`, botId,
    JSON.stringify({ predicate_name: 'webViewMessageSent' }));
  insert('messages.setBotCallbackAnswer', 'query:880004', botId, JSON.stringify({ value: true }));

  const userClient = makePermanentClient(loadAuthKey(userId));
  const botClient = makeBotClient();
  const results: Record<string, any> = {};
  try {
    await userClient.connect();
    const me = await userClient.getMe();
    if (String(me?.id) !== userId.toString()) throw new Error(`user auth mismatch: ${String(me?.id)}`);

    const callback = await userClient.invoke(new Api.messages.GetBotCallbackAnswer({
      peer: selfPeer(), msgId: callbackMsgId, data: Buffer.from(callbackData),
    }));
    results.getBotCallbackAnswer = { constructor: rpcName(callback), cacheTime: callback?.cacheTime, message: callback?.message };

    const webData = await userClient.invoke(new Api.messages.SendWebViewData({
      bot: inputBot(), randomId: webDataRandomId, buttonText: 'Open', data: 'payload',
    }));
    results.sendWebViewData = { constructor: rpcName(webData), date: webData?.date, seq: webData?.seq };

    const app = await userClient.invoke(new Api.messages.GetBotApp({
      app: new Api.InputBotAppShortName({ botId: inputBot(), shortName: 'demo' }), hash: BigInt(1),
    }));
    results.getBotApp = { constructor: rpcName(app), appId: app?.app?.id, shortName: app?.app?.shortName };

    const appWebView = await userClient.invoke(new Api.messages.RequestAppWebView({
      peer: selfPeer(), app: new Api.InputBotAppShortName({ botId: inputBot(), shortName: 'demo' }),
      startParam: 'start', platform: 'web',
    }));
    results.requestAppWebView = { constructor: rpcName(appWebView), queryId: appWebView?.queryId?.toString?.(), url: appWebView?.url };
    results.webViewRequestRows = sql(`SELECT count(*) FROM apifull_webview_request WHERE user_id=${userId} AND bot_id=${botId} AND kind='requestAppWebView'`);

    const requested = await userClient.invoke(new SendBotRequestedPeer({
      peer: selfPeer(), msgId: callbackMsgId, buttonId: 7, requestedPeers: [selfPeer()],
    }));
    results.sendBotRequestedPeer = { constructor: rpcName(requested), date: requested?.date, seq: requested?.seq };

    const paid = await userClient.invoke(new Api.messages.SendPaidReaction({
      peer: new Api.InputPeerUser({ userId, accessHash: userAccessHash }), msgId: 3, count: 2, randomId: paidRandomId,
    }));
    results.sendPaidReaction = { constructor: rpcName(paid), date: paid?.date, seq: paid?.seq };

    const suggested = await userClient.invoke(new Api.messages.ToggleSuggestedPostApproval({
      peer: selfPeer(), msgId: 44,
    }));
    results.toggleSuggestedPostApproval = { constructor: rpcName(suggested), date: suggested?.date, seq: suggested?.seq };

    await botClient.connect();
    const authorization = await botClient.invoke(new ImportBotAuthorization({
      apiId, apiHash, botAuthToken: botToken,
    }));
    const botMe = authorization?.user || await botClient.getMe();
    if (String(botMe?.id) !== botId.toString()) throw new Error(`bot auth mismatch: ${String(botMe?.id)}`);
    results.botAuthorization = { constructor: rpcName(authorization), userId: String(botMe?.id), username: botMe?.username };

    const sent = await botClient.invoke(new Api.messages.SendWebViewResultMessage({
      botQueryId: botQuery,
      result: new Api.InputBotInlineResult({
        id: 'result-1', type: 'article',
        sendMessage: new Api.InputBotInlineMessageText({ message: 'hello' }),
      }),
    }));
    results.sendWebViewResultMessage = { constructor: rpcName(sent), msgId: sent?.msgId?.id?.toString?.() };

    const answer = await botClient.invoke(new SetBotCallbackAnswer({
      queryId: BigInt(880004), message: 'accepted', cacheTime: 30,
    }));
    results.setBotCallbackAnswer = { constructor: rpcName(answer), value: answer?.value };
  } finally {
    userClient.destroy();
    botClient.destroy();
    cleanup();
  }
  console.log(JSON.stringify({
    transport: `DC${dcId} TCPObfuscated -> gateway -> session -> APIFull/PostgreSQL`,
    userId: userId.toString(), botId: botId.toString(), results,
    controlPlaneRowsAfterCleanup: sql(`SELECT count(*) FROM apifull_business_control_plane WHERE caller_user_id IN (0,${userId},${botId}) AND operation LIKE 'messages.%'`),
  }, (_key, value) => typeof value === 'bigint' ? value.toString() : value));
}

main().then(() => process.exit(0)).catch((error) => {
  try { cleanup(); } catch { /* best effort */ }
  console.error(`webview/bot business probe failed: ${error instanceof Error ? error.stack || error.message : String(error)}`);
  process.exit(1);
});
