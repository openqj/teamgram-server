import { execFileSync } from 'node:child_process';
import path from 'node:path';
import { createRequire } from 'node:module';

const gramjsDir = process.env.TEAMGRAM_GRAMJS_DIR || '/Users/mac/open/test/telegram-tt-master';
const dcId = Number(process.env.TG_DC_ID || '2');
const userId = BigInt(process.env.PERSONAL_CHANNEL_USER_ID || '136907714');
const channelId = BigInt(process.env.PERSONAL_CHANNEL_ID || '1');
const userCacheKey = `user_data.2#${userId}`;
const globalAny = globalThis as typeof globalThis & { self?: any; addEventListener?: () => void };
globalAny.self ??= globalThis;
globalAny.addEventListener ??= () => {};
globalAny.self.addEventListener ??= globalAny.addEventListener;

function sql(query: string): string {
  return execFileSync('docker', ['exec', 'teamgram-postgres', 'psql', '-v', 'ON_ERROR_STOP=1', '-At', '-q', '-F', '\t', '-U', 'teamgram', '-d', 'teamgram', '-c', query], {
    encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'],
  }).trim();
}

function sqlWrite(query: string): void {
  execFileSync('docker', ['exec', 'teamgram-postgres', 'psql', '-v', 'ON_ERROR_STOP=1', '-At', '-q', '-F', '\t', '-U', 'teamgram', '-d', 'teamgram', '-c', query], {
    stdio: ['ignore', 'ignore', 'ignore'],
  });
}

type CacheSnapshot = { present: boolean; value: string };
function cacheSnapshot(): CacheSnapshot {
  const present = execFileSync('docker', ['exec', 'redis', 'redis-cli', 'EXISTS', userCacheKey], { encoding: 'utf8' }).trim() === '1';
  const raw = present ? execFileSync('docker', ['exec', 'redis', 'redis-cli', '--raw', 'GET', userCacheKey], { encoding: 'utf8' }) : '';
  return { present, value: raw.endsWith('\n') ? raw.slice(0, -1) : raw };
}

function restoreCache(snapshot: CacheSnapshot): void {
  if (snapshot.present) execFileSync('docker', ['exec', 'redis', 'redis-cli', 'SET', userCacheKey, snapshot.value], { stdio: 'ignore' });
  else execFileSync('docker', ['exec', 'redis', 'redis-cli', 'DEL', userCacheKey], { stdio: 'ignore' });
}

function loadAuthKey(): string {
  const body = sql(`SELECT encode(k.body, 'base64') FROM auth_users u JOIN auth_keys k USING(auth_key_id)
    JOIN auth_key_infos i USING(auth_key_id) WHERE u.user_id=${userId} AND u.state=0 AND u.deleted=0
    AND k.deleted=0 AND i.deleted=0 AND i.auth_key_type=0 ORDER BY u.date_active DESC,u.id DESC LIMIT 1`);
  if (!body) throw new Error(`no active production auth key found for user ${userId}`);
  return Buffer.from(body, 'base64').toString('hex');
}

const requireModule = createRequire(import.meta.url);
const TelegramClient = requireModule(path.join(gramjsDir, 'src/lib/gramjs/client/TelegramClient.ts')).default;
const CallbackSession = requireModule(path.join(gramjsDir, 'src/lib/gramjs/sessions/CallbackSession.ts')).default;
const Api = requireModule(path.join(gramjsDir, 'src/lib/gramjs/tl/index.ts')).Api;
const Connection = requireModule(path.join(gramjsDir, 'src/lib/gramjs/network/connection/TCPObfuscated.ts')).ConnectionTCPObfuscated;
const quietLogger = { debug() {}, info() {}, warn() {}, error() {} };
const rpcName = (value: any): string => value?.className || value?.constructor?.name || typeof value;

function makeClient(authKeyHex: string): any {
  const session = new CallbackSession({ mainDcId: dcId, keys: { [dcId]: authKeyHex } }, () => {});
  const load = session.load.bind(session);
  session.load = async () => { await load(); (session as any)._serverAddress = process.env.TG_DC_HOST || '127.0.0.1'; (session as any)._port = Number(process.env.TG_DC_PORT || '11443'); };
  const client = new TelegramClient(session, 1, '0123456789abcdef0123456789abcdef', {
    connection: Connection, fallbackConnection: Connection,
    shouldAllowHttpTransport: false, shouldForceHttpTransport: false,
    timeout: 20, requestRetries: 1, connectionRetries: 1,
    connectionRetriesToFallback: 0, autoReconnect: false, baseLogger: quietLogger,
  });
  (client as any)._borrowExportedSender = async () => undefined;
  (client as any)._updateLoop = async () => undefined;
  return client;
}

async function main() {
  const owner = sql(`SELECT creator_user_id,access_hash FROM channels WHERE id=${channelId} AND deleted=0 LIMIT 1`).split('\t');
  if (owner.length < 2 || owner[0] !== String(userId)) throw new Error(`channel ${channelId} is not owned by user ${userId}`);
  const channelHash = BigInt(owner[1]);
  const beforeValue = sql(`SELECT personal_channel_id FROM users WHERE id=${userId} AND deleted=0 LIMIT 1`);
  if (!beforeValue) throw new Error(`user ${userId} not found`);
  const before = BigInt(beforeValue);
  const beforeCache = cacheSnapshot();
  const client = makeClient(loadAuthKey());
  try {
    await client.connect();
    if (String((await client.getMe())?.id) !== String(userId)) throw new Error('auth key user mismatch');
    const result = await client.invoke(new Api.account.UpdatePersonalChannel({
      channel: new Api.InputChannel({ channelId, accessHash: channelHash }),
    }));
    const ok = result === true || result?.value === true || result?.value === 1 || rpcName(result).toLowerCase().includes('true');
    if (!ok) throw new Error(`account.updatePersonalChannel returned ${rpcName(result)} value=${result?.value ?? result}`);
    const afterDB = BigInt(sql(`SELECT personal_channel_id FROM users WHERE id=${userId} LIMIT 1`));
    if (afterDB !== channelId) throw new Error(`personal channel DB value ${afterDB} != ${channelId}`);
    const full = await client.invoke(new Api.users.GetFullUser({ id: new Api.InputUserSelf() }));
    if (rpcName(full) !== 'users.UserFull') throw new Error(`users.getFullUser returned ${rpcName(full)}`);
    console.log(JSON.stringify({ userId: String(userId), channelId: String(channelId), result: rpcName(result), db: String(afterDB), fullUser: rpcName(full), personalChannelField: full?.fullUser?.personalChannelId === undefined ? 'not-exposed-by-production-plugin' : String(full.fullUser.personalChannelId) }));
  } finally {
    try { sqlWrite(`UPDATE users SET personal_channel_id=${before} WHERE id=${userId}`); } finally {
      try { restoreCache(beforeCache); } finally { client.destroy(); }
    }
  }
  const restoredDB = BigInt(sql(`SELECT personal_channel_id FROM users WHERE id=${userId} LIMIT 1`));
  const restoredCache = cacheSnapshot();
  if (restoredDB !== before || restoredCache.present !== beforeCache.present || restoredCache.value !== beforeCache.value) {
    throw new Error(`personal channel restore mismatch: db ${restoredDB}/${before}, cache ${JSON.stringify(restoredCache)}/${JSON.stringify(beforeCache)}`);
  }
  console.log(JSON.stringify({ restored: true, originalPersonalChannel: String(before), cacheRestored: true }));
}

main().then(() => process.exit(0)).catch((error) => {
  console.error(`personal-channel production roundtrip failed: ${error instanceof Error ? error.stack || error.message : String(error)}`);
  process.exit(1);
});
