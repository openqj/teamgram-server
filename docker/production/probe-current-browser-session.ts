import fs from 'node:fs';
import path from 'node:path';
import { execFileSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { createRequire } from 'node:module';

const gramjsDir = process.env.TEAMGRAM_GRAMJS_DIR || '/Users/mac/open/test/telegram-tt-master';
const leveldbDir = process.env.CODEX_BROWSER_LEVELDB
  || '/Users/mac/Library/Application Support/Codex/Default/Partitions/codex-browser-app/Local Storage/leveldb';
const dcId = Number(process.env.TG_DC_ID || '2');
const dcHost = process.env.TG_DC_HOST || '127.0.0.1';
const dcPort = Number(process.env.TG_DC_PORT || '11443');

const globalAny = globalThis as typeof globalThis & { self?: any; addEventListener?: () => void };
globalAny.self ??= globalThis;
globalAny.addEventListener ??= () => {};
globalAny.self.addEventListener ??= globalAny.addEventListener;

type StoredSession = { authKeyHex: string; userId: string; authKeyId: string };

function authKeyId(authKeyHex: string): string {
  const digest = createHash('sha1').update(Buffer.from(authKeyHex, 'hex')).digest();
  return Buffer.from(digest.subarray(-8)).reverse().readBigUInt64BE(0).toString();
}

function loadStoredSessions(): StoredSession[] {
  const files = fs.readdirSync(leveldbDir)
    .filter((name) => /\.(ldb|log)$/.test(name))
    .map((name) => path.join(leveldbDir, name));
  const matches: Array<{ mtime: number; authKeyHex: string; userId: string }> = [];
  for (const file of files) {
    let raw: string;
    try {
      raw = execFileSync('strings', [file], { encoding: 'utf8', maxBuffer: 16 * 1024 * 1024 });
    } catch {
      continue;
    }
    const re = /\{"dcId":2,"dc2_auth_key":"([0-9a-f]+)","userId":"(\d+)"/g;
    for (const match of raw.matchAll(re)) {
      matches.push({
        mtime: fs.statSync(file).mtimeMs,
        authKeyHex: match[1],
        userId: match[2],
      });
    }
  }
  matches.sort((a, b) => b.mtime - a.mtime);
  const seen = new Set<string>();
  const sessions: StoredSession[] = [];
  for (const session of matches) {
    const id = authKeyId(session.authKeyHex);
    if (seen.has(id)) continue;
    seen.add(id);
    sessions.push({ ...session, authKeyId: id });
  }
  if (sessions.length === 0) throw new Error('no browser auth session found');
  return sessions;
}

const requireModule = createRequire(import.meta.url);
const TelegramClient = requireModule(path.join(gramjsDir, 'src/lib/gramjs/client/TelegramClient.ts')).default;
const CallbackSession = requireModule(path.join(gramjsDir, 'src/lib/gramjs/sessions/CallbackSession.ts')).default;
const Api = requireModule(path.join(gramjsDir, 'src/lib/gramjs/tl/index.ts')).Api;
const ConnectionTCPObfuscated = requireModule(path.join(
  gramjsDir,
  'src/lib/gramjs/network/connection/TCPObfuscated.ts',
)).ConnectionTCPObfuscated;

const quietLogger = { debug() {}, info() {}, warn() {}, error() {} };

function makeClient(authKeyHex: string): any {
  const session = new CallbackSession({ mainDcId: dcId, keys: { [dcId]: authKeyHex } }, () => {});
  const load = session.load.bind(session);
  session.load = async () => {
    await load();
    (session as any)._serverAddress = dcHost;
    (session as any)._port = dcPort;
  };
  return new TelegramClient(session, Number(process.env.TG_API_ID || '1'), process.env.TG_API_HASH || '0123456789abcdef0123456789abcdef', {
    connection: ConnectionTCPObfuscated,
    fallbackConnection: ConnectionTCPObfuscated,
    shouldAllowHttpTransport: false,
    shouldForceHttpTransport: false,
    timeout: 20,
    requestRetries: 1,
    connectionRetries: 1,
    connectionRetriesToFallback: 0,
    retryDelay: 250,
    autoReconnect: false,
    baseLogger: quietLogger,
  });
}

function className(value: any): string {
  return value?.className || value?.constructor?.name || typeof value;
}

function isClass(value: any, name: string): boolean {
  const actual = className(value);
  return actual === name || actual.endsWith(`.${name}`);
}

function readPersistedBrowserSettings(userId: string): any | null {
  const sql = `SELECT v FROM apifull_kv WHERE k='webbrowser:${userId}:' LIMIT 1`;
  const command = `MYSQL_PWD="$MYSQL_ROOT_PASSWORD" mysql -uroot --batch --skip-column-names teamgram -e ${JSON.stringify(sql)}`;
  const value = execFileSync('docker', ['exec', 'mysql', 'sh', '-lc', command], { encoding: 'utf8' }).trim();
  return value ? JSON.parse(value) : null;
}

function normalizeBrowserState(state: any): string {
  const normalizeException = (item: any) => ({ domain: item?.domain || '', url: item?.url || '', title: item?.title || '' });
  return JSON.stringify({
    openExternalBrowser: Boolean(state?.open_external_browser),
    displayCloseButton: Boolean(state?.display_close_button),
    external: (state?.external || []).map(normalizeException),
    inapp: (state?.inapp || []).map(normalizeException),
  });
}

class GetBusinessChatLinksRequest {
  CONSTRUCTOR_ID = 0x6f70dde1;
  SUBCLASS_OF_ID = 0;
  className = 'account.GetBusinessChatLinks';
  classType = 'request';

  getBytes() {
    const bytes = new Uint8Array(4);
    new DataView(bytes.buffer).setUint32(0, this.CONSTRUCTOR_ID, true);
    return bytes;
  }

  readResult(reader: any) {
    return reader.tgReadObject();
  }
}

function tlInt(value: number): Uint8Array {
  const bytes = new Uint8Array(4);
  new DataView(bytes.buffer).setInt32(0, value, true);
  return bytes;
}

function tlLong(value: unknown): Uint8Array {
  const bytes = new Uint8Array(8);
  const big = BigInt(value === undefined || value === null ? '0' : String(value));
  new DataView(bytes.buffer).setBigInt64(0, BigInt.asIntN(64, big), true);
  return bytes;
}

function tlString(value: string): Uint8Array {
  const payload = Buffer.from(value, 'utf8');
  const headerLength = payload.length < 254 ? 1 : 4;
  const totalLength = Math.ceil((headerLength + payload.length) / 4) * 4;
  const bytes = new Uint8Array(totalLength);
  if (headerLength === 1) {
    bytes[0] = payload.length;
    bytes.set(payload, 1);
  } else {
    bytes[0] = 254;
    bytes[1] = payload.length & 0xff;
    bytes[2] = (payload.length >>> 8) & 0xff;
    bytes[3] = (payload.length >>> 16) & 0xff;
    bytes.set(payload, 4);
  }
  return bytes;
}

function tlRequestBytes(constructorId: number, ...parts: Uint8Array[]): Uint8Array {
  return Uint8Array.from(Buffer.concat([
    Buffer.from(tlInt(constructorId)),
    ...parts.map((part) => Buffer.from(part)),
  ]));
}

abstract class RawBrowserSettingsRequest {
  SUBCLASS_OF_ID = 0;
  classType = 'request';
  abstract CONSTRUCTOR_ID: number;
  abstract getBytes(): Uint8Array;
  readResult(reader: any) { return reader.tgReadObject(); }
}

class GetWebBrowserSettingsRequest extends RawBrowserSettingsRequest {
  CONSTRUCTOR_ID = 0x56655768;
  constructor(private hash: unknown) { super(); }
  getBytes() { return tlRequestBytes(this.CONSTRUCTOR_ID, tlLong(this.hash)); }
}

class UpdateWebBrowserSettingsRequest extends RawBrowserSettingsRequest {
  CONSTRUCTOR_ID = 0x9adf82fe;
  constructor(private openExternal: boolean, private displayClose: boolean) { super(); }
  getBytes() {
    const flags = (this.openExternal ? 1 : 0) | (this.displayClose ? 2 : 0);
    return tlRequestBytes(this.CONSTRUCTOR_ID, tlInt(flags));
  }
}

class ToggleWebBrowserSettingsExceptionRequest extends RawBrowserSettingsRequest {
  CONSTRUCTOR_ID = 0x60ed4229;
  constructor(private url: string, private options: { delete?: boolean; openExternal?: boolean }) { super(); }
  getBytes() {
    const hasOpen = this.options.openExternal !== undefined;
    const flags = (hasOpen ? 1 : 0) | (this.options.delete ? 2 : 0);
    const boolConstructor = this.options.openExternal ? 0x997275b5 : 0xbc799737;
    return tlRequestBytes(
      this.CONSTRUCTOR_ID,
      tlInt(flags),
      ...(hasOpen ? [tlInt(boolConstructor)] : []),
      tlString(this.url),
    );
  }
}

function getWebBrowserSettings(client: any, hash: unknown) {
  return client.invoke(new GetWebBrowserSettingsRequest(hash));
}

function updateWebBrowserSettings(client: any, openExternal: boolean, displayClose: boolean) {
  return client.invoke(new UpdateWebBrowserSettingsRequest(openExternal, displayClose));
}

function toggleWebBrowserSettingsException(client: any, url: string, options: { delete?: boolean; openExternal?: boolean }) {
  return client.invoke(new ToggleWebBrowserSettingsExceptionRequest(url, options));
}

async function main() {
  const configuredAuthKey = process.env.CODEX_BROWSER_AUTH_B64;
  const sessions = configuredAuthKey
    ? [{
      authKeyHex: Buffer.from(configuredAuthKey, 'base64').toString('hex'),
      userId: process.env.CODEX_BROWSER_USER_ID || 'unknown',
      authKeyId: authKeyId(Buffer.from(configuredAuthKey, 'base64').toString('hex')),
    }]
    : loadStoredSessions();
  const requestedIndex = Number.parseInt(process.env.CODEX_BROWSER_SESSION_INDEX || '0', 10);
  const ordered = Number.isInteger(requestedIndex) && requestedIndex >= 0
    ? [sessions[requestedIndex], ...sessions.filter((_, index) => index !== requestedIndex)].filter(Boolean)
    : sessions;
  let lastError: unknown;
  for (const stored of ordered) {
    const client = makeClient(stored.authKeyHex);
    let succeeded = false;
    try {
      // The primary DC request path is the acceptance target. GramJS otherwise
      // opens an extra media sender during connect, which needs a separate
      // media auth key and can retry forever when auditing a single key.
      (client as any)._borrowExportedSender = async () => undefined;
      (client as any)._updateLoop = async () => undefined;
      console.log(JSON.stringify({ event: 'connect_start', userId: stored.userId, authKeyId: stored.authKeyId }));
      await client.connect();
      console.log(JSON.stringify({ event: 'connected', authKeyId: stored.authKeyId }));
      const config = await client.invoke(new Api.help.GetConfig());
      console.log(JSON.stringify({ event: 'config', type: className(config) }));
      const photos = await client.invoke(new Api.photos.GetUserPhotos({
        userId: new Api.InputUserSelf(),
        offset: 0,
        maxId: 0n,
        limit: 20,
      }));
      console.log(JSON.stringify({ event: 'photos', type: className(photos) }));
      const linksRequest = Api.account.GetBusinessChatLinks
        ? new Api.account.GetBusinessChatLinks({})
        : new GetBusinessChatLinksRequest();
      const links = await client.invoke(linksRequest);
      console.log(JSON.stringify({
        userId: stored.userId,
        authKeyId: stored.authKeyId,
        configType: className(config),
        photosType: className(photos),
        photosCount: Array.isArray(photos?.photos) ? photos.photos.length : null,
        linksType: className(links),
        linksCount: Array.isArray(links?.links) ? links.links.length : null,
      }));

      const browserSettings = await getWebBrowserSettings(client, 0n);
      if (!isClass(browserSettings, 'WebBrowserSettings')) {
        throw new Error(`unexpected browser settings type: ${className(browserSettings)}`);
      }
      const originalPersistedSettings = readPersistedBrowserSettings(stored.userId);
      const originalOpen = Boolean(browserSettings.openExternalBrowser);
      const originalClose = Boolean(browserSettings.displayCloseButton);
      if (originalOpen !== Boolean(originalPersistedSettings?.open_external_browser)
        || originalClose !== Boolean(originalPersistedSettings?.display_close_button)) {
        throw new Error('browser settings RPC differs from the persisted MySQL row');
      }
      const originalHash = browserSettings.hash;
      const unchanged = await getWebBrowserSettings(client, originalHash);
      if (!isClass(unchanged, 'WebBrowserSettingsNotModified')) {
        throw new Error(`browser settings hash was not honored: ${className(unchanged)}`);
      }
      const probeOpen = !originalOpen;
      const probeClose = !originalClose;
      const toggled = await updateWebBrowserSettings(client, probeOpen, probeClose);
      if (!isClass(toggled, 'WebBrowserSettings')) {
        throw new Error(`unexpected browser settings update type: ${className(toggled)}`);
      }
      if (Boolean(toggled.openExternalBrowser) !== probeOpen || Boolean(toggled.displayCloseButton) !== probeClose) {
        throw new Error('browser settings update did not persist the requested values');
      }
      const toggledDbState = readPersistedBrowserSettings(stored.userId);
      if (Boolean(toggledDbState?.open_external_browser) !== probeOpen
        || Boolean(toggledDbState?.display_close_button) !== probeClose) {
        throw new Error('browser settings update was not persisted to MySQL');
      }

      const probeUrl = `https://probe.invalid/teamgram-${Date.now()}-${process.pid}`;
      let exceptionMayExist = false;
      try {
        exceptionMayExist = true;
        const exceptionUpdate = await toggleWebBrowserSettingsException(client, probeUrl, { openExternal: true });
        if (!isClass(exceptionUpdate, 'Updates')) {
          throw new Error(`unexpected browser exception update type: ${className(exceptionUpdate)}`);
        }
        const afterAdd = await getWebBrowserSettings(client, 0n);
        const exceptions = [...(afterAdd?.externalExceptions || []), ...(afterAdd?.inappExceptions || [])];
        if (!exceptions.some((item: any) => item?.url === probeUrl)) {
          throw new Error('browser exception was not persisted');
        }
        const exceptionDbState = readPersistedBrowserSettings(stored.userId);
        const persistedExceptions = [...(exceptionDbState?.external || []), ...(exceptionDbState?.inapp || [])];
        if (!persistedExceptions.some((item: any) => item?.url === probeUrl)) {
          throw new Error('browser exception is missing from MySQL');
        }
      } finally {
        try {
          if (exceptionMayExist) {
            const exceptionDelete = await toggleWebBrowserSettingsException(client, probeUrl, { delete: true });
            if (!isClass(exceptionDelete, 'Updates')) {
              throw new Error(`unexpected browser exception delete type: ${className(exceptionDelete)}`);
            }
          }
        } finally {
          const restored = await updateWebBrowserSettings(client, originalOpen, originalClose);
          if (!isClass(restored, 'WebBrowserSettings')) {
            throw new Error(`unexpected browser settings restore type: ${className(restored)}`);
          }
          if (Boolean(restored.openExternalBrowser) !== originalOpen || Boolean(restored.displayCloseButton) !== originalClose) {
            throw new Error('browser settings did not restore the original values');
          }
        }
      }
      const finalSettings = await getWebBrowserSettings(client, 0n);
      const remainingExceptions = [...(finalSettings?.externalExceptions || []), ...(finalSettings?.inappExceptions || [])];
      if (remainingExceptions.some((item: any) => item?.url === probeUrl)) {
        throw new Error('temporary browser exception remained after cleanup');
      }
      const restoredDbState = readPersistedBrowserSettings(stored.userId);
      if (normalizeBrowserState(restoredDbState) !== normalizeBrowserState(originalPersistedSettings)) {
        throw new Error('browser settings MySQL row did not restore the original semantic state');
      }
      console.log(JSON.stringify({
        event: 'browser_settings',
        type: className(browserSettings),
        hashType: className(unchanged),
        exception: 'added-and-removed',
        restored: true,
      }));

      const langPack = await client.invoke(new Api.langpack.GetLangPack({ langPack: 'weba', langCode: 'en' }));
      const langPackLanguages = await client.invoke(new Api.langpack.GetLanguages({ langPack: 'weba' }));
      const langPackLanguage = await client.invoke(new Api.langpack.GetLanguage({ langPack: 'weba', langCode: 'en' }));
      const selectedStrings = await client.invoke(new Api.langpack.GetStrings({
        langPack: 'weba',
        langCode: 'en',
        keys: ['Month1', 'SettingsFoldersDesc', 'Month1'],
      }));
      const fullDifference = await client.invoke(new Api.langpack.GetDifference({
        langPack: 'weba',
        langCode: 'en',
        fromVersion: 0,
      }));
      const langPackNotModified = await client.invoke(new Api.langpack.GetDifference({
        langPack: 'weba',
        langCode: 'en',
        fromVersion: langPack.version,
      }));
      const languageCodes = Array.isArray(langPackLanguages) ? langPackLanguages.map((item: any) => item?.langCode) : [];
      const selectedByKey = new Map((Array.isArray(selectedStrings) ? selectedStrings : []).map((item: any) => [item?.key, item?.value]));
      if (!isClass(langPack, 'LangPackDifference') || langPack.langCode !== 'en' || langPack.strings.length < 1000) {
        throw new Error(`unexpected English langpack response: ${className(langPack)}`);
      }
      if (languageCodes.length !== 1 || languageCodes[0] !== 'en' || !isClass(langPackLanguage, 'LangPackLanguage')) {
        throw new Error('English langpack language catalogue mismatch');
      }
      if (selectedByKey.get('Month1') !== 'January' || selectedByKey.get('SettingsFoldersDesc') !== 'Sort chats into folders' || selectedStrings.length !== 2) {
        throw new Error('English langpack key lookup mismatch');
      }
      if (!isClass(fullDifference, 'LangPackDifference') || fullDifference.strings.length !== langPack.strings.length) {
        throw new Error('English langpack full difference did not return the current catalogue');
      }
      if (!isClass(langPackNotModified, 'LangPackDifference') || langPackNotModified.strings.length !== 0) {
        throw new Error('English langpack difference did not honor the current version');
      }
      console.log(JSON.stringify({
        event: 'langpack',
        packType: className(langPack),
        languageCount: languageCodes.length,
        languageType: className(langPackLanguage),
        stringCount: langPack.strings.length,
        lookupCount: selectedStrings.length,
        differenceStringCount: fullDifference.strings.length,
        notModifiedStringCount: langPackNotModified.strings.length,
        version: langPack.version,
      }));
      succeeded = true;
      return;
    } catch (error) {
      lastError = error;
      console.error(JSON.stringify({ event: 'connect_failed', authKeyId: stored.authKeyId, error: error?.errorMessage || error?.message || String(error) }));
    } finally {
      try { client.destroy(); } catch { /* best effort */ }
      if (succeeded) process.exit(0);
    }
  }
  throw lastError || new Error('all browser auth sessions failed');
}

void main().catch((error) => {
  console.error(error?.errorMessage || error?.message || String(error));
  process.exit(1);
});
