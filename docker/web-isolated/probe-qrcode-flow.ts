import path from 'node:path';
import { createRequire } from 'node:module';

const root = process.env.TEAMGRAM_GRAMJS_DIR || path.resolve(process.cwd(), '../../telegram-tt-master');
const requireModule = createRequire(import.meta.url);
const gramClient = requireModule(path.join(root, 'src/lib/gramjs/client/TelegramClient.ts'));
const gramSession = requireModule(path.join(root, 'src/lib/gramjs/sessions/Memory.ts'));
const gramTl = requireModule(path.join(root, 'src/lib/gramjs/tl/index.ts'));
const gramConnection = requireModule(path.join(root, 'src/lib/gramjs/network/connection/TCPObfuscated.ts'));

const TelegramClient = gramClient.default;
const MemorySession = gramSession.default;
const Api = gramTl.Api;
const WebSocketConnection = gramConnection.ConnectionTCPObfuscated;
const API_ID = 1;
const API_HASH = '0123456789abcdef0123456789abcdef';
const PHONE = '12025550102';

const quietLogger = { debug() {}, info() {}, warn() {}, error() {} };

function newClient() {
  return new TelegramClient(new MemorySession(), API_ID, API_HASH, {
    connection: WebSocketConnection,
    fallbackConnection: WebSocketConnection,
    shouldAllowHttpTransport: false,
    shouldForceHttpTransport: false,
    timeout: 15,
    requestRetries: 2,
    connectionRetries: 2,
    connectionRetriesToFallback: 0,
    retryDelay: 250,
    autoReconnect: false,
    baseLogger: quietLogger,
  });
}

function className(value: any) {
  return value?.className || value?.constructor?.name || '';
}

async function main() {
  const globalAny = globalThis as typeof globalThis & { self?: any; addEventListener?: () => void };
  globalAny.self ??= globalThis;
  globalAny.addEventListener ??= () => {};
  globalAny.self.addEventListener ??= globalAny.addEventListener;

  const authorized = newClient();
  const pending = newClient();
  try {
    await authorized.start({
      phoneNumber: PHONE,
      phoneCode: async () => '12345',
      password: async () => '',
      onError: (error: unknown) => { throw error; },
    });
    await pending.connect();

    const exported = await pending.invoke(new Api.auth.ExportLoginToken({
      apiId: API_ID,
      apiHash: API_HASH,
      exceptIds: [],
    }));
    if (className(exported) !== 'auth.LoginToken' || !exported.token) {
      throw new Error(`unexpected export result: ${className(exported)}`);
    }

    const accepted = await authorized.invoke(new Api.auth.AcceptLoginToken({
      token: Buffer.from(exported.token),
    }));
    if (!accepted || !className(accepted).toLowerCase().includes('authorization')) {
      throw new Error(`unexpected accept result: ${className(accepted)}`);
    }

    const completed = await pending.invoke(new Api.auth.ExportLoginToken({
      apiId: API_ID,
      apiHash: API_HASH,
      exceptIds: [],
    }));
    if (className(completed) !== 'auth.LoginTokenSuccess' || !completed.authorization?.user) {
      throw new Error(`unexpected completion result: ${className(completed)}`);
    }

    console.log(`qr flow passed: ${className(exported)} -> ${className(accepted)} -> ${className(completed)}`);
  } finally {
    authorized.destroy();
    pending.destroy();
  }
}

void main()
  .then(() => process.exit(0))
  .catch((error) => {
    console.error(`qr flow failed: ${error instanceof Error ? error.stack || error.message : String(error)}`);
    process.exit(1);
  });
