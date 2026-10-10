import { randomBytes } from 'node:crypto';
import { execFileSync } from 'node:child_process';
import path from 'node:path';
import { createRequire } from 'node:module';

const userId = 136907714;
const gramjsDir = process.env.TEAMGRAM_GRAMJS_DIR || '/Users/mac/open/test/telegram-tt-master';
const dcId = Number(process.env.TG_DC_ID || '2');
const dcHost = process.env.TG_DC_HOST || '127.0.0.1';
const dcPort = Number(process.env.TG_DC_PORT || '11443');
const globalAny = globalThis as typeof globalThis & { self?: any; addEventListener?: () => void };
globalAny.self ??= globalThis;
globalAny.addEventListener ??= () => {};
globalAny.self.addEventListener ??= globalAny.addEventListener;

function postgres(query: string): string {
  return execFileSync('docker', [
    'exec', 'teamgram-postgres', 'psql', '-v', 'ON_ERROR_STOP=1', '-At', '-q', '-F', '\t', '-U', 'teamgram', '-d', 'teamgram', '-c', query,
  ], { encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'] }).trim();
}

function requiredValue(query: string, name: string): string {
  const value = postgres(query);
  if (!value) throw new Error(`${name} is absent from production PostgreSQL`);
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

function updateChats(value: any): any[] {
  return [
    ...(Array.isArray(value?.chats) ? value.chats : []),
    ...(Array.isArray(value?.updates?.chats) ? value.updates.chats : []),
  ];
}

function hasMessage(value: any, expected: string, seen = new Set<any>()): boolean {
  if (!value || typeof value !== 'object' || seen.has(value)) return false;
  seen.add(value);
  if (value.message === expected) return true;
  return (Array.isArray(value) ? value : Object.values(value)).some((item) => hasMessage(item, expected, seen));
}

async function waitForPersistedMessage(channelId: bigint, text: string): Promise<{ messageId: number; events: number; requestRows: number }> {
  for (let attempt = 0; attempt < 20; attempt += 1) {
    const row = postgres(`
      SELECT m.message_id,
        (SELECT COUNT(*) FROM apifull_channel_event e WHERE e.channel_id=m.channel_id),
        (SELECT COUNT(*) FROM apifull_channel_message_request r WHERE r.channel_id=m.channel_id AND r.message_id=m.message_id)
      FROM apifull_channel_message m
      WHERE m.channel_id=${channelId} AND m.message='${text}'
      ORDER BY m.message_id DESC LIMIT 1
    `);
    if (row) {
      const [messageId, events, requestRows] = row.split('\t').map(Number);
      if (events > 0 && requestRows > 0) return { messageId, events, requestRows };
    }
    await new Promise((resolve) => setTimeout(resolve, 250));
  }
  throw new Error('sent channel message or its replay event was not persisted');
}

function channelRows(title: string): string {
  return postgres(`
    SELECT id, access_hash FROM apifull_channel
    WHERE creator_user_id=${userId} AND title='${title}'
    ORDER BY id DESC LIMIT 1
  `);
}

function rowCounts(channelId: bigint): Record<string, number> {
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

async function main() {
  const authKey = requiredValue(`
    SELECT encode(k.body, 'base64')
    FROM auth_users u
    JOIN auth_keys k USING (auth_key_id)
    JOIN auth_key_infos i USING (auth_key_id)
    WHERE u.user_id=${userId} AND u.state=0 AND u.deleted=0
      AND k.deleted=0 AND i.deleted=0 AND i.auth_key_type=0
    ORDER BY u.date_active DESC,u.id DESC
    LIMIT 1
  `, 'active production auth key');
  const client = makeClient(authKey);
  const title = `r20-delete-${Date.now()}-${randomBytes(4).toString('hex')}`;
  const messageText = `${title}-persisted-message`;
  let channelId = 0n;
  let accessHash = 0n;
  let failure: unknown;
  const evidence: Record<string, unknown> = {};

  try {
    await client.connect();
    const me = await client.getMe();
    if (String(me?.id) !== String(userId)) throw new Error('production auth key user mismatch');

    const created = await client.invoke(new Api.channels.CreateChannel({
      broadcast: true,
      title,
      about: 'r20 production channel deletion cleanup probe',
    }));
    const channel = updateChats(created).find((item: any) => item?.title === title);
    if (!channel?.id || channel.accessHash === undefined) {
      throw new Error(`channel creation returned ${rpcName(created)} without channel identity`);
    }
    channelId = BigInt(String(channel.id));
    accessHash = BigInt(String(channel.accessHash));
    evidence.create = rpcName(created);
    evidence.channelId = channelId.toString();

    const input = new Api.InputChannel({ channelId, accessHash });
    const peer = new Api.InputPeerChannel({ channelId, accessHash });
    const randomId = BigInt(Date.now()) * 1000n + BigInt(randomBytes(2).readUInt16BE(0));
    const sent = await client.invoke(new Api.messages.SendMessage({
      peer,
      message: messageText,
      randomId,
    }));
    if (!rpcName(sent).toLowerCase().includes('updates')) {
      throw new Error(`channel send returned ${rpcName(sent)}`);
    }
    const persisted = await waitForPersistedMessage(channelId, messageText);
    if (persisted.requestRows !== 1) throw new Error('first send did not persist exactly one random_id mapping');
    const replay = await client.invoke(new Api.messages.SendMessage({ peer, message: messageText, randomId }));
    if (!rpcName(replay).toLowerCase().includes('updates')) {
      throw new Error(`channel retry returned ${rpcName(replay)}`);
    }
    const replayed = await waitForPersistedMessage(channelId, messageText);
    if (replayed.messageId !== persisted.messageId || replayed.events !== persisted.events || replayed.requestRows !== 1) {
      throw new Error(`channel retry duplicated persisted state: first=${JSON.stringify(persisted)} retry=${JSON.stringify(replayed)}`);
    }
    evidence.send = rpcName(sent);
    evidence.messageId = persisted.messageId;
    evidence.eventsBeforeDelete = persisted.events;
    evidence.retry = rpcName(replay);
    evidence.retryReusedMessageId = replayed.messageId;
    evidence.requestRowsBeforeDelete = replayed.requestRows;

    const difference = await client.invoke(new Api.updates.GetChannelDifference({
      force: true,
      channel: input,
      filter: new Api.ChannelMessagesFilterEmpty(),
      pts: 0,
      limit: 100,
    }));
    if (!rpcName(difference).toLowerCase().includes('channeldifference') || !hasMessage(difference, messageText)) {
      throw new Error(`channel difference returned ${rpcName(difference)} without the persisted message`);
    }
    evidence.difference = rpcName(difference);

    const deleted = await client.invoke(new Api.channels.DeleteChannel({ channel: input }));
    if (!rpcName(deleted).toLowerCase().includes('updates')) {
      throw new Error(`channel delete returned ${rpcName(deleted)}`);
    }
    evidence.delete = rpcName(deleted);
  } catch (error) {
    failure = error;
  }

  if (channelId === 0n) {
    try {
      const row = channelRows(title);
      if (row) {
        const [id, hash] = row.split('\t');
        channelId = BigInt(id);
        accessHash = BigInt(hash);
      }
    } catch (error) {
      evidence.lookupError = String(error);
    }
  }

  if (channelId > 0n) {
    try {
      if (Number(postgres(`SELECT COUNT(*) FROM apifull_channel WHERE id=${channelId}`)) > 0) {
        await client.invoke(new Api.channels.DeleteChannel({
          channel: new Api.InputChannel({ channelId, accessHash }),
        }));
        evidence.cleanup = 'delete retried';
      }
    } catch (error) {
      evidence.cleanupError = String(error);
    }
  }
  client.destroy();

  if (channelId === 0n) throw failure || new Error('production probe did not create a channel');
  const counts = rowCounts(channelId);
  evidence.deletedRows = counts;
  if (Object.values(counts).some((count) => count !== 0)) {
    throw new Error(`temporary production channel cleanup left rows: ${JSON.stringify(evidence)}`);
  }
  if (failure) throw failure;
  console.log(JSON.stringify(evidence));
}

main().then(() => process.exit(0)).catch((error) => {
  console.error(`production channel delete probe failed: ${error instanceof Error ? error.stack || error.message : String(error)}`);
  process.exit(1);
});
