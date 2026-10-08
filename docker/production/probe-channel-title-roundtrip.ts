import { randomBytes } from 'node:crypto';
import { execFileSync } from 'node:child_process';
import path from 'node:path';
import { createRequire } from 'node:module';

for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) {
  delete process.env[key];
}

const userId = process.env.CHANNEL_TITLE_USER_ID || '136907714';
const dcId = Number(process.env.TG_DC_ID || '2');
const dcHost = process.env.TG_DC_HOST || '127.0.0.1';
const dcPort = Number(process.env.TG_DC_PORT || '11443');
const gramjsDir = process.env.TEAMGRAM_GRAMJS_DIR || '/Users/mac/open/test/telegram-tt-master';

const globalAny = globalThis as typeof globalThis & { self?: any; addEventListener?: () => void };
globalAny.self ??= globalThis;
globalAny.addEventListener ??= () => {};
globalAny.self.addEventListener ??= globalAny.addEventListener;

function postgres(query: string): string {
  return execFileSync('docker', [
    'exec', 'teamgram-postgres', 'psql', '-v', 'ON_ERROR_STOP=1', '-At', '-q', '-F', '\t', '-U', 'teamgram', '-d', 'teamgram', '-c', query,
  ], { encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'] }).trim();
}

function loadAuthKey(): string {
  const body = postgres(`
    SELECT k.body
    FROM auth_users u
    JOIN auth_keys k USING (auth_key_id)
    JOIN auth_key_infos i USING (auth_key_id)
    WHERE u.user_id=${userId} AND u.state=0 AND u.deleted=0
      AND k.deleted=0 AND i.deleted=0 AND i.auth_key_type=0
    ORDER BY u.date_active DESC, u.id DESC
    LIMIT 1
  `);
  const key = Buffer.from(body, 'base64');
  if (key.length !== 256) throw new Error('active production auth key is absent or invalid');
  return key.toString('hex');
}

function channelRows(channelId: bigint): Record<string, number> {
  const checks: Record<string, string> = {
    channel: `SELECT COUNT(*) FROM apifull_channel WHERE id=${channelId}`,
    members: `SELECT COUNT(*) FROM apifull_channel_member WHERE channel_id=${channelId}`,
    messages: `SELECT COUNT(*) FROM apifull_channel_message WHERE channel_id=${channelId}`,
    requests: `SELECT COUNT(*) FROM apifull_channel_message_request WHERE channel_id=${channelId}`,
    sequence: `SELECT COUNT(*) FROM apifull_channel_message_seq WHERE channel_id=${channelId}`,
    events: `SELECT COUNT(*) FROM apifull_channel_event WHERE channel_id=${channelId}`,
    hidden: `SELECT COUNT(*) FROM apifull_channel_message_hidden WHERE channel_id=${channelId}`,
    contentReads: `SELECT COUNT(*) FROM apifull_channel_message_content_read WHERE channel_id=${channelId}`,
    readState: `SELECT COUNT(*) FROM apifull_channel_read_state WHERE channel_id=${channelId}`,
    adminLog: `SELECT COUNT(*) FROM apifull_channel_admin_log WHERE channel_id=${channelId}`,
  };
  return Object.fromEntries(Object.entries(checks).map(([name, query]) => [name, Number(postgres(query))]));
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

function findChannel(value: any, title: string): any {
  const chats = [
    ...(Array.isArray(value?.chats) ? value.chats : []),
    ...(Array.isArray(value?.updates?.chats) ? value.updates.chats : []),
  ];
  return chats.find((chat: any) => chat?.title === title);
}

function assertTitle(value: any, channelId: bigint, title: string): void {
  const found = findChannel(value, title);
  if (!rpcName(value).toLowerCase().includes('updates') || rpcName(found) !== 'Channel') {
    throw new Error(`channels.editTitle response did not include expected Channel(${title})`);
  }
  const stored = postgres(`SELECT title FROM apifull_channel WHERE id=${channelId} AND creator_user_id=${userId}`);
  if (stored !== title) throw new Error(`PostgreSQL title readback mismatch: ${stored}`);
}

async function main() {
  const client = makeClient(loadAuthKey());
  const title = `r28-title-${Date.now()}-${randomBytes(4).toString('hex')}`;
  const renamed = `${title}-edited`;
  let channelId = 0n;
  let accessHash = 0n;
  let failure: unknown;
  let cleanupError: unknown;
  let evidence: Record<string, unknown> = {};

  try {
    await client.connect();
    const me = await client.getMe();
    if (String(me?.id) !== userId) throw new Error('production auth key user mismatch');
    const created = await client.invoke(new Api.channels.CreateChannel({ broadcast: true, title, about: 'temporary production title probe' }));
    const channel = findChannel(created, title);
    if (!channel?.id || channel.accessHash === undefined) {
      throw new Error(`channels.createChannel returned ${rpcName(created)} without channel identity`);
    }
    channelId = BigInt(String(channel.id));
    accessHash = BigInt(String(channel.accessHash));
    const inputChannel = new Api.InputChannel({ channelId, accessHash });

    const changed = await client.invoke(new Api.channels.EditTitle({ channel: inputChannel, title: renamed }));
    assertTitle(changed, channelId, renamed);
    const changedReadback = await client.invoke(new Api.channels.GetChannels({ id: [inputChannel] }));
    if (findChannel(changedReadback, renamed)?.title !== renamed) throw new Error('renamed title did not read back through channels.getChannels');

    const restored = await client.invoke(new Api.channels.EditTitle({ channel: inputChannel, title }));
    assertTitle(restored, channelId, title);
    const restoredReadback = await client.invoke(new Api.channels.GetChannels({ id: [inputChannel] }));
    if (findChannel(restoredReadback, title)?.title !== title) throw new Error('original title did not restore through channels.getChannels');
    evidence = {
      createChannel: rpcName(created),
      editTitle: [rpcName(changed), rpcName(restored)],
      readbacks: [rpcName(changedReadback), rpcName(restoredReadback)],
      titleRestoredBeforeDelete: true,
    };
  } catch (error) {
    failure = error;
  }

  if (channelId === 0n) {
    try {
      const row = postgres(`SELECT id,access_hash FROM apifull_channel
        WHERE creator_user_id=${userId} AND title IN ('${title}','${renamed}') ORDER BY id DESC LIMIT 1`);
      if (row) {
        const [id, hash] = row.split('\t');
        channelId = BigInt(id);
        accessHash = BigInt(hash);
      }
    } catch (error) {
      cleanupError = error;
    }
  }

  if (channelId > 0n) {
    try {
      if (Number(postgres(`SELECT COUNT(*) FROM apifull_channel WHERE id=${channelId}`)) > 0) {
        const inputChannel = new Api.InputChannel({ channelId, accessHash });
        const currentTitle = postgres(`SELECT title FROM apifull_channel WHERE id=${channelId}`);
        if (currentTitle !== title) {
          await client.invoke(new Api.channels.EditTitle({ channel: inputChannel, title }));
        }
        const deleted = await client.invoke(new Api.channels.DeleteChannel({ channel: inputChannel }));
        if (!rpcName(deleted).toLowerCase().includes('updates')) throw new Error(`channels.deleteChannel returned ${rpcName(deleted)}`);
        evidence.deleteChannel = rpcName(deleted);
      }
    } catch (error) {
      cleanupError = error;
    }
  }
  client.destroy();

  if (channelId === 0n) throw failure || new Error('production probe did not create a channel');
  const remaining = channelRows(channelId);
  evidence.channelRowsAfterDelete = remaining;
  if (Object.values(remaining).some((count) => count !== 0)) {
    throw new Error(`temporary title channel cleanup left production rows: ${JSON.stringify(evidence)}`);
  }
  if (cleanupError) throw cleanupError;
  if (failure) throw failure;
  console.log(JSON.stringify({
    transport: `DC${dcId} WebSocket -> gateway -> session -> APIFull/PostgreSQL`,
    ...evidence,
    writes: 3,
  }));
}

main().then(() => process.exit(0)).catch((error) => {
  console.error(`channel title production probe failed: ${error instanceof Error ? error.stack || error.message : String(error)}`);
  process.exit(1);
});
