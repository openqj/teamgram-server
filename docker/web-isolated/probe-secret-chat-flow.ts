import path from 'node:path';
import { createRequire } from 'node:module';

const root = process.env.TEAMGRAM_GRAMJS_DIR || path.resolve(process.cwd(), '../../telegram-tt-master');
const requireModule = createRequire(import.meta.url);
const clientModule = requireModule(path.join(root, 'src/lib/gramjs/client/TelegramClient.ts'));
const sessionModule = requireModule(path.join(root, 'src/lib/gramjs/sessions/Memory.ts'));
const tlModule = requireModule(path.join(root, 'src/lib/gramjs/tl/index.ts'));
const connectionModule = requireModule(path.join(root, 'src/lib/gramjs/network/connection/TCPObfuscated.ts'));

const TelegramClient = clientModule.default;
const MemorySession = sessionModule.default;
const Api = tlModule.Api;
const Connection = connectionModule.ConnectionTCPObfuscated;
const API_ID = 1;
const API_HASH = '0123456789abcdef0123456789abcdef';
const quietLogger = { debug() {}, info() {}, warn() {}, error() {} };

function requireCondition(condition: unknown, message: string): asserts condition {
  if (!condition) throw new Error(message);
}

function rpcName(value: any): string {
  return value?.className || value?.constructor?.name || '';
}

function newClient() {
  return new TelegramClient(new MemorySession(), API_ID, API_HASH, {
    connection: Connection,
    fallbackConnection: Connection,
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
}

async function login(phone: string) {
  const client = newClient();
  await client.start({
    phoneNumber: phone,
    phoneCode: async () => '12345',
    password: async () => '',
    firstAndLastNames: async () => ['Isolated', 'Secret'],
    qrCode: async () => {},
    onError: (error: Error) => { throw error; },
    webAuthTokenFailed: () => {},
    onPasskeyOption: () => {},
    initialMethod: 'phoneNumber',
  });
  requireCondition((await client.getMe())?.id !== undefined, `login failed for ${phone}`);
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

  const alice = await login('12025550101');
  const bob = await login('12025550102');
  try {
    // The reduced Web GramJS schema currently exposes the encrypted result
    // types but not the messages.* request constructors. Keep this probe
    // explicit about that client-side limitation instead of reporting a
    // server failure for a request the client cannot encode.
    if (typeof Api.messages.RequestEncryption !== 'function'
      || typeof Api.messages.AcceptEncryption !== 'function'
      || typeof Api.messages.SendEncrypted !== 'function') {
      console.log('secret chat flow skipped: Web GramJS request constructors are unavailable');
      return;
    }
    const bobUser = await alice.invoke(new Api.contacts.Search({ q: 'isolated_test_02', limit: 10 }));
    const user = bobUser?.users?.find((candidate: any) => candidate?.username === 'isolated_test_02');
    requireCondition(user?.id !== undefined && user?.accessHash !== undefined, 'contacts.search did not return Bob');

    const randomId = Math.floor(Date.now() / 1000);
    const requested = await alice.invoke(new Api.messages.RequestEncryption({
      userId: new Api.InputUser({ userId: user.id, accessHash: user.accessHash }),
      randomId,
      // The server validates the public value range; this probe checks the
      // durable lifecycle without implementing a full DH exchange.
      gA: Buffer.from([2]),
    }));
    requireCondition(rpcName(requested) === 'EncryptedChatWaiting', `request returned ${rpcName(requested)}`);
    requireCondition(requested.id !== undefined && requested.accessHash !== undefined, 'request lacks encrypted chat identity');

    const peer = new Api.InputEncryptedChat({ chatId: requested.id, accessHash: requested.accessHash });
    const accepted = await bob.invoke(new Api.messages.AcceptEncryption({
      peer,
      gB: Buffer.from([3]),
      keyFingerprint: BigInt(randomId),
    }));
    requireCondition(rpcName(accepted) === 'EncryptedChat', `accept returned ${rpcName(accepted)}`);

    const sent = await alice.invoke(new Api.messages.SendEncrypted({
      peer,
      randomId: BigInt(Date.now()) * 1000n,
      data: Buffer.from('isolated-secret-payload'),
    }));
    requireCondition(rpcName(sent).toLowerCase().includes('sentencrypted'), `send returned ${rpcName(sent)}`);

    const discarded = await alice.invoke(new Api.messages.DiscardEncryption({ chatId: requested.id, deleteHistory: true }));
    requireCondition(discarded === true || rpcName(discarded).toLowerCase().includes('true'), `discard returned ${rpcName(discarded)}`);
    console.log(`secret chat flow passed: ${rpcName(requested)} -> ${rpcName(accepted)} -> ${rpcName(sent)} -> ${rpcName(discarded)}`);
  } finally {
    alice.destroy();
    bob.destroy();
  }
}

void run().then(() => process.exit(0)).catch((error) => {
  console.error(`secret chat flow failed: ${error instanceof Error ? error.stack || error.message : String(error)}`);
  process.exit(1);
});
