import { createRequire } from 'node:module';
import { execFileSync } from 'node:child_process';
import path from 'node:path';

const globalAny = globalThis as typeof globalThis & { self?: any; addEventListener?: () => void };
globalAny.self ??= globalThis;
globalAny.addEventListener ??= () => {};
globalAny.self.addEventListener ??= globalAny.addEventListener;

for (const key of ['TEAMGRAM_GRAMJS_DIR', 'A_AUTH_B64', 'A_USER_ID']) {
  if (!process.env[key]) throw new Error(`${key} is required`);
}

const root = process.env.TEAMGRAM_GRAMJS_DIR!;
const requireModule = createRequire(import.meta.url);
const TelegramClient = requireModule(path.join(root, 'src/lib/gramjs/client/TelegramClient.ts')).default;
const CallbackSession = requireModule(path.join(root, 'src/lib/gramjs/sessions/CallbackSession.ts')).default;
const Api = requireModule(path.join(root, 'src/lib/gramjs/tl/index.ts')).Api;
const Connection = requireModule(path.join(root, 'src/lib/gramjs/network/connection/TCPObfuscated.ts')).ConnectionTCPObfuscated;
const dcId = Number(process.env.TG_DC_ID || '2');
const userId = BigInt(process.env.A_USER_ID!);
const quietLogger = { debug() {}, info() {}, warn() {}, error() {} };

function concatBytes(...parts: Uint8Array[]): Uint8Array {
  const out = new Uint8Array(parts.reduce((sum, part) => sum + part.length, 0));
  let offset = 0;
  for (const part of parts) { out.set(part, offset); offset += part.length; }
  return out;
}
function int32(value: number): Uint8Array {
  const out = new Uint8Array(4); new DataView(out.buffer).setInt32(0, value, true); return out;
}
function tlString(value: string): Uint8Array {
  const raw = new TextEncoder().encode(value); const header = raw.length < 254 ? 1 : 4;
  const out = new Uint8Array((header + raw.length + 3) & ~3);
  if (header === 1) out[0] = raw.length; else new DataView(out.buffer).setUint32(0, raw.length, true);
  out.set(raw, header); return out;
}
class RawRequest {
  SUBCLASS_OF_ID = 0; classType = 'request';
  constructor(public readonly CONSTRUCTOR_ID: number, private readonly payload: Uint8Array, public readonly className: string) {}
  getBytes() { return concatBytes(int32(this.CONSTRUCTOR_ID), this.payload); }
  readResult(reader: any) { return reader.tgReadObject(); }
}
function workHours(api: any) {
  const value = new api.BusinessWorkHours({ openNow: true, timezoneId: 'UTC', weeklyOpen: [new api.BusinessWeeklyOpen({ startMinute: 60, endMinute: 120 })] });
  return new RawRequest(0x4b00e066, concatBytes(int32(1), value.getBytes()), 'account.updateBusinessWorkHours');
}
function location(api: any) {
  const geo = new api.InputGeoPoint({ lat: 31.2304, long: 121.4737 });
  return new RawRequest(0x9e6b131a, concatBytes(int32(3), geo.getBytes(), tlString('Layer229 temporary probe')), 'account.updateBusinessLocation');
}
function greeting(api: any) {
  const recipients = new api.InputBusinessRecipients({ contacts: true });
  const message = new api.InputBusinessGreetingMessage({ shortcutId: 0, recipients, noActivityDays: 7 });
  return new RawRequest(0x66cdafc4, concatBytes(int32(1), message.getBytes()), 'account.updateBusinessGreetingMessage');
}
function away(api: any) {
  const recipients = new api.InputBusinessRecipients({ contacts: true });
  const message = new api.InputBusinessAwayMessage({ offlineOnly: true, schedule: new api.BusinessAwayMessageScheduleOutsideWorkHours(), shortcutId: 0, recipients });
  return new RawRequest(0xa26a7fa5, concatBytes(int32(1), message.getBytes()), 'account.updateBusinessAwayMessage');
}
function intro(api: any) {
  const value = new api.InputBusinessIntro({ title: 'Layer229 probe', description: 'temporary' });
  return new RawRequest(0xa614d034, concatBytes(int32(1), value.getBytes()), 'account.updateBusinessIntro');
}
function clear(id: number, name: string) { return new RawRequest(id, int32(0), name); }
function setMainProfileTab(api: any) { return new RawRequest(0x5dee78b0, new api.ProfileTabPosts().getBytes(), 'account.setMainProfileTab'); }
function mainTab(): number {
  const output = execFileSync('docker', ['exec', 'teamgram-postgres', 'psql', '-v', 'ON_ERROR_STOP=1', '-At', '-q', '-F', '\t', '-U', 'teamgram', '-d', 'teamgram', '-c', `SELECT main_tab FROM users WHERE id=${userId}`], { encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'] });
  return Number(output.trim());
}
function restoreMainTab(value: number) {
  execFileSync('docker', ['exec', 'teamgram-postgres', 'psql', '-v', 'ON_ERROR_STOP=1', '-At', '-q', '-F', '\t', '-U', 'teamgram', '-d', 'teamgram', '-c', `UPDATE users SET main_tab=${value} WHERE id=${userId}`], { stdio: ['ignore', 'ignore', 'ignore'] });
}
function presence(): Record<string, boolean> {
  const keys = [`bhours:${userId}`, `bloc:${userId}`, `greet:${userId}`, `greet:away:${userId}`, `bintro:${userId}`];
  const sql = `SELECT k,CHAR_LENGTH(v) FROM apifull_kv WHERE k IN (${keys.map((key) => `'${key}'`).join(',')})`;
  const output = execFileSync('docker', ['exec', 'teamgram-postgres', 'psql', '-v', 'ON_ERROR_STOP=1', '-At', '-q', '-F', '\t', '-U', 'teamgram', '-d', 'teamgram', '-c', sql], { encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'] });
  const values = new Map(output.split(/\r?\n/).filter(Boolean).map((line) => { const [key, length] = line.split('\t'); return [key, Number(length) > 0] as const; }));
  return { workHours: values.get(keys[0]) === true, location: values.get(keys[1]) === true, greeting: values.get(keys[2]) === true, away: values.get(keys[3]) === true, intro: values.get(keys[4]) === true };
}
function makeClient(auth: string): any {
  const session = new CallbackSession({ mainDcId: dcId, keys: { [dcId]: Buffer.from(auth, 'base64').toString('hex') } }, () => {});
  const load = session.load.bind(session); session.load = async () => { await load(); (session as any)._serverAddress = process.env.TG_DC_HOST || '127.0.0.1'; (session as any)._port = Number(process.env.TG_DC_PORT || '11443'); };
  return new TelegramClient(session, 1, '0123456789abcdef0123456789abcdef', { connection: Connection, fallbackConnection: Connection, shouldAllowHttpTransport: false, shouldForceHttpTransport: false, timeout: 20, requestRetries: 2, connectionRetries: 2, connectionRetriesToFallback: 0, retryDelay: 250, autoReconnect: false, baseLogger: quietLogger });
}
async function main() {
  const client = makeClient(process.env.A_AUTH_B64!); const results: Record<string, unknown> = {};
  try {
    await client.connect(); const me = await client.getMe();
    if (String(me?.id) !== String(userId)) throw new Error('auth key user mismatch');
    const mainTabBefore = mainTab();
    const mainTabWrite = await client.invoke(setMainProfileTab(Api));
    const mainTabAfter = mainTab();
    restoreMainTab(mainTabBefore);
    const mainTabFinal = mainTab();
    if (!(mainTabWrite === true || mainTabWrite?.className?.toLowerCase().includes('bool')) || mainTabAfter !== 1 || mainTabFinal !== mainTabBefore) {
      throw new Error(`account.setMainProfileTab readback failed: result=${mainTabWrite?.className || typeof mainTabWrite} before=${mainTabBefore} after=${mainTabAfter} final=${mainTabFinal}`);
    }
    results.mainProfileTab = { result: mainTabWrite?.className || typeof mainTabWrite, before: mainTabBefore, after: mainTabAfter, restored: mainTabFinal };
    const writes = [await client.invoke(workHours(Api)), await client.invoke(location(Api)), await client.invoke(greeting(Api)), await client.invoke(away(Api)), await client.invoke(intro(Api))];
    if (writes.some((value) => !(value === true || value?.className?.toLowerCase().includes('bool')))) throw new Error(`business write returned ${writes.map((value) => value?.className || typeof value).join(',')}`);
    results.writes = writes.map((value) => value?.className || typeof value); results.persisted = presence();
    for (const request of [clear(0x4b00e066, 'account.updateBusinessWorkHours'), clear(0x9e6b131a, 'account.updateBusinessLocation'), clear(0x66cdafc4, 'account.updateBusinessGreetingMessage'), clear(0xa26a7fa5, 'account.updateBusinessAwayMessage'), clear(0xa614d034, 'account.updateBusinessIntro')]) await client.invoke(request);
    results.cleared = presence(); console.log(JSON.stringify(results));
  } finally { client.destroy(); }
}
main().then(() => process.exit(0)).catch((error) => { console.error(`business profile probe failed: ${error instanceof Error ? error.stack || error.message : String(error)}`); process.exit(1); });
