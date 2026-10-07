import { createHash } from 'node:crypto';
import { execFileSync } from 'node:child_process';
import path from 'node:path';
import { createRequire } from 'node:module';

const gramjsDir = process.env.TEAMGRAM_GRAMJS_DIR || '/Users/mac/open/test/telegram-tt-master';
const dcId = Number(process.env.TG_DC_ID || '2');
const dcHost = process.env.TG_DC_HOST || '127.0.0.1';
const dcPort = Number(process.env.TG_DC_PORT || '11443');
const userId = process.env.LOOK_USER_ID || '136907714';

const globalAny = globalThis as typeof globalThis & { self?: any; addEventListener?: () => void };
globalAny.self ??= globalThis;
globalAny.addEventListener ??= () => {};
globalAny.self.addEventListener ??= globalAny.addEventListener;

function sql(query: string): string {
  return execFileSync('docker', [
    'exec', 'mysql', 'mysql', '-N', '-s', '-uteamgram', '-pteamgram', '-Dteamgram', '-e', query,
  ], { encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'] }).trim();
}

function loadAuthKey(): string {
  const body = sql(`
    SELECT k.body
    FROM auth_users u
    JOIN auth_keys k USING (auth_key_id)
    JOIN auth_key_infos i USING (auth_key_id)
    WHERE u.user_id=${userId} AND u.state=0 AND u.deleted=0
      AND k.deleted=0 AND i.deleted=0 AND i.auth_key_type=0
    ORDER BY u.date_active DESC, u.id DESC
    LIMIT 1
  `);
  if (!body) throw new Error(`no active production auth key found for user ${userId}`);
  return Buffer.from(body, 'base64').toString('hex');
}

function kvHash(key: string): string {
  const value = sql(`SELECT v FROM apifull_kv WHERE k='${key}'`);
  return createHash('sha256').update(value).digest('hex');
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

function makeClient(authKeyHex: string): any {
  const session = new CallbackSession({ mainDcId: dcId, keys: { [dcId]: authKeyHex } }, () => {});
  const load = session.load.bind(session);
  session.load = async () => {
    await load();
    (session as any)._serverAddress = dcHost;
    (session as any)._port = dcPort;
  };
  return new TelegramClient(session, 1, '0123456789abcdef0123456789abcdef', {
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

function tlString(value: string): Uint8Array {
  const data = new TextEncoder().encode(value);
  if (data.length >= 254) throw new Error('format is unexpectedly large');
  const out = new Uint8Array((1 + data.length + 3) & ~3);
  out[0] = data.length;
  out.set(data, 1);
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
  return {
    className,
    classType: 'request',
    CONSTRUCTOR_ID: constructorId,
    getBytes: () => payload,
    readResult: (reader: any) => reader.tgReadObject(),
  };
}

function getThemesRequest(format: string, hash: bigint): any {
  return rawRequest('account.GetThemes', 0x7206e458, concat(
    int32(0x7206e458), tlString(format), int64(hash),
  ));
}

function getChatThemesRequest(hash: bigint): any {
  return rawRequest('account.GetChatThemes', 0xd638de89, concat(
    int32(0xd638de89), int64(hash),
  ));
}

function ensureType(value: any, suffix: string, method: string): void {
  const name = rpcName(value);
  if (!name.toLowerCase().endsWith(suffix.toLowerCase())) {
    throw new Error(`${method} returned ${name}, expected ${suffix}`);
  }
}

async function main() {
  const before = {
    wallpaper: kvHash(`wallpaper:${userId}`),
    theme: kvHash(`theme:${userId}`),
  };
  const client = makeClient(loadAuthKey());
  try {
    (client as any)._borrowExportedSender = async () => undefined;
    (client as any)._updateLoop = async () => undefined;
    await client.connect();
    const me = await client.getMe();
    if (String(me?.id) !== userId) throw new Error(`auth key user mismatch: ${String(me?.id)}`);

    const contentSettings = await client.invoke(new Api.account.GetContentSettings());
    ensureType(contentSettings, 'contentSettings', 'account.getContentSettings');

    const wallpapers = await client.invoke(new Api.account.GetWallPapers({ hash: 0n }));
    ensureType(wallpapers, 'wallpapers', 'account.getWallPapers');
    const wallpaperItems = Array.isArray(wallpapers.wallpapers) ? wallpapers.wallpapers : [];
    const wallpapersNotModified = await client.invoke(new Api.account.GetWallPapers({
      hash: BigInt(wallpapers.hash || 0),
    }));
    ensureType(wallpapersNotModified, 'notmodified', 'account.getWallPapers(hash)');

    const themes = await client.invoke(getThemesRequest('android', 0n));
    ensureType(themes, 'themes', 'account.getThemes');
    const themeItems = Array.isArray(themes.themes) ? themes.themes : [];
    const themesNotModified = await client.invoke(getThemesRequest('android', BigInt(themes.hash || 0)));
    ensureType(themesNotModified, 'notmodified', 'account.getThemes(hash)');

    const chatThemes = await client.invoke(getChatThemesRequest(0n));
    ensureType(chatThemes, 'themes', 'account.getChatThemes');
    const chatThemeItems = Array.isArray(chatThemes.themes) ? chatThemes.themes : [];
    const chatThemesNotModified = await client.invoke(getChatThemesRequest(BigInt(chatThemes.hash || 0)));
    ensureType(chatThemesNotModified, 'notmodified', 'account.getChatThemes(hash)');

    const after = {
      wallpaper: kvHash(`wallpaper:${userId}`),
      theme: kvHash(`theme:${userId}`),
    };
    if (before.wallpaper !== after.wallpaper || before.theme !== after.theme) {
      throw new Error('account look KV changed during read-only probe');
    }
    if (wallpaperItems.length !== 0 || themeItems.length !== 0 || chatThemeItems.length !== 0) {
      throw new Error(`production fixture changed: wallpapers=${wallpaperItems.length}, themes=${themeItems.length}, chatThemes=${chatThemeItems.length}`);
    }
    console.log(JSON.stringify({
      userId: String(me.id),
      contentSettingsType: rpcName(contentSettings),
      wallpaperType: rpcName(wallpapers),
      wallpaperHash: String(wallpapers.hash),
      wallpaperCount: wallpaperItems.length,
      wallpaperHashResponseType: rpcName(wallpapersNotModified),
      themesType: rpcName(themes),
      themesHash: String(themes.hash),
      themesCount: themeItems.length,
      themesHashResponseType: rpcName(themesNotModified),
      chatThemesType: rpcName(chatThemes),
      chatThemesHash: String(chatThemes.hash),
      chatThemesCount: chatThemeItems.length,
      chatThemesHashResponseType: rpcName(chatThemesNotModified),
      kvSha256: after,
      writes: 0,
    }));
  } finally {
    client.destroy();
  }
}

main().then(() => process.exit(0)).catch((error) => {
  console.error(`account look production probe failed: ${error instanceof Error ? error.stack || error.message : String(error)}`);
  process.exit(1);
});
