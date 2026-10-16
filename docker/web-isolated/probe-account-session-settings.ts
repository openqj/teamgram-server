import path from 'node:path';
import { createRequire } from 'node:module';
import process from 'node:process';
import { authSession, loadFixtureAuthKey } from './fixture-auth';
import { SessionProbe, MultiSessionProbeTracker } from './session-probe-framework';
import * as fs from 'node:fs';

const API_ID = 1;
const API_HASH = '0123456789abcdef0123456789abcdef';
const ALICE = { phone: '12025550109', username: 'isolated_test_09' };
const BOB = { phone: '12025550110', username: 'isolated_test_10' };
const permitUndeployedURLResponse = process.env.PERMIT_UNDEPLOYED_URL_AUTH_RESPONSE === '1';
const quietLogger = { debug() {}, info() {}, warn() {}, error() {} };

type Actor = {
  client: any;
  id: any;
  probe?: SessionProbe;
};

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
  const callbackSessionModule = requireModule(path.join(root, 'src/lib/gramjs/sessions/CallbackSession.ts'));
  const tlModule = requireModule(path.join(root, 'src/lib/gramjs/tl/index.ts'));
  const connectionModule = requireModule(path.join(root, 'src/lib/gramjs/network/connection/TCPObfuscated.ts'));
  return {
    TelegramClient: clientModule.default,
    MemorySession: sessionModule.default,
    CallbackSession: callbackSessionModule.default,
    Api: tlModule.Api,
    Connection: connectionModule.ConnectionTCPObfuscated,
  };
}

async function login(gram: any, fixture: typeof ALICE, authKeyOffset = 0): Promise<Actor> {
  const authKey = loadFixtureAuthKey(fixture.username, authKeyOffset);
  const session = authKey
    ? authSession(gram, authKey, 2, process.env.TG_DC_HOST || '127.0.0.1', Number(process.env.TG_DC_PORT || '31443'))
    : new gram.MemorySession();
  const client = new gram.TelegramClient(session, API_ID, API_HASH, {
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
    if (authKey) {
      await client.connect();
    } else {
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
    }
    const me = await client.getMe();
    requireCondition(me?.id !== undefined, `login failed for ${fixture.username}`);

    const probe = new SessionProbe(`session_${fixture.username}_${authKeyOffset}`, client, me.id, authKey || '');
    await probe.connect();
    await probe.captureState();

    return { client, id: me.id, probe };
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
  const fixtureAuthAvailable = Boolean(loadFixtureAuthKey(ALICE.username));
  const alice = await login(gram, ALICE);
  let bob: Actor | undefined;
  let aliceSecond: Actor | undefined;
  let secondAuthorizationHash: bigint | undefined;
  let webStateOwned = false;
  let passwordEmailOwned = false;

  const tracker = new MultiSessionProbeTracker();
  if (alice.probe) tracker.registerSession(`alice_${0}`, alice.probe);

  try {
    // Capture baseline state
    if (alice.probe) await alice.probe.captureState();

    const authorizationsBefore = await alice.client.invoke(new Api.account.GetAuthorizations());
    if (alice.probe) await alice.probe.captureState();
    requireCondition(rpcName(authorizationsBefore).includes('Authorizations'), `getAuthorizations returned ${rpcName(authorizationsBefore)}`);
    const existingHashes = new Set(authorizationHashes(authorizationsBefore).map(String));

    aliceSecond = await login(gram, ALICE, 1);
    if (aliceSecond.probe) tracker.registerSession(`alice_${1}`, aliceSecond.probe);

    const authorizationsAfterLogin = await alice.client.invoke(new Api.account.GetAuthorizations());
    if (alice.probe) await alice.probe.captureState();
    const current = (authorizationsAfterLogin?.authorizations || []).filter((authorization: any) => authorization?.current);
    requireCondition(current.length === 1, `getAuthorizations current count = ${current.length}, want 1`);
    secondAuthorizationHash = authorizationHashes(authorizationsAfterLogin)
      .find((hash) => !existingHashes.has(String(hash)));
    if (secondAuthorizationHash !== undefined) {
      const resetAuthorization = await alice.client.invoke(new Api.account.ResetAuthorization({
        hash: secondAuthorizationHash,
      }));
      if (alice.probe) await alice.probe.captureState();
      requireCondition(isTrue(resetAuthorization), 'account.resetAuthorization did not return true');
      const authorizationsAfterReset = await alice.client.invoke(new Api.account.GetAuthorizations());
      if (alice.probe) await alice.probe.captureState();
      requireCondition(
        !authorizationHashes(authorizationsAfterReset).some((hash) => hash === secondAuthorizationHash),
        'account.resetAuthorization left the secondary session listed',
      );
      console.log(`PASS account authorization lifecycle user=${String(alice.id)} remote=${String(secondAuthorizationHash)}`);
      secondAuthorizationHash = undefined;
    } else if (fixtureAuthAvailable) {
      console.log(`PASS account authorization fixture reconnect user=${String(alice.id)}`);
    } else {
      throw new Error('second session was absent from account.getAuthorizations');
    }

    const aliceWebBefore = await alice.client.invoke(new Api.account.GetWebAuthorizations());
    if (alice.probe) await alice.probe.captureState();
    requireCondition((aliceWebBefore?.authorizations || []).length === 0, 'Alice web authorizations fixture is not empty');
    webStateOwned = true;
    bob = await login(gram, BOB);
    if (bob.probe) tracker.registerSession(`bob_${0}`, bob.probe);
    const bobWebBefore = await bob.client.invoke(new Api.account.GetWebAuthorizations());
    if (bob.probe) await bob.probe.captureState();
    requireCondition((bobWebBefore?.authorizations || []).length === 0, 'Bob web authorizations fixture is not empty');

    const tag = `account-session-settings-${Date.now()}-${process.pid}`;
    const firstURL = `https://example.invalid/${tag}/one`;
    const firstHash = webAuthorizationHash(firstURL);
    const requested = await alice.client.invoke(new Api.messages.RequestUrlAuth({ url: firstURL }));
    if (alice.probe) await alice.probe.captureState();
    requireCondition(rpcName(requested).includes('UrlAuthResultDefault'), `requestUrlAuth returned ${rpcName(requested)}`);
    const accepted = await alice.client.invoke(new Api.messages.AcceptUrlAuth({
      url: firstURL,
      matchCode: `${tag}-one`,
    }));
    if (alice.probe) await alice.probe.captureState();
    requireCondition(rpcName(accepted).includes('UrlAuthResultAccepted'), `acceptUrlAuth returned ${rpcName(accepted)}`);
    const acceptedURL = accepted?.url ?? accepted?.url_STRING ?? accepted?.url_FLAGSTRING?.value;
    if (acceptedURL !== firstURL) {
      const message = `acceptUrlAuth URL = ${String(acceptedURL)}, keys=${Object.keys(accepted || {}).join(',')}`;
      if (!permitUndeployedURLResponse) throw new Error(message);
      console.warn(`KNOWN_UNDEPLOYED_RESPONSE_GAP ${message}`);
    }

    const aliceWebAfterFirst = await alice.client.invoke(new Api.account.GetWebAuthorizations());
    if (alice.probe) await alice.probe.captureState();
    requireCondition(hasWebAuthorization(aliceWebAfterFirst, firstHash), 'getWebAuthorizations omitted accepted URL');
    const firstEntry = (aliceWebAfterFirst.authorizations || []).find((authorization: any) => int64(authorization.hash) === firstHash);
    requireCondition(firstEntry?.domain === 'example.invalid', `web authorization domain = ${firstEntry?.domain}`);
    requireCondition(Number(firstEntry?.dateCreated) > 0 && Number(firstEntry?.dateActive) > 0, 'web authorization timestamps are empty');

    const bobWebAfterAliceWrite = await bob.client.invoke(new Api.account.GetWebAuthorizations());
    if (bob.probe) await bob.probe.captureState();
    requireCondition(!hasWebAuthorization(bobWebAfterAliceWrite, firstHash), 'Alice web authorization leaked to Bob');
    const bobReset = await bob.client.invoke(new Api.account.ResetWebAuthorization({ hash: firstHash }));
    if (bob.probe) await bob.probe.captureState();
    requireCondition(isTrue(bobReset), 'Bob resetWebAuthorization did not return true');
    const aliceWebAfterBobReset = await alice.client.invoke(new Api.account.GetWebAuthorizations());
    if (alice.probe) await alice.probe.captureState();
    requireCondition(hasWebAuthorization(aliceWebAfterBobReset, firstHash), 'Bob resetWebAuthorization removed Alice state');

    const resetFirst = await alice.client.invoke(new Api.account.ResetWebAuthorization({ hash: firstHash }));
    if (alice.probe) await alice.probe.captureState();
    requireCondition(isTrue(resetFirst), 'account.resetWebAuthorization did not return true');
    const aliceWebAfterFirstReset = await alice.client.invoke(new Api.account.GetWebAuthorizations());
    if (alice.probe) await alice.probe.captureState();
    requireCondition(!hasWebAuthorization(aliceWebAfterFirstReset, firstHash), 'single reset left accepted URL listed');

    const additionalURLs = [`https://example.invalid/${tag}/two`, `https://example.invalid/${tag}/three`];
    const additionalHashes = additionalURLs.map(webAuthorizationHash);
    for (const [index, url] of additionalURLs.entries()) {
      const request = await alice.client.invoke(new Api.messages.RequestUrlAuth({ url }));
      if (alice.probe) await alice.probe.captureState();
      requireCondition(rpcName(request).includes('UrlAuthResultDefault'), `requestUrlAuth ${index} returned ${rpcName(request)}`);
      const accept = await alice.client.invoke(new Api.messages.AcceptUrlAuth({ url, matchCode: `${tag}-${index + 2}` }));
      if (alice.probe) await alice.probe.captureState();
      requireCondition(rpcName(accept).includes('UrlAuthResultAccepted'), `acceptUrlAuth ${index} returned ${rpcName(accept)}`);
    }
    const aliceWebBeforeAllReset = await alice.client.invoke(new Api.account.GetWebAuthorizations());
    if (alice.probe) await alice.probe.captureState();
    requireCondition(additionalHashes.every((hash) => hasWebAuthorization(aliceWebBeforeAllReset, hash)), 'web authorization batch write was not read back');
    const resetAll = await alice.client.invoke(new Api.account.ResetWebAuthorizations());
    if (alice.probe) await alice.probe.captureState();
    requireCondition(isTrue(resetAll), 'account.resetWebAuthorizations did not return true');
    const aliceWebAfterAllReset = await alice.client.invoke(new Api.account.GetWebAuthorizations());
    if (alice.probe) await alice.probe.captureState();
    requireCondition((aliceWebAfterAllReset?.authorizations || []).length === 0, 'resetWebAuthorizations left web authorizations listed');
    console.log(`PASS account web authorization lifecycle user=${String(alice.id)} isolated-from=${String(bob.id)}`);

    const password = await alice.client.invoke(new Api.account.GetPassword());
    if (alice.probe) await alice.probe.captureState();
    requireCondition(!password?.hasPassword, 'Alice password fixture is not empty');
    const emptyPassword = new Api.InputCheckPasswordEmpty();
    const passwordSettingsBefore = await alice.client.invoke(new Api.account.GetPasswordSettings({ password: emptyPassword }));
    if (alice.probe) await alice.probe.captureState();
    requireCondition(!passwordSettingsBefore?.email, 'Alice password settings fixture has an email');
    const testEmail = `${tag}@example.invalid`;
    const savedPasswordSettings = await alice.client.invoke(new Api.account.UpdatePasswordSettings({
      password: emptyPassword,
      newSettings: new Api.account.PasswordInputSettings({ email: testEmail }),
    }));
    if (alice.probe) await alice.probe.captureState();
    requireCondition(isTrue(savedPasswordSettings), 'account.updatePasswordSettings did not return true');
    passwordEmailOwned = true;
    const passwordSettingsAfterSave = await alice.client.invoke(new Api.account.GetPasswordSettings({
      password: new Api.InputCheckPasswordEmpty(),
    }));
    if (alice.probe) await alice.probe.captureState();
    requireCondition(passwordSettingsAfterSave?.email === testEmail, 'password settings email was not read back');

    const invalidPassword = new Api.InputCheckPasswordSRP({
      srpId: 1n,
      A: new Uint8Array(256),
      M1: new Uint8Array(32),
    });
    await expectPasswordHashInvalid(() => alice.client.invoke(new Api.account.GetPasswordSettings({ password: invalidPassword })));
    if (alice.probe) await alice.probe.captureState();
    await expectPasswordHashInvalid(() => alice.client.invoke(new Api.account.UpdatePasswordSettings({
      password: invalidPassword,
      newSettings: new Api.account.PasswordInputSettings({ email: `rejected-${testEmail}` }),
    })));
    if (alice.probe) await alice.probe.captureState();
    const passwordSettingsAfterFailure = await alice.client.invoke(new Api.account.GetPasswordSettings({
      password: new Api.InputCheckPasswordEmpty(),
    }));
    if (alice.probe) await alice.probe.captureState();
    requireCondition(passwordSettingsAfterFailure?.email === testEmail, 'invalid password proof changed settings');

    const clearPasswordSettings = await alice.client.invoke(new Api.account.UpdatePasswordSettings({
      password: new Api.InputCheckPasswordEmpty(),
      newSettings: new Api.account.PasswordInputSettings({ email: '' }),
    }));
    if (alice.probe) await alice.probe.captureState();
    requireCondition(isTrue(clearPasswordSettings), 'password settings cleanup did not return true');
    passwordEmailOwned = false;
    const passwordSettingsAfterClear = await alice.client.invoke(new Api.account.GetPasswordSettings({
      password: new Api.InputCheckPasswordEmpty(),
    }));
    if (alice.probe) await alice.probe.captureState();
    requireCondition(!passwordSettingsAfterClear?.email, 'password settings cleanup was not read back');
    console.log(`PASS account password settings write/read/error/cleanup user=${String(alice.id)}`);

    // Export session tracking report
    const report = {
      testName: 'probe-account-session-settings',
      timestamp: Date.now(),
      sessions: {
        alice: alice.probe?.getState(),
        bob: bob?.probe?.getState(),
        aliceSecond: aliceSecond?.probe?.getState(),
      },
      violations: {
        alice: alice.probe?.getViolations() || [],
        bob: bob?.probe?.getViolations() || [],
        aliceSecond: aliceSecond?.probe?.getViolations() || [],
      },
      crossSessionConsistency: tracker.getCrossSessionViolations(),
    };
    const reportFile = `/tmp/teamgram-probe-account-session-settings-${Date.now()}.json`;
    fs.writeFileSync(reportFile, JSON.stringify(report, null, 2));
    console.log(`\n✓ Session tracking report saved to ${reportFile}`);
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
