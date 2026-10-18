import { execFileSync } from 'node:child_process';
import path from 'node:path';
import { createRequire } from 'node:module';

// Real Layer 229 signup probe. It uses an ephemeral unregistered phone and
// deletes the created account before returning, then verifies that no active
// user or authorization rows remain. Authsession key metadata may remain
// active because the current delete contract only unbinds auth_users.
const gramjsDir = process.env.TEAMGRAM_GRAMJS_DIR || '/Users/mac/open/test/telegram-tt-master';
const postgresContainer = process.env.TEAMGRAM_POSTGRES_CONTAINER || 'teamgram-postgres';
const dcId = Number(process.env.TG_DC_ID || '2');
const dcHost = process.env.TG_DC_HOST || '127.0.0.1';
const dcPort = Number(process.env.TG_DC_PORT || '11443');
const apiId = Number(process.env.TG_API_ID || '1');
const apiHash = process.env.TG_API_HASH || '0123456789abcdef0123456789abcdef';
const phoneNumber = process.env.TEAMGRAM_AUTH_SIGNUP_PHONE
  || `+1202555${String(Date.now() % 10000).padStart(4, '0')}`;
const firstName = `Signup Probe ${Date.now()}`;
const lastName = 'Ephemeral';
const providerURL = process.env.TEAMGRAM_CODE_PROVIDER_URL || 'http://127.0.0.1:23193';
const providerKey = process.env.TEAMGRAM_CODE_PROVIDER_KEY || 'web-e2e-code-key';
const providerSecret = process.env.TEAMGRAM_CODE_PROVIDER_SECRET || 'web-e2e-code-secret';

if (!/^\+?\d{8,15}$/.test(phoneNumber)) {
  throw new Error('TEAMGRAM_AUTH_SIGNUP_PHONE must contain 8 to 15 digits');
}

const globalAny = globalThis as typeof globalThis & { self?: any; addEventListener?: () => void };
globalAny.self ??= globalThis;
globalAny.addEventListener ??= () => {};
globalAny.self.addEventListener ??= globalAny.addEventListener;

const requireModule = createRequire(import.meta.url);
const TelegramClient = requireModule(path.join(gramjsDir, 'src/lib/gramjs/client/TelegramClient.ts')).default;
const CallbackSession = requireModule(path.join(gramjsDir, 'src/lib/gramjs/sessions/CallbackSession.ts')).default;
const Api = requireModule(path.join(gramjsDir, 'src/lib/gramjs/tl/index.ts')).Api;
const ConnectionTCPObfuscated = requireModule(
  path.join(gramjsDir, 'src/lib/gramjs/network/connection/TCPObfuscated.ts'),
).ConnectionTCPObfuscated;
const quietLogger = { debug() {}, info() {}, warn() {}, error() {} };

// The GramJS runtime bundle used by this repository predates the generated
// account.deleteAccount request even though its declarations include it. Keep
// the probe self-contained by encoding the Layer 229 request directly.
function encodeTLString(value: string): Uint8Array {
  const bytes = Buffer.from(value, 'utf8');
  let prefix: Uint8Array;
  if (bytes.length < 254) {
    prefix = Uint8Array.of(bytes.length);
  } else {
    prefix = new Uint8Array([254, bytes.length & 0xff, (bytes.length >>> 8) & 0xff, (bytes.length >>> 16) & 0xff]);
  }
  const size = prefix.length + bytes.length;
  const result = new Uint8Array(size + ((4 - (size % 4)) % 4));
  result.set(prefix);
  result.set(bytes, prefix.length);
  return result;
}

function concatBytes(...parts: Uint8Array[]): Uint8Array {
  const result = new Uint8Array(parts.reduce((size, part) => size + part.length, 0));
  let offset = 0;
  for (const part of parts) {
    result.set(part, offset);
    offset += part.length;
  }
  return result;
}

class DeleteAccountRequest {
  static CONSTRUCTOR_ID = 0xa2c0cf74;

  readonly className = 'account.deleteAccount';
  readonly classType = 'request';
  readonly CONSTRUCTOR_ID = DeleteAccountRequest.CONSTRUCTOR_ID;

  constructor(readonly reason: string) {}

  getBytes(): Uint8Array {
    const constructorId = new Uint8Array(4);
    new DataView(constructorId.buffer).setUint32(0, DeleteAccountRequest.CONSTRUCTOR_ID, true);
    const flags = new Uint8Array(4);
    return concatBytes(constructorId, flags, encodeTLString(this.reason));
  }

  readResult(reader: any): any {
    return reader.tgReadObject();
  }
}

function className(value: any): string {
  return value?.className || value?.constructor?.name || typeof value;
}

function sql(value: string): string {
  return `'${value.replaceAll("'", "''")}'`;
}

function postgres(query: string): string {
  return execFileSync('docker', [
    'exec', postgresContainer, 'psql', '-Atq', '-F', '\t', '-v', 'ON_ERROR_STOP=1',
    '-U', 'teamgram', '-d', 'teamgram', '-c', query,
  ], { encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] }).trim();
}

function forceLayer229(client: any): void {
  const initWith = client._initWith.bind(client);
  client._initWith = (query: any) => {
    const original = initWith(query);
    if (className(original) !== 'InvokeWithLayer' || className(original.query) !== 'InitConnection') {
      throw new Error('INVALID_INIT_CONNECTION_WRAPPER');
    }
    return new Api.InvokeWithLayer({ layer: 229, query: original.query });
  };
}

function makeClient(): any {
  const session = new CallbackSession({ mainDcId: dcId, keys: {} }, () => {});
  const load = session.load.bind(session);
  session.load = async () => {
    await load();
    (session as any)._serverAddress = dcHost;
    (session as any)._port = dcPort;
  };
  const client = new TelegramClient(session, apiId, apiHash, {
    connection: ConnectionTCPObfuscated,
    fallbackConnection: ConnectionTCPObfuscated,
    shouldAllowHttpTransport: false,
    shouldForceHttpTransport: false,
    timeout: 20,
    requestRetries: 1,
    connectionRetries: 1,
    connectionRetriesToFallback: 0,
    retryDelay: 250,
    autoReconnect: false,
    baseLogger: quietLogger,
  });
  forceLayer229(client);
  (client as any)._borrowExportedSender = async () => undefined;
  (client as any)._updateLoop = async () => undefined;
  return client;
}

async function latestDelivery(challengeId: string): Promise<any> {
  const response = await fetch(`${providerURL}/latest?challenge_id=${encodeURIComponent(challengeId)}`, {
    headers: {
      Authorization: `Bearer ${providerKey}`,
      'X-Provider-Secret': providerSecret,
    },
  });
  if (!response.ok) throw new Error(`provider latest returned HTTP ${response.status}`);
  return response.json();
}

function isTrue(value: any): boolean {
  return value === true || value?.valueOf?.() === true || className(value).toLowerCase().includes('true');
}

async function disconnectWithBoundedWait(client: any): Promise<void> {
  await Promise.race([
    Promise.resolve(client.disconnect()).catch(() => undefined),
    new Promise<void>((resolve) => setTimeout(resolve, 2000)),
  ]);
}

function sanitizeError(value: unknown): string {
  const message = String((value as any)?.message || value || 'unknown error');
  return message.replaceAll(phoneNumber, '<phone>');
}

async function main(): Promise<void> {
  const client = makeClient();
  let challengeId = '';
  let userId = '';
  let accountDeleted = false;
  let signInCompleted = false;
  try {
    await client.connect();
    const sent = await client.invoke(new Api.auth.SendCode({
      phoneNumber,
      apiId,
      apiHash,
      settings: new Api.CodeSettings({}),
    }));
    challengeId = String(sent?.phoneCodeHash || '');
    if (!challengeId) throw new Error('auth.sendCode returned no phone_code_hash');

    const delivery = await latestDelivery(challengeId);
    if (String(delivery?.challenge_id) !== challengeId || String(delivery?.channel) !== 'sms') {
      throw new Error('code provider returned an unexpected challenge');
    }
    if (!String(delivery?.code || '').trim()) throw new Error('code provider returned an empty code');

    const signIn = await client.invoke(new Api.auth.SignIn({
      phoneNumber,
      phoneCodeHash: challengeId,
      phoneCode: String(delivery.code),
    }));
    signInCompleted = true;
    if (!className(signIn).endsWith('AuthorizationSignUpRequired')) {
      throw new Error(`auth.signIn returned ${className(signIn)}, expected AuthorizationSignUpRequired`);
    }

    const authorization = await client.invoke(new Api.auth.SignUp({
      phoneNumber,
      phoneCodeHash: challengeId,
      firstName,
      lastName,
      noJoinedNotifications: true,
    }));
    if (!className(authorization).endsWith('Authorization')) {
      throw new Error(`auth.signUp returned ${className(authorization)}, expected Authorization`);
    }
    userId = String(authorization?.user?.id || '');
    if (!/^\d+$/.test(userId) || userId === '0') throw new Error('auth.signUp returned no user ID');

    const created = postgres(`SELECT phone, first_name, deleted FROM users WHERE id=${userId}`);
    if (created !== `${phoneNumber.replace(/^\+/, '')}\t${firstName}\tf`) {
      throw new Error(`signup PostgreSQL row mismatch: ${created}`);
    }

    const deleted = await client.invoke(new DeleteAccountRequest('Layer 229 signup probe cleanup') as any);
    if (!isTrue(deleted)) throw new Error(`account.deleteAccount returned ${className(deleted)}`);
    accountDeleted = true;

    const activeUser = postgres(`SELECT count(*) FROM users WHERE id=${userId} AND deleted=FALSE`);
    const activeAuth = postgres(`SELECT count(*) FROM auth_users WHERE user_id=${userId} AND deleted=FALSE`);
    if (activeUser !== '0' || activeAuth !== '0') {
      throw new Error(`signup cleanup left active rows: users=${activeUser}, auth_users=${activeAuth}`);
    }

    console.log(JSON.stringify({
      ok: true,
      layer: 229,
      transport: `DC${dcId} TCPObfuscated -> gateway -> session -> Authorization/User/Authsession`,
      signIn: className(signIn),
      signUp: className(authorization),
      userId,
      accountDelete: className(deleted),
      postgres: { created: true, activeUserRowsAfterDelete: 0, activeAuthRowsAfterDelete: 0 },
    }));
  } finally {
    if (userId && !accountDeleted) {
      try {
        const deleted = await client.invoke(new DeleteAccountRequest('Layer 229 signup probe failure cleanup') as any);
        accountDeleted = isTrue(deleted);
      } catch {
        // Preserve the original failure. The caller can inspect userId in logs.
      }
    } else if (challengeId && !signInCompleted) {
      try {
        await client.invoke(new Api.auth.CancelCode({ phoneNumber, phoneCodeHash: challengeId }));
      } catch {
        // The challenge may already have expired.
      }
    }
    await disconnectWithBoundedWait(client);
  }
}

main()
  .then(() => setImmediate(() => process.exit(0)))
  .catch((error) => {
    console.error(JSON.stringify({ ok: false, error: sanitizeError(error) }));
    setImmediate(() => process.exit(1));
  });
