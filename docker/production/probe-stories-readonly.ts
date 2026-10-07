import { createHash } from 'node:crypto';
import { execFileSync } from 'node:child_process';
import path from 'node:path';
import { createRequire } from 'node:module';

const gramjsDir = process.env.TEAMGRAM_GRAMJS_DIR || '/Users/mac/open/test/telegram-tt-master';
const dcId = Number(process.env.TG_DC_ID || '2');
const dcHost = process.env.TG_DC_HOST || '127.0.0.1';
const dcPort = Number(process.env.TG_DC_PORT || '11443');
const userId = process.env.STORY_USER_ID || '136907714';

const globalAny = globalThis as typeof globalThis & { self?: any; addEventListener?: () => void };
globalAny.self ??= globalThis;
globalAny.addEventListener ??= () => {};
globalAny.self.addEventListener ??= globalAny.addEventListener;

function sql(query: string): string {
  return execFileSync('docker', [
    'exec', 'mysql', 'mysql', '-N', '-s', '-uteamgram', '-pteamgram', '-Dteamgram', '-e', query,
  ], { encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'] }).trim();
}

function loadAuthKey(): string {
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
  return Buffer.from(body, 'base64').toString('hex');
}

function storySnapshot(): string {
  const value = sql(`SELECT v FROM apifull_kv WHERE k='story:${userId}'`);
  if (!value) throw new Error(`no production story record found for user ${userId}`);
  return createHash('sha256').update(value).digest('hex');
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

function storyIds(result: any): number[] {
  return (Array.isArray(result?.stories) ? result.stories : [])
    .map((item: any) => Number(item?.id))
    .filter((id: number) => Number.isInteger(id));
}

async function main() {
  const before = storySnapshot();
  const client = makeClient(loadAuthKey());
  try {
    (client as any)._borrowExportedSender = async () => undefined;
    (client as any)._updateLoop = async () => undefined;
    await client.connect();
    const me = await client.getMe();
    if (String(me?.id) !== userId) throw new Error(`auth key user mismatch: ${String(me?.id)}`);

    const peer = new Api.InputPeerSelf();
    const archive = await client.invoke(new Api.stories.GetStoriesArchive({
      peer, offsetId: 0, limit: 100,
    }));
    if (!rpcName(archive).toLowerCase().endsWith('stories')) {
      throw new Error(`stories.getStoriesArchive returned ${rpcName(archive)}`);
    }
    const archiveIds = storyIds(archive);
    if (archiveIds.length !== 8 || archiveIds.join(',') !== '1,2,3,4,5,6,7,8') {
      throw new Error(`unexpected archive stories: ${archiveIds.join(',')}`);
    }

    const byId = await client.invoke(new Api.stories.GetStoriesByID({
      peer: new Api.InputPeerSelf(), id: [1, 2, 3],
    }));
    if (!rpcName(byId).toLowerCase().endsWith('stories')) {
      throw new Error(`stories.getStoriesByID returned ${rpcName(byId)}`);
    }
    const byIdIds = storyIds(byId);
    if (byIdIds.length !== 3 || byIdIds.join(',') !== '1,2,3') {
      throw new Error(`unexpected stories by id: ${byIdIds.join(',')}`);
    }

    const after = storySnapshot();
    if (before !== after) throw new Error('story KV changed during read-only probe');
    console.log(JSON.stringify({
      userId: String(me.id),
      archiveType: rpcName(archive),
      archiveCount: archiveIds.length,
      archiveIds,
      byIdType: rpcName(byId),
      byIdCount: byIdIds.length,
      byIdIds,
      storyKvSha256: after,
      writes: 0,
    }));
  } finally {
    client.destroy();
  }
}

main().then(() => process.exit(0)).catch((error) => {
  console.error(`stories production probe failed: ${error instanceof Error ? error.stack || error.message : String(error)}`);
  process.exit(1);
});
