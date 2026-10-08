import { randomBytes } from 'node:crypto';
import { execFileSync } from 'node:child_process';
import path from 'node:path';
import { createRequire } from 'node:module';

for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) {
  delete process.env[key];
}

const ownerUserId = '136907714';
const resolverUserId = '136907713';
const dcId = Number(process.env.TG_DC_ID || '2');
const dcHost = process.env.TG_DC_HOST || '127.0.0.1';
const dcPort = Number(process.env.TG_DC_PORT || '11443');
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
    'exec', 'teamgram-postgres', 'psql', '-v', 'ON_ERROR_STOP=1', '-At', '-q', '-F', '\t', '-U', 'teamgram', '-d', 'teamgram', '-c', query,
  ], { encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'] }).trim();
}

function loadAuthKey(userId: string): string {
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
  if (key.length !== 256) throw new Error(`production auth key for ${userId} has an invalid length`);
  return key.toString('hex');
}

function ownerUsername(): string {
  return sql(`SELECT COALESCE(username, '') FROM users WHERE id=${ownerUserId}`);
}

function activeOwnerUsernameRows(): number {
  return Number(sql(`SELECT COUNT(*) FROM username WHERE peer_type=2 AND peer_id=${ownerUserId} AND deleted=0`));
}

function candidateUsernameRows(username: string): string[] {
  const output = sql(`SELECT peer_type, peer_id, deleted FROM username WHERE username='${username}' ORDER BY id`);
  return output ? output.split('\n') : [];
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

async function main() {
  const username = `audit${Date.now().toString(36)}${randomBytes(4).toString('hex')}`;
  if (ownerUsername() !== '' || activeOwnerUsernameRows() !== 0 || candidateUsernameRows(username).length !== 0) {
    throw new Error('production username precondition failed; owner already has a username or candidate is occupied');
  }

  const ownerClient = makeClient(loadAuthKey(ownerUserId));
  const resolverClient = makeClient(loadAuthKey(resolverUserId));
  let updateAttempted = false;
  let failure: unknown;
  let cleanupError: unknown;
  const evidence: Record<string, unknown> = {};

  try {
    await ownerClient.connect();
    const owner = await ownerClient.getMe();
    if (String(owner?.id) !== ownerUserId) throw new Error('username owner auth key user mismatch');

    await resolverClient.connect();
    const resolver = await resolverClient.getMe();
    if (String(resolver?.id) !== resolverUserId) throw new Error('username resolver auth key user mismatch');

    updateAttempted = true;
    const updated = await ownerClient.invoke(new Api.account.UpdateUsername({ username }));
    if (!matchesType(updated, 'User') || String(updated?.id) !== ownerUserId || updated?.username !== username) {
      throw new Error(`account.updateUsername returned ${rpcName(updated)} or the wrong user`);
    }
    if (ownerUsername() !== username || activeOwnerUsernameRows() !== 1
      || JSON.stringify(candidateUsernameRows(username)) !== JSON.stringify([`2\t${ownerUserId}\t0`])) {
      throw new Error('account.updateUsername did not persist the expected user and username index rows');
    }

    const resolved = await resolverClient.invoke(new Api.contacts.ResolveUsername({ username }));
    if (!matchesType(resolved, 'ResolvedPeer') || String(resolved?.peer?.userId) !== ownerUserId) {
      throw new Error(`contacts.resolveUsername returned ${rpcName(resolved)} or the wrong user peer`);
    }
    const resolvedUser = Array.isArray(resolved.users)
      ? resolved.users.find((item: any) => String(item?.id) === ownerUserId)
      : undefined;
    if (!matchesType(resolvedUser, 'User') || resolvedUser.username !== username) {
      throw new Error('contacts.resolveUsername did not hydrate the temporary username owner');
    }

    evidence.update = rpcName(updated);
    evidence.resolve = rpcName(resolved);
    evidence.resolvedUser = rpcName(resolvedUser);
    evidence.usernameIndexMatched = true;
  } catch (error) {
    failure = error;
  }

  if (updateAttempted) {
    try {
      const cleared = await ownerClient.invoke(new Api.account.UpdateUsername({ username: '' }));
      if (!matchesType(cleared, 'User') || String(cleared?.id) !== ownerUserId || cleared?.username) {
        throw new Error(`account.updateUsername cleanup returned ${rpcName(cleared)} or retained a username`);
      }
      if (ownerUsername() !== '' || activeOwnerUsernameRows() !== 0 || candidateUsernameRows(username).length !== 0) {
        throw new Error('production username cleanup left user or username index rows');
      }
      evidence.cleanup = 'username cleared through account.updateUsername; PostgreSQL rows absent';
    } catch (error) {
      cleanupError = error;
    }
  }

  ownerClient.destroy();
  resolverClient.destroy();
  if (cleanupError) throw cleanupError;
  if (failure) throw failure;
  if (!updateAttempted) throw new Error('production user username flow did not run');

  console.log(JSON.stringify({
    backend: process.env.TEAMGRAM_BACKEND_TAG || 'r24',
    transport: `DC${dcId} WebSocket -> gateway -> session -> Usernames BFF -> User service -> PostgreSQL`,
    ownerUserId,
    resolverUserId,
    ...evidence,
    ownerUsernameAfterCleanup: ownerUsername(),
    activeUserUsernameRowsAfterCleanup: activeOwnerUsernameRows(),
    businessWrites: 2,
  }));
}

main().then(() => process.exit(0)).catch((error) => {
  console.error(`user username resolution production probe failed: ${error instanceof Error ? error.stack || error.message : String(error)}`);
  process.exit(1);
});
