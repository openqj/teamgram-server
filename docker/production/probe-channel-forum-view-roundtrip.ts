import { randomBytes } from 'node:crypto';
import { execFileSync } from 'node:child_process';
import path from 'node:path';
import { createRequire } from 'node:module';

for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) {
  delete process.env[key];
}

const userId = process.env.CHANNEL_FORUM_USER_ID || '136907714';
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

function postgresWrite(query: string): void {
  execFileSync('docker', [
    'exec', 'teamgram-postgres', 'psql', '-v', 'ON_ERROR_STOP=1', '-At', '-q', '-F', '\t', '-U', 'teamgram', '-d', 'teamgram', '-c', query,
  ], { stdio: ['ignore', 'ignore', 'ignore'] });
}

function loadAuthKey(): string {
  const body = postgres(`
    SELECT encode(k.body, 'base64')
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

function settingsKey(channelId: bigint): string {
  return `forum:channel:${channelId}:settings`;
}

function readSettings(channelId: bigint): { enabled: boolean; tabs: boolean; viewAsMessages: boolean } {
  const raw = postgres(`SELECT v FROM apifull_kv WHERE k='${settingsKey(channelId)}'`);
  if (!raw) throw new Error('temporary forum settings row is absent');
  return JSON.parse(raw);
}

function updateClasses(value: any): string[] {
  return Array.isArray(value?.updates) ? value.updates.map((item: any) => rpcName(item)) : [];
}

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

function assertUpdates(value: any, method: string): void {
  if (!rpcName(value).toLowerCase().includes('updates')) {
    throw new Error(`${method} returned ${rpcName(value)} instead of Updates`);
  }
}

async function main() {
  const client = makeClient(loadAuthKey());
  const title = `r28-forum-view-${Date.now()}-${randomBytes(4).toString('hex')}`;
  let channelId = 0n;
  let accessHash = 0n;
  let forumKeyOwned = false;
  let failure: unknown;
  let cleanupError: unknown;
  let evidence: Record<string, unknown> = {};

  try {
    await client.connect();
    const me = await client.getMe();
    if (String(me?.id) !== userId) throw new Error('production auth key user mismatch');

    const created = await client.invoke(new Api.channels.CreateChannel({
      megagroup: true,
      title,
      about: 'temporary production forum view probe',
    }));
    const channel = findChannel(created, title);
    if (!channel?.id || channel.accessHash === undefined) {
      throw new Error(`channels.createChannel returned ${rpcName(created)} without channel identity`);
    }
    channelId = BigInt(String(channel.id));
    accessHash = BigInt(String(channel.accessHash));
    const inputChannel = new Api.InputChannel({ channelId, accessHash });
    const key = settingsKey(channelId);
    const topicKey = `forum:channel:${channelId}`;
    if (Number(postgres(`SELECT COUNT(*) FROM apifull_kv WHERE k IN ('${topicKey}', '${key}')`)) !== 0) {
      throw new Error('temporary forum settings key already exists; refusing to overwrite it');
    }
    forumKeyOwned = true;

    const forumEnabled = await client.invoke(new Api.channels.ToggleForum({
      channel: inputChannel,
      enabled: true,
      tabs: false,
    }));
    assertUpdates(forumEnabled, 'channels.toggleForum(enable)');
    const initialSettings = readSettings(channelId);
    if (!initialSettings.enabled || initialSettings.tabs || initialSettings.viewAsMessages) {
      throw new Error('forum enable state did not match its PostgreSQL settings row');
    }

    const viewEnabled = await client.invoke(new Api.channels.ToggleViewForumAsMessages({
      channel: inputChannel,
      enabled: true,
    }));
    assertUpdates(viewEnabled, 'channels.toggleViewForumAsMessages(enable)');
    const enabledClasses = updateClasses(viewEnabled);
    const enabledSettings = readSettings(channelId);
    if (!enabledClasses.some((name) => name.toLowerCase().includes('updatechannelviewforumasmessages'))
      || !enabledSettings.enabled || !enabledSettings.viewAsMessages) {
      throw new Error('enabled view-as-messages state did not match Updates and PostgreSQL');
    }

    const viewDisabled = await client.invoke(new Api.channels.ToggleViewForumAsMessages({
      channel: inputChannel,
      enabled: false,
    }));
    assertUpdates(viewDisabled, 'channels.toggleViewForumAsMessages(disable)');
    const disabledClasses = updateClasses(viewDisabled);
    const disabledSettings = readSettings(channelId);
    if (!disabledClasses.some((name) => name.toLowerCase().includes('updatechannelviewforumasmessages'))
      || !disabledSettings.enabled || disabledSettings.viewAsMessages) {
      throw new Error('disabled view-as-messages state did not match Updates and PostgreSQL');
    }
    evidence = {
      createChannel: rpcName(created),
      toggleForum: rpcName(forumEnabled),
      toggleViewForumAsMessages: [rpcName(viewEnabled), rpcName(viewDisabled)],
      viewEnabledUpdates: enabledClasses,
      viewDisabledUpdates: disabledClasses,
      settingsWhileEnabled: enabledSettings,
      settingsAfterDisable: disabledSettings,
    };
  } catch (error) {
    failure = error;
  }

  if (channelId === 0n) {
    try {
      const row = postgres(`SELECT id,access_hash FROM apifull_channel
        WHERE creator_user_id=${userId} AND title='${title}' ORDER BY id DESC LIMIT 1`);
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
        if (forumKeyOwned) {
          try {
            await client.invoke(new Api.channels.ToggleViewForumAsMessages({ channel: inputChannel, enabled: false }));
            await client.invoke(new Api.channels.ToggleForum({ channel: inputChannel, enabled: false, tabs: false }));
          } catch {
            // Channel deletion below remains the final cleanup path for this temporary object.
          }
        }
        const deleted = await client.invoke(new Api.channels.DeleteChannel({ channel: inputChannel }));
        assertUpdates(deleted, 'channels.deleteChannel');
        evidence.deleteChannel = rpcName(deleted);
      }
    } catch (error) {
      cleanupError = error;
    }
    if (forumKeyOwned) {
      try {
        const key = settingsKey(channelId);
        const topicKey = `forum:channel:${channelId}`;
        postgresWrite(`DELETE FROM apifull_kv WHERE k IN ('${topicKey}', '${key}')`);
        evidence.forumKvRowsAfterCleanup = Number(postgres(`SELECT COUNT(*) FROM apifull_kv WHERE k IN ('${topicKey}', '${key}')`));
      } catch (error) {
        cleanupError ??= error;
      }
    }
  }

  client.destroy();

  if (channelId === 0n) throw failure || new Error('production probe did not create a channel');
  const remaining = channelRows(channelId);
  evidence.channelRowsAfterDelete = remaining;
  if (Object.values(remaining).some((count) => count !== 0)
    || (forumKeyOwned && evidence.forumKvRowsAfterCleanup !== 0)) {
    throw new Error(`temporary forum probe cleanup left production rows: ${JSON.stringify(evidence)}`);
  }
  if (cleanupError) throw cleanupError;
  if (failure) throw failure;
  console.log(JSON.stringify({
    transport: `DC${dcId} WebSocket -> gateway -> session -> APIFull/PostgreSQL`,
    ...evidence,
    writes: 6,
  }));
}

const requireModule = createRequire(import.meta.url);
const TelegramClient = requireModule(path.join(gramjsDir, 'src/lib/gramjs/client/TelegramClient.ts')).default;
const CallbackSession = requireModule(path.join(gramjsDir, 'src/lib/gramjs/sessions/CallbackSession.ts')).default;
const Api = requireModule(path.join(gramjsDir, 'src/lib/gramjs/tl/index.ts')).Api;
const Connection = requireModule(path.join(gramjsDir, 'src/lib/gramjs/network/connection/TCPObfuscated.ts')).ConnectionTCPObfuscated;
const quietLogger = { debug() {}, info() {}, warn() {}, error() {} };

main().then(() => process.exit(0)).catch((error) => {
  console.error(`channel forum view roundtrip probe failed: ${error instanceof Error ? error.stack || error.message : String(error)}`);
  process.exit(1);
});
