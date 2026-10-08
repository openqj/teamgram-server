import { execFileSync } from 'node:child_process';
import path from 'node:path';
import { createRequire } from 'node:module';

const gramjsDir = process.env.TEAMGRAM_GRAMJS_DIR || '/Users/mac/open/test/telegram-tt-master';
const dcId = Number(process.env.TG_DC_ID || '2');
const userId = process.env.AUTO_DOWNLOAD_USER_ID || '136907714';
const globalAny = globalThis as typeof globalThis & { self?: any; addEventListener?: () => void };
globalAny.self ??= globalThis;
globalAny.addEventListener ??= () => {};
globalAny.self.addEventListener ??= globalAny.addEventListener;

function sql(query: string): string {
  return execFileSync('docker', [
    'exec', 'teamgram-postgres', 'psql', '-v', 'ON_ERROR_STOP=1', '-At', '-q', '-F', '\t', '-U', 'teamgram', '-d', 'teamgram', '-c', query,
  ], { encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'] }).trim();
}

function loadAuthKey(): string {
  const body = execFileSync('docker', ['exec', 'teamgram-postgres', 'psql', '-v', 'ON_ERROR_STOP=1', '-At', '-q', '-F', '\t', '-U', 'teamgram', '-d', 'teamgram', '-c', `SELECT k.body FROM auth_users u JOIN auth_keys k USING(auth_key_id) JOIN auth_key_infos i USING(auth_key_id) WHERE u.user_id=${userId} AND u.state=0 AND u.deleted=0 AND k.deleted=0 AND i.deleted=0 AND i.auth_key_type=0 ORDER BY u.date_active DESC,u.id DESC LIMIT 1`], { encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'] }).trim();
  if (!body) throw new Error(`no active production auth key found for user ${userId}`);
  return Buffer.from(body, 'base64').toString('hex');
}

const requireModule = createRequire(import.meta.url);
const TelegramClient = requireModule(path.join(gramjsDir, 'src/lib/gramjs/client/TelegramClient.ts')).default;
const CallbackSession = requireModule(path.join(gramjsDir, 'src/lib/gramjs/sessions/CallbackSession.ts')).default;
const Api = requireModule(path.join(gramjsDir, 'src/lib/gramjs/tl/index.ts')).Api;
const Connection = requireModule(path.join(gramjsDir, 'src/lib/gramjs/network/connection/TCPObfuscated.ts')).ConnectionTCPObfuscated;
const quietLogger = { debug() {}, info() {}, warn() {}, error() {} };
function rpcName(value: any): string { return value?.className || value?.constructor?.name || typeof value; }
function request(): any {
  return { className: 'account.GetAutoDownloadSettings', classType: 'request', CONSTRUCTOR_ID: 0x56da0b3f,
    getBytes: () => new Uint8Array([0x3f, 0x0b, 0xda, 0x56]), readResult: (reader: any) => reader.tgReadObject() };
}
function makeClient(authKeyHex: string): any {
  const session = new CallbackSession({ mainDcId: dcId, keys: { [dcId]: authKeyHex } }, () => {});
  const load = session.load.bind(session);
  session.load = async () => { await load(); (session as any)._serverAddress = process.env.TG_DC_HOST || '127.0.0.1'; (session as any)._port = Number(process.env.TG_DC_PORT || '11443'); };
  return new TelegramClient(session, 1, '0123456789abcdef0123456789abcdef', { connection: Connection, fallbackConnection: Connection, shouldAllowHttpTransport: false, shouldForceHttpTransport: false, timeout: 20, requestRetries: 1, connectionRetries: 1, connectionRetriesToFallback: 0, autoReconnect: false, baseLogger: quietLogger });
}
function snapshot(): Record<string, { present: boolean; hash: string }> {
  const out: Record<string, { present: boolean; hash: string }> = {};
  for (const slot of ['low', 'medium', 'high']) {
    const row = sql(`SELECT COUNT(*), COALESCE(MAX(encode(digest(v::text, 'sha256'), 'hex')), '') FROM apifull_kv WHERE k='auto_download:${userId}:${slot}'`);
    const [count, hash] = row.split('\t');
    out[slot] = { present: Number(count) === 1, hash: hash || '' };
  }
  return out;
}
async function main() {
  const before = snapshot();
  const client = makeClient(loadAuthKey());
  try {
    (client as any)._borrowExportedSender = async () => undefined;
    (client as any)._updateLoop = async () => undefined;
    await client.connect();
    const me = await client.getMe();
    if (String(me?.id) !== userId) throw new Error('auth key user mismatch');
    const result = await client.invoke(request());
    if (rpcName(result) !== 'account.AutoDownloadSettings') throw new Error(`getAutoDownloadSettings returned ${rpcName(result)}`);
    for (const slot of ['low', 'medium', 'high']) if (rpcName(result?.[slot]) !== 'AutoDownloadSettings') throw new Error(`slot ${slot} returned ${rpcName(result?.[slot])}`);
    const after = snapshot();
    if (JSON.stringify(before) !== JSON.stringify(after)) throw new Error('auto-download KV changed during read-only probe');
    console.log(JSON.stringify({ userId, result: rpcName(result), slots: { low: rpcName(result.low), medium: rpcName(result.medium), high: rpcName(result.high) }, kv: after, writes: 0 }));
  } finally { client.destroy(); }
}
main().then(() => process.exit(0)).catch((error) => { console.error(`auto-download production probe failed: ${error instanceof Error ? error.stack || error.message : String(error)}`); process.exit(1); });
