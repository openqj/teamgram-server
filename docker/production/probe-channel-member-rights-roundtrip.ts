import { randomBytes } from 'node:crypto';
import { execFileSync } from 'node:child_process';
import path from 'node:path';
import { createRequire } from 'node:module';

for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) {
  delete process.env[key];
}

const ownerId = process.env.CHANNEL_RIGHTS_OWNER_ID || '136907714';
const targetId = process.env.CHANNEL_RIGHTS_TARGET_ID || '136907713';
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

function loadAuthKey(userId: string): string {
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
  if (key.length !== 256) throw new Error(`active production auth key for user ${userId} is absent or invalid`);
  return key.toString('hex');
}

function targetAccessHash(): bigint {
  const value = postgres(`SELECT access_hash FROM users WHERE id=${targetId} AND deleted=0 LIMIT 1`);
  if (!value) throw new Error('target production user is absent or deleted');
  return BigInt(value);
}

function memberRow(channelId: bigint): { adminRights: string; rank: string; bannedRights: string; bannedBy: string; bannedAt: string } | null {
  const row = postgres(`SELECT COALESCE(NULLIF(admin_rights,''),'__EMPTY__'),
      COALESCE(NULLIF(admin_rank,''),'__EMPTY__'),COALESCE(NULLIF(banned_rights,''),'__EMPTY__'),
      COALESCE(banned_by_user_id,0),COALESCE(banned_at,0)
    FROM apifull_channel_member WHERE channel_id=${channelId} AND user_id=${targetId}`);
  if (!row) return null;
  const [adminRights, rank, bannedRights, bannedBy, bannedAt] = row.split('\t');
  const emptyAsString = (value: string) => value === '__EMPTY__' ? '' : value;
  return { adminRights: emptyAsString(adminRights), rank: emptyAsString(rank), bannedRights: emptyAsString(bannedRights), bannedBy, bannedAt };
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

function createdChannel(value: any, title: string): any {
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

function participant(value: any): any {
  return value?.participant;
}

function assertParticipant(value: any, expected: string): any {
  const part = participant(value);
  if (!rpcName(value).toLowerCase().includes('channelparticipant') || rpcName(part) !== expected) {
    throw new Error(`channels.getParticipant returned ${rpcName(value)} with ${rpcName(part)}, expected ${expected}`);
  }
  return part;
}

async function main() {
  const client = makeClient(loadAuthKey(ownerId));
  const targetHash = targetAccessHash();
  const title = `r28-member-rights-${Date.now()}-${randomBytes(4).toString('hex')}`;
  let channelId = 0n;
  let accessHash = 0n;
  let failure: unknown;
  let cleanupError: unknown;
  let evidence: Record<string, unknown> = {};

  try {
    await client.connect();
    const me = await client.getMe();
    if (String(me?.id) !== ownerId || ownerId === targetId) throw new Error('production owner identity is invalid');

    const created = await client.invoke(new Api.channels.CreateChannel({
      megagroup: true,
      title,
      about: 'temporary production member-rights probe',
    }));
    const channel = createdChannel(created, title);
    if (!channel?.id || channel.accessHash === undefined) {
      throw new Error(`channels.createChannel returned ${rpcName(created)} without channel identity`);
    }
    channelId = BigInt(String(channel.id));
    accessHash = BigInt(String(channel.accessHash));
    const inputChannel = new Api.InputChannel({ channelId, accessHash });
    const inputUser = new Api.InputUser({ userId: BigInt(targetId), accessHash: targetHash });
    const inputPeer = new Api.InputPeerUser({ userId: BigInt(targetId), accessHash: targetHash });

    const invited = await client.invoke(new Api.channels.InviteToChannel({ channel: inputChannel, users: [inputUser] }));
    if (!rpcName(invited).toLowerCase().includes('invitedusers') || !memberRow(channelId)) {
      throw new Error(`channels.inviteToChannel returned ${rpcName(invited)} without persisted target membership`);
    }

    const promoted = await client.invoke(new Api.channels.EditAdmin({
      channel: inputChannel,
      userId: inputUser,
      adminRights: new Api.ChatAdminRights({ manageTopics: true }),
      rank: 'temporary',
    }));
    assertUpdates(promoted, 'channels.editAdmin(promote)');
    const promotedRow = memberRow(channelId);
    const promotedRights = JSON.parse(promotedRow?.adminRights || '{}');
    const promotedReadback = await client.invoke(new Api.channels.GetParticipant({ channel: inputChannel, participant: inputPeer }));
    const promotedParticipant = assertParticipant(promotedReadback, 'ChannelParticipantAdmin');
    if (!promotedRights.manage_topics || promotedRow?.rank !== 'temporary'
      || promotedParticipant?.adminRights?.manageTopics !== true || promotedParticipant?.rank !== 'temporary') {
      throw new Error('admin promotion did not match PostgreSQL and channels.getParticipant');
    }

    const demoted = await client.invoke(new Api.channels.EditAdmin({
      channel: inputChannel,
      userId: inputUser,
      adminRights: new Api.ChatAdminRights({}),
    }));
    assertUpdates(demoted, 'channels.editAdmin(demote)');
    const demotedRow = memberRow(channelId);
    const demotedParticipant = assertParticipant(
      await client.invoke(new Api.channels.GetParticipant({ channel: inputChannel, participant: inputPeer })),
      'ChannelParticipant',
    );
    if (demotedRow?.adminRights || demotedRow?.rank) {
      throw new Error(`admin demotion left stored rights: ${JSON.stringify(demotedRow)}`);
    }
    if (rpcName(demotedParticipant) !== 'ChannelParticipant') {
      throw new Error(`admin demotion read back ${rpcName(demotedParticipant)}`);
    }

    const banned = await client.invoke(new Api.channels.EditBanned({
      channel: inputChannel,
      participant: inputPeer,
      bannedRights: new Api.ChatBannedRights({ sendMessages: true, untilDate: Math.floor(Date.now() / 1000) + 3600 }),
    }));
    assertUpdates(banned, 'channels.editBanned(restrict)');
    const bannedRow = memberRow(channelId);
    const persistedBan = JSON.parse(bannedRow?.bannedRights || '{}');
    const bannedParticipant = assertParticipant(
      await client.invoke(new Api.channels.GetParticipant({ channel: inputChannel, participant: inputPeer })),
      'ChannelParticipantBanned',
    );
    if (!persistedBan.send_messages || Number(bannedRow?.bannedAt) <= 0
      || bannedParticipant?.bannedRights?.sendMessages !== true) {
      throw new Error('send-message restriction did not match PostgreSQL and channels.getParticipant');
    }

    const unbanned = await client.invoke(new Api.channels.EditBanned({
      channel: inputChannel,
      participant: inputPeer,
      bannedRights: new Api.ChatBannedRights({ untilDate: 0 }),
    }));
    assertUpdates(unbanned, 'channels.editBanned(clear)');
    const clearedRow = memberRow(channelId);
    const clearedParticipant = assertParticipant(
      await client.invoke(new Api.channels.GetParticipant({ channel: inputChannel, participant: inputPeer })),
      'ChannelParticipant',
    );
    if (clearedRow?.bannedRights || Number(clearedRow?.bannedBy) !== 0 || Number(clearedRow?.bannedAt) !== 0
      || rpcName(clearedParticipant) !== 'ChannelParticipant') {
      throw new Error('restriction clear did not restore the regular member in PostgreSQL and channels.getParticipant');
    }

    evidence = {
      createChannel: rpcName(created),
      inviteToChannel: rpcName(invited),
      editAdmin: [rpcName(promoted), rpcName(demoted)],
      editBanned: [rpcName(banned), rpcName(unbanned)],
      participantReadbacks: [
        'ChannelParticipantAdmin(manageTopics=true)',
        'ChannelParticipant',
        'ChannelParticipantBanned(sendMessages=true)',
        'ChannelParticipant',
      ],
    };
  } catch (error) {
    failure = error;
  }

  if (channelId === 0n) {
    try {
      const row = postgres(`SELECT id,access_hash FROM apifull_channel
        WHERE creator_user_id=${ownerId} AND title='${title}' ORDER BY id DESC LIMIT 1`);
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
        const inputPeer = new Api.InputPeerUser({ userId: BigInt(targetId), accessHash: targetHash });
        try {
          await client.invoke(new Api.channels.EditBanned({
            channel: inputChannel,
            participant: inputPeer,
            bannedRights: new Api.ChatBannedRights({ untilDate: 0 }),
          }));
        } catch {}
        try {
          await client.invoke(new Api.channels.EditAdmin({
            channel: inputChannel,
            userId: new Api.InputUser({ userId: BigInt(targetId), accessHash: targetHash }),
            adminRights: new Api.ChatAdminRights({}),
          }));
        } catch {}
        const deleted = await client.invoke(new Api.channels.DeleteChannel({ channel: inputChannel }));
        assertUpdates(deleted, 'channels.deleteChannel');
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
    throw new Error(`temporary member-rights channel cleanup left production rows: ${JSON.stringify(evidence)}`);
  }
  if (cleanupError) throw cleanupError;
  if (failure) throw failure;
  console.log(JSON.stringify({
    transport: `DC2 WebSocket -> gateway -> session -> APIFull/User service/PostgreSQL`,
    ...evidence,
    accountRowsChanged: false,
  }));
}

main().then(() => process.exit(0)).catch((error) => {
  console.error(`channel member rights production probe failed: ${error instanceof Error ? error.stack || error.message : String(error)}`);
  process.exit(1);
});
