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
    SELECT encode(k.body, 'base64')
    FROM auth_users u
    JOIN auth_keys k USING (auth_key_id)
    JOIN auth_key_infos i USING (auth_key_id)
    WHERE u.user_id=${userId} AND u.state=0 AND u.deleted=false
      AND k.deleted=false AND i.deleted=false AND i.auth_key_type=0
    ORDER BY u.date_active DESC, u.id DESC
    LIMIT 1
  `);
  const key = Buffer.from(body, 'base64');
  if (key.length !== 256) throw new Error('active production auth key is absent or invalid');
  return key.toString('hex');
}

function settingRow(): string {
  return postgres(`
    SELECT id, value, CASE WHEN deleted THEN 1 ELSE 0 END FROM user_settings
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
  if (!baselineRow[0] || !['true', 'false'].includes(baselineRow[1]) || baselineRow[2] !== '0') {
    throw new Error('the selected account must have an active sensitive_enabled row; refusing to mutate it');
  }
  const baselineEnabled = baselineRow[1] === 'true';
  const temporaryEnabled = !baselineEnabled;

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
      || Boolean(baseline.sensitiveEnabled) !== baselineEnabled || baseline.sensitiveCanChange !== true) {
      throw new Error(`unexpected baseline ${rpcName(baseline)}: sensitiveEnabled=${String(baseline?.sensitiveEnabled)}, sensitiveCanChange=${String(baseline?.sensitiveCanChange)}`);
    }

    changed = true;
    const temporary = await client.invoke(new Api.account.SetContentSettings({ sensitiveEnabled: temporaryEnabled }));
    setType = rpcName(temporary);
    if (!boolTrue(temporary)) throw new Error(`account.setContentSettings(${temporaryEnabled}) returned ${setType}`);

    const temporaryReadback = await client.invoke(new Api.account.GetContentSettings());
    enabledType = rpcName(temporaryReadback);
    if (Boolean(temporaryReadback?.sensitiveEnabled) !== temporaryEnabled || !temporaryReadback?.sensitiveCanChange) {
      throw new Error(`account.getContentSettings did not read back ${temporaryEnabled}: ${enabledType}`);
    }
    const written = settingRow().split('\t');
    if (written[0] !== baselineRow[0] || written[1] !== String(temporaryEnabled) || written[2] !== '0') {
      throw new Error('the User service did not persist the temporary setting');
    }

    const restored = await client.invoke(new Api.account.SetContentSettings({ sensitiveEnabled: baselineEnabled }));
    disabledType = rpcName(restored);
    if (!boolTrue(restored)) throw new Error(`account.setContentSettings(${baselineEnabled}) returned ${disabledType}`);
    const restoredReadback = await client.invoke(new Api.account.GetContentSettings());
    if (Boolean(restoredReadback?.sensitiveEnabled) !== baselineEnabled) {
      throw new Error(`account.getContentSettings did not read back ${baselineEnabled} after restoration`);
    }
    const restoredRow = settingRow().split('\t');
    if (restoredRow[0] !== baselineRow[0] || restoredRow[1] !== baselineRow[1] || restoredRow[2] !== '0') {
      throw new Error('the User service did not persist the restored setting');
    }
  } catch (error) {
    primaryError = error;
  } finally {
    if (connected && changed) {
      try {
        const current = await client.invoke(new Api.account.GetContentSettings());
        if (Boolean(current?.sensitiveEnabled) !== baselineEnabled) {
          const restored = await client.invoke(new Api.account.SetContentSettings({ sensitiveEnabled: baselineEnabled }));
          if (!boolTrue(restored)) throw new Error(`failed to restore sensitive_enabled=${baselineEnabled} via RPC`);
        }
        const row = settingRow().split('\t');
        if (row[0] !== baselineRow[0] || row[1] !== baselineRow[1] || row[2] !== '0') {
          throw new Error('content setting storage did not return to its original row and value');
        }
        const finalState = await client.invoke(new Api.account.GetContentSettings());
        if (Boolean(finalState?.sensitiveEnabled) !== baselineEnabled || settingRow() !== before) {
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
    setTemporaryType: setType,
    restoreType: disabledType,
    baselineEnabled,
    temporaryEnabled,
    temporaryReadback: true,
    restoredReadback: true,
    storageRestored: true,
    rpcWrites: 2,
    cleanup: 'restored the original sensitive_enabled value through the RPC',
  }));
}

main().then(() => process.exit(0)).catch((error) => {
  console.error(`content settings roundtrip probe failed: ${error instanceof Error ? error.stack || error.message : String(error)}`);
  process.exit(1);
});
