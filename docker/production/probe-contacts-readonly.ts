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

type ContactSnapshot = { id: number; lastSeenAt?: number; expires?: number };

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

function loadContactSnapshot(): ContactSnapshot[] {
  const output = sql(`
    SELECT c.contact_user_id, p.last_seen_at, p.expires
    FROM user_contacts c
    LEFT JOIN user_presences p ON p.user_id=c.contact_user_id
    WHERE c.owner_user_id=${userId} AND c.is_deleted=0
    ORDER BY c.contact_user_id
  `);
  if (!output) return [];
  return output.split('\n').map((line) => {
    const [id, lastSeenAt, expires] = line.split('\t');
    if (!id) throw new Error('production contact snapshot contains an invalid row');
    return {
      id: Number(id),
      ...(lastSeenAt ? { lastSeenAt: Number(lastSeenAt) } : {}),
      ...(expires ? { expires: Number(expires) } : {}),
    };
  });
}

function contactHash(ids: number[]): bigint {
  const mod = 0x80000000n;
  return ids.reduce((hash, id) => (hash * 20261n + mod + BigInt(id)) % mod, 0n);
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
  const before = loadContactSnapshot();
  const client = makeClient(loadAuthKey());
  try {
    await client.connect();
    const me = await client.getMe();
    if (String(me?.id) !== userId) throw new Error('auth key user mismatch');

    const contactIds = await client.invoke(new Api.contacts.GetContactIDs({ hash: 0n }));
    if (!Array.isArray(contactIds)) throw new Error('contacts.getContactIDs did not return a vector');
    const expectedIds = before.map((row) => row.id);
    if (contactIds.length !== expectedIds.length || contactIds.some((id: number, index: number) => id !== expectedIds[index])) {
      throw new Error('contacts.getContactIDs did not match active production user_contacts rows');
    }

    const unchanged = await client.invoke(new Api.contacts.GetContactIDs({ hash: contactHash(expectedIds) }));
    if (!Array.isArray(unchanged) || unchanged.length !== 0) {
      throw new Error('contacts.getContactIDs did not return an empty vector for the current contact hash');
    }

    const statuses = await client.invoke(new Api.contacts.GetStatuses({}));
    if (!Array.isArray(statuses) || statuses.length !== before.length) {
      throw new Error('contacts.getStatuses did not return one status per active contact');
    }
    for (let index = 0; index < before.length; index++) {
      const row = before[index];
      const item = statuses[index];
      if (Number(item?.userId) !== row.id) throw new Error('contacts.getStatuses returned an unexpected contact order');
      const statusType = rpcName(item?.status);
      if (row.lastSeenAt === undefined) {
        if (!statusType.endsWith('UserStatusEmpty')) throw new Error('contacts.getStatuses did not return empty status for missing presence');
      } else if (Math.floor(Date.now() / 1000) <= row.lastSeenAt + 60) {
        if (!statusType.endsWith('UserStatusOnline') || Number(item.status.expires) !== row.lastSeenAt + 60) {
          throw new Error('contacts.getStatuses online status did not match production presence');
        }
      } else if (!statusType.endsWith('UserStatusOffline') || Number(item.status.wasOnline) !== row.lastSeenAt) {
        throw new Error('contacts.getStatuses offline status did not match production presence');
      }
    }

    const fullContacts = await client.invoke(new Api.contacts.GetContacts({ hash: 0n }));
    if (!matchesType(fullContacts, 'Contacts') || matchesType(fullContacts, 'ContactsNotModified')
      || !Array.isArray(fullContacts.contacts) || !Array.isArray(fullContacts.users)) {
      throw new Error(`contacts.getContacts(hash=0) returned malformed ${rpcName(fullContacts)}`);
    }
    const returnedContactIds = fullContacts.contacts.map((contact: any) => Number(contact?.userId));
    const returnedUserIds = fullContacts.users.map((user: any) => Number(user?.id)).sort((a: number, b: number) => a - b);
    if (JSON.stringify(returnedContactIds) !== JSON.stringify(expectedIds)
      || JSON.stringify(returnedUserIds) !== JSON.stringify(expectedIds)
      || Number(fullContacts.savedCount) !== 0) {
      throw new Error('contacts.getContacts full response did not match active production contacts and hydrated users');
    }
    const unchangedContacts = await client.invoke(new Api.contacts.GetContacts({ hash: contactHash(expectedIds) }));
    if (!matchesType(unchangedContacts, 'ContactsNotModified')) {
      throw new Error(`contacts.getContacts(current hash) returned ${rpcName(unchangedContacts)}, want contacts.contactsNotModified`);
    }

    const after = loadContactSnapshot();
    if (JSON.stringify(before) !== JSON.stringify(after)) throw new Error('contact or presence rows changed during read-only probe');
    console.log(JSON.stringify({
      backend: process.env.TEAMGRAM_BACKEND_TAG || 'r24',
      transport: `DC${dcId} WebSocket -> gateway -> session -> Contacts BFF -> User service -> MySQL`,
      authenticatedUserId: userId,
      methods: {
        'contacts.getContactIDs': { type: 'Vector<int>', rows: contactIds.length, matchesMySql: true, hashMatchEmpty: true },
        'contacts.getStatuses': {
          type: 'Vector<ContactStatus>',
          rows: statuses.length,
          presenceMatched: statuses.length,
          statusTypes: statuses.map((item: any) => rpcName(item.status)),
        },
        'contacts.getContacts': {
          fullType: rpcName(fullContacts),
          contacts: returnedContactIds.length,
          hydratedUsers: returnedUserIds.length,
          savedCount: Number(fullContacts.savedCount),
          matchingHashType: rpcName(unchangedContacts),
        },
      },
      contactAndPresenceSnapshotsUnchanged: true,
      businessWrites: 0,
    }));
  } finally {
    client.destroy();
  }
}

main().then(() => process.exit(0)).catch((error) => {
  console.error(`contacts read-only production probe failed: ${error instanceof Error ? error.stack || error.message : String(error)}`);
  process.exit(1);
});
