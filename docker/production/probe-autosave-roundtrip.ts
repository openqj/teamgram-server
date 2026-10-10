import { randomBytes } from 'node:crypto';
import { execFileSync } from 'node:child_process';
import path from 'node:path';
import { createRequire } from 'node:module';

for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) {
  delete process.env[key];
}

const userId = process.env.AUTOSAVE_USER_ID || '136907713';
const dcId = Number(process.env.TG_DC_ID || '2');
const dcHost = process.env.TG_DC_HOST || '127.0.0.1';
const dcPort = Number(process.env.TG_DC_PORT || '11443');
const gramjsDir = process.env.TEAMGRAM_GRAMJS_DIR || '/Users/mac/open/test/telegram-tt-master';
const kvKey = `autosave:${userId}`;

const globalAny = globalThis as typeof globalThis & { self?: any; addEventListener?: () => void };
globalAny.self ??= globalThis;
globalAny.addEventListener ??= () => {};
globalAny.self.addEventListener ??= globalAny.addEventListener;

function postgres(query: string): string {
  return execFileSync('docker', [
    'exec', 'teamgram-postgres', 'psql', '-v', 'ON_ERROR_STOP=1', '-At', '-q', '-F', '\t', '-U', 'teamgram', '-d', 'teamgram', '-c', query,
  ], { encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'] }).trim();
}

function kvSnapshot(): { rows: number; hash: string } {
  const row = postgres(`SELECT COUNT(*), COALESCE(MAX(encode(digest(v::text, 'sha256'), 'hex')), '') FROM apifull_kv WHERE k='${kvKey}'`);
  const [rows, hash] = row.split('\t');
  return { rows: Number(rows), hash: hash || '' };
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

const requireModule = createRequire(import.meta.url);
const TelegramClient = requireModule(path.join(gramjsDir, 'src/lib/gramjs/client/TelegramClient.ts')).default;
const CallbackSession = requireModule(path.join(gramjsDir, 'src/lib/gramjs/sessions/CallbackSession.ts')).default;
const Connection = requireModule(path.join(gramjsDir, 'src/lib/gramjs/network/connection/TCPObfuscated.ts')).ConnectionTCPObfuscated;
const quietLogger = { debug() {}, info() {}, warn() {}, error() {} };

function rpcName(value: any): string {
  return value?.className || value?.constructor?.name || typeof value;
}

function boolTrue(value: any): boolean {
  return value === true || value?.value === true || value?.value === 1;
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

function concat(...parts: Uint8Array[]): Uint8Array {
  const out = new Uint8Array(parts.reduce((size, part) => size + part.length, 0));
  let offset = 0;
  for (const part of parts) {
    out.set(part, offset);
    offset += part.length;
  }
  return out;
}

function rawRequest(className: string, constructorId: number, payload: Uint8Array): any {
  return { className, classType: 'request', CONSTRUCTOR_ID: constructorId,
    getBytes: () => payload, readResult: (reader: any) => reader.tgReadObject() };
}

function getAutoSaveRequest(): any {
  return rawRequest('account.GetAutoSaveSettings', 0xadcbbcda, int32(0xadcbbcda));
}

function deleteAutoSaveExceptionsRequest(): any {
  return rawRequest('account.DeleteAutoSaveExceptions', 0x53bc0020, int32(0x53bc0020));
}

function autoSaveSettingsBytes(videoMaxSize: bigint): Uint8Array {
  return concat(int32(0xc84834ce), int32(7), int64(videoMaxSize));
}

function saveAutoSaveUsersRequest(videoMaxSize: bigint): any {
  const constructorId = 0xd69b8361;
  return rawRequest('account.SaveAutoSaveSettings', constructorId, concat(
    int32(constructorId), int32(1), autoSaveSettingsBytes(videoMaxSize),
  ));
}

function saveAutoSavePeerRequest(peerId: bigint, videoMaxSize: bigint): any {
  const constructorId = 0xd69b8361;
  return rawRequest('account.SaveAutoSaveSettings', constructorId, concat(
    int32(constructorId), int32(8), int32(0x35a95cb9), int64(peerId), autoSaveSettingsBytes(videoMaxSize),
  ));
}

function isType(value: any, name: string): boolean {
  return rpcName(value) === name || rpcName(value).endsWith(`.${name}`);
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

function assertEmptySettings(value: any): void {
  if (!isType(value, 'AutoSaveSettings')
    || value.photos === true || value.videos === true || String(value.videoMaxSize || '0') !== '0') {
    throw new Error('automatic-save settings did not return to their default values');
  }
}

function assertMarkerSettings(value: any, markerVideoMaxSize: bigint): void {
  if (!isType(value, 'AutoSaveSettings') || value.photos !== true || value.videos !== true
    || String(value.videoMaxSize) !== String(markerVideoMaxSize)) {
    throw new Error('saved automatic-save settings did not read back with their typed fields');
  }
}

async function main() {
  const before = kvSnapshot();
  if (before.rows !== 0) throw new Error(`refusing to overwrite existing ${kvKey} state`);

  const client = makeClient(loadAuthKey());
  const probeChatId = (1n << 60n) | BigInt(`0x${randomBytes(7).toString('hex')}`);
  const markerVideoMaxSize = BigInt(1000000 + randomBytes(4).readUInt32BE(0) % 1000000000);
  let connected = false;
  let saveStarted = false;
  let ownedHash = '';
  let primaryError: unknown;
  let cleanupError: unknown;
  let saveType = '';
  let deleteType = '';

  try {
    await client.connect();
    connected = true;
    const me = await client.getMe();
    if (String(me?.id) !== userId) throw new Error('production auth key user mismatch');

    const baseline = await client.invoke(getAutoSaveRequest());
    if (!isType(baseline, 'AutoSaveSettings')
      || !Array.isArray(baseline.exceptions) || baseline.exceptions.length !== 0
      || !Array.isArray(baseline.chats) || baseline.chats.length !== 0
      || !Array.isArray(baseline.users) || baseline.users.length !== 0) {
      throw new Error('refusing to test delete against non-empty automatic-save state');
    }
    assertEmptySettings(baseline.usersSettings);
    assertEmptySettings(baseline.chatsSettings);
    assertEmptySettings(baseline.broadcastsSettings);

    saveStarted = true;
    const savedSettings = await client.invoke(saveAutoSaveUsersRequest(markerVideoMaxSize));
    saveType = rpcName(savedSettings);
    if (!boolTrue(savedSettings)) throw new Error(`account.saveAutoSaveSettings(users) returned ${saveType}`);

    let afterSave = kvSnapshot();
    if (afterSave.rows !== 1 || !/^[a-f0-9]{64}$/.test(afterSave.hash)) {
      throw new Error('save did not create one guarded automatic-save KV row');
    }
    ownedHash = afterSave.hash;

    const settingsReadback = await client.invoke(getAutoSaveRequest());
    if (!isType(settingsReadback, 'AutoSaveSettings') || settingsReadback.exceptions?.length !== 0) {
      throw new Error('automatic-save settings readback returned an unexpected shape');
    }
    assertMarkerSettings(settingsReadback.usersSettings, markerVideoMaxSize);
    assertEmptySettings(settingsReadback.chatsSettings);
    assertEmptySettings(settingsReadback.broadcastsSettings);

    const savedException = await client.invoke(saveAutoSavePeerRequest(probeChatId, markerVideoMaxSize));
    if (!boolTrue(savedException)) throw new Error(`account.saveAutoSaveSettings(peer) returned ${rpcName(savedException)}`);

    afterSave = kvSnapshot();
    if (afterSave.rows !== 1 || !/^[a-f0-9]{64}$/.test(afterSave.hash)) {
      throw new Error('save did not create one guarded automatic-save KV row');
    }
    ownedHash = afterSave.hash;

    const roundTrip = await client.invoke(getAutoSaveRequest());
    const exception = roundTrip?.exceptions?.[0];
    if (!isType(roundTrip, 'AutoSaveSettings') || roundTrip.exceptions?.length !== 1
      || rpcName(exception) !== 'AutoSaveException'
      || rpcName(exception.peer) !== 'PeerChat' || String(exception.peer.chatId) !== String(probeChatId)
      || exception.settings?.photos !== true || exception.settings?.videos !== true
      || String(exception.settings?.videoMaxSize) !== String(markerVideoMaxSize)) {
      throw new Error('saved automatic-save exception did not read back with its typed fields');
    }
    assertMarkerSettings(roundTrip.usersSettings, markerVideoMaxSize);

    const deleted = await client.invoke(deleteAutoSaveExceptionsRequest());
    deleteType = rpcName(deleted);
    if (!boolTrue(deleted)) throw new Error(`account.deleteAutoSaveExceptions returned ${deleteType}`);

    afterSave = kvSnapshot();
    if (afterSave.rows !== 1 || !/^[a-f0-9]{64}$/.test(afterSave.hash)) {
      throw new Error('automatic-save KV row disappeared or became ambiguous before cleanup');
    }
    ownedHash = afterSave.hash;

    const afterDelete = await client.invoke(getAutoSaveRequest());
    if (!isType(afterDelete, 'AutoSaveSettings') || afterDelete.exceptions?.length !== 0
      || !Array.isArray(afterDelete.chats) || afterDelete.chats.length !== 0
      || !Array.isArray(afterDelete.users) || afterDelete.users.length !== 0) {
      throw new Error('deleteAutoSaveExceptions did not remove the temporary exception');
    }
    assertMarkerSettings(afterDelete.usersSettings, markerVideoMaxSize);
    assertEmptySettings(afterDelete.chatsSettings);
    assertEmptySettings(afterDelete.broadcastsSettings);
  } catch (error) {
    primaryError = error;
  } finally {
    if (connected && saveStarted) {
      try {
        const state = await client.invoke(getAutoSaveRequest());
        const exception = state?.exceptions?.[0];
        const markerPresent = isType(state, 'AutoSaveSettings')
          && state.exceptions?.length <= 1
          && state.usersSettings?.photos === true
          && state.usersSettings?.videos === true
          && String(state.usersSettings?.videoMaxSize) === String(markerVideoMaxSize);
        if (!markerPresent) throw new Error('automatic-save marker changed during probe cleanup');
        if (state?.exceptions?.length === 1
          && rpcName(exception.peer) === 'PeerChat'
          && String(exception.peer.chatId) === String(probeChatId)) {
          const deleted = await client.invoke(deleteAutoSaveExceptionsRequest());
          if (!boolTrue(deleted)) throw new Error('failed to clear the temporary exception through RPC');
        } else if (state?.exceptions?.length !== 0) {
          throw new Error('unexpected automatic-save exceptions found during cleanup');
        }
        const current = kvSnapshot();
        if (current.rows !== 1 || !/^[a-f0-9]{64}$/.test(current.hash)) {
          throw new Error('automatic-save state is missing or ambiguous during cleanup');
        }
        ownedHash = current.hash;
      } catch (error) {
        cleanupError = error;
      }
    }

    try {
      const current = kvSnapshot();
      if (current.rows === 1 && ownedHash && current.hash === ownedHash) {
        const removed = postgres(`DELETE FROM apifull_kv WHERE k='${kvKey}' AND encode(digest(v::text, 'sha256'), 'hex')='${ownedHash}' RETURNING 1`);
        if (removed !== '1') throw new Error('guarded cleanup did not delete exactly the probe-owned KV row');
      } else if (current.rows !== 0) {
        throw new Error('automatic-save KV changed outside the probe; left it untouched');
      }
      const restored = kvSnapshot();
      if (restored.rows !== before.rows || restored.hash !== before.hash) {
        throw new Error('automatic-save KV did not return to its original absent state');
      }
      if (connected && saveStarted) {
        const restoredState = await client.invoke(getAutoSaveRequest());
        if (!isType(restoredState, 'AutoSaveSettings') || restoredState.exceptions?.length !== 0) {
          throw new Error('API did not return default automatic-save state after KV restoration');
        }
        assertEmptySettings(restoredState.usersSettings);
        assertEmptySettings(restoredState.chatsSettings);
        assertEmptySettings(restoredState.broadcastsSettings);
      }
    } catch (error) {
      cleanupError ??= error;
    }
    client.destroy();
  }

  if (cleanupError) throw new Error(`automatic-save cleanup failed: ${String(cleanupError)}`);
  if (primaryError) throw primaryError;
  if (kvSnapshot().rows !== 0) throw new Error('automatic-save probe left production state behind');
  console.log(JSON.stringify({
    transport: `DC${dcId} WebSocket -> gateway -> session -> APIFull/PostgreSQL`,
    getType: 'AutoSaveSettings',
    saveType,
    deleteType,
    exceptionRoundTrip: true,
    kvRestoredAbsent: true,
    rpcWrites: 3,
    sqlCleanup: 'removed the exact probe-owned KV value by SHA-256 guard',
  }));
}

main().then(() => process.exit(0)).catch((error) => {
  console.error(`automatic-save production probe failed: ${error instanceof Error ? error.stack || error.message : String(error)}`);
  process.exit(1);
});
