import path from 'node:path';
import { createRequire } from 'node:module';

for (const key of ['TEAMGRAM_GRAMJS_DIR', 'AUTH_B64']) {
  if (!process.env[key]) throw new Error(`${key} is required`);
}

const gramjsDir = process.env.TEAMGRAM_GRAMJS_DIR!;
const dcId = Number(process.env.TG_DC_ID || '2');
const globalAny = globalThis as typeof globalThis & { self?: any; addEventListener?: () => void };
globalAny.self ??= globalThis;
globalAny.addEventListener ??= () => {};
globalAny.self.addEventListener ??= globalAny.addEventListener;

const requireModule = createRequire(import.meta.url);
const TelegramClient = requireModule(path.join(gramjsDir, 'src/lib/gramjs/client/TelegramClient.ts')).default;
const CallbackSession = requireModule(path.join(gramjsDir, 'src/lib/gramjs/sessions/CallbackSession.ts')).default;
const Api = requireModule(path.join(gramjsDir, 'src/lib/gramjs/tl/index.ts')).Api;
const Connection = requireModule(path.join(gramjsDir, 'src/lib/gramjs/network/connection/TCPObfuscated.ts')).ConnectionTCPObfuscated;

async function main() {
  const authKeyHex = Buffer.from(process.env.AUTH_B64!, 'base64').toString('hex');
  const session = new CallbackSession({ mainDcId: dcId, keys: { [dcId]: authKeyHex } }, () => {});
  const load = session.load.bind(session);
  session.load = async () => {
    await load();
    (session as any)._serverAddress = process.env.TG_DC_HOST || '127.0.0.1';
    (session as any)._port = Number(process.env.TG_DC_PORT || '11443');
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
    baseLogger: { debug() {}, info() {}, warn() {}, error() {} },
  });
  (client as any)._borrowExportedSender = async () => undefined;
  (client as any)._updateLoop = async () => undefined;

  try {
    await client.connect();
    await client.getMe();
    const result = await client.invoke(new Api.account.GetContentSettings());
    const name = result?.className || result?.constructor?.name || typeof result;
    if (!String(name).toLowerCase().includes('contentsettings')) {
      throw new Error(`unexpected account.getContentSettings result: ${name}`);
    }
    console.log(JSON.stringify({
      type: name,
      sensitiveEnabled: result.sensitiveEnabled === true,
      sensitiveCanChange: result.sensitiveCanChange === true,
    }));
  } finally {
    client.destroy();
  }
}

main().catch((error) => {
  console.error(error);
  process.exit(1);
}).then(() => {
  process.exit(0);
});
