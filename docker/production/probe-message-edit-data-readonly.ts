import { createHash } from 'node:crypto';
import { execFileSync } from 'node:child_process';
import path from 'node:path';
import { createRequire } from 'node:module';

for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) {
  delete process.env[key];
}

const userId = 136907714;
const dcId = Number(process.env.TG_DC_ID || '2');
const dcHost = process.env.TG_DC_HOST || '127.0.0.1';
const dcPort = Number(process.env.TG_DC_PORT || '11443');
const gramjsDir = process.env.TEAMGRAM_GRAMJS_DIR || '/Users/mac/open/test/telegram-tt-master';

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

function mysql(query: string): string {
  return execFileSync('docker', [
    'exec', 'mysql', 'mysql', '-N', '-s', '-uteamgram', '-pteamgram', '-Dteamgram', '-e', query,
  ], { encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'] }).trim();
}

function loadAuthKey(): string {
  const body = mysql(`
    SELECT k.body FROM auth_users u
    JOIN auth_keys k USING (auth_key_id)
    JOIN auth_key_infos i USING (auth_key_id)
    WHERE u.user_id=${userId} AND u.state=0 AND u.deleted=0
      AND k.deleted=0 AND i.deleted=0 AND i.auth_key_type=0
    ORDER BY u.date_active DESC, u.id DESC LIMIT 1
  `);
  if (!body) throw new Error('active production auth key is absent');
  const key = Buffer.from(body, 'base64');
  if (key.length !== 256) throw new Error('production auth key has an invalid length');
  return key.toString('hex');
}

function rpcName(value: any): string {
  return value?.className || value?.constructor?.name || typeof value;
}

function tlInt(value: number): Uint8Array {
  const bytes = new Uint8Array(4);
  new DataView(bytes.buffer).setInt32(0, value, true);
  return bytes;
}

function rawRequest(constructorId: number, ...parts: Uint8Array[]): any {
  return {
    CONSTRUCTOR_ID: constructorId,
    SUBCLASS_OF_ID: 0,
    className: 'messages.GetMessageEditData',
    classType: 'request',
    getBytes: () => Uint8Array.from(Buffer.concat([
      Buffer.from(tlInt(constructorId)),
      ...parts.map((part) => Buffer.from(part)),
    ])),
    readResult: (reader: any) => reader.tgReadObject(),
  };
}

function makeClient(authKeyHex: string): any {
  const session = new CallbackSession({ mainDcId: dcId, keys: { [dcId]: authKeyHex } }, () => {});
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

async function main() {
  const rowBefore = mysql(`
    SELECT c.id, c.access_hash, m.message_id, m.sender_user_id, SHA2(m.message, 256)
    FROM apifull_channel_message m JOIN apifull_channel c ON c.id=m.channel_id
    WHERE c.creator_user_id=${userId} ORDER BY c.id DESC, m.message_id DESC LIMIT 1
  `);
  if (!rowBefore) throw new Error('owned production channel message is absent');
  const [channelID, accessHash, messageID] = rowBefore.split('\t');
  const inputPeer = new Api.InputPeerChannel({ channelId: BigInt(channelID), accessHash: BigInt(accessHash) });
  const client = makeClient(loadAuthKey());
  try {
    await client.connect();
    const me = await client.getMe();
    if (String(me?.id) !== String(userId)) throw new Error('auth key user mismatch');
    const response = await client.invoke(rawRequest(
      0xfda68d36,
      Buffer.from(inputPeer.getBytes()),
      Buffer.from(tlInt(Number(messageID))),
    ));
    const type = rpcName(response);
    if (!type.endsWith('MessageEditData')) throw new Error(`unexpected response type: ${type}`);
    const after = mysql(`
      SELECT c.id, c.access_hash, m.message_id, m.sender_user_id, SHA2(m.message, 256)
      FROM apifull_channel_message m JOIN apifull_channel c ON c.id=m.channel_id
      WHERE c.creator_user_id=${userId} ORDER BY c.id DESC, m.message_id DESC LIMIT 1
    `);
    if (after !== rowBefore) throw new Error('getMessageEditData changed the production message row');
    console.log(JSON.stringify({
      backend: process.env.TEAMGRAM_BACKEND_TAG || 'r25',
      transport: `DC${dcId} WebSocket -> gateway -> session -> Messages BFF -> APIFull/MySQL`,
      method: 'messages.getMessageEditData',
      responseType: type,
      caption: Boolean(response?.caption),
      channelId: channelID,
      messageId: Number(messageID),
      messageRowSha256: createHash('sha256').update(rowBefore).digest('hex'),
      persistedMessageUnchanged: true,
      businessWrites: 0,
    }));
  } finally {
    client.destroy();
  }
}

main().then(() => process.exit(0)).catch((error) => {
  console.error(`message edit data production probe failed: ${error instanceof Error ? error.stack || error.message : String(error)}`);
  process.exit(1);
});
