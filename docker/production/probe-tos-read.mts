import path from 'node:path';
import { createRequire } from 'node:module';

for (const key of ['TEAMGRAM_GRAMJS_DIR', 'AUTH_B64', 'EXPECTED_USER_ID']) {
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
const quietLogger = { debug() {}, info() {}, warn() {}, error() {} };

function rpcName(value: any): string {
  return value?.className || value?.constructor?.name || typeof value;
}

class GetTermsOfServiceUpdateRequest {
  CONSTRUCTOR_ID = 0x2ca51fd1;
  SUBCLASS_OF_ID = 0;
  className = 'help.GetTermsOfServiceUpdate';
  classType = 'request';

  getBytes() {
    const bytes = new Uint8Array(4);
    new DataView(bytes.buffer).setUint32(0, this.CONSTRUCTOR_ID, true);
    return bytes;
  }

  readResult(reader: any) {
    return reader.tgReadObject();
  }
}

async function main() {
  const session = new CallbackSession({
    mainDcId: dcId,
    keys: { [dcId]: Buffer.from(process.env.AUTH_B64!, 'base64').toString('hex') },
  }, () => {});
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
    baseLogger: quietLogger,
  });
  // A one-shot audit must not start GramJS's long-running update poller.
  (client as any)._updateLoop = async () => undefined;

  try {
    await client.connect();
    const result: any = await client.invoke(new GetTermsOfServiceUpdateRequest());
    const terms = result?.termsOfService;
    console.log(JSON.stringify({
      userId: process.env.EXPECTED_USER_ID,
      result: rpcName(result),
      expires: Number(result?.expires || 0),
      terms: terms ? {
        id: terms.id?.data || null,
        text: terms.text || '',
        popup: Boolean(terms.popup),
        entities: Array.isArray(terms.entities) ? terms.entities.length : null,
      } : null,
    }));
    // Avoid leaving the GramJS sender's socket/event handles in a one-shot probe.
    client.disconnect();
    process.exit(0);
  } finally {
    client.disconnect();
  }
}

main().catch((error) => {
  console.error(String(error));
  process.exitCode = 1;
});
