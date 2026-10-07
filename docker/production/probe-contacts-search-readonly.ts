import { execFileSync } from 'node:child_process';
import path from 'node:path';
import { createRequire } from 'node:module';

for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) {
  delete process.env[key];
}

const userId = '136907714';
const dcId = 2;
const dcHost = '127.0.0.1';
const dcPort = 11443;
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
  const key = Buffer.from(body, 'base64');
  if (key.length !== 256) throw new Error('production auth key has an invalid length');
  return key.toString('hex');
}

function loadSearchSample(): { id: string; query: string; expectedBucket: 'my_results' | 'results' } {
  const row = sql(`
    SELECT u.id, u.first_name, u.last_name, IF(c.contact_user_id IS NULL, 0, 1)
    FROM users u
    LEFT JOIN user_contacts c ON c.owner_user_id=${userId} AND c.contact_user_id=u.id AND c.is_deleted=0
    WHERE u.id<>${userId} AND u.deleted=0
      AND (CHAR_LENGTH(u.first_name)>=3 OR CHAR_LENGTH(u.last_name)>=3)
    ORDER BY (c.contact_user_id IS NOT NULL) DESC, u.id DESC
    LIMIT 1
  `);
  const [id, firstName, lastName, isContact] = row.split('\t');
  if (!id || isContact === undefined) throw new Error(`no searchable production user sample found for user ${userId}`);
  const query = firstName.length >= 3 ? firstName : lastName;
  if (query.length < 3) throw new Error('production MySQL sample does not meet contacts.search minimum query length');
  return { id, query, expectedBucket: isContact === '1' ? 'my_results' : 'results' };
}

function rpcName(value: any): string {
  return value?.className || value?.constructor?.name || typeof value;
}

function matchesType(value: any, type: string): boolean {
  const actual = rpcName(value);
  return actual === type || actual.endsWith(`.${type}`);
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

async function preflightWebSocket(): Promise<void> {
  await new Promise<void>((resolve, reject) => {
    const socket = new WebSocket(`ws://${dcHost}:${dcPort}/apiws`, 'binary');
    let settled = false;
    const finish = (error?: Error) => {
      if (settled) return;
      settled = true;
      clearTimeout(timer);
      socket.close();
      if (error) reject(error);
      else resolve();
    };
    const timer = setTimeout(() => finish(new Error('DC2 WebSocket preflight timed out')), 5000);
    socket.addEventListener('open', () => finish());
    socket.addEventListener('error', () => finish(new Error('DC2 WebSocket preflight failed')));
    socket.addEventListener('close', () => {
      if (!settled) finish(new Error('DC2 WebSocket closed before opening'));
    });
  });
}

async function main() {
  await preflightWebSocket();

  const sample = loadSearchSample();
  const client = makeClient(loadAuthKey());
  try {
    await client.connect();
    const me = await client.getMe();
    if (String(me?.id) !== userId) throw new Error('auth key user mismatch');

    const found = await client.invoke(new Api.contacts.Search({ q: sample.query, limit: 50 }));
    if (!matchesType(found, 'Found')) throw new Error(`contacts.search returned ${rpcName(found)}`);
    if (!Array.isArray(found.myResults) || !Array.isArray(found.results)
      || !Array.isArray(found.users) || !Array.isArray(found.chats)) {
      throw new Error('contacts.search returned malformed contacts.Found vectors');
    }

    const expectedPeers = sample.expectedBucket === 'my_results' ? found.myResults : found.results;
    const otherPeers = sample.expectedBucket === 'my_results' ? found.results : found.myResults;
    const matchingPeers = expectedPeers.filter((peer: any) => matchesType(peer, 'PeerUser')
      && String(peer.userId) === sample.id);
    const wrongBucketPeers = otherPeers.filter((peer: any) => matchesType(peer, 'PeerUser')
      && String(peer.userId) === sample.id);
    const user = found.users.find((item: any) => String(item?.id) === sample.id);
    if (matchingPeers.length !== 1 || wrongBucketPeers.length !== 0 || !matchesType(user, 'User')) {
      throw new Error('contacts.search did not return the MySQL user in its authoritative contact bucket with a hydrated User');
    }

    console.log(JSON.stringify({
      backend: process.env.TEAMGRAM_BACKEND_TAG || 'r24',
      transport: `DC${dcId} WebSocket -> session -> contacts.search`,
      authenticatedUserId: String(me.id),
      method: 'contacts.search',
      responseType: rpcName(found).includes('.') ? rpcName(found) : `contacts.${rpcName(found)}`,
      productionMySqlSampleId: sample.id,
      bucketMatchesMySqlContactList: true,
      sample: {
        bucket: sample.expectedBucket,
        peerType: rpcName(matchingPeers[0]),
        userType: rpcName(user),
      },
      myResultsCount: found.myResults.length,
      resultsCount: found.results.length,
      usersCount: found.users.length,
      chatsCount: found.chats.length,
      businessWrites: 0,
    }));
  } finally {
    client.destroy();
  }
}

main().then(() => process.exit(0)).catch((error) => {
  console.error(`contacts.search production read-only probe failed: ${error instanceof Error ? error.stack || error.message : String(error)}`);
  process.exit(1);
});
