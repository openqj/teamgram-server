import { execFileSync } from 'node:child_process';
import path from 'node:path';
import { createRequire } from 'node:module';

for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) {
  delete process.env[key];
}

const userId = process.env.REACTION_NOTIFY_USER_ID || '136907713';
const dcId = Number(process.env.TG_DC_ID || '2');
const dcHost = process.env.TG_DC_HOST || '127.0.0.1';
const dcPort = Number(process.env.TG_DC_PORT || '11443');
const gramjsDir = process.env.TEAMGRAM_GRAMJS_DIR || '/Users/mac/open/test/telegram-tt-master';
const settingsKeys = [`react:${userId}:notify_settings`, `b18:${userId}:react`];

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
    WHERE u.user_id=${userId} AND u.state=0 AND u.deleted=0
      AND k.deleted=0 AND i.deleted=0 AND i.auth_key_type=0
    ORDER BY u.date_active DESC, u.id DESC
    LIMIT 1
  `);
  const key = Buffer.from(body, 'base64');
  if (key.length !== 256) throw new Error('active production auth key is absent or invalid');
  return key.toString('hex');
}

function snapshot(): Record<string, { rows: number; hash: string; value: string }> {
  return Object.fromEntries(settingsKeys.map((key) => {
    const row = postgres(`SELECT COUNT(*), COALESCE(MAX(encode(digest(v::text, 'sha256'), 'hex')), ''), COALESCE(MAX(encode(convert_to(v::text, 'UTF8'), 'hex')), '') FROM apifull_kv WHERE k='${key}'`);
    const [rows, hash, valueHex] = row.split('\t');
    return [key, { rows: Number(rows), hash: hash || '', value: Buffer.from(valueHex || '', 'hex').toString('utf8') }];
  }));
}

const requireModule = createRequire(import.meta.url);
const TelegramClient = requireModule(path.join(gramjsDir, 'src/lib/gramjs/client/TelegramClient.ts')).default;
const CallbackSession = requireModule(path.join(gramjsDir, 'src/lib/gramjs/sessions/CallbackSession.ts')).default;
const Connection = requireModule(path.join(gramjsDir, 'src/lib/gramjs/network/connection/TCPObfuscated.ts')).ConnectionTCPObfuscated;
const quietLogger = { debug() {}, info() {}, warn() {}, error() {} };

function rpcName(value: any): string {
  return value?.className || value?.constructor?.name || typeof value;
}

function int32(value: number): Uint8Array {
  const out = new Uint8Array(4);
  new DataView(out.buffer).setInt32(0, value, true);
  return out;
}

function int64(value: bigint): Uint8Array {
  const out = new Uint8Array(8);
  new DataView(out.buffer).setBigInt64(0, value, true);
  return out;
}

function setSettingsRequest(): any {
  const constructorId = 0x316ce548;
  const settings = Buffer.concat([
    Buffer.from(int32(0x71e4ea58)),
    Buffer.from(int32(0)),
    Buffer.from(int32(0xff6c8049)),
    Buffer.from(int64(0n)),
    Buffer.from(int32(0x997275b5)),
  ]);
  const payload = Buffer.concat([Buffer.from(int32(constructorId)), settings]);
  return {
    className: 'account.SetReactionsNotifySettings',
    classType: 'request',
    CONSTRUCTOR_ID: constructorId,
    getBytes: () => Uint8Array.from(payload),
    readResult: (reader: any) => reader.tgReadObject(),
  };
}

function deleteProbeRows(current: Record<string, { rows: number; hash: string; value: string }>): void {
  const expected = [
    JSON.stringify({ sound_predicate: 'notificationSoundRingtone', show_previews: true }),
    '0',
  ];
  for (let index = 0; index < settingsKeys.length; index += 1) {
    const key = settingsKeys[index];
    const row = current[key];
    if (row.rows === 0) continue;
    if (row.rows !== 1 || row.value !== expected[index] || !/^[a-f0-9]{64}$/.test(row.hash)) {
      throw new Error(`reaction settings key ${key} changed unexpectedly; left it untouched`);
    }
    const deleted = postgres(`DELETE FROM apifull_kv WHERE k='${key}' AND encode(digest(v::text, 'sha256'), 'hex')='${row.hash}' RETURNING 1`);
    if (deleted !== '1') throw new Error(`guarded cleanup did not delete the probe-owned ${key} row`);
  }
}

function getSettingsRequest(): any {
  const constructorId = 0x06dd654c;
  return {
    className: 'account.GetReactionsNotifySettings',
    classType: 'request',
    CONSTRUCTOR_ID: constructorId,
    getBytes: () => int32(constructorId),
    readResult: (reader: any) => reader.tgReadObject(),
  };
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
  const before = snapshot();
  if (settingsKeys.some((key) => before[key].rows !== 0)) {
    throw new Error('refusing to mutate pre-existing reaction notification settings');
  }
  const client = makeClient(loadAuthKey());
  let connected = false;
  let writeStarted = false;
  let primaryError: unknown;
  let cleanupError: unknown;
  let setType = '';
  let initial: any;
  try {
    await client.connect();
    connected = true;
    const me = await client.getMe();
    if (String(me?.id) !== userId) throw new Error('production auth key user mismatch');

    initial = await client.invoke(getSettingsRequest());
    if (!rpcName(initial).endsWith('ReactionsNotifySettings')) {
      throw new Error(`account.getReactionsNotifySettings returned ${rpcName(initial)}`);
    }
    if (before[settingsKeys[0]].rows === 0 && before[settingsKeys[1]].rows === 0) {
      if (rpcName(initial.sound) !== 'NotificationSoundRingtone'
        || String(initial.sound.id) !== '0' || initial.showPreviews !== false
        || initial.messagesNotifyFrom != null || initial.storiesNotifyFrom != null
        || initial.pollVotesNotifyFrom != null) {
        throw new Error('empty production keys did not return the expected default reaction settings');
      }
    }

    writeStarted = true;
    const updated = await client.invoke(setSettingsRequest());
    setType = rpcName(updated);
    if (!rpcName(updated).endsWith('ReactionsNotifySettings') || updated.showPreviews !== true
      || rpcName(updated.sound) !== 'NotificationSoundRingtone' || String(updated.sound.id) !== '0') {
      throw new Error(`account.setReactionsNotifySettings returned unexpected ${rpcName(updated)} value`);
    }
    const readback = await client.invoke(getSettingsRequest());
    if (!rpcName(readback).endsWith('ReactionsNotifySettings') || readback.showPreviews !== true
      || rpcName(readback.sound) !== 'NotificationSoundRingtone' || String(readback.sound.id) !== '0') {
      throw new Error('reaction notification settings did not read back the probe marker');
    }
    const written = snapshot();
    if (written[settingsKeys[0]].rows !== 1
      || written[settingsKeys[0]].value !== JSON.stringify({ sound_predicate: 'notificationSoundRingtone', show_previews: true })
      || written[settingsKeys[1]].rows !== 1 || written[settingsKeys[1]].value !== '0') {
      throw new Error('reaction notification KV rows do not match the probe marker');
    }

    const after = snapshot();
    if (after[settingsKeys[0]].rows !== 1 || after[settingsKeys[1]].rows !== 1) {
      throw new Error('probe did not create exactly the two expected reaction settings rows');
    }
  } catch (error) {
    primaryError = error;
  } finally {
    if (connected && writeStarted) {
      try {
        const current = snapshot();
        deleteProbeRows(current);
        const restored = snapshot();
        if (JSON.stringify(restored) !== JSON.stringify(before)) {
          throw new Error('reaction notification KV did not return to its preflight-empty state');
        }
        const defaultReadback = await client.invoke(getSettingsRequest());
        if (!rpcName(defaultReadback).endsWith('ReactionsNotifySettings')
          || defaultReadback.showPreviews !== false
          || rpcName(defaultReadback.sound) !== 'NotificationSoundRingtone'
          || String(defaultReadback.sound.id) !== '0') {
          throw new Error('reaction notification API did not return to defaults after cleanup');
        }
      } catch (error) {
        cleanupError = error;
      }
    }
    client.destroy();
  }
  if (cleanupError) throw new Error(`reaction notification cleanup failed: ${String(cleanupError)}`);
  if (primaryError) throw primaryError;
  console.log(JSON.stringify({
    transport: `DC${dcId} WebSocket -> gateway -> session -> APIFull/PostgreSQL`,
    getType: 'ReactionsNotifySettings',
    setType,
    defaultSoundType: rpcName(initial?.sound),
    defaultSoundId: String(initial?.sound?.id),
    markerReadback: true,
    kvRestoredAbsent: true,
    sqlCleanup: 'removed the two exact probe-owned values by SHA-256 guard',
    rpcWrites: 1,
  }));
}

main().then(() => process.exit(0)).catch((error) => {
  console.error(`reaction notification production probe failed: ${error instanceof Error ? error.stack || error.message : String(error)}`);
  process.exit(1);
});
