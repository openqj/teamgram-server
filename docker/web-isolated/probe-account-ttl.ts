import path from 'node:path';
import { createRequire } from 'node:module';
import process from 'node:process';

const API_ID = 1;
const API_HASH = '0123456789abcdef0123456789abcdef';
const FIXTURE = { phone: '12025550101', username: 'isolated_test_01' };
const ALLOWED_TTLS = [30, 90, 180, 182, 183, 365, 548, 730];
const quietLogger = { debug() {}, info() {}, warn() {}, error() {} };

function requireCondition(condition: unknown, message: string): asserts condition {
  if (!condition) throw new Error(message);
}

function isTrue(value: unknown) {
  const name = (value as { className?: string; constructor?: { name?: string } })?.className
    || (value as { constructor?: { name?: string } })?.constructor?.name
    || '';
  return value === true || name.toLowerCase().includes('true');
}

async function loadGramJs(root: string) {
  const requireModule = createRequire(import.meta.url);
  const clientModule = requireModule(path.join(root, 'src/lib/gramjs/client/TelegramClient.ts'));
  const sessionModule = requireModule(path.join(root, 'src/lib/gramjs/sessions/Memory.ts'));
  const tlModule = requireModule(path.join(root, 'src/lib/gramjs/tl/index.ts'));
  const connectionModule = requireModule(path.join(root, 'src/lib/gramjs/network/connection/TCPObfuscated.ts'));
  return {
    TelegramClient: clientModule.default,
    MemorySession: sessionModule.default,
    Api: tlModule.Api,
    Connection: connectionModule.ConnectionTCPObfuscated,
  };
}

async function login(gram: any) {
  const client = new gram.TelegramClient(new gram.MemorySession(), API_ID, API_HASH, {
    connection: gram.Connection,
    fallbackConnection: gram.Connection,
    shouldAllowHttpTransport: false,
    shouldForceHttpTransport: false,
    timeout: 15,
    requestRetries: 2,
    connectionRetries: 1,
    connectionRetriesToFallback: 0,
    retryDelay: 250,
    autoReconnect: false,
    baseLogger: quietLogger,
  });
  await client.start({
    phoneNumber: FIXTURE.phone,
    phoneCode: async () => '12345',
    password: async () => '',
    firstAndLastNames: async () => ['Isolated', 'Profile'],
    qrCode: async () => {},
    onError: (error: Error) => { throw error; },
    webAuthTokenFailed: () => {},
    onPasskeyOption: () => {},
    initialMethod: 'phoneNumber',
  });
  requireCondition((await client.getMe())?.id !== undefined, `login failed for ${FIXTURE.username}`);
  return client;
}

async function run() {
  requireCondition(process.env.TG_DC_HOST === '127.0.0.1', 'probe must target the isolated gateway');
  requireCondition(process.env.TG_DC_PORT === '31443', 'probe must target the isolated gateway');
  requireCondition(process.env.TEAMGRAM_GRAMJS_DIR, 'TEAMGRAM_GRAMJS_DIR is required');

  const globalAny = globalThis as typeof globalThis & { self?: any; addEventListener?: () => void };
  globalAny.self ??= globalThis;
  globalAny.addEventListener ??= () => {};
  globalAny.self.addEventListener ??= globalAny.addEventListener;

  const gram = await loadGramJs(process.env.TEAMGRAM_GRAMJS_DIR);
  const { Api } = gram;
  const client = await login(gram);
  let original: number | undefined;
  try {
    const before = await client.invoke(new Api.account.GetAccountTTL());
    original = Number(before?.days);
    requireCondition(ALLOWED_TTLS.includes(original), `fixture returned unsupported TTL ${original}`);
    const candidate = original === 30 ? 90 : 30;
    const updated = await client.invoke(new Api.account.SetAccountTTL({
      ttl: new Api.AccountDaysTTL({ days: candidate }),
    }));
    requireCondition(isTrue(updated), 'account.setAccountTTL did not return true');
    const after = await client.invoke(new Api.account.GetAccountTTL());
    requireCondition(Number(after?.days) === candidate, `account.getAccountTTL returned ${after?.days}, want ${candidate}`);
    console.log(`PASS account TTL write/readback ${original} -> ${candidate}`);
  } finally {
    try {
      if (original !== undefined) {
        await client.invoke(new Api.account.SetAccountTTL({
          ttl: new Api.AccountDaysTTL({ days: original }),
        }));
      }
    } finally {
      client.destroy();
    }
  }
}

void run()
  .then(() => process.exit(0))
  .catch((error) => {
    console.error(error instanceof Error && error.stack ? error.stack : String(error));
    process.exit(1);
  });
