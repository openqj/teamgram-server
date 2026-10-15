import { execFileSync } from 'node:child_process';
import path from 'node:path';
import { createRequire } from 'node:module';

const gramjsDir = process.env.TEAMGRAM_GRAMJS_DIR || '/Users/mac/open/test/telegram-tt-master';
const userId = process.env.MESSAGE_AUTHOR_USER_ID || '7';
const dcId = Number(process.env.TG_DC_ID || '2');
const dcHost = process.env.TG_DC_HOST || '127.0.0.1';
const dcPort = Number(process.env.TG_DC_PORT || '11443');

const globalAny = globalThis as typeof globalThis & { self?: any; addEventListener?: () => void };
globalAny.self ??= globalThis;
globalAny.addEventListener ??= () => {};
globalAny.self.addEventListener ??= globalAny.addEventListener;

type Fixture = {
  channelMessageId: number;
  channelId: string;
  channelAccessHash: string;
  senderUserId: string;
};

if (!/^\d+$/.test(userId)) throw new Error('MESSAGE_AUTHOR_USER_ID must be numeric');

function sql(query: string): string {
  return execFileSync('docker', [
    'exec', 'teamgram-postgres', 'psql', '-v', 'ON_ERROR_STOP=1', '-At', '-q', '-F', '\t', '-U', 'teamgram', '-d', 'teamgram', '-c', query,
  ], { encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'] }).trim();
}

function loadAuthKey(): string {
  const body = sql(`SELECT encode(k.body, 'base64') FROM auth_users u JOIN auth_keys k USING(auth_key_id)
    JOIN auth_key_infos i USING(auth_key_id) WHERE u.user_id=${userId} AND u.state=0 AND u.deleted=FALSE
    AND k.deleted=FALSE AND i.deleted=FALSE AND i.auth_key_type=0 ORDER BY u.date_active DESC,u.id DESC LIMIT 1`);
  if (!body) throw new Error(`no active production auth key found for user ${userId}`);
  return Buffer.from(body, 'base64').toString('hex');
}

function loadFixture(): Fixture {
  const raw = sql(`SELECT json_build_object(
      'channelMessageId', cm.message_id,
      'channelId', c.id::text,
      'channelAccessHash', c.access_hash::text,
      'senderUserId', cm.sender_user_id::text)
    FROM apifull_channel c
    JOIN apifull_channel_message cm ON cm.channel_id=c.id
    WHERE c.creator_user_id=${userId} AND cm.message_id > 0
    ORDER BY c.id, cm.message_id LIMIT 1`);
  if (!raw) throw new Error(`no owned production channel message found for user ${userId}`);
  const fixture = JSON.parse(raw) as Fixture;
  if (!Number.isSafeInteger(fixture.channelMessageId)) {
    throw new Error('production channel message IDs are invalid');
  }
  return fixture;
}

function postgresSnapshot(fixture: Fixture): string {
  const raw = sql(`SELECT json_build_object(
      'message', encode(digest(CONCAT_WS('|',cm.channel_id,cm.message_id,cm.sender_user_id,
        cm.date,cm.message,cm.edited,cm.edited_at,cm.reply_to_msg_id,cm.reply_to_top_id,
        cm.content_json,cm.pinned)::text, 'sha256'), 'hex'),
      'channel', encode(digest(CONCAT_WS('|',c.id,c.access_hash,c.creator_user_id,c.title,c.username)::text, 'sha256'), 'hex'))
    FROM apifull_channel_message cm
    JOIN apifull_channel c ON c.id=cm.channel_id
    WHERE cm.channel_id=${fixture.channelId} AND cm.message_id=${fixture.channelMessageId} LIMIT 1`);
  if (!raw) throw new Error('production message/channel snapshot could not be read back');
  return raw;
}

const requireModule = createRequire(import.meta.url);
const TelegramClient = requireModule(path.join(gramjsDir, 'src/lib/gramjs/client/TelegramClient.ts')).default;
const CallbackSession = requireModule(path.join(gramjsDir, 'src/lib/gramjs/sessions/CallbackSession.ts')).default;
const Api = requireModule(path.join(gramjsDir, 'src/lib/gramjs/tl/index.ts')).Api;
const Connection = requireModule(path.join(gramjsDir, 'src/lib/gramjs/network/connection/TCPObfuscated.ts')).ConnectionTCPObfuscated;

function rpcName(value: any): string {
  return value?.className || value?.constructor?.name || typeof value;
}

function makeClient(authKeyHex: string): any {
  const session = new CallbackSession({ mainDcId: dcId, keys: { [dcId]: authKeyHex } }, () => {});
  const load = session.load.bind(session);
  session.load = async () => {
    await load();
    (session as any)._serverAddress = dcHost;
    (session as any)._port = dcPort;
  };
  return new TelegramClient(session, 1, '0123456789abcdef0123456789abcdef', {
    connection: Connection,
    fallbackConnection: Connection,
    shouldAllowHttpTransport: false,
    shouldForceHttpTransport: false,
    timeout: 20,
    requestRetries: 1,
    connectionRetries: 1,
    connectionRetriesToFallback: 0,
    autoReconnect: false,
    baseLogger: { debug() {}, info() {}, warn() {}, error() {} },
  });
}

function makeGetMessageAuthorRequest(channel: any, id: number): any {
  const request: any = new Api.channels.GetFullChannel({ channel });
  request.className = 'channels.GetMessageAuthor';
  request.CONSTRUCTOR_ID = 0xece2a0e6;
  request.id = id;
  request.getBytes = () => {
    const channelBytes = channel.getBytes();
    const encoded = new Uint8Array(8 + channelBytes.length);
    const view = new DataView(encoded.buffer);
    view.setUint32(0, 0xece2a0e6, true);
    encoded.set(channelBytes, 4);
    view.setInt32(4 + channelBytes.length, id, true);
    return encoded;
  };
  request.readResult = (reader: any) => reader.tgReadObject();
  return request;
}

async function main() {
  const fixture = loadFixture();
  const before = postgresSnapshot(fixture);
  const client = makeClient(loadAuthKey());
  try {
    (client as any)._borrowExportedSender = async () => undefined;
    (client as any)._updateLoop = async () => undefined;
    await client.connect();
    const me = await client.getMe();
    if (String(me?.id) !== userId) throw new Error('auth key user mismatch');

    const inputChannel = new Api.InputChannel({
      channelId: BigInt(fixture.channelId),
      accessHash: BigInt(fixture.channelAccessHash),
    });
    const result = await client.invoke(makeGetMessageAuthorRequest(inputChannel, fixture.channelMessageId));
    const resultType = rpcName(result);
    if (resultType !== 'User' || String(result?.id) !== fixture.senderUserId) {
      throw new Error(`native channel message ID returned ${resultType} id=${String(result?.id)}`);
    }

    const missingMessageId = fixture.channelMessageId + 1000000;
    const boxIDResult = await client.invoke(makeGetMessageAuthorRequest(inputChannel, missingMessageId));
    const boxIDResultType = rpcName(boxIDResult);
    if (boxIDResultType !== 'UserEmpty') {
      throw new Error(`missing native message ID returned ${boxIDResultType}, expected UserEmpty`);
    }

    let accessError = '';
    try {
      await client.invoke(makeGetMessageAuthorRequest(new Api.InputChannel({
        channelId: BigInt(fixture.channelId),
        accessHash: BigInt(fixture.channelAccessHash) + 1n,
      }), fixture.channelMessageId));
    } catch (error: any) {
      accessError = String(error?.errorMessage || error?.message || error);
    }
    if (!accessError.includes('CHANNEL_INVALID')) {
      throw new Error(`invalid channel access hash returned ${accessError || 'success'}, expected CHANNEL_INVALID`);
    }

    const after = postgresSnapshot(fixture);
    if (before !== after) throw new Error('production message or channel rows changed during read-only probe');
    console.log(JSON.stringify({
      userId: String(me.id),
      rpc: 'channels.getMessageAuthor#ece2a0e6',
      productionChannelId: fixture.channelId,
      channelMessageId: fixture.channelMessageId,
      missingMessageId,
      senderUserId: fixture.senderUserId,
      resultType,
      resultUserId: String(result.id),
      missingMessageIdResultType: boxIDResultType,
      invalidAccessHashError: accessError,
      postgresReadbackSha256: JSON.parse(after),
      writes: 0,
    }));
  } finally {
    client.destroy();
  }
}

main().then(() => process.exit(0)).catch((error) => {
  console.error(`channels.getMessageAuthor production probe failed: ${error instanceof Error ? error.stack || error.message : String(error)}`);
  process.exit(1);
});
