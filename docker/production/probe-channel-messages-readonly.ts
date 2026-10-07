import { createHash } from 'node:crypto';
import { execFileSync } from 'node:child_process';
import path from 'node:path';
import { createRequire } from 'node:module';

const userId = 136907714;
const outsiderUserId = 136907713;
const gramjsDir = process.env.TEAMGRAM_GRAMJS_DIR || '/Users/mac/open/test/telegram-tt-master';
const dcId = Number(process.env.TG_DC_ID || '2');
const dcHost = process.env.TG_DC_HOST || '127.0.0.1';
const dcPort = Number(process.env.TG_DC_PORT || '11443');

const globalAny = globalThis as typeof globalThis & { self?: any; addEventListener?: () => void };
globalAny.self ??= globalThis;
globalAny.addEventListener ??= () => {};
globalAny.self.addEventListener ??= globalAny.addEventListener;

function mysql(query: string): string {
  return execFileSync('docker', [
    'exec', 'mysql', 'mysql', '-N', '-s', '-uteamgram', '-pteamgram', '-Dteamgram', '-e', query,
  ], { encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'] }).trim();
}

function requiredValue(query: string, name: string): string {
  const value = mysql(query);
  if (!value) throw new Error(`${name} is absent from production MySQL`);
  return value;
}

const requireModule = createRequire(import.meta.url);
const TelegramClient = requireModule(path.join(gramjsDir, 'src/lib/gramjs/client/TelegramClient.ts')).default;
const CallbackSession = requireModule(path.join(gramjsDir, 'src/lib/gramjs/sessions/CallbackSession.ts')).default;
const Api = requireModule(path.join(gramjsDir, 'src/lib/gramjs/tl/index.ts')).Api;
const Connection = requireModule(path.join(gramjsDir, 'src/lib/gramjs/network/connection/TCPObfuscated.ts')).ConnectionTCPObfuscated;
const quietLogger = { debug() {}, info() {}, warn() {}, error() {} };

function rpcName(value: any): string {
  return value?.className || value?.constructor?.name || typeof value;
}

function sha256(value: string): string {
  return createHash('sha256').update(value).digest('hex');
}

function makeClient(authKeyB64: string): any {
  const session = new CallbackSession({
    mainDcId: dcId,
    keys: { [dcId]: Buffer.from(authKeyB64, 'base64').toString('hex') },
  }, () => {});
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

async function expectRpcError(name: string, code: string, action: () => Promise<unknown>): Promise<void> {
  try {
    await action();
  } catch (error) {
    if (!String(error).includes(code)) throw new Error(`${name} returned ${String(error)}, expected ${code}`);
    return;
  }
  throw new Error(`${name} unexpectedly succeeded`);
}

async function main() {
  const authKey = requiredValue(`
    SELECT k.body
    FROM auth_users u
    JOIN auth_keys k USING (auth_key_id)
    JOIN auth_key_infos i USING (auth_key_id)
    WHERE u.user_id=${userId} AND u.state=0 AND u.deleted=0
      AND k.deleted=0 AND i.deleted=0 AND i.auth_key_type=0
    ORDER BY u.date_active DESC,u.id DESC
    LIMIT 1
  `, 'active production auth key');
  const outsiderAuthKey = requiredValue(`
    SELECT k.body
    FROM auth_users u
    JOIN auth_keys k USING (auth_key_id)
    JOIN auth_key_infos i USING (auth_key_id)
    WHERE u.user_id=${outsiderUserId} AND u.state=0 AND u.deleted=0
      AND k.deleted=0 AND i.deleted=0 AND i.auth_key_type=0
    ORDER BY u.date_active DESC,u.id DESC
    LIMIT 1
  `, 'active outsider production auth key');
  const queryMessage = `
    SELECT c.id, c.access_hash, m.message_id, m.sender_user_id, m.date, SHA2(m.message, 256)
    FROM apifull_channel_message m
    JOIN apifull_channel c ON c.id=m.channel_id
    WHERE c.creator_user_id=${userId}
    ORDER BY c.id DESC, m.message_id DESC
    LIMIT 1
  `;
  const [rawChannelId, rawAccessHash, rawMessageId, rawSender, rawDate, contentHash] =
    requiredValue(queryMessage, 'owned production channel message').split('\t');
  const channelId = BigInt(rawChannelId);
  const accessHash = BigInt(rawAccessHash);
  const messageId = Number(rawMessageId);
  const senderId = BigInt(rawSender);
  const messageDate = Number(rawDate);
  if (!Number.isSafeInteger(messageId) || !Number.isSafeInteger(messageDate)) {
    throw new Error('production channel message contains an invalid numeric field');
  }
  const historyQuery = `
    SELECT m.message_id, m.sender_user_id, m.date, SHA2(m.message, 256)
    FROM apifull_channel_message m
    WHERE m.channel_id=${channelId} AND NOT EXISTS (
      SELECT 1 FROM apifull_channel_message_hidden h
      WHERE h.user_id=${userId} AND h.channel_id=m.channel_id AND h.message_id=m.message_id
    )
    ORDER BY m.message_id DESC
    LIMIT 20
  `;
  const expectedHistory = mysql(historyQuery).split('\n').filter(Boolean).map((line) => {
    const [id, sender, date, hash] = line.split('\t');
    return { id: Number(id), sender: BigInt(sender), date: Number(date), hash };
  });
  if (expectedHistory.length === 0) throw new Error('production channel history is unexpectedly empty');

  const client = makeClient(authKey);
  let outsiderClient: any;
  try {
    await client.connect();
    const me = await client.getMe();
    if (String(me?.id) !== String(userId)) throw new Error('auth key user mismatch');
    const inputChannel = new Api.InputChannel({ channelId, accessHash });
    const request = (channel: any) => new Api.channels.GetMessages({
      channel,
      id: [new Api.InputMessageID({ id: messageId })],
    });
    const result = await client.invoke(request(inputChannel));
    const actualType = rpcName(result);
    const messages = Array.isArray(result?.messages) ? result.messages : [];
    const message = messages[0];
    const messageTextHash = message ? sha256(String(message.message ?? '')) : '';
    const chats = Array.isArray(result?.chats) ? result.chats : [];
    if (!actualType.endsWith('Messages')
      || messages.length !== 1
      || Number(message?.id) !== messageId
      || BigInt(message?.fromId?.userId ?? 0) !== senderId
      || BigInt(message?.peerId?.channelId ?? 0) !== channelId
      || Number(message?.date) !== messageDate
      || messageTextHash !== contentHash
      || !chats.some((chat: any) => BigInt(chat?.id ?? 0) === channelId)) {
      throw new Error(`channel message response mismatch: type=${actualType}, messages=${messages.length}, id=${String(message?.id)}, hashMatch=${messageTextHash === contentHash}`);
    }
    await expectRpcError('wrong access hash', 'CHANNEL_INVALID', () => client.invoke(request(
      new Api.InputChannel({ channelId, accessHash: accessHash + 1n }),
    )));

    outsiderClient = makeClient(outsiderAuthKey);
    await outsiderClient.connect();
    const outsiderMe = await outsiderClient.getMe();
    if (String(outsiderMe?.id) !== String(outsiderUserId)) throw new Error('outsider auth key user mismatch');
    await expectRpcError('nonmember channel read', 'USER_NOT_PARTICIPANT', () => outsiderClient.invoke(request(inputChannel)));

    const inputPeer = new Api.InputPeerChannel({ channelId, accessHash });
    const historyRequest = (peer: any) => new Api.messages.GetHistory({
      peer,
      offsetId: 0,
      offsetDate: 0,
      addOffset: 0,
      limit: 20,
      maxId: 0,
      minId: 0,
      hash: 0n,
    });
    const history = await client.invoke(historyRequest(inputPeer));
    const historyType = rpcName(history);
    const historyMessages = Array.isArray(history?.messages) ? history.messages : [];
    const historyMatches = historyMessages.length === expectedHistory.length
      && historyMessages.every((item: any, index: number) => {
        const expected = expectedHistory[index];
        return Number(item?.id) === expected.id
          && BigInt(item?.fromId?.userId ?? 0) === expected.sender
          && BigInt(item?.peerId?.channelId ?? 0) === channelId
          && Number(item?.date) === expected.date
          && sha256(String(item?.message ?? '')) === expected.hash;
      });
    if (!historyType.endsWith('Messages') || !historyMatches) {
      throw new Error(`messages.getHistory mismatch: type=${historyType}, returned=${historyMessages.length}, mysql=${expectedHistory.length}`);
    }
    if (!Array.isArray(history?.chats) || !history.chats.some((chat: any) => BigInt(chat?.id ?? 0) === channelId)) {
      throw new Error('messages.getHistory omitted the channel entity');
    }
    await expectRpcError('history wrong access hash', 'CHANNEL_INVALID', () => client.invoke(historyRequest(
      new Api.InputPeerChannel({ channelId, accessHash: accessHash + 1n }),
    )));
    await expectRpcError('history outsider read', 'USER_NOT_PARTICIPANT', () => outsiderClient.invoke(historyRequest(inputPeer)));

    const rowAfter = mysql(queryMessage);
    const rowBefore = [rawChannelId, rawAccessHash, rawMessageId, rawSender, rawDate, contentHash].join('\t');
    if (rowAfter !== rowBefore) throw new Error('read-only getMessages changed the persisted channel message row');
    if (mysql(historyQuery) !== expectedHistory.map((row) => [row.id, row.sender, row.date, row.hash].join('\t')).join('\n')) {
      throw new Error('read-only history changed the persisted channel message rows');
    }
    console.log(JSON.stringify({
      userId: String(me.id),
      transport: `DC${dcId} WebSocket -> gateway -> session -> BFF -> APIFull/MySQL`,
      resultType: actualType,
      channelId: String(channelId),
      messageId,
      senderUserId: String(senderId),
      date: messageDate,
      contentHashMatched: true,
      channelEntityIncluded: true,
      wrongHashRejected: true,
      nonmemberRejected: true,
      persistedMessageUnchanged: true,
      getHistory: {
        type: historyType,
        messages: historyMessages.length,
        mysqlMessages: expectedHistory.length,
        newestFirst: true,
        allFieldsMatched: true,
      },
      persistedHistoryUnchanged: true,
      writes: 0,
    }));
  } finally {
    outsiderClient?.destroy();
    client.destroy();
  }
}

main().then(() => process.exit(0)).catch((error) => {
  console.error(`channel messages production probe failed: ${error instanceof Error ? error.stack || error.message : String(error)}`);
  process.exit(1);
});
