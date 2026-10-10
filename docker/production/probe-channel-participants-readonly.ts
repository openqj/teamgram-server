import { execFileSync } from 'node:child_process';
import path from 'node:path';
import { createRequire } from 'node:module';

for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) {
  delete process.env[key];
}

const userId = process.env.CHANNEL_PARTICIPANTS_USER_ID || '136907714';
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

function channelRow(): { id: string; accessHash: string } {
  const row = postgres(`
    SELECT c.id, c.access_hash
    FROM apifull_channel c
    WHERE c.creator_user_id=${userId} AND c.participants_hidden=0
      AND EXISTS (SELECT 1 FROM apifull_channel_message m WHERE m.channel_id=c.id)
      AND NOT EXISTS (SELECT 1 FROM apifull_channel_member m WHERE m.channel_id=c.id)
    ORDER BY c.created_at DESC, c.id DESC
    LIMIT 1
  `);
  const [id, accessHash] = row.split('\t');
  if (!id || !accessHash) throw new Error('no visible owned channel with messages and no explicit member rows');
  return { id, accessHash };
}

function channelSnapshot(id: string): string {
  return postgres(`
    SELECT CONCAT_WS('|', c.id, c.access_hash, c.creator_user_id, c.participants_hidden,
      c.created_at,
      (SELECT COUNT(*) FROM apifull_channel_member m WHERE m.channel_id=c.id),
      (SELECT COUNT(*) FROM apifull_channel_message x WHERE x.channel_id=c.id),
      (SELECT COUNT(*) FROM apifull_channel_event e WHERE e.channel_id=c.id))
    FROM apifull_channel c
    WHERE c.id=${id} AND c.creator_user_id=${userId}
  `);
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

function assertCreator(result: any, name: string): void {
  const rows = result?.participants;
  if (!rpcName(result).toLowerCase().includes('channelparticipants') || !Array.isArray(rows)) {
    throw new Error(`${name} returned ${rpcName(result)} without a participant vector`);
  }
  if (Number(result.count) !== 1 || rows.length !== 1
    || String(rows[0]?.userId) !== userId
    || !rpcName(rows[0]).toLowerCase().includes('channelparticipantcreator')) {
    throw new Error(`${name} did not return the persisted channel creator as its only participant`);
  }
  if (!Array.isArray(result.users) || !result.users.some((user: any) => String(user?.id) === userId)) {
    throw new Error(`${name} did not hydrate the creator user`);
  }
}

async function main() {
  const channel = channelRow();
  const before = channelSnapshot(channel.id);
  if (!before) throw new Error('selected production channel disappeared before the probe');

  const client = makeClient(loadAuthKey());
  try {
    await client.connect();
    const me = await client.getMe();
    if (String(me?.id) !== userId) throw new Error('production auth key user mismatch');

    const inputChannel = new Api.InputChannel({
      channelId: BigInt(channel.id),
      accessHash: BigInt(channel.accessHash),
    });
    const recent = await client.invoke(new Api.channels.GetParticipants({
      channel: inputChannel,
      filter: new Api.ChannelParticipantsRecent(),
      offset: 0,
      limit: 20,
      hash: 0n,
    }));
    assertCreator(recent, 'channels.getParticipants(recent)');

    const admins = await client.invoke(new Api.channels.GetParticipants({
      channel: inputChannel,
      filter: new Api.ChannelParticipantsAdmins(),
      offset: 0,
      limit: 20,
      hash: 0n,
    }));
    assertCreator(admins, 'channels.getParticipants(admins)');

    const participant = await client.invoke(new Api.channels.GetParticipant({
      channel: inputChannel,
      participant: new Api.InputPeerSelf(),
    }));
    if (!rpcName(participant).toLowerCase().includes('channelparticipant')
      || !rpcName(participant?.participant).toLowerCase().includes('channelparticipantcreator')
      || String(participant?.participant?.userId) !== userId
      || !Array.isArray(participant?.users)
      || !participant.users.some((user: any) => String(user?.id) === userId)) {
      throw new Error(`channels.getParticipant(self) returned an unexpected result: ${rpcName(participant)}`);
    }

    const after = channelSnapshot(channel.id);
    if (after !== before) throw new Error('channel, member, message, or event rows changed during the read-only probe');
    console.log(JSON.stringify({
      transport: `DC${dcId} WebSocket -> gateway -> session -> BFF -> APIFull/PostgreSQL`,
      channel: 'existing owned channel; messages present; no explicit member rows',
      recent: { type: rpcName(recent), count: recent.count, participant: rpcName(recent.participants[0]) },
      admins: { type: rpcName(admins), count: admins.count, participant: rpcName(admins.participants[0]) },
      getParticipant: { type: rpcName(participant), participant: rpcName(participant.participant) },
      creatorUserHydrated: true,
      storedRowsUnchanged: true,
      writes: 0,
    }));
  } finally {
    client.destroy();
  }
}

main().then(() => process.exit(0)).catch((error) => {
  console.error(`channel participants read-only probe failed: ${error instanceof Error ? error.stack || error.message : String(error)}`);
  process.exit(1);
});
