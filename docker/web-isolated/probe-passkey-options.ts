import path from 'node:path';
import { createRequire } from 'node:module';

const API_ID = 1;
const API_HASH = '0123456789abcdef0123456789abcdef';
const PHONE = '12025550109';
const quietLogger = { debug() {}, info() {}, warn() {}, error() {} };

function requireCondition(condition: unknown, message: string): asserts condition {
  if (!condition) throw new Error(message);
}

function rpcName(value: unknown): string {
  return (value as { className?: string; constructor?: { name?: string } })?.className
    || (value as { constructor?: { name?: string } })?.constructor?.name
    || typeof value;
}

async function main() {
  const root = process.env.TEAMGRAM_GRAMJS_DIR;
  requireCondition(root, 'TEAMGRAM_GRAMJS_DIR is required');
  requireCondition(process.env.TG_DC_HOST === '127.0.0.1', 'probe must target the isolated gateway');
  requireCondition(process.env.TG_DC_PORT === '31443', 'probe must target the isolated gateway');

  const globalAny = globalThis as typeof globalThis & { self?: any; addEventListener?: () => void };
  globalAny.self ??= globalThis;
  globalAny.addEventListener ??= () => {};
  globalAny.self.addEventListener ??= globalAny.addEventListener;

  const requireModule = createRequire(import.meta.url);
  const gramClient = requireModule(path.join(root, 'src/lib/gramjs/client/TelegramClient.ts'));
  const gramSession = requireModule(path.join(root, 'src/lib/gramjs/sessions/Memory.ts'));
  const gramTl = requireModule(path.join(root, 'src/lib/gramjs/tl/index.ts'));
  const gramConnection = requireModule(path.join(root, 'src/lib/gramjs/network/connection/TCPObfuscated.ts'));
  const TelegramClient = gramClient.default;
  const MemorySession = gramSession.default;
  const Api = gramTl.Api;
  const Connection = gramConnection.ConnectionTCPObfuscated;
  const client = new TelegramClient(new MemorySession(), API_ID, API_HASH, {
    connection: Connection,
    fallbackConnection: Connection,
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

  try {
    await client.start({
      phoneNumber: PHONE,
      phoneCode: async () => '12345',
      password: async () => '',
      onError: (error: unknown) => { throw error; },
      onPasskeyOption: () => {},
      qrCode: async () => {},
    });

    const loginOptions = await client.invoke(new Api.auth.InitPasskeyLogin({
      apiId: API_ID,
      apiHash: API_HASH,
    }));
    requireCondition(rpcName(loginOptions).includes('PasskeyLoginOptions'), `auth.initPasskeyLogin returned ${rpcName(loginOptions)}`);
    const loginData = JSON.parse(loginOptions.options.data);
    requireCondition(loginData?.publicKey?.challenge, 'login options omitted publicKey.challenge');
    requireCondition(loginData?.publicKey?.rpId === 'passkey.teamgram.local', 'login options RP ID mismatch');

    const registrationOptions = await client.invoke(new Api.account.InitPasskeyRegistration());
    requireCondition(rpcName(registrationOptions).includes('PasskeyRegistrationOptions'), `account.initPasskeyRegistration returned ${rpcName(registrationOptions)}`);
    const registrationData = JSON.parse(registrationOptions.options.data);
    requireCondition(registrationData?.publicKey?.challenge, 'registration options omitted publicKey.challenge');
    requireCondition(registrationData?.publicKey?.rp?.id === 'passkey.teamgram.local', 'registration options RP ID mismatch');
    const me = await client.getMe();
    const userHandle = Buffer.from(registrationData.publicKey.user.id, 'base64url').toString();
    requireCondition(userHandle === `2:${String(me.id)}`, `registration user handle = ${userHandle}, want dc:user`);

    const passkeys = await client.invoke(new Api.account.GetPasskeys());
    requireCondition(rpcName(passkeys).includes('Passkeys') && Array.isArray(passkeys.passkeys), `account.getPasskeys returned ${rpcName(passkeys)}`);
    const deleted = await client.invoke(new Api.account.DeletePasskey({ id: Buffer.alloc(16, 7).toString('base64url') }));
    requireCondition(rpcName(deleted).toLowerCase().includes('bool') || deleted === false, `account.deletePasskey returned ${rpcName(deleted)}`);
    console.log(`PASS passkey ceremony options login=${loginData.publicKey.challenge.length} registration=${registrationData.publicKey.challenge.length} stored=${passkeys.passkeys.length}`);
  } finally {
    client.destroy();
  }
}

void main()
  .then(() => process.exit(0))
  .catch((error) => {
    console.error(`passkey options probe failed: ${error instanceof Error ? error.stack || error.message : String(error)}`);
    process.exit(1);
  });
