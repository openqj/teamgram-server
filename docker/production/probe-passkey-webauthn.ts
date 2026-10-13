import { createRequire } from 'node:module';
import { execFileSync } from 'node:child_process';
import { createServer } from 'node:https';
import { mkdtempSync, readFileSync, rmSync, unlinkSync } from 'node:fs';
import { tmpdir } from 'node:os';
import path from 'node:path';

for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) {
  delete process.env[key];
}

const gramjsDir = process.env.TEAMGRAM_GRAMJS_DIR || '/Users/mac/open/test/telegram-tt-master';
const dcId = Number(process.env.TG_DC_ID || '2');
const dcHost = process.env.TG_DC_HOST || '127.0.0.1';
const dcPort = Number(process.env.TG_DC_PORT || '11443');
const apiId = Number(process.env.TG_API_ID || '1');
const apiHash = process.env.TG_API_HASH || '0123456789abcdef0123456789abcdef';
const userId = Number(process.env.PASSKEY_USER_ID || '9');
const postgresContainer = process.env.TEAMGRAM_POSTGRES_CONTAINER || 'teamgram-postgres';
const chromePath = process.env.CHROME_PATH || '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome';
const rpHost = process.env.PASSKEY_RP_HOST || 'passkey.teamgram.local';
const rpPort = Number(process.env.PASSKEY_RP_PORT || '8443');

if (!Number.isSafeInteger(userId) || userId <= 0) throw new Error('PASSKEY_USER_ID must be a positive integer');

const globalAny = globalThis as typeof globalThis & { self?: any; addEventListener?: () => void };
globalAny.self ??= globalThis;
globalAny.addEventListener ??= () => {};
globalAny.self.addEventListener ??= globalAny.addEventListener;

const requireModule = createRequire(import.meta.url);
const TelegramClient = requireModule(path.join(gramjsDir, 'src/lib/gramjs/client/TelegramClient.ts')).default;
const CallbackSession = requireModule(path.join(gramjsDir, 'src/lib/gramjs/sessions/CallbackSession.ts')).default;
const MemorySession = requireModule(path.join(gramjsDir, 'src/lib/gramjs/sessions/Memory.ts')).default;
const Api = requireModule(path.join(gramjsDir, 'src/lib/gramjs/tl/index.ts')).Api;
const Connection = requireModule(path.join(gramjsDir, 'src/lib/gramjs/network/connection/TCPObfuscated.ts')).ConnectionTCPObfuscated;
const { chromium } = requireModule(path.join(gramjsDir, 'node_modules/playwright'));
const quietLogger = { debug() {}, info() {}, warn() {}, error() {} };

function rpcName(value: any): string {
  return value?.className || value?.constructor?.name || typeof value;
}

function isExpectedCleanupNoise(args: unknown[]): boolean {
  if (args.length !== 1) return false;
  const [value] = args;
  if (value instanceof Error && value.message === 'Not connected') return true;
  return typeof value === 'string'
    && /^Socket .* closed\. Code: 1005, reason: , was clean: true$/.test(value);
}

async function waitForSenderLoops(sender: any): Promise<void> {
  const loops = [sender?._sendLoopHandle, sender?._recvLoopHandle, sender?._longPollLoopHandle]
    .filter((loop): loop is Promise<unknown> => Boolean(loop && typeof loop.then === 'function'));
  if (!loops.length) return;
  await Promise.race([
    Promise.allSettled(loops).then(() => undefined),
    new Promise<void>((resolve) => setTimeout(resolve, 500)),
  ]);
}

async function destroyClient(client: any): Promise<void> {
  const sender = client?._sender;
  const originalConsoleError = console.error;
  console.error = (...args: unknown[]) => {
    if (!isExpectedCleanupNoise(args)) originalConsoleError(...args);
  };
  try {
    client.destroy();
    await waitForSenderLoops(sender);
    // WebSocket close events can arrive after the sender loops settle. Keep
    // the narrow cleanup filter active for one bounded close-event window.
    await new Promise<void>((resolve) => setTimeout(resolve, 250));
  } finally {
    console.error = originalConsoleError;
  }
}

async function closeProbeServer(server: any): Promise<void> {
  if (!server.listening) return;
  await new Promise<void>((resolve) => {
    let settled = false;
    const finish = () => {
      if (settled) return;
      settled = true;
      resolve();
    };
    server.close(finish);
    server.closeAllConnections?.();
    const timeout = setTimeout(() => {
      server.closeAllConnections?.();
      server.unref?.();
      finish();
    }, 1000);
    (timeout as unknown as { unref?: () => void }).unref?.();
  });
}

function unrefOneShotHandles(): void {
  for (const handle of process._getActiveHandles() as any[]) {
    if (handle.constructor?.name === 'MessagePort') handle.unref?.();
  }
}

function sql(query: string): string {
  return execFileSync('docker', [
    'exec', postgresContainer, 'psql', '-v', 'ON_ERROR_STOP=1', '-Atq', '-U', 'teamgram', '-d', 'teamgram', '-c', query,
  ], { encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] }).trim();
}

function loadAuthKey(): string {
  const query = [
    "SELECT encode(k.body, 'base64')",
    'FROM auth_users u JOIN auth_keys k USING (auth_key_id) JOIN auth_key_infos i USING (auth_key_id)',
    `WHERE u.user_id = ${userId} AND u.deleted = false AND k.deleted = false AND i.deleted = false AND i.auth_key_type = 0`,
    'ORDER BY u.date_active DESC, u.id DESC LIMIT 1',
  ].join(' ');
  const value = sql(query);
  if (!value) throw new Error(`no permanent auth key found for user ${userId}`);
  return value;
}

function makeClient(authKeyBase64: string): any {
  const authKeyHex = Buffer.from(authKeyBase64, 'base64').toString('hex');
  const session = new CallbackSession({ mainDcId: dcId, keys: { [dcId]: authKeyHex } }, () => {});
  const load = session.load.bind(session);
  session.load = async () => {
    await load();
    (session as any)._serverAddress = dcHost;
    (session as any)._port = dcPort;
  };
  const client = new TelegramClient(session, apiId, apiHash, {
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
  (client as any)._borrowExportedSender = async () => undefined;
  (client as any)._updateLoop = async () => undefined;
  return client;
}

function makeUnauthenticatedClient(): any {
  const session = new MemorySession();
  session.setDC(dcId, dcHost, dcPort);
  const client = new TelegramClient(session, apiId, apiHash, {
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
  (client as any)._borrowExportedSender = async () => undefined;
  (client as any)._updateLoop = async () => undefined;
  return client;
}

function makeInputCredential(credential: any, registration: boolean): any {
  const response = registration
    ? new Api.InputPasskeyResponseRegister({
      clientData: new Api.DataJSON({ data: Buffer.from(credential.response.clientDataJSON, 'base64url').toString('utf8') }),
      attestationData: Buffer.from(credential.response.attestationObject, 'base64url'),
    })
    : new Api.InputPasskeyResponseLogin({
      clientData: new Api.DataJSON({ data: Buffer.from(credential.response.clientDataJSON, 'base64url').toString('utf8') }),
      authenticatorData: Buffer.from(credential.response.authenticatorData, 'base64url'),
      signature: Buffer.from(credential.response.signature, 'base64url'),
      userHandle: Buffer.from(credential.response.userHandle || '', 'base64url').toString('utf8'),
    });
  return new Api.InputPasskeyCredentialPublicKey({
    id: credential.id,
    rawId: credential.rawId,
    response,
  });
}

function createCertificate(tempDir: string): { key: string; cert: string } {
  const key = path.join(tempDir, 'key.pem');
  const cert = path.join(tempDir, 'cert.pem');
  execFileSync('openssl', [
    'req', '-x509', '-newkey', 'rsa:2048', '-nodes', '-days', '1',
    '-subj', `/CN=${rpHost}`, '-keyout', key, '-out', cert,
  ], { stdio: 'ignore' });
  return { key, cert };
}

async function listen(server: any): Promise<number> {
  await new Promise<void>((resolve, reject) => {
    server.once('error', reject);
    server.listen(rpPort, '127.0.0.1', () => resolve());
  });
  server.unref?.();
  return (server.address() as { port: number }).port;
}

async function browserCredential(page: any, mode: 'registration' | 'login', option: any): Promise<any> {
  return page.evaluate(async ({ mode: operation, option: rawOption }: { mode: string; option: any }) => {
    const publicKeyCredential = (globalThis as any).PublicKeyCredential;
    const publicKey = operation === 'registration'
      ? publicKeyCredential.parseCreationOptionsFromJSON(rawOption.publicKey)
      : publicKeyCredential.parseRequestOptionsFromJSON(rawOption.publicKey);
    const credential = operation === 'registration'
      ? await navigator.credentials.create({ publicKey })
      : await navigator.credentials.get({ publicKey });
    if (!credential) throw new Error(`navigator.credentials.${operation === 'registration' ? 'create' : 'get'} returned null`);
    return (credential as any).toJSON();
  }, { mode, option });
}

async function main() {
  const tempDir = mkdtempSync(path.join(tmpdir(), 'teamgram-passkey-'));
  const { key, cert } = createCertificate(tempDir);
  const server = createServer({ key: readFileSync(key), cert: readFileSync(cert) }, (_request, response) => {
    response.writeHead(200, { 'content-type': 'text/html; charset=utf-8' });
    response.end('<!doctype html><title>Teamgram Passkey Acceptance</title>');
  });
  const port = await listen(server);
  let browser: any;
  let context: any;
  let userClient: any;
  let loginClient: any;
  let credentialId = '';
  const beforeCredentials = Number(sql(`SELECT count(*) FROM apifull_passkey_credential WHERE user_id = ${userId} AND deleted = false`));
  const beforeSessions = Number(sql(`SELECT count(*) FROM apifull_passkey_session WHERE used = false AND expires_at > extract(epoch from now())::bigint`));

  try {
    userClient = makeClient(loadAuthKey());
    await userClient.connect();
    const me = await userClient.getMe();
    if (String(me?.id) !== String(userId)) throw new Error(`registration auth key resolved to ${String(me?.id)}`);

    const cdpContext = await chromium.launch({
      headless: true,
      executablePath: chromePath,
      args: [`--host-resolver-rules=MAP ${rpHost} 127.0.0.1`],
    });
    browser = cdpContext;
    context = await browser.newContext({ ignoreHTTPSErrors: true });
    const page = await context.newPage();
    await page.goto(`https://${rpHost}:${port}/`, { waitUntil: 'domcontentloaded' });
    const cdp = await context.newCDPSession(page);
    await cdp.send('WebAuthn.enable');
    await cdp.send('WebAuthn.addVirtualAuthenticator', {
      options: {
        protocol: 'ctap2',
        transport: 'internal',
        hasResidentKey: true,
        hasUserVerification: true,
        isUserVerified: true,
        automaticPresenceSimulation: true,
      },
    });

    const registrationResult = await userClient.invoke(new Api.account.InitPasskeyRegistration());
    if (rpcName(registrationResult) !== 'account.PasskeyRegistrationOptions') {
      throw new Error(`account.initPasskeyRegistration returned ${rpcName(registrationResult)}`);
    }
    const registrationOption = JSON.parse(registrationResult.options.data);
    if (registrationOption?.publicKey?.rp?.id !== rpHost) throw new Error('registration RP ID mismatch');
    if (!String(registrationOption?.publicKey?.user?.id || '').length) throw new Error('registration user handle missing');
    const registrationCredential = await browserCredential(page, 'registration', registrationOption);
    const registered = await userClient.invoke(new Api.account.RegisterPasskey({
      credential: makeInputCredential(registrationCredential, true),
    }));
    if (rpcName(registered) !== 'Passkey') throw new Error(`account.registerPasskey returned ${rpcName(registered)}`);
    credentialId = String(registered.id || '');
    if (!credentialId || credentialId !== registrationCredential.id) throw new Error('registered credential ID mismatch');
    const passkeys = await userClient.invoke(new Api.account.GetPasskeys());
    if (rpcName(passkeys) !== 'account.Passkeys' || !passkeys.passkeys.some((item: any) => item.id === credentialId)) {
      throw new Error('registered passkey was not readable through account.getPasskeys');
    }

    loginClient = makeUnauthenticatedClient();
    await loginClient.connect();
    const loginResult = await loginClient.invoke(new Api.auth.InitPasskeyLogin({ apiId, apiHash }));
    if (rpcName(loginResult) !== 'auth.PasskeyLoginOptions') throw new Error(`auth.initPasskeyLogin returned ${rpcName(loginResult)}`);
    const loginOption = JSON.parse(loginResult.options.data);
    if (loginOption?.publicKey?.rpId !== rpHost) throw new Error('login RP ID mismatch');
    const loginCredential = await browserCredential(page, 'login', loginOption);
    const authorization = await loginClient.invoke(new Api.auth.FinishPasskeyLogin({
      credential: makeInputCredential(loginCredential, false),
    }));
    if (rpcName(authorization) !== 'auth.Authorization' || String(authorization.user?.id) !== String(userId)) {
      throw new Error(`auth.finishPasskeyLogin returned ${rpcName(authorization)} for user ${String(authorization.user?.id)}`);
    }
    const loggedInMe = await loginClient.getMe();
    if (String(loggedInMe?.id) !== String(userId)) throw new Error('passkey login authsession did not resolve the registered user');

    const afterCredentials = Number(sql(`SELECT count(*) FROM apifull_passkey_credential WHERE user_id = ${userId} AND deleted = false`));
    const afterSessions = Number(sql(`SELECT count(*) FROM apifull_passkey_session WHERE used = false AND expires_at > extract(epoch from now())::bigint`));
    console.log(JSON.stringify({
      ok: true,
      transport: `DC${dcId} TCPObfuscated -> gateway -> session -> Passkey/PostgreSQL`,
      userId: String(userId),
      origin: `https://${rpHost}:${port}`,
      registration: { constructor: rpcName(registered), credentialIdLength: credentialId.length },
      login: { constructor: rpcName(authorization), userId: String(authorization.user.id) },
      activeCredentials: { before: beforeCredentials, afterRegistration: afterCredentials },
      pendingSessions: { before: beforeSessions, afterLogin: afterSessions },
    }));
  } finally {
    if (userClient && credentialId) {
      try {
        await userClient.invoke(new Api.account.DeletePasskey({ id: credentialId }));
      } catch {
        // Best-effort cleanup; the result is checked below.
      }
    }
    if (loginClient) {
      try {
        await loginClient.invoke(new Api.auth.LogOut());
      } catch {
        // The auth key may already have been closed by the transport.
      }
      await destroyClient(loginClient);
    }
    if (userClient) {
      await destroyClient(userClient);
    }
    if (context) await context.close().catch(() => {});
    if (browser) await browser.close().catch(() => {});
    await closeProbeServer(server);
    rmSync(tempDir, { recursive: true, force: true });
    try { unlinkSync(key); } catch {}
    try { unlinkSync(cert); } catch {}
  }
}

main().catch((error) => {
  console.error(`passkey WebAuthn probe failed: ${error instanceof Error ? error.stack || error.message : String(error)}`);
  process.exitCode = 1;
}).finally(unrefOneShotHandles);
