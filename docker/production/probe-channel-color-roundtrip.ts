import { randomBytes } from 'node:crypto';
import { execFileSync } from 'node:child_process';
import path from 'node:path';
import { createRequire } from 'node:module';

for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) {
  delete process.env[key];
}

const userId = process.env.CHANNEL_COLOR_USER_ID || '136907714';
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

function createdChats(value: any): any[] {
  return [
    ...(Array.isArray(value?.chats) ? value.chats : []),
    ...(Array.isArray(value?.updates?.chats) ? value.updates.chats : []),
  ];
}

function storedColors(channelId: bigint): string {
  return postgres(`SELECT COALESCE(color,-1), COALESCE(profile_color,-1)
    FROM apifull_channel WHERE id=${channelId} AND creator_user_id=${userId}`);
}

function cleanupRows(channelId: bigint): Record<string, number> {
  const queries: Record<string, string> = {
    channel: `SELECT COUNT(*) FROM apifull_channel WHERE id=${channelId}`,
    members: `SELECT COUNT(*) FROM apifull_channel_member WHERE channel_id=${channelId}`,
    messages: `SELECT COUNT(*) FROM apifull_channel_message WHERE channel_id=${channelId}`,
    events: `SELECT COUNT(*) FROM apifull_channel_event WHERE channel_id=${channelId}`,
    adminLog: `SELECT COUNT(*) FROM apifull_channel_admin_log WHERE channel_id=${channelId}`,
  };
  return Object.fromEntries(Object.entries(queries).map(([key, query]) => [
    key,
    Number(postgres(query)),
  ]));
}

const requireModule = createRequire(import.meta.url);
const TelegramClient = requireModule(path.join(gramjsDir, 'src/lib/gramjs/client/TelegramClient.ts')).default;
const CallbackSession = requireModule(path.join(gramjsDir, 'src/lib/gramjs/sessions/CallbackSession.ts')).default;
const Api = requireModule(path.join(gramjsDir, 'src/lib/gramjs/tl/index.ts')).Api;
const Connection = requireModule(path.join(gramjsDir, 'src/lib/gramjs/network/connection/TCPObfuscated.ts')).ConnectionTCPObfuscated;
const quietLogger = { debug() {}, info() {}, warn() {}, error() {} };

class UpdateColorRequest {
  CONSTRUCTOR_ID = 0xd8aa3671;
  SUBCLASS_OF_ID = 0;
  className = 'channels.UpdateColor';
  classType = 'request';

  constructor(private readonly channel: any, private readonly color: number, private readonly forProfile: boolean) {}

  getBytes(): Uint8Array {
    const header = Buffer.alloc(8);
    header.writeUInt32LE(this.CONSTRUCTOR_ID, 0);
    header.writeInt32LE((this.forProfile ? 2 : 0) | 4, 4);
    const color = Buffer.alloc(4);
    color.writeInt32LE(this.color, 0);
    return Uint8Array.from(Buffer.concat([header, Buffer.from(this.channel.getBytes()), color]));
  }

  readResult(reader: any): any {
    return reader.tgReadObject();
  }
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

async function main() {
  const authKey = loadAuthKey();
  const client = makeClient(authKey);
  const title = `r28-color-${Date.now()}-${randomBytes(4).toString('hex')}`;
  let channelId = 0n;
  let accessHash = 0n;
  let failure: unknown;
  let cleanupError: unknown;
  let evidence: Record<string, unknown> = {};

  try {
    await client.connect();
    const me = await client.getMe();
    if (String(me?.id) !== userId) throw new Error('production auth key user mismatch');

    const colors = await client.invoke(new Api.help.GetPeerColors({ hash: 0 }));
    const profileColors = await client.invoke(new Api.help.GetPeerProfileColors({ hash: 0 }));
    const colorId = Number(colors?.colors?.[0]?.colorId);
    const profileColorId = Number(profileColors?.colors?.[0]?.colorId);
    if (!rpcName(colors).toLowerCase().includes('peercolors') || !Number.isInteger(colorId) || colorId < 0
      || !rpcName(profileColors).toLowerCase().includes('peercolors') || !Number.isInteger(profileColorId) || profileColorId < 0) {
      throw new Error('peer color catalogs did not return usable color options');
    }

    const created = await client.invoke(new Api.channels.CreateChannel({
      broadcast: true,
      title,
      about: 'temporary production peer color probe',
    }));
    const channel = createdChats(created).find((item: any) => item?.title === title);
    if (!channel?.id || channel.accessHash === undefined) {
      throw new Error(`channels.createChannel returned ${rpcName(created)} without channel identity`);
    }
    channelId = BigInt(String(channel.id));
    accessHash = BigInt(String(channel.accessHash));
    const inputChannel = new Api.InputChannel({ channelId, accessHash });

    const colorUpdate = await client.invoke(new UpdateColorRequest(inputChannel, colorId, false));
    if (!rpcName(colorUpdate).toLowerCase().includes('updates')) {
      throw new Error(`channels.updateColor(peer) returned ${rpcName(colorUpdate)}`);
    }
    const profileUpdate = await client.invoke(new UpdateColorRequest(inputChannel, profileColorId, true));
    if (!rpcName(profileUpdate).toLowerCase().includes('updates')) {
      throw new Error(`channels.updateColor(profile) returned ${rpcName(profileUpdate)}`);
    }

    const chats = await client.invoke(new Api.channels.GetChannels({ id: [inputChannel] }));
    const updatedChannel = Array.isArray(chats?.chats)
      ? chats.chats.find((item: any) => String(item?.id) === channelId.toString())
      : undefined;
    const dbColors = storedColors(channelId).split('\t').map(Number);
    if (!rpcName(chats).toLowerCase().includes('chats') || rpcName(updatedChannel) !== 'Channel'
      || Number(updatedChannel?.color?.color) !== colorId
      || Number(updatedChannel?.profileColor?.color) !== profileColorId
      || dbColors[0] !== colorId || dbColors[1] !== profileColorId) {
      throw new Error('channel peer/profile color response did not match the catalog and PostgreSQL values');
    }
    evidence = {
      getPeerColors: rpcName(colors),
      peerColorOptions: colors.colors.length,
      getPeerProfileColors: rpcName(profileColors),
      profileColorOptions: profileColors.colors.length,
      createChannel: rpcName(created),
      updateColor: [rpcName(colorUpdate), rpcName(profileUpdate)],
      getChannels: rpcName(chats),
      peerColorReadback: dbColors[0],
      profileColorReadback: dbColors[1],
    };
  } catch (error) {
    failure = error;
  }

  if (channelId === 0n) {
    try {
      const row = postgres(`SELECT id, access_hash FROM apifull_channel
        WHERE creator_user_id=${userId} AND title='${title}' LIMIT 1`);
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
      const exists = Number(postgres(`SELECT COUNT(*) FROM apifull_channel WHERE id=${channelId}`)) > 0;
      if (exists) {
        const deleted = await client.invoke(new Api.channels.DeleteChannel({
          channel: new Api.InputChannel({ channelId, accessHash }),
        }));
        if (!rpcName(deleted).toLowerCase().includes('updates')) {
          throw new Error(`channels.deleteChannel returned ${rpcName(deleted)}`);
        }
        evidence.deleteChannel = rpcName(deleted);
      }
    } catch (error) {
      cleanupError = error;
    }
  }
  client.destroy();

  if (channelId === 0n) throw failure || new Error('production probe did not create a channel');
  const remaining = cleanupRows(channelId);
  evidence.channelRowsAfterDelete = remaining;
  if (Object.values(remaining).some((count) => count !== 0)) {
    throw new Error(`temporary color channel cleanup left production rows: ${JSON.stringify(evidence)}`);
  }
  if (cleanupError) throw cleanupError;
  if (failure) throw failure;
  console.log(JSON.stringify({
    transport: `DC${dcId} WebSocket -> gateway -> session -> BFF -> APIFull/PostgreSQL`,
    ...evidence,
    writes: 2,
  }));
}

main().then(() => process.exit(0)).catch((error) => {
  console.error(`channel color roundtrip probe failed: ${error instanceof Error ? error.stack || error.message : String(error)}`);
  process.exit(1);
});
