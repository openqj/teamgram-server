import { execFileSync } from 'node:child_process';
import path from 'node:path';
import { createRequire } from 'node:module';

// One-shot acceptance probe for the real MTProto verification-code path. The
// auth key is read into this process only; neither its body nor the code/hash
// is printed or persisted.
const gramjsDir = process.env.TEAMGRAM_GRAMJS_DIR || '/Users/mac/open/test/telegram-tt-master';
const dcId = Number(process.env.TG_DC_ID || '2');
const dcHost = process.env.TG_DC_HOST || '127.0.0.1';
const dcPort = Number(process.env.TG_DC_PORT || '11443');
const apiId = Number(process.env.TG_API_ID || '1');
const apiHash = process.env.TG_API_HASH || '0123456789abcdef0123456789abcdef';
const targetUserId = process.env.TEAMGRAM_AUTH_PROBE_USER_ID || '2';
const phoneNumber = process.env.TEAMGRAM_AUTH_PROBE_PHONE
  || `+1202555${String(Date.now() % 10000).padStart(4, '0')}`;
const providerURL = process.env.TEAMGRAM_CODE_PROVIDER_URL || 'http://127.0.0.1:23193';
const providerKey = process.env.TEAMGRAM_CODE_PROVIDER_KEY || 'web-e2e-code-key';
const providerSecret = process.env.TEAMGRAM_CODE_PROVIDER_SECRET || 'web-e2e-code-secret';

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

function className(value: any): string {
  return value?.className || value?.constructor?.name || typeof value;
}

function loadAuthKey(): { authKeyBase64: string; userId: string } {
  const query = [
    "SELECT encode(k.body, 'base64'), u.user_id",
    'FROM auth_users u',
    'JOIN auth_keys k USING (auth_key_id)',
    'JOIN auth_key_infos i USING (auth_key_id)',
    `WHERE u.user_id = ${Number.parseInt(targetUserId, 10) || 0}`,
    'AND u.deleted = false AND k.deleted = false AND i.deleted = false',
    'AND i.auth_key_type = 0',
    'ORDER BY u.date_active DESC, u.id DESC LIMIT 1',
  ].join(' ');
  const output = execFileSync('docker', [
    'exec', 'teamgram-postgres', 'psql', '-U', 'teamgram', '-d', 'teamgram',
    '-Atq', '-F', '\t', '-c', query,
  ], { encoding: 'utf8' }).trim();
  const [authKeyBase64, userId] = output.split('\t');
  if (!authKeyBase64 || !userId) throw new Error('no permanent auth key found for probe user');
  return { authKeyBase64, userId };
}

function makeClient(authKeyBase64: string): any {
  const authKeyHex = Buffer.from(authKeyBase64, 'base64').toString('hex');
  const session = new CallbackSession({ mainDcId: dcId, keys: { [dcId]: authKeyHex } }, () => {});
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
  // Keep this probe on the supplied primary key. GramJS otherwise attempts a
  // media sender that needs a separately exported auth key.
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

async function disconnectWithBoundedWait(client: any): Promise<void> {
  await Promise.race([
    Promise.resolve(client.disconnect()).catch(() => undefined),
    new Promise<void>((resolve) => setTimeout(resolve, 2000)),
  ]);
}

function sanitizeError(value: unknown): { className: string; message: string } {
  const error = value as any;
  let message = String(error?.message || value || 'unknown error');
  message = message.replaceAll(phoneNumber, '<phone>');
  return { className: className(error), message };
}

async function main() {
  const { authKeyBase64, userId } = loadAuthKey();
  const client = makeClient(authKeyBase64);
  let challengeId = '';
  let signInCompleted = false;
  try {
    await client.connect();
    const me = await client.getMe();
    if (String(me?.id) !== userId) throw new Error('loaded auth key resolved to a different user');

    const sent = await client.invoke(new Api.auth.SendCode({
      phoneNumber,
      apiId,
      apiHash,
      settings: new Api.CodeSettings({}),
    }));
    challengeId = String(sent?.phoneCodeHash || '');
    if (!challengeId) throw new Error('auth.sendCode returned no phone_code_hash');
    const type = className(sent?.type);
    if (!type.endsWith('SentCodeTypeSms')) throw new Error(`auth.sendCode returned ${type}, expected SentCodeTypeSms`);

    const delivery = await latestDelivery(challengeId);
    if (String(delivery?.challenge_id) !== challengeId) throw new Error('provider challenge_id mismatch');
    if (String(delivery?.channel) !== 'sms') throw new Error('provider delivery channel was not sms');
    if (!String(delivery?.code || '').trim()) throw new Error('provider returned an empty code');

    const authorization = await client.invoke(new Api.auth.SignIn({
      phoneNumber,
      phoneCodeHash: challengeId,
      phoneCode: String(delivery.code),
    }));
    signInCompleted = true;
    const authorizationType = className(authorization);
    if (!authorizationType.endsWith('AuthorizationSignUpRequired')) {
      throw new Error(`auth.signIn returned ${authorizationType}, expected AuthorizationSignUpRequired`);
    }

    console.log(JSON.stringify({
      ok: true,
      authUserId: userId,
      sendCodeType: type,
      providerChannel: delivery.channel,
      providerPurpose: delivery.purpose,
      providerCodeLength: String(delivery.code).length,
      signInType: authorizationType,
      challengeIdLength: challengeId.length,
    }));
  } catch (error) {
    console.log(JSON.stringify({ ok: false, ...sanitizeError(error), signInCompleted }));
    process.exitCode = 1;
  } finally {
    if (challengeId && !signInCompleted) {
      try {
        await client.invoke(new Api.auth.CancelCode({ phoneNumber, phoneCodeHash: challengeId }));
      } catch {
        // The challenge may already have been consumed by a failed RPC.
      }
    }
    await disconnectWithBoundedWait(client);
  }
}

main()
  .then(() => {
    setImmediate(() => process.exit(process.exitCode ?? 0));
  })
  .catch((error) => {
    console.log(JSON.stringify({ ok: false, ...sanitizeError(error), signInCompleted: false }));
    setImmediate(() => process.exit(1));
  });
