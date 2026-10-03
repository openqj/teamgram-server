import path from 'node:path';
import { createRequire } from 'node:module';
import process from 'node:process';

const API_ID = 1;
const API_HASH = '0123456789abcdef0123456789abcdef';
const PHONE = '12025550101';

async function run() {
  if (process.env.TG_DC_HOST !== '127.0.0.1' || process.env.TG_DC_PORT !== '31443') {
    throw new Error('upload probe must target 127.0.0.1:31443');
  }
  const globalAny = globalThis as typeof globalThis & { self?: any; addEventListener?: () => void };
  globalAny.self ??= globalThis;
  globalAny.addEventListener ??= () => {};
  globalAny.self.addEventListener ??= globalAny.addEventListener;
  const navigatorAny = globalThis as typeof globalThis & { navigator?: { locks?: { request: (name: string, callback: () => Promise<unknown>) => Promise<unknown> } } };
  navigatorAny.navigator ??= {};
  navigatorAny.navigator.locks ??= { request: async (_name, callback) => callback() };
  const requireModule = createRequire(import.meta.url);
  const root = process.env.TEAMGRAM_GRAMJS_DIR!;
  const gramJs = requireModule(path.join(root, 'src/lib/gramjs/client/TelegramClient.ts'));
  const sessionModule = requireModule(path.join(root, 'src/lib/gramjs/sessions/Memory.ts'));
  const tlModule = requireModule(path.join(root, 'src/lib/gramjs/tl/index.ts'));
  const connectionModule = requireModule(path.join(root, 'src/lib/gramjs/network/connection/TCPObfuscated.ts'));
  const Api = tlModule.Api;
  const MemorySession = sessionModule.default;
  class AuthKeyMemorySession extends MemorySession {
    private authKeys = new Map<number, any>();

    getAuthKey(dcId = this.dcId) {
      return this.authKeys.get(dcId);
    }

    setAuthKey(authKey: any, dcId = this.dcId) {
      if (authKey) this.authKeys.set(dcId, authKey);
    }
  }
  // The Web client uses CallbackSession, which retains its current DC auth key.
  // Keep that property here so each upload worker can share the authenticated key.
  const session = new AuthKeyMemorySession();
  const client = new gramJs.default(session, API_ID, API_HASH, {
    connection: connectionModule.ConnectionTCPObfuscated,
    fallbackConnection: connectionModule.ConnectionTCPObfuscated,
    shouldAllowHttpTransport: false,
    shouldForceHttpTransport: false,
    timeout: 10,
    requestRetries: 1,
    connectionRetries: 1,
    connectionRetriesToFallback: 0,
    retryDelay: 250,
    autoReconnect: false,
    baseLogger: { debug() {}, info() {}, warn() {}, error() {} },
  });
  try {
    await client.start({
      phoneNumber: PHONE,
      phoneCode: async () => '12345',
      password: async () => '',
      firstAndLastNames: async () => ['Isolated', 'Upload'],
      qrCode: async () => {},
      onError: (error: Error) => { throw error; },
      webAuthTokenFailed: () => {},
      onPasskeyOption: () => {},
      initialMethod: 'phoneNumber',
    });
    const me = await client.getMe();
    console.log(`login PASS user=${String(me.id)} dc=${client.session.dcId} auth_key=${String(client.session.getAuthKey()?.keyId)}`);
    const direct = await client.invoke(new Api.upload.SaveFilePart({
      fileId: BigInt(Date.now()) * 1_000_000n + 1n,
      filePart: 0,
      bytes: new TextEncoder().encode('direct-connection-upload-probe'),
    }));
    console.log(`main connection upload.saveFilePart PASS ${direct?.className || String(direct)}`);
    const uploaded = await client.uploadFile({
      file: new File([new TextEncoder().encode('exported-sender-upload-probe')], 'upload-probe.txt', { type: 'text/plain' }),
      workers: 1,
    });
    console.log(`exported sender upload.saveFilePart PASS ${uploaded?.className}`);

    const targetDc = client.session.dcId === 1 ? 2 : 1;
    const crossDcSender = await client.getSender(targetDc, 99);
    try {
      const crossDcUpload = await crossDcSender.send(new Api.upload.SaveFilePart({
        fileId: BigInt(Date.now()) * 1_000_000n + 2n,
        filePart: 0,
        bytes: new TextEncoder().encode('cross-dc-authorization-upload-probe'),
      }));
      console.log(`cross-DC authorization upload.saveFilePart PASS ${crossDcUpload?.className || String(crossDcUpload)}`);
    } finally {
      client.releaseExportedSender(crossDcSender);
    }
  } finally {
    client.destroy();
  }
}

void run()
  .then(() => process.exit(0))
  .catch((error) => {
    console.error(`upload repro fatal: ${error instanceof Error && error.stack ? error.stack : String(error)}`);
    process.exit(1);
  });
