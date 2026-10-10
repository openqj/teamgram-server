import { execFileSync } from 'node:child_process';
import path from 'node:path';
import { createRequire } from 'node:module';

const gramjsDir = process.env.TEAMGRAM_GRAMJS_DIR || '/Users/mac/open/test/telegram-tt-master';
const dcId = Number(process.env.TG_DC_ID || '2');
const userId = '136907714';

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

function loadAuthKey(): string {
  const body = sql(`
    SELECT encode(k.body, 'base64')
    FROM auth_users u
    JOIN auth_keys k USING (auth_key_id)
    JOIN auth_key_infos i USING (auth_key_id)
    WHERE u.user_id=${userId} AND u.state=0 AND u.deleted=0
      AND k.deleted=0 AND i.deleted=0 AND i.auth_key_type=0
    ORDER BY u.date_active DESC, u.id DESC
    LIMIT 1
  `);
  if (!body) throw new Error(`no active production auth key found for user ${userId}`);
  return Buffer.from(body, 'base64').toString('hex');
}

function loadMessageTarget(): { channelId: bigint; accessHash: bigint; messageId: number; expectedViews: number } {
  const value = sql(`
    SELECT c.id,c.access_hash,m.message_id,COUNT(r.user_id)
    FROM apifull_channel c
    JOIN apifull_channel_message m ON m.channel_id=c.id
    LEFT JOIN apifull_channel_read_state r
      ON r.channel_id=m.channel_id AND r.read_max_id>=m.message_id
    WHERE c.creator_user_id=${userId}
    GROUP BY c.id,c.access_hash,m.message_id
    ORDER BY COUNT(r.user_id) DESC,c.id DESC,m.message_id DESC
    LIMIT 1
  `);
  const [channelId, accessHash, messageId, expectedViews] = value.split('\t');
  if (!channelId || !accessHash || !messageId || expectedViews === undefined) {
    throw new Error(`no owned production channel message found for user ${userId}`);
  }
  const target = {
    channelId: BigInt(channelId),
    accessHash: BigInt(accessHash),
    messageId: Number(messageId),
    expectedViews: Number(expectedViews),
  };
  if (target.channelId <= 0n || target.accessHash === 0n || !Number.isInteger(target.messageId) || target.messageId <= 0
    || !Number.isInteger(target.expectedViews) || target.expectedViews < 0) {
    throw new Error('production channel message query returned invalid fields');
  }
  return target;
}

function rpcName(value: any): string {
  return value?.className || value?.constructor?.name || typeof value;
}

function isType(value: any, name: string): boolean {
  const actual = rpcName(value);
  return actual === name || actual.endsWith(`.${name}`);
}

function makeClient(authKeyHex: string): any {
  const session = new CallbackSession({ mainDcId: dcId, keys: { [dcId]: authKeyHex } }, () => {});
  const load = session.load.bind(session);
  session.load = async () => {
    await load();
    (session as any)._serverAddress = process.env.TG_DC_HOST || '127.0.0.1';
    (session as any)._port = Number(process.env.TG_DC_PORT || '11443');
  };
  return new TelegramClient(session, 1, '0123456789abcdef0123456789abcdef', {
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
}

async function main() {
  const target = loadMessageTarget();
  const client = makeClient(loadAuthKey());
  try {
    (client as any)._borrowExportedSender = async () => undefined;
    (client as any)._updateLoop = async () => undefined;
    await client.connect();
    const me = await client.getMe();
    if (String(me?.id) !== userId) throw new Error('auth key user mismatch');

    const result = await client.invoke(new Api.stats.GetMessageStats({
      channel: new Api.InputChannel({ channelId: target.channelId, accessHash: target.accessHash }),
      msgId: target.messageId,
    }));
    if (!isType(result, 'MessageStats')) throw new Error(`stats.getMessageStats returned ${rpcName(result)}`);
    const graph = result.viewsGraph;
    if (!isType(graph, 'StatsGraph') || !isType(graph.json, 'DataJSON')) {
      throw new Error(`message views graph was malformed: ${rpcName(graph)} / ${rpcName(graph?.json)}`);
    }
    const graphData = JSON.parse(String(graph.json.data));
    if (Number(graphData.count) !== target.expectedViews) {
      throw new Error(`views graph disagrees with production DB: api=${graphData.count}, db=${target.expectedViews}`);
    }

    console.log(JSON.stringify({
      userId: String(me.id),
      resultType: rpcName(result),
      channelId: String(target.channelId),
      messageId: target.messageId,
      viewsGraphType: rpcName(graph),
      viewsGraphData: graphData,
      productionDatabaseViews: target.expectedViews,
      writes: 0,
    }));
  } finally {
    client.destroy();
  }
}

main().then(() => process.exit(0)).catch((error) => {
  console.error(`message stats production probe failed: ${error instanceof Error ? error.stack || error.message : String(error)}`);
  process.exit(1);
});
