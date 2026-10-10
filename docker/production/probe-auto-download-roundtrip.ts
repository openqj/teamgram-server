import { execFileSync } from 'node:child_process';
import path from 'node:path';
import { createRequire } from 'node:module';

for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) {
  delete process.env[key];
}

const gramjsDir = process.env.TEAMGRAM_GRAMJS_DIR || '/Users/mac/open/test/telegram-tt-master';
const dcId = Number(process.env.TG_DC_ID || '2');
const dcHost = process.env.TG_DC_HOST || '127.0.0.1';
const dcPort = Number(process.env.TG_DC_PORT || '11443');
const userId = process.env.AUTO_DOWNLOAD_USER_ID || '136907714';
const slots = ['low', 'medium', 'high'];
const probeKey = `auto_download:${userId}:low`;

const globalAny = globalThis as typeof globalThis & { self?: any; addEventListener?: () => void };
globalAny.self ??= globalThis;
globalAny.addEventListener ??= () => {};
globalAny.self.addEventListener ??= globalAny.addEventListener;

function postgres(query: string): string {
  return execFileSync('docker', [
    'exec', 'teamgram-postgres', 'psql', '-v', 'ON_ERROR_STOP=1', '-At', '-q', '-F', '\t', '-U', 'teamgram', '-d', 'teamgram', '-c', query,
  ], { encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'] }).trim();
}

function storeSnapshot(): Record<string, { present: boolean; hash: string }> {
  return Object.fromEntries(slots.map((slot) => {
    const key = `auto_download:${userId}:${slot}`;
    const row = postgres(`SELECT COUNT(*), COALESCE(MAX(encode(digest(v::text, 'sha256'), 'hex')), '') FROM apifull_kv WHERE k='${key}'`);
    const [count, hash] = row.split('\t');
    const present = Number(count) === 1;
    return [slot, { present, hash: present ? hash : '' }];
  }));
}

function deleteIfHashMatches(key: string, hash: string): void {
  const result = postgres(`DELETE FROM apifull_kv WHERE k='${key}' AND encode(digest(v::text, 'sha256'), 'hex')='${hash}' RETURNING 1`);
  if (result !== '1') throw new Error('guarded PostgreSQL cleanup did not delete the probe-owned value');
}

function loadAuthKey(): string {
  const body = execFileSync('docker', [
    'exec', 'teamgram-postgres', 'psql', '-v', 'ON_ERROR_STOP=1', '-At', '-q', '-F', '\t', '-U', 'teamgram', '-d', 'teamgram', '-c',
    `SELECT encode(k.body, 'base64') FROM auth_users u JOIN auth_keys k USING(auth_key_id) JOIN auth_key_infos i USING(auth_key_id) WHERE u.user_id=${userId} AND u.state=0 AND u.deleted=0 AND k.deleted=0 AND i.deleted=0 AND i.auth_key_type=0 ORDER BY u.date_active DESC,u.id DESC LIMIT 1`,
  ], { encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'] }).trim();
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

function isType(value: any, name: string): boolean {
  return rpcName(value) === name || rpcName(value).endsWith(`.${name}`);
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

function getSettingsRequest(): any {
  const constructorId = 0x56da0b3f;
  return rawRequest('account.GetAutoDownloadSettings', constructorId, int32(constructorId));
}

function saveLowSettingsRequest(): any {
  const methodId = 0x76f36233;
  const settings = concat(
    int32(0xbaa57628), int32(1), int32(111111), int64(222222n), int64(333333n),
    int32(444), int32(3), int32(5),
  );
  return rawRequest('account.SaveAutoDownloadSettings', methodId,
    concat(int32(methodId), int32(1), settings));
}

function settingsFingerprint(value: any): string {
  if (!isType(value, 'AutoDownloadSettings')) throw new Error(`unexpected auto-download slot ${rpcName(value)}`);
  return JSON.stringify({
    disabled: value.disabled === true,
    videoPreloadLarge: value.videoPreloadLarge === true,
    audioPreloadNext: value.audioPreloadNext === true,
    phonecallsLessData: value.phonecallsLessData === true,
    storiesPreload: value.storiesPreload === true,
    photoSizeMax: Number(value.photoSizeMax),
    videoSizeMax: String(value.videoSizeMax),
    fileSizeMax: String(value.fileSizeMax),
    videoUploadMaxbitrate: Number(value.videoUploadMaxbitrate),
    smallQueueActiveOperationsMax: Number(value.smallQueueActiveOperationsMax),
    largeQueueActiveOperationsMax: Number(value.largeQueueActiveOperationsMax),
  });
}

function assertProbeSettings(value: any): void {
  if (!isType(value, 'AutoDownloadSettings') || value.disabled !== true
    || Number(value.photoSizeMax) !== 111111 || String(value.videoSizeMax) !== '222222'
    || String(value.fileSizeMax) !== '333333' || Number(value.videoUploadMaxbitrate) !== 444
    || Number(value.smallQueueActiveOperationsMax) !== 3
    || Number(value.largeQueueActiveOperationsMax) !== 5) {
    throw new Error('saved low-tier auto-download marker did not read back');
  }
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
  const before = storeSnapshot();
  if (before.low.present) throw new Error(`refusing to overwrite existing ${probeKey} state`);

  const client = makeClient(loadAuthKey());
  let connected = false;
  let writeStarted = false;
  let ownedHash = '';
  let primaryError: unknown;
  let cleanupError: unknown;
  let setType = '';
  let baselineFingerprint = '';
  try {
    await client.connect();
    connected = true;
    const me = await client.getMe();
    if (String(me?.id) !== userId) throw new Error('production auth key user mismatch');

    const baseline = await client.invoke(getSettingsRequest());
    if (!isType(baseline, 'AutoDownloadSettings') && !isType(baseline, 'account.AutoDownloadSettings')) {
      throw new Error(`account.getAutoDownloadSettings returned ${rpcName(baseline)}`);
    }
    if (!isType(baseline.low, 'AutoDownloadSettings') || baseline.low.disabled === true) {
      throw new Error('refusing to replace a non-default low-tier auto-download setting');
    }
    baselineFingerprint = settingsFingerprint(baseline.low);
    const otherSlots = JSON.stringify({
      medium: settingsFingerprint(baseline.medium),
      high: settingsFingerprint(baseline.high),
    });

    writeStarted = true;
    const saved = await client.invoke(saveLowSettingsRequest());
    setType = rpcName(saved);
    if (!(saved === true || saved?.value === true || saved?.value === 1 || rpcName(saved).toLowerCase().includes('true'))) {
      throw new Error(`account.saveAutoDownloadSettings returned ${setType}`);
    }

    const updated = await client.invoke(getSettingsRequest());
    assertProbeSettings(updated.low);
    if (JSON.stringify({
      medium: settingsFingerprint(updated.medium),
      high: settingsFingerprint(updated.high),
    }) !== otherSlots) {
      throw new Error('saving the low-tier setting changed another auto-download tier');
    }
    const written = storeSnapshot().low;
    if (!written.present || !/^[a-f0-9]{64}$/.test(written.hash)) {
      throw new Error('save did not persist a uniquely guarded low-tier Redis value');
    }
    ownedHash = written.hash;
  } catch (error) {
    primaryError = error;
  } finally {
    if (connected && writeStarted) {
      try {
        const state = await client.invoke(getSettingsRequest());
        const low = state?.low;
        const current = storeSnapshot();
        if (current.low.present) {
          if (!ownedHash && isType(low, 'AutoDownloadSettings')
            && Number(low.photoSizeMax) === 111111 && String(low.videoSizeMax) === '222222'
            && String(low.fileSizeMax) === '333333' && low.disabled === true) {
            ownedHash = current.low.hash;
          }
          if (!ownedHash || current.low.hash !== ownedHash) {
            throw new Error('low-tier Redis value changed outside the probe; left it untouched');
          }
          deleteIfHashMatches(probeKey, current.low.hash);
        } else if (state && isType(low, 'AutoDownloadSettings')
          && Number(low.photoSizeMax) === 111111) {
          throw new Error('API returned the probe marker but its Redis key is absent');
        }
        const restored = storeSnapshot();
        if (JSON.stringify(restored) !== JSON.stringify(before)) {
          throw new Error('auto-download Redis keys did not return to their preflight state');
        }
        const defaultReadback = await client.invoke(getSettingsRequest());
        if (settingsFingerprint(defaultReadback.low) !== baselineFingerprint) {
          throw new Error('low-tier auto-download API did not return to its baseline value');
        }
      } catch (error) {
        cleanupError = error;
      }
    }
    client.destroy();
  }

  if (cleanupError) throw new Error(`auto-download cleanup failed: ${String(cleanupError)}`);
  if (primaryError) throw primaryError;
  console.log(JSON.stringify({
    transport: `DC${dcId} WebSocket -> gateway -> session -> AutoDownload BFF/PostgreSQL`,
    getType: 'account.AutoDownloadSettings',
    setType,
    lowTierRoundTrip: true,
    otherTiersUnchanged: true,
    postgresRestored: true,
    rpcWrites: 1,
  }));
}

main().then(() => process.exit(0)).catch((error) => {
  console.error(`auto-download production probe failed: ${error instanceof Error ? error.stack || error.message : String(error)}`);
  process.exit(1);
});
