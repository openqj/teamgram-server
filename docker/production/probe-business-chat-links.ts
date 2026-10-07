import path from 'node:path';
import { createRequire } from 'node:module';

type Client = any;

const required = ['TEAMGRAM_GRAMJS_DIR', 'A_AUTH_B64', 'B_AUTH_B64', 'A_USER_ID', 'B_USER_ID'];
for (const key of required) {
  if (!process.env[key]) throw new Error(`${key} is required`);
}

const gramjsDir = process.env.TEAMGRAM_GRAMJS_DIR!;
const dcId = Number(process.env.TG_DC_ID || '2');
const apiId = Number(process.env.TG_API_ID || '1');
const apiHash = process.env.TG_API_HASH || '0123456789abcdef0123456789abcdef';
const aUserId = BigInt(process.env.A_USER_ID!);
const bUserId = BigInt(process.env.B_USER_ID!);

const globalAny = globalThis as typeof globalThis & { self?: any; addEventListener?: () => void };
globalAny.self ??= globalThis;
globalAny.addEventListener ??= () => {};
globalAny.self.addEventListener ??= globalAny.addEventListener;

const requireModule = createRequire(import.meta.url);
const TelegramClient = requireModule(path.join(gramjsDir, 'src/lib/gramjs/client/TelegramClient.ts')).default;
const CallbackSession = requireModule(path.join(gramjsDir, 'src/lib/gramjs/sessions/CallbackSession.ts')).default;
const Api = requireModule(path.join(gramjsDir, 'src/lib/gramjs/tl/index.ts')).Api;
const ConnectionTCPObfuscated = requireModule(path.join(gramjsDir, 'src/lib/gramjs/network/connection/TCPObfuscated.ts')).ConnectionTCPObfuscated;
const quietLogger = { debug() {}, info() {}, warn() {}, error() {} };

function typeName(value: any): string {
  return value?.className || value?.constructor?.name || typeof value;
}

function requireType(value: any, name: string): void {
  if (typeName(value) !== name) throw new Error(`expected ${name}, got ${typeName(value)}`);
}

function tlInt(value: number): Uint8Array {
  const bytes = new Uint8Array(4);
  new DataView(bytes.buffer).setInt32(0, value, true);
  return bytes;
}

function tlString(value: string): Uint8Array {
  const payload = Buffer.from(value, 'utf8');
  const headerLength = payload.length < 254 ? 1 : 4;
  const bytes = new Uint8Array(Math.ceil((headerLength + payload.length) / 4) * 4);
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

abstract class RawBusinessChatLinkRequest {
  SUBCLASS_OF_ID = 0;
  classType = 'request';
  abstract CONSTRUCTOR_ID: number;
  abstract getBytes(): Uint8Array;
  readResult(reader: any) { return reader.tgReadObject(); }
}

class GetBusinessChatLinksRequest extends RawBusinessChatLinkRequest {
  CONSTRUCTOR_ID = 0x6f70dde1;
  getBytes() { return tlRequestBytes(this.CONSTRUCTOR_ID); }
}

class CreateBusinessChatLinkRequest extends RawBusinessChatLinkRequest {
  CONSTRUCTOR_ID = 0x8851e68e;
  constructor(private message: string, private title: string) { super(); }
  getBytes() {
    const flags = this.title ? 2 : 0;
    const input = tlRequestBytes(0x11679fa7, tlInt(flags), tlString(this.message), ...(this.title ? [tlString(this.title)] : []));
    return tlRequestBytes(this.CONSTRUCTOR_ID, input);
  }
}

class EditBusinessChatLinkRequest extends RawBusinessChatLinkRequest {
  CONSTRUCTOR_ID = 0x8c3410af;
  constructor(private slug: string, private message: string, private title: string) { super(); }
  getBytes() {
    const flags = this.title ? 2 : 0;
    const input = tlRequestBytes(0x11679fa7, tlInt(flags), tlString(this.message), ...(this.title ? [tlString(this.title)] : []));
    return tlRequestBytes(this.CONSTRUCTOR_ID, tlString(this.slug), input);
  }
}

class DeleteBusinessChatLinkRequest extends RawBusinessChatLinkRequest {
  CONSTRUCTOR_ID = 0x60073674;
  constructor(private slug: string) { super(); }
  getBytes() { return tlRequestBytes(this.CONSTRUCTOR_ID, tlString(this.slug)); }
}

function makeClient(authKeyBase64: string): Client {
  const keyHex = Buffer.from(authKeyBase64, 'base64').toString('hex');
  const session = new CallbackSession({ mainDcId: dcId, keys: { [dcId]: keyHex } }, () => {});
  const load = session.load.bind(session);
  session.load = async () => {
    await load();
    (session as any)._serverAddress = process.env.TG_DC_HOST || '127.0.0.1';
    (session as any)._port = Number(process.env.TG_DC_PORT || '11443');
  };
  const client = new TelegramClient(session, apiId, apiHash, {
    connection: ConnectionTCPObfuscated,
    fallbackConnection: ConnectionTCPObfuscated,
    shouldAllowHttpTransport: false,
    shouldForceHttpTransport: false,
    timeout: 20,
    requestRetries: 2,
    connectionRetries: 2,
    connectionRetriesToFallback: 0,
    retryDelay: 250,
    autoReconnect: false,
    baseLogger: quietLogger,
  });
  (client as any)._borrowExportedSender = async () => undefined;
  (client as any)._updateLoop = async () => undefined;
  return client;
}

async function main() {
  const a = makeClient(process.env.A_AUTH_B64!);
  const b = makeClient(process.env.B_AUTH_B64!);
  const tag = `codex-link-${Date.now()}-${process.pid}`;
  const message = `temporary ${tag}`;
  const editedMessage = `edited ${tag}`;
  let slug = '';
  let initialCount = 0;
  let operationError: unknown;
  const cleanupErrors: string[] = [];

  try {
    await a.connect();
    await b.connect();
    const [meA, meB] = await Promise.all([a.getMe(), b.getMe()]);
    if (String(meA?.id) !== String(aUserId) || String(meB?.id) !== String(bUserId)) {
      throw new Error('the authenticated accounts do not match the configured user IDs');
    }

    const before = await a.invoke(new GetBusinessChatLinksRequest());
    requireType(before, 'account.BusinessChatLinks');
    initialCount = Array.isArray(before.links) ? before.links.length : 0;

    const created = await a.invoke(new CreateBusinessChatLinkRequest(message, `Probe ${tag}`));
    requireType(created, 'BusinessChatLink');
    if (typeof created.link !== 'string' || !/^[a-f0-9]{48}$/.test(created.link)) {
      throw new Error('created business link slug is not an opaque URL-safe token');
    }
    slug = created.link;

    let listed = await a.invoke(new GetBusinessChatLinksRequest());
    requireType(listed, 'account.BusinessChatLinks');
    if (!Array.isArray(listed.links) || !listed.links.some((item: any) => item.link === slug && item.message === message)) {
      throw new Error('owner listing did not read the created link');
    }

    let resolved = await b.invoke(new Api.account.ResolveBusinessChatLink({ slug }));
    requireType(resolved, 'account.ResolvedBusinessChatLinks');
    if (String(resolved.peer?.userId) !== String(aUserId) || resolved.message !== message) {
      throw new Error('second user did not resolve the owner and message');
    }

    const edited = await a.invoke(new EditBusinessChatLinkRequest(slug, editedMessage, `Edited ${tag}`));
    requireType(edited, 'BusinessChatLink');
    if (edited.link !== slug || edited.message !== editedMessage) throw new Error('edited link did not preserve its slug and new message');

    resolved = await b.invoke(new Api.account.ResolveBusinessChatLink({ slug }));
    requireType(resolved, 'account.ResolvedBusinessChatLinks');
    if (resolved.message !== editedMessage) throw new Error('second user did not read the edited link');

    const deleted = await a.invoke(new DeleteBusinessChatLinkRequest(slug));
    if (deleted !== true && typeName(deleted).toLowerCase() !== 'booltrue') {
      throw new Error(`delete returned ${typeName(deleted)}`);
    }
    slug = '';

    try {
      await b.invoke(new Api.account.ResolveBusinessChatLink({ slug: created.link }));
      throw new Error('deleted slug unexpectedly resolved');
    } catch (error) {
      if (!String(error).includes('INPUT_REQUEST_INVALID')) throw error;
    }

    listed = await a.invoke(new GetBusinessChatLinksRequest());
    requireType(listed, 'account.BusinessChatLinks');
    if (!Array.isArray(listed.links) || listed.links.length !== initialCount) {
      throw new Error('owner listing count did not return to its initial value');
    }
  } catch (error) {
    operationError = error;
  }

  try {
    const beforeCleanup = await a.invoke(new GetBusinessChatLinksRequest());
    const cleanupSlugs = new Set<string>(slug ? [slug] : []);
    for (const item of Array.isArray(beforeCleanup.links) ? beforeCleanup.links : []) {
      if (item.message === message || item.message === editedMessage) cleanupSlugs.add(item.link);
    }
    for (const cleanupSlug of cleanupSlugs) {
      await a.invoke(new DeleteBusinessChatLinkRequest(cleanupSlug));
    }
    const afterCleanup = await a.invoke(new GetBusinessChatLinksRequest());
    if (Array.isArray(afterCleanup.links) && afterCleanup.links.some((item: any) => item.message === message || item.message === editedMessage)) {
      cleanupErrors.push('tagged temporary link remains in the owner listing');
    }
  } catch (error) {
    cleanupErrors.push(`could not verify business link cleanup (${typeName(error)})`);
  }

  // GramJS' disconnect waits for a transport close callback that is not
  // delivered by the production gateway after raw Layer 229 requests. The
  // existing production probes use destroy(), which closes sockets without
  // waiting and lets the process terminate deterministically.
  try { a.destroy(); } catch { /* best effort */ }
  try { b.destroy(); } catch { /* best effort */ }
  if (operationError && cleanupErrors.length) {
    throw new Error(`operation failed (${typeName(operationError)}); ${cleanupErrors.join('; ')}`);
  }
  if (operationError) throw operationError;
  if (cleanupErrors.length) throw new Error(cleanupErrors.join('; '));
  console.log(JSON.stringify({
    tag,
    users: 2,
    created: 'BusinessChatLink',
    ownerRead: 'account.BusinessChatLinks',
    crossUserResolve: 'account.ResolvedBusinessChatLinks',
    edit: 'BusinessChatLink',
    delete: 'boolTrue',
    cleanup: 'verified',
  }));
  // GramJS may retain an internal MTProto timer after destroy(). The RPCs and
  // cleanup above are complete, so make the probe's process result explicit.
  process.exit(0);
}

main().catch((error) => {
  console.error(error);
  process.exit(1);
});
