import path from 'node:path';
import { createRequire } from 'node:module';

for (const key of ['TEAMGRAM_GRAMJS_DIR', 'A_AUTH_B64', 'A_USER_ID']) {
  if (!process.env[key]) throw new Error(`${key} is required`);
}

const root = process.env.TEAMGRAM_GRAMJS_DIR!;
const dcId = Number(process.env.TG_DC_ID || '2');
const requireModule = createRequire(import.meta.url);
const TelegramClient = requireModule(path.join(root, 'src/lib/gramjs/client/TelegramClient.ts')).default;
const CallbackSession = requireModule(path.join(root, 'src/lib/gramjs/sessions/CallbackSession.ts')).default;
const Api = requireModule(path.join(root, 'src/lib/gramjs/tl/index.ts')).Api;
const Connection = requireModule(path.join(root, 'src/lib/gramjs/network/connection/TCPObfuscated.ts')).ConnectionTCPObfuscated;
const quietLogger = { debug() {}, info() {}, warn() {}, error() {} };
const globalAny = globalThis as typeof globalThis & { self?: any; addEventListener?: () => void };
globalAny.self ??= globalThis;
globalAny.addEventListener ??= () => {};
globalAny.self.addEventListener ??= globalAny.addEventListener;

function makeClient(auth: string): any {
  const session = new CallbackSession({
    mainDcId: dcId,
    keys: { [dcId]: Buffer.from(auth, 'base64').toString('hex') },
  }, () => {});
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
    requestRetries: 2,
    connectionRetries: 2,
    connectionRetriesToFallback: 0,
    retryDelay: 250,
    autoReconnect: false,
    baseLogger: quietLogger,
  });
}

const rpcName = (value: any) => value?.className || value?.constructor?.name || typeof value;

async function main() {
  const client = makeClient(process.env.A_AUTH_B64!);
  try {
    await client.connect();
    const me = await client.getMe();
    if (String(me?.id) !== String(process.env.A_USER_ID)) throw new Error('auth key user mismatch');
    const result = await client.invoke(new Api.messages.GetSuggestedDialogFilters());
    const filters = Array.isArray(result) ? result : (Array.isArray(result?.filters) ? result.filters : []);
    if (filters.length !== 3 || filters.some((item: any) => rpcName(item) !== 'DialogFilterSuggested')) {
      throw new Error(`unexpected suggested filters: ${rpcName(result)} count=${filters.length}`);
    }
    console.log(JSON.stringify({ type: rpcName(result), count: filters.length, titles: filters.map((item: any) => item?.filter?.title || '') }));
  } finally {
    client.destroy();
  }
}

main().then(() => process.exit(0)).catch((error) => {
  console.error(`suggested dialog filters probe failed: ${error instanceof Error ? error.stack || error.message : String(error)}`);
  process.exit(1);
});
