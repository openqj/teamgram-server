import path from 'node:path';
import { createRequire } from 'node:module';
import process from 'node:process';

const API_ID = 1;
const API_HASH = '0123456789abcdef0123456789abcdef';
const ALICE = { phone: '12025550109', username: 'isolated_test_09' };
const BOB = { phone: '12025550110', username: 'isolated_test_10' };
const permitUndeployedURLResponse = process.env.PERMIT_UNDEPLOYED_URL_AUTH_RESPONSE === '1';
const quietLogger = { debug() {}, info() {}, warn() {}, error() {} };

function requireCondition(condition: unknown, message: string): asserts condition {
  if (!condition) throw new Error(message);
}

function rpcName(value: unknown): string {
  return (value as { className?: string; constructor?: { name?: string } })?.className
    || (value as { constructor?: { name?: string } })?.constructor?.name
    || typeof value;
}

function isTrue(value: unknown): boolean {
  return value === true || rpcName(value).toLowerCase().includes('true');
}

function int64(value: unknown): bigint {
  return BigInt(String(value));
}

function webAuthorizationHash(url: string): bigint {
  let hash = 0xcbf29ce484222325n;
  for (const byte of Buffer.from(url)) {
    hash = BigInt.asUintN(64, (hash ^ BigInt(byte)) * 0x100000001b3n);
  }
  const positive = hash & 0x7fffffffffffffffn;
  return positive === 0n ? 1n : positive;
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

async function login(gram: any, fixture: typeof ALICE) {
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
  try {
    await client.start({
      phoneNumber: fixture.phone,
      phoneCode: async () => '12345',
      password: async () => '',
      firstAndLastNames: async () => ['Isolated', 'Session Settings'],
      qrCode: async () => {},
      onError: (error: Error) => { throw error; },
      webAuthTokenFailed: () => {},
      onPasskeyOption: () => {},
      initialMethod: 'phoneNumber',
    });
    const me = await client.getMe();
    requireCondition(me?.id !== undefined, `login failed for ${fixture.username}`);
    return { client, id: me.id };
  } catch (error) {
    client.destroy();
    throw error;
  }
}

function authorizationHashes(result: any): bigint[] {
  return (result?.authorizations || [])
    .filter((authorization: any) => !authorization?.current && authorization?.hash !== undefined)
    .map((authorization: any) => int64(authorization.hash));
}

function hasWebAuthorization(result: any, hash: bigint): boolean {
  return (result?.authorizations || []).some((authorization: any) => int64(authorization?.hash) === hash);
}

async function expectPasswordHashInvalid(action: () => Promise<unknown>) {
  try {
    await action();
  } catch (error) {
    requireCondition(String(error).includes('PASSWORD_HASH_INVALID'), `unexpected password error: ${String(error)}`);
    return;
  }
  throw new Error('invalid password proof unexpectedly succeeded');
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
  const alice = await login(gram, ALICE);
  let bob: { client: any; id: any } | undefined;
  let aliceSecond: { client: any; id: any } | undefined;
  let secondAuthorizationHash: bigint | undefined;
  let webStateOwned = false;
  let passwordEmailOwned = false;

  try {
    const authorizationsBefore = await alice.client.invoke(new Api.account.GetAuthorizations());
    requireCondition(rpcName(authorizationsBefore).includes('Authorizations'), `getAuthorizations returned ${rpcName(authorizationsBefore)}`);
    const existingHashes = new Set(authorizationHashes(authorizationsBefore).map(String));

    aliceSecond = await login(gram, ALICE);
    const authorizationsAfterLogin = await alice.client.invoke(new Api.account.GetAuthorizations());
    const current = (authorizationsAfterLogin?.authorizations || []).filter((authorization: any) => authorization?.current);
    requireCondition(current.length === 1, `getAuthorizations current count = ${current.length}, want 1`);
    secondAuthorizationHash = authorizationHashes(authorizationsAfterLogin)
      .find((hash) => !existingHashes.has(String(hash)));
    requireCondition(secondAuthorizationHash !== undefined, 'second session was absent from account.getAuthorizations');

    const resetAuthorization = await alice.client.invoke(new Api.account.ResetAuthorization({
      hash: secondAuthorizationHash,
    }));
    requireCondition(isTrue(resetAuthorization), 'account.resetAuthorization did not return true');
    const authorizationsAfterReset = await alice.client.invoke(new Api.account.GetAuthorizations());
    requireCondition(
      !authorizationHashes(authorizationsAfterReset).some((hash) => hash === secondAuthorizationHash),
      'account.resetAuthorization left the secondary session listed',
    );
    console.log(`PASS account authorization lifecycle user=${String(alice.id)} remote=${String(secondAuthorizationHash)}`);
    secondAuthorizationHash = undefined;

    const aliceWebBefore = await alice.client.invoke(new Api.account.GetWebAuthorizations());
    requireCondition((aliceWebBefore?.authorizations || []).length === 0, 'Alice web authorizations fixture is not empty');
    webStateOwned = true;
    bob = await login(gram, BOB);
    const bobWebBefore = await bob.client.invoke(new Api.account.GetWebAuthorizations());
    requireCondition((bobWebBefore?.authorizations || []).length === 0, 'Bob web authorizations fixture is not empty');

    const tag = `account-session-settings-${Date.now()}-${process.pid}`;
    const firstURL = `https://example.invalid/${tag}/one`;
    const firstHash = webAuthorizationHash(firstURL);
    const requested = await alice.client.invoke(new Api.messages.RequestUrlAuth({ url: firstURL }));
    requireCondition(rpcName(requested).includes('UrlAuthResultDefault'), `requestUrlAuth returned ${rpcName(requested)}`);
    const accepted = await alice.client.invoke(new Api.messages.AcceptUrlAuth({
      url: firstURL,
      matchCode: `${tag}-one`,
    }));
    requireCondition(rpcName(accepted).includes('UrlAuthResultAccepted'), `acceptUrlAuth returned ${rpcName(accepted)}`);
    const acceptedURL = accepted?.url ?? accepted?.url_STRING ?? accepted?.url_FLAGSTRING?.value;
    if (acceptedURL !== firstURL) {
      const message = `acceptUrlAuth URL = ${String(acceptedURL)}, keys=${Object.keys(accepted || {}).join(',')}`;
      if (!permitUndeployedURLResponse) throw new Error(message);
      console.warn(`KNOWN_UNDEPLOYED_RESPONSE_GAP ${message}`);
    }

    const aliceWebAfterFirst = await alice.client.invoke(new Api.account.GetWebAuthorizations());
    requireCondition(hasWebAuthorization(aliceWebAfterFirst, firstHash), 'getWebAuthorizations omitted accepted URL');
    const firstEntry = (aliceWebAfterFirst.authorizations || []).find((authorization: any) => int64(authorization.hash) === firstHash);
    requireCondition(firstEntry?.domain === 'example.invalid', `web authorization domain = ${firstEntry?.domain}`);
    requireCondition(Number(firstEntry?.dateCreated) > 0 && Number(firstEntry?.dateActive) > 0, 'web authorization timestamps are empty');

    const bobWebAfterAliceWrite = await bob.client.invoke(new Api.account.GetWebAuthorizations());
    requireCondition(!hasWebAuthorization(bobWebAfterAliceWrite, firstHash), 'Alice web authorization leaked to Bob');
    const bobReset = await bob.client.invoke(new Api.account.ResetWebAuthorization({ hash: firstHash }));
    requireCondition(isTrue(bobReset), 'Bob resetWebAuthorization did not return true');
    const aliceWebAfterBobReset = await alice.client.invoke(new Api.account.GetWebAuthorizations());
    requireCondition(hasWebAuthorization(aliceWebAfterBobReset, firstHash), 'Bob resetWebAuthorization removed Alice state');

    const resetFirst = await alice.client.invoke(new Api.account.ResetWebAuthorization({ hash: firstHash }));
    requireCondition(isTrue(resetFirst), 'account.resetWebAuthorization did not return true');
    const aliceWebAfterFirstReset = await alice.client.invoke(new Api.account.GetWebAuthorizations());
    requireCondition(!hasWebAuthorization(aliceWebAfterFirstReset, firstHash), 'single reset left accepted URL listed');

    const additionalURLs = [`https://example.invalid/${tag}/two`, `https://example.invalid/${tag}/three`];
    const additionalHashes = additionalURLs.map(webAuthorizationHash);
    for (const [index, url] of additionalURLs.entries()) {
      const request = await alice.client.invoke(new Api.messages.RequestUrlAuth({ url }));
      requireCondition(rpcName(request).includes('UrlAuthResultDefault'), `requestUrlAuth ${index} returned ${rpcName(request)}`);
      const accept = await alice.client.invoke(new Api.messages.AcceptUrlAuth({ url, matchCode: `${tag}-${index + 2}` }));
      requireCondition(rpcName(accept).includes('UrlAuthResultAccepted'), `acceptUrlAuth ${index} returned ${rpcName(accept)}`);
    }
    const aliceWebBeforeAllReset = await alice.client.invoke(new Api.account.GetWebAuthorizations());
    requireCondition(additionalHashes.every((hash) => hasWebAuthorization(aliceWebBeforeAllReset, hash)), 'web authorization batch write was not read back');
    const resetAll = await alice.client.invoke(new Api.account.ResetWebAuthorizations());
    requireCondition(isTrue(resetAll), 'account.resetWebAuthorizations did not return true');
    const aliceWebAfterAllReset = await alice.client.invoke(new Api.account.GetWebAuthorizations());
    requireCondition((aliceWebAfterAllReset?.authorizations || []).length === 0, 'resetWebAuthorizations left web authorizations listed');
    console.log(`PASS account web authorization lifecycle user=${String(alice.id)} isolated-from=${String(bob.id)}`);

    const password = await alice.client.invoke(new Api.account.GetPassword());
    requireCondition(!password?.hasPassword, 'Alice password fixture is not empty');
    const emptyPassword = new Api.InputCheckPasswordEmpty();
    const passwordSettingsBefore = await alice.client.invoke(new Api.account.GetPasswordSettings({ password: emptyPassword }));
    requireCondition(!passwordSettingsBefore?.email, 'Alice password settings fixture has an email');
    const testEmail = `${tag}@example.invalid`;
    const savedPasswordSettings = await alice.client.invoke(new Api.account.UpdatePasswordSettings({
      password: emptyPassword,
      newSettings: new Api.account.PasswordInputSettings({ email: testEmail }),
    }));
    requireCondition(isTrue(savedPasswordSettings), 'account.updatePasswordSettings did not return true');
    passwordEmailOwned = true;
    const passwordSettingsAfterSave = await alice.client.invoke(new Api.account.GetPasswordSettings({
      password: new Api.InputCheckPasswordEmpty(),
    }));
    requireCondition(passwordSettingsAfterSave?.email === testEmail, 'password settings email was not read back');

    const invalidPassword = new Api.InputCheckPasswordSRP({
      srpId: 1n,
      A: new Uint8Array(256),
      M1: new Uint8Array(32),
    });
    await expectPasswordHashInvalid(() => alice.client.invoke(new Api.account.GetPasswordSettings({ password: invalidPassword })));
    await expectPasswordHashInvalid(() => alice.client.invoke(new Api.account.UpdatePasswordSettings({
      password: invalidPassword,
      newSettings: new Api.account.PasswordInputSettings({ email: `rejected-${testEmail}` }),
    })));
    const passwordSettingsAfterFailure = await alice.client.invoke(new Api.account.GetPasswordSettings({
      password: new Api.InputCheckPasswordEmpty(),
    }));
    requireCondition(passwordSettingsAfterFailure?.email === testEmail, 'invalid password proof changed settings');

    const clearPasswordSettings = await alice.client.invoke(new Api.account.UpdatePasswordSettings({
      password: new Api.InputCheckPasswordEmpty(),
      newSettings: new Api.account.PasswordInputSettings({ email: '' }),
    }));
    requireCondition(isTrue(clearPasswordSettings), 'password settings cleanup did not return true');
    passwordEmailOwned = false;
    const passwordSettingsAfterClear = await alice.client.invoke(new Api.account.GetPasswordSettings({
      password: new Api.InputCheckPasswordEmpty(),
    }));
    requireCondition(!passwordSettingsAfterClear?.email, 'password settings cleanup was not read back');
    console.log(`PASS account password settings write/read/error/cleanup user=${String(alice.id)}`);
  } finally {
    if (secondAuthorizationHash !== undefined) {
      try { await alice.client.invoke(new Api.account.ResetAuthorization({ hash: secondAuthorizationHash })); } catch {}
    }
    if (passwordEmailOwned) {
      try {
        await alice.client.invoke(new Api.account.UpdatePasswordSettings({
          password: new Api.InputCheckPasswordEmpty(),
          newSettings: new Api.account.PasswordInputSettings({ email: '' }),
        }));
      } catch {}
    }
    if (webStateOwned) {
      try { await alice.client.invoke(new Api.account.ResetWebAuthorizations()); } catch {}
    }
    if (bob) {
      try { await bob.client.invoke(new Api.account.ResetWebAuthorizations()); } catch {}
      bob.client.destroy();
    }
    if (aliceSecond) aliceSecond.client.destroy();
    alice.client.destroy();
  }
}

void run()
  .then(() => process.exit(0))
  .catch((error) => {
    console.error(error instanceof Error && error.stack ? error.stack : String(error));
    process.exit(1);
  });
