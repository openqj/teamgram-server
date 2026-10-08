import { execFileSync } from 'node:child_process';
import path from 'node:path';
import { createRequire } from 'node:module';

for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) {
  delete process.env[key];
}

const userId = process.env.CONTENT_SETTINGS_USER_ID || '136907714';
const dcId = Number(process.env.TG_DC_ID || '2');
const dcHost = process.env.TG_DC_HOST || '127.0.0.1';
const dcPort = Number(process.env.TG_DC_PORT || '11443');
const gramjsDir = process.env.TEAMGRAM_GRAMJS_DIR || '/Users/mac/open/test/telegram-tt-master';

const globalAny = globalThis as typeof globalThis & { self?: any; addEventListener?: () => void };
globalAny.self ??= globalThis;
globalAny.addEventListener ??= () => {};
globalAny.self.addEventListener ??= globalAny.addEventListener;

function postgres(query: string): string {
  return execFileSync('docker', [
    'exec', 'teamgram-postgres', 'psql', '-v', 'ON_ERROR_STOP=1', '-At', '-q', '-F', '\t', '-U', 'teamgram', '-d', 'teamgram', '-c', query,
  ], { encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'] }).trim();
}

function loadAuthKey(): string {
  const body = postgres(`
    SELECT k.body
    FROM auth_users u
    JOIN auth_keys k USING (auth_key_id)
    JOIN auth_key_infos i USING (auth_key_id)
    WHERE u.user_id=${userId} AND u.state=0 AND u.deleted=0
      AND k.deleted=0 AND i.deleted=0 AND i.auth_key_type=0
    ORDER BY u.date_active DESC, u.id DESC
    LIMIT 1
  `);
  const key = Buffer.from(body, 'base64');
  if (key.length !== 256) throw new Error('active production auth key is absent or invalid');
  return key.toString('hex');
}

function settingRow(): string {
  return postgres(`
    SELECT id, value, deleted FROM user_settings
    WHERE user_id=${userId} AND key2='sensitive_enabled'
    ORDER BY id LIMIT 1
  `);
}

const requireModule = createRequire(import.meta.url);
const TelegramClient = requireModule(path.join(gramjsDir, 'src/lib/gramjs/client/TelegramClient.ts')).default;
const CallbackSession = requireModule(path.join(gramjsDir, 'src/lib/gramjs/sessions/CallbackSession.ts')).default;
const Api = requireModule(path.join(gramjsDir, 'src/lib/gramjs/tl/index.ts')).Api;
const Connection = requireModule(path.join(gramjsDir, 'src/lib/gramjs/network/connection/TCPObfuscated.ts')).ConnectionTCPObfuscated;
const quietLogger = { debug() {}, info() {}, warn() {}, error() {} };

function rpcName(value: any): string {
  return value?.className || value?.constructor?.name || typeof value;
}

function boolTrue(value: any): boolean {
  return value === true || value?.value === true || value?.value === 1;
}

function makeClient(authKeyHex: string): any {
  const session = new CallbackSession({ mainDcId: dcId, keys: { [dcId]: authKeyHex } }, () => {});
  const load = session.load.bind(session);
  session.load = async () => {
    await load();
    (session as any)._serverAddress = dcHost;
    (session as any)._port = dcPort;
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
  (client as any)._borrowExportedSender = async () => undefined;
  (client as any)._updateLoop = async () => undefined;
  return client;
}

async function main() {
  const before = settingRow();
  const baselineRow = before.split('\t');
  if (!baselineRow[0] || baselineRow[1] !== 'false' || baselineRow[2] !== '0') {
    throw new Error('the selected account must have an active false sensitive_enabled row; refusing to mutate it');
  }

  const client = makeClient(loadAuthKey());
  let connected = false;
  let changed = false;
  let primaryError: unknown;
  let cleanupError: unknown;
  let setType = '';
  let enabledType = '';
  let disabledType = '';
  try {
    await client.connect();
    connected = true;
    const me = await client.getMe();
    if (String(me?.id) !== userId) throw new Error('production auth key user mismatch');

    const baseline = await client.invoke(new Api.account.GetContentSettings());
    if (!rpcName(baseline).toLowerCase().includes('contentsettings')
      || baseline.sensitiveEnabled === true || baseline.sensitiveCanChange !== true) {
      throw new Error(`unexpected baseline ${rpcName(baseline)}: sensitiveEnabled=${String(baseline?.sensitiveEnabled)}, sensitiveCanChange=${String(baseline?.sensitiveCanChange)}`);
    }

    changed = true;
    const enabled = await client.invoke(new Api.account.SetContentSettings({ sensitiveEnabled: true }));
    setType = rpcName(enabled);
    if (!boolTrue(enabled)) throw new Error(`account.setContentSettings(true) returned ${setType}`);

    const enabledReadback = await client.invoke(new Api.account.GetContentSettings());
    enabledType = rpcName(enabledReadback);
    if (!enabledReadback?.sensitiveEnabled || !enabledReadback?.sensitiveCanChange) {
      throw new Error(`account.getContentSettings did not read back true: ${enabledType}`);
    }
    const written = settingRow().split('\t');
    if (written[0] !== baselineRow[0] || written[1] !== 'true' || written[2] !== '0') {
      throw new Error('the User service did not persist the enabled setting');
    }

    const disabled = await client.invoke(new Api.account.SetContentSettings({ sensitiveEnabled: false }));
    disabledType = rpcName(disabled);
    if (!boolTrue(disabled)) throw new Error(`account.setContentSettings(false) returned ${disabledType}`);
    const disabledReadback = await client.invoke(new Api.account.GetContentSettings());
    if (disabledReadback?.sensitiveEnabled === true) {
      throw new Error('account.getContentSettings did not read back false after restoration');
    }
    const restored = settingRow().split('\t');
    if (restored[0] !== baselineRow[0] || restored[1] !== 'false' || restored[2] !== '0') {
      throw new Error('the User service did not persist the restored false value');
    }
  } catch (error) {
    primaryError = error;
  } finally {
    if (connected && changed) {
      try {
        const current = await client.invoke(new Api.account.GetContentSettings());
        if (current?.sensitiveEnabled === true) {
          const restored = await client.invoke(new Api.account.SetContentSettings({ sensitiveEnabled: false }));
          if (!boolTrue(restored)) throw new Error('failed to restore sensitive_enabled=false via RPC');
        }
        const row = settingRow().split('\t');
        if (row[0] !== baselineRow[0] || row[1] !== 'false' || row[2] !== '0') {
          throw new Error('content setting storage did not return to its original row and value');
        }
        const finalState = await client.invoke(new Api.account.GetContentSettings());
        if (finalState?.sensitiveEnabled === true || settingRow() !== before) {
          throw new Error('content setting did not return to its pre-probe state');
        }
      } catch (error) {
        cleanupError = error;
      }
    }
    client.destroy();
  }

  if (cleanupError) throw new Error(`content settings cleanup failed: ${String(cleanupError)}`);
  if (primaryError) throw primaryError;
  if (!changed || settingRow() !== before) throw new Error('content settings probe did not restore the production row state');
  console.log(JSON.stringify({
    transport: `DC${dcId} WebSocket -> gateway -> session -> NSFW BFF -> User service/PostgreSQL`,
    getType: enabledType,
    setEnabledType: setType,
    setDisabledType: disabledType,
    enabledReadback: true,
    disabledReadback: true,
    storageRestored: true,
    rpcWrites: 2,
    cleanup: 'restored the original false override through the RPC',
  }));
}

main().then(() => process.exit(0)).catch((error) => {
  console.error(`content settings roundtrip probe failed: ${error instanceof Error ? error.stack || error.message : String(error)}`);
  process.exit(1);
});
