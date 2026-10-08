import { randomBytes } from 'node:crypto';
import { execFileSync } from 'node:child_process';
import path from 'node:path';
import { createRequire } from 'node:module';

for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) {
  delete process.env[key];
}

const ownerUserId = '136907714';
const resolverUserId = '136907713';
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

function sql(query: string): string {
  return execFileSync('docker', [
    'exec', 'teamgram-postgres', 'psql', '-v', 'ON_ERROR_STOP=1', '-At', '-q', '-F', '\t', '-U', 'teamgram', '-d', 'teamgram', '-c', query,
  ], { encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'] }).trim();
}

function loadAuthKey(userId: string): string {
  const body = sql(`
    SELECT k.body
    FROM auth_users u
    JOIN auth_keys k USING (auth_key_id)
    JOIN auth_key_infos i USING (auth_key_id)
    WHERE u.user_id=${userId} AND u.state=0 AND u.deleted=0
      AND k.deleted=0 AND i.deleted=0 AND i.auth_key_type=0
    ORDER BY u.date_active DESC, u.id DESC
    LIMIT 1
  `);
  if (!body) throw new Error(`no active production auth key found for user ${userId}`);
  const key = Buffer.from(body, 'base64');
  if (key.length !== 256) throw new Error('production auth key has an invalid length');
  return key.toString('hex');
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

function rpcName(value: any): string {
  return value?.className || value?.constructor?.name || typeof value;
}

function matchesType(value: any, type: string): boolean {
  const actual = rpcName(value);
  return actual === type || actual.endsWith(`.${type}`);
}

function boolTrue(value: any): boolean {
  return value === true || value?.value === true || value?.value === 1;
}

function updateChats(value: any): any[] {
  return [
    ...(Array.isArray(value?.chats) ? value.chats : []),
    ...(Array.isArray(value?.updates?.chats) ? value.updates.chats : []),
  ];
}

function channelRow(title: string): string {
  return sql(`
    SELECT id, access_hash FROM apifull_channel
    WHERE creator_user_id=${ownerUserId} AND title='${title}'
    ORDER BY id DESC LIMIT 1
  `);
}

function activeUsernameCount(username: string): number {
  return Number(sql(`SELECT COUNT(*) FROM username WHERE username='${username}' AND deleted=0`));
}

function cleanupCounts(channelId: bigint, username: string): { channel: number; activeUsername: number } {
  return {
    channel: Number(sql(`SELECT COUNT(*) FROM apifull_channel WHERE id=${channelId}`)),
    activeUsername: activeUsernameCount(username),
  };
}

function creatorChannelSnapshot(): string {
  return sql(`
    SELECT COUNT(*), COALESCE(encode(digest(string_agg(CONCAT_WS(':', id, access_hash, username), ',' ORDER BY id), 'sha256'), 'hex'), 'empty')
    FROM apifull_channel WHERE creator_user_id=${ownerUserId}
  `);
}

async function main() {
  const title = `r21-username-${Date.now()}-${randomBytes(4).toString('hex')}`;
  const username = `audit${Date.now().toString(36)}${randomBytes(4).toString('hex')}`;
  const channelsBefore = creatorChannelSnapshot();
  if (activeUsernameCount(username) !== 0) throw new Error('generated production username is already occupied');

  const ownerClient = makeClient(loadAuthKey(ownerUserId));
  const resolverClient = makeClient(loadAuthKey(resolverUserId));
  let channelId = 0n;
  let accessHash = 0n;
  let failure: unknown;
  let cleanupError: unknown;
  const evidence: Record<string, unknown> = {};

  try {
    await ownerClient.connect();
    const owner = await ownerClient.getMe();
    if (String(owner?.id) !== ownerUserId) throw new Error('channel owner auth key user mismatch');

    const created = await ownerClient.invoke(new Api.channels.CreateChannel({
      broadcast: true,
      title,
      about: 'temporary production username resolution probe',
    }));
    const channel = updateChats(created).find((item: any) => item?.title === title);
    if (!channel?.id || channel.accessHash === undefined) {
      throw new Error(`channels.createChannel returned ${rpcName(created)} without channel identity`);
    }
    channelId = BigInt(String(channel.id));
    accessHash = BigInt(String(channel.accessHash));

    const inputChannel = new Api.InputChannel({ channelId, accessHash });
    const updated = await ownerClient.invoke(new Api.channels.UpdateUsername({ channel: inputChannel, username }));
    if (!boolTrue(updated)) throw new Error(`channels.updateUsername returned ${rpcName(updated)}`);

    const admined = await ownerClient.invoke(new Api.channels.GetAdminedPublicChannels({}));
    const adminedChats = updateChats(admined);
    const listedChannel = adminedChats.find((item: any) => String(item?.id) === channelId.toString());
    if (!matchesType(admined, 'Chats') || !matchesType(listedChannel, 'Channel')
      || listedChannel.title !== title || listedChannel.username !== username) {
      throw new Error(`channels.getAdminedPublicChannels returned ${rpcName(admined)} without the new public channel`);
    }

    await resolverClient.connect();
    const resolver = await resolverClient.getMe();
    if (String(resolver?.id) !== resolverUserId) throw new Error('username resolver auth key user mismatch');
    const resolved = await resolverClient.invoke(new Api.contacts.ResolveUsername({ username }));
    if (!matchesType(resolved, 'ResolvedPeer') || String(resolved?.peer?.channelId) !== channelId.toString()) {
      throw new Error(`contacts.resolveUsername returned ${rpcName(resolved)} or the wrong peer`);
    }
    const resolvedChannel = Array.isArray(resolved.chats)
      ? resolved.chats.find((item: any) => String(item?.id) === channelId.toString())
      : undefined;
    if (!matchesType(resolvedChannel, 'Channel') || resolvedChannel.title !== title) {
      throw new Error('contacts.resolveUsername did not hydrate the newly-created channel');
    }
    if (Boolean(resolvedChannel.creator)) throw new Error('non-member resolved channel was incorrectly marked as creator');

    evidence.create = rpcName(created);
    evidence.updateUsername = 'BooleanTrue';
    evidence.getAdminedPublicChannels = rpcName(admined);
    evidence.adminedPublicChannelMatches = true;
    evidence.resolve = rpcName(resolved);
    evidence.resolvedChat = rpcName(resolvedChannel);
    evidence.resolvedChannelIdMatches = true;
    evidence.resolvedUsernameMatches = resolvedChannel.username === username;
    if (!evidence.resolvedUsernameMatches) throw new Error('resolved channel username did not match the assigned username');
  } catch (error) {
    failure = error;
  }

  if (channelId === 0n) {
    try {
      const row = channelRow(title);
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
      const exists = Number(sql(`SELECT COUNT(*) FROM apifull_channel WHERE id=${channelId}`)) > 0;
      if (exists) {
        const inputChannel = new Api.InputChannel({ channelId, accessHash });
        const cleared = await ownerClient.invoke(new Api.channels.UpdateUsername({ channel: inputChannel, username: '' }));
        if (!boolTrue(cleared)) throw new Error(`username cleanup returned ${rpcName(cleared)}`);
        const deleted = await ownerClient.invoke(new Api.channels.DeleteChannel({ channel: inputChannel }));
        if (!matchesType(deleted, 'Updates')) throw new Error(`channel cleanup returned ${rpcName(deleted)}`);
        evidence.cleanup = 'username cleared; channel deleted';
      }
    } catch (error) {
      cleanupError = error;
    }
  }

  ownerClient.destroy();
  resolverClient.destroy();
  const channelsAfter = creatorChannelSnapshot();
  evidence.ownerChannelSnapshotRestored = channelsBefore === channelsAfter;
  if (channelsBefore !== channelsAfter) {
    throw new Error(`owner channel rows changed after cleanup: before=${channelsBefore}, after=${channelsAfter}`);
  }
  if (channelId > 0n) {
    const remaining = cleanupCounts(channelId, username);
    evidence.remainingRows = remaining;
    if (remaining.channel !== 0 || remaining.activeUsername !== 0) {
      throw new Error(`temporary production channel cleanup left active rows: ${JSON.stringify(evidence)}`);
    }
  }
  if (cleanupError) throw cleanupError;
  if (failure) throw failure;
  if (channelId === 0n) throw new Error('production probe did not create a channel');

  console.log(JSON.stringify({
    backend: 'r28',
    transport: `DC${dcId} WebSocket -> gateway -> session -> APIFull/Usernames/User -> PostgreSQL`,
    ownerUserId,
    resolverUserId,
    channelId: channelId.toString(),
    ...evidence,
    businessWrites: 4,
  }));
}

main().then(() => process.exit(0)).catch((error) => {
  console.error(`channel username resolution production probe failed: ${error instanceof Error ? error.stack || error.message : String(error)}`);
  process.exit(1);
});
