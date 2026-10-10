import { execFileSync } from 'node:child_process';
import path from 'node:path';
import { createRequire } from 'node:module';

const userId = 136907714;
const gramjsDir = process.env.TEAMGRAM_GRAMJS_DIR || '/Users/mac/open/test/telegram-tt-master';
const dcId = Number(process.env.TG_DC_ID || '2');
const dcHost = process.env.TG_DC_HOST || '127.0.0.1';
const dcPort = Number(process.env.TG_DC_PORT || '11443');
const cutoff = Math.floor(Date.now() / 1000) - 30 * 24 * 60 * 60;

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
const Connection = requireModule(path.join(gramjsDir, 'src/lib/gramjs/network/connection/TCPObfuscated.ts')).ConnectionTCPObfuscated;
const quietLogger = { debug() {}, info() {}, warn() {}, error() {} };

function rpcName(value: any): string {
  return value?.className || value?.constructor?.name || typeof value;
}

function int32(value: number): Uint8Array {
  const bytes = new Uint8Array(4);
  new DataView(bytes.buffer).setInt32(0, value, true);
  return bytes;
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

function parseRows(value: string): Array<{ id: string; date: number }> {
  if (!value) return [];
  return value.split('\n').map((line) => {
    const [id, rawDate] = line.split('\t');
    const date = Number(rawDate);
    if (!id || !Number.isSafeInteger(date)) throw new Error(`invalid production channel row: ${line}`);
    return { id, date };
  });
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
  const channelRows = parseRows(postgres(`
    SELECT c.id, COALESCE(MAX(m.date), c.created_at) AS last_active
    FROM apifull_channel c
    LEFT JOIN apifull_channel_message m ON m.channel_id=c.id
    WHERE c.creator_user_id=${userId}
    GROUP BY c.id, c.created_at
    ORDER BY last_active ASC, c.id ASC
  `));
  const expected = channelRows.filter((row) => row.date <= cutoff);

  const client = makeClient(authKey);
  try {
    await client.connect();
    const me = await client.getMe();
    if (String(me?.id) !== String(userId)) throw new Error('auth key user mismatch');
    const request = {
      CONSTRUCTOR_ID: 0x11e831ee,
      SUBCLASS_OF_ID: 0,
      className: 'channels.GetInactiveChannels',
      classType: 'request',
      getBytes: () => int32(0x11e831ee),
      readResult: (reader: any) => reader.tgReadObject(),
    };
    const result = await client.invoke(request);
    const actualName = rpcName(result);
    const actualChats = Array.isArray(result?.chats) ? result.chats : [];
    const actualDates = Array.isArray(result?.dates) ? result.dates.map(Number) : [];
    if (!actualName.endsWith('InactiveChats')
      || actualChats.length !== expected.length
      || actualDates.length !== expected.length) {
      throw new Error(`inactive channels mismatch: type=${actualName}, chats=${actualChats.length}, dates=${actualDates.length}, postgres=${expected.length}`);
    }
    for (let index = 0; index < expected.length; index += 1) {
      if (String(actualChats[index]?.id) !== expected[index].id
        || actualDates[index] !== expected[index].date) {
        throw new Error(`inactive channel row ${index} differs from production PostgreSQL`);
      }
    }
    console.log(JSON.stringify({
      userId: String(me.id),
      transport: `DC${dcId} WebSocket -> gateway -> session -> BFF -> PostgreSQL`,
      resultType: actualName,
      cutoff,
      creatorChannelsInPostgreSQL: channelRows.length,
      inactiveChannelsInPostgreSQL: expected.length,
      returnedChats: actualChats.length,
      returnedDates: actualDates.length,
      returnedUsers: Array.isArray(result?.users) ? result.users.length : null,
      writes: 0,
    }));
  } finally {
    client.destroy();
  }
}

main().then(() => process.exit(0)).catch((error) => {
  console.error(`inactive channels production probe failed: ${error instanceof Error ? error.stack || error.message : String(error)}`);
  process.exit(1);
});
