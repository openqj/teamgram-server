import { randomBytes } from 'node:crypto';
import { execFileSync } from 'node:child_process';
import path from 'node:path';
import { createRequire } from 'node:module';

for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) {
  delete process.env[key];
}

const userId = process.env.CHANNEL_LOCATION_USER_ID || '136907714';
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
  const queries: Record<string, string> = {
    channel: `SELECT COUNT(*) FROM apifull_channel WHERE id=${channelId}`,
    members: `SELECT COUNT(*) FROM apifull_channel_member WHERE channel_id=${channelId}`,
    messages: `SELECT COUNT(*) FROM apifull_channel_message WHERE channel_id=${channelId}`,
    events: `SELECT COUNT(*) FROM apifull_channel_event WHERE channel_id=${channelId}`,
    adminLog: `SELECT COUNT(*) FROM apifull_channel_admin_log WHERE channel_id=${channelId}`,
  };
  return Object.fromEntries(Object.entries(queries).map(([key, query]) => [key, Number(postgres(query))]));
}

function storedLocation(channelId: bigint): string {
  return postgres(`SELECT CONCAT(COALESCE(location_lat::text,'NULL'),'|',
    COALESCE(location_long::text,'NULL'),'|',location_address)
    FROM apifull_channel WHERE id=${channelId} AND creator_user_id=${userId}`);
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

function boolTrue(value: any): boolean {
  return value === true || value?.value === true || value?.value === 1;
}

function tlString(value: string): Buffer {
  const data = Buffer.from(value, 'utf8');
  let header: Buffer;
  if (data.length < 254) {
    header = Buffer.from([data.length]);
  } else {
    header = Buffer.alloc(4);
    header[0] = 254;
    header.writeUIntLE(data.length, 1, 3);
  }
  const padding = Buffer.alloc((4 - ((header.length + data.length) % 4)) % 4);
  return Buffer.concat([header, data, padding]);
}

class EditLocationRequest {
  CONSTRUCTOR_ID = 0x58e63f6d;
  SUBCLASS_OF_ID = 0;
  className = 'channels.EditLocation';
  classType = 'request';

  constructor(private readonly channel: any, private readonly geoPoint: any, private readonly address: string) {}

  getBytes(): Uint8Array {
    const constructor = Buffer.alloc(4);
    constructor.writeUInt32LE(this.CONSTRUCTOR_ID, 0);
    return Uint8Array.from(Buffer.concat([
      constructor,
      Buffer.from(this.channel.getBytes()),
      Buffer.from(this.geoPoint.getBytes()),
      tlString(this.address),
    ]));
  }

  readResult(reader: any): any {
    return reader.tgReadObject();
  }
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

function createdChats(value: any): any[] {
  return [
    ...(Array.isArray(value?.chats) ? value.chats : []),
    ...(Array.isArray(value?.updates?.chats) ? value.updates.chats : []),
  ];
}

function assertLocation(full: any, lat: number, long: number, address: string): void {
  const location = full?.fullChat?.location;
  const geoPoint = location?.geoPoint;
  if (!rpcName(full).toLowerCase().includes('chatfull')
    || !rpcName(location).toLowerCase().includes('channellocation')
    || Number(geoPoint?.lat) !== lat || Number(geoPoint?.long) !== long || location?.address !== address) {
    throw new Error('channels.getFullChannel location readback mismatch');
  }
}

async function main() {
  const client = makeClient(loadAuthKey());
  const title = `r28-location-${Date.now()}-${randomBytes(4).toString('hex')}`;
  let channelId = 0n;
  let accessHash = 0n;
  let failure: unknown;
  let cleanupError: unknown;
  let evidence: Record<string, unknown> = {};

  try {
    await client.connect();
    const me = await client.getMe();
    if (String(me?.id) !== userId) throw new Error('production auth key user mismatch');

    const created = await client.invoke(new Api.channels.CreateChannel({
      broadcast: true,
      title,
      about: 'temporary production location probe',
    }));
    const channel = createdChats(created).find((item: any) => item?.title === title);
    if (!channel?.id || channel.accessHash === undefined) {
      throw new Error(`channels.createChannel returned ${rpcName(created)} without channel identity`);
    }
    channelId = BigInt(String(channel.id));
    accessHash = BigInt(String(channel.accessHash));
    const inputChannel = new Api.InputChannel({ channelId, accessHash });
    const lat = 31.2304;
    const long = 121.4737;
    const address = 'Shanghai';

    const updated = await client.invoke(new EditLocationRequest(
      inputChannel,
      new Api.InputGeoPoint({ lat, long }),
      address,
    ));
    if (!boolTrue(updated)) throw new Error(`channels.editLocation returned ${rpcName(updated)}`);
    const full = await client.invoke(new Api.channels.GetFullChannel({ channel: inputChannel }));
    assertLocation(full, lat, long, address);
    const stored = storedLocation(channelId).split('|');
    if (Number(stored[0]) !== lat || Number(stored[1]) !== long || stored[2] !== address) {
      throw new Error('channels.editLocation PostgreSQL readback mismatch');
    }

    const cleared = await client.invoke(new EditLocationRequest(
      inputChannel,
      new Api.InputGeoPointEmpty(),
      'ignored',
    ));
    if (!boolTrue(cleared)) throw new Error(`channels.editLocation(clear) returned ${rpcName(cleared)}`);
    const emptyFull = await client.invoke(new Api.channels.GetFullChannel({ channel: inputChannel }));
    if (!rpcName(emptyFull?.fullChat?.location).toLowerCase().includes('channellocationempty')
      || storedLocation(channelId) !== 'NULL|NULL|') {
      throw new Error(`channels.editLocation empty-point clear mismatch: type=${rpcName(emptyFull?.fullChat?.location)}, postgres=${storedLocation(channelId)}`);
    }

    evidence = {
      createChannel: rpcName(created),
      editLocation: [rpcName(updated), rpcName(cleared)],
      getFullChannel: [rpcName(full), rpcName(emptyFull)],
      coordinateAddressReadback: true,
      clearReadback: true,
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
      if (Number(postgres(`SELECT COUNT(*) FROM apifull_channel WHERE id=${channelId}`)) > 0) {
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
  const remaining = channelRows(channelId);
  evidence.channelRowsAfterDelete = remaining;
  if (Object.values(remaining).some((count) => count !== 0)) {
    throw new Error(`temporary location channel cleanup left production rows: ${JSON.stringify(evidence)}`);
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
  console.error(`channel location roundtrip probe failed: ${error instanceof Error ? error.stack || error.message : String(error)}`);
  process.exit(1);
});
