import path from 'node:path';
import { createRequire } from 'node:module';

for (const key of ['TEAMGRAM_GRAMJS_DIR', 'A_AUTH_B64', 'A_USER_ID']) {
  if (!process.env[key]) throw new Error(`${key} is required`);
}

const root = process.env.TEAMGRAM_GRAMJS_DIR!;
const dcId = Number(process.env.TG_DC_ID || '2');
const dcHost = process.env.TG_DC_HOST || '127.0.0.1';
const dcPort = Number(process.env.TG_DC_PORT || '11443');
const userId = String(process.env.A_USER_ID);

const globalAny = globalThis as typeof globalThis & { self?: any; addEventListener?: () => void };
globalAny.self ??= globalThis;
globalAny.addEventListener ??= () => {};
globalAny.self.addEventListener ??= globalAny.addEventListener;

const requireModule = createRequire(import.meta.url);
const TelegramClient = requireModule(path.join(root, 'src/lib/gramjs/client/TelegramClient.ts')).default;
const CallbackSession = requireModule(path.join(root, 'src/lib/gramjs/sessions/CallbackSession.ts')).default;
const Api = requireModule(path.join(root, 'src/lib/gramjs/tl/index.ts')).Api;
const Connection = requireModule(path.join(root, 'src/lib/gramjs/network/connection/TCPObfuscated.ts')).ConnectionTCPObfuscated;
const quietLogger = { debug() {}, info() {}, warn() {}, error() {} };

class RawRequest {
  SUBCLASS_OF_ID = 0;
  classType = 'request';

  constructor(
    public readonly CONSTRUCTOR_ID: number,
    public readonly className: string,
    private readonly payload: Uint8Array,
  ) {}

  getBytes(): Uint8Array {
    return concat(int32(this.CONSTRUCTOR_ID), this.payload);
  }

  readResult(reader: any): any {
    return reader.tgReadObject();
  }
}

function int32(value: number): Uint8Array {
  const out = new Uint8Array(4);
  new DataView(out.buffer).setInt32(0, value, true);
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

function tlString(value: string): Uint8Array {
  const bytes = new TextEncoder().encode(value);
  if (bytes.length >= 254) throw new Error('temporary shortcut is unexpectedly long');
  return concat(Uint8Array.from([bytes.length]), bytes, new Uint8Array((3 - bytes.length) & 3));
}

function checkQuickReplyShortcut(shortcut: string): RawRequest {
  return new RawRequest(0xf1d0fbd3, 'messages.checkQuickReplyShortcut', tlString(shortcut));
}

function editQuickReplyShortcut(shortcutId: number, shortcut: string): RawRequest {
  return new RawRequest(0x5c003cef, 'messages.editQuickReplyShortcut', concat(int32(shortcutId), tlString(shortcut)));
}

function deleteQuickReplyShortcut(shortcutId: number): RawRequest {
  return new RawRequest(0x3cc04740, 'messages.deleteQuickReplyShortcut', int32(shortcutId));
}

function reorderQuickReplies(order: number[]): RawRequest {
  return new RawRequest(0x60331907, 'messages.reorderQuickReplies', concat(
    int32(0x1cb5c415),
    int32(order.length),
    ...order.map(int32),
  ));
}

function rpcName(value: any): string {
  return value?.className || value?.constructor?.name || typeof value;
}

function isType(value: any, name: string): boolean {
  const actual = rpcName(value);
  return actual === name || actual.endsWith(`.${name}`);
}

function isTrue(value: any): boolean {
  if (value === true) return true;
  if (value instanceof Boolean) return value.valueOf();
  return rpcName(value).toLowerCase().includes('true');
}

function makeClient(): any {
  const session = new CallbackSession({
    mainDcId: dcId,
    keys: { [dcId]: Buffer.from(process.env.A_AUTH_B64!, 'base64').toString('hex') },
  }, () => {});
  const load = session.load.bind(session);
  session.load = async () => {
    await load();
    (session as any)._serverAddress = dcHost;
    (session as any)._port = dcPort;
  };
  return new TelegramClient(session, Number(process.env.TG_API_ID || '1'), process.env.TG_API_HASH || '0123456789abcdef0123456789abcdef', {
    connection: Connection,
    fallbackConnection: Connection,
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
}

function quickReplies(result: any): any[] {
  return Array.isArray(result?.quickReplies) ? result.quickReplies : [];
}

function shortcutIds(result: any): number[] {
  return quickReplies(result).map((item) => Number(item?.shortcutId));
}

function containsShortcut(result: any, id: number, shortcut: string): boolean {
  return quickReplies(result).some((item) => Number(item?.shortcutId) === id && item?.shortcut === shortcut);
}

async function main() {
  const client = makeClient();
  const tag = `codex-quick-reply-${Date.now()}-${process.pid}`;
  const shortcutA = `${tag}-a`;
  const shortcutB = `${tag}-b`;
  const shortcutAId = 2_000_000_000 + (process.pid % 100_000);
  const shortcutBId = shortcutAId + 1;
  let originalOrder: number[] = [];
  let touched = false;
  let operationError: unknown;
  const cleanupErrors: string[] = [];

  try {
    await client.connect();
    const me = await client.getMe();
    if (String(me?.id) !== userId) throw new Error('auth key user mismatch');

    const before = await client.invoke(new Api.messages.GetQuickReplies({ hash: 0n }));
    if (!isType(before, 'QuickReplies')) throw new Error(`messages.getQuickReplies returned ${rpcName(before)}`);
    originalOrder = shortcutIds(before);
    if (originalOrder.includes(shortcutAId) || originalOrder.includes(shortcutBId)) {
      throw new Error('temporary shortcut IDs unexpectedly existed before probe');
    }

    const availableA = await client.invoke(checkQuickReplyShortcut(shortcutA));
    if (!isTrue(availableA)) throw new Error(`new shortcut A was not available: ${rpcName(availableA)}`);
    const availableB = await client.invoke(checkQuickReplyShortcut(shortcutB));
    if (!isTrue(availableB)) throw new Error(`new shortcut B was not available: ${rpcName(availableB)}`);

    const createdA = await client.invoke(editQuickReplyShortcut(shortcutAId, shortcutA));
    if (!isTrue(createdA)) throw new Error(`edit shortcut A returned ${rpcName(createdA)}`);
    touched = true;
    const createdB = await client.invoke(editQuickReplyShortcut(shortcutBId, shortcutB));
    if (!isTrue(createdB)) throw new Error(`edit shortcut B returned ${rpcName(createdB)}`);

    const occupied = await client.invoke(checkQuickReplyShortcut(shortcutA));
    if (isTrue(occupied)) throw new Error('created shortcut was reported as available');
    const created = await client.invoke(new Api.messages.GetQuickReplies({ hash: 0n }));
    if (!isType(created, 'QuickReplies') || !containsShortcut(created, shortcutAId, shortcutA) || !containsShortcut(created, shortcutBId, shortcutB)) {
      throw new Error(`created shortcuts were not readable: ${rpcName(created)}`);
    }

    const reorderedIds = [shortcutBId, shortcutAId, ...originalOrder];
    const reordered = await client.invoke(reorderQuickReplies(reorderedIds));
    if (!isTrue(reordered)) throw new Error(`reorder returned ${rpcName(reordered)}`);
    const afterReorder = await client.invoke(new Api.messages.GetQuickReplies({ hash: 0n }));
    const afterOrder = shortcutIds(afterReorder);
    if (!isType(afterReorder, 'QuickReplies') || afterOrder.length !== reorderedIds.length || afterOrder.some((id, index) => id !== reorderedIds[index])) {
      throw new Error(`reorder readback mismatch: ${JSON.stringify(afterOrder)}`);
    }

    const deletedA = await client.invoke(deleteQuickReplyShortcut(shortcutAId));
    if (!isTrue(deletedA)) throw new Error(`delete shortcut A returned ${rpcName(deletedA)}`);
    const deletedB = await client.invoke(deleteQuickReplyShortcut(shortcutBId));
    if (!isTrue(deletedB)) throw new Error(`delete shortcut B returned ${rpcName(deletedB)}`);
    touched = false;

    const afterDelete = await client.invoke(new Api.messages.GetQuickReplies({ hash: 0n }));
    const finalOrder = shortcutIds(afterDelete);
    if (!isType(afterDelete, 'QuickReplies') || finalOrder.some((id) => id === shortcutAId || id === shortcutBId)) {
      throw new Error(`temporary shortcuts remained after delete: ${JSON.stringify(finalOrder)}`);
    }
    if (finalOrder.some((id, index) => id !== originalOrder[index]) || finalOrder.length !== originalOrder.length) {
      throw new Error(`original order was not restored: ${JSON.stringify(finalOrder)}`);
    }

    console.log(JSON.stringify({
      userId,
      before: originalOrder.length,
      checkAvailable: true,
      checkOccupied: false,
      reordered: true,
      deleted: true,
      cleanup: 'verified',
    }));
  } catch (error) {
    operationError = error;
  }

  try {
    if (touched) {
      await client.invoke(deleteQuickReplyShortcut(shortcutAId));
      await client.invoke(deleteQuickReplyShortcut(shortcutBId));
    }
    if (originalOrder.length > 0) {
      await client.invoke(reorderQuickReplies(originalOrder));
    }
    const afterCleanup = await client.invoke(new Api.messages.GetQuickReplies({ hash: 0n }));
    const remaining = shortcutIds(afterCleanup);
    if (remaining.includes(shortcutAId) || remaining.includes(shortcutBId)) cleanupErrors.push('temporary shortcuts remain after cleanup');
    if (originalOrder.length > 0 && (remaining.length !== originalOrder.length || remaining.some((id, index) => id !== originalOrder[index]))) {
      cleanupErrors.push(`original order was not restored after cleanup: ${JSON.stringify(remaining)}`);
    }
  } catch (error) {
    cleanupErrors.push(`cleanup failed (${rpcName(error)})`);
  }

  client.destroy();
  if (operationError && cleanupErrors.length) throw new Error(`operation failed (${operationError}); ${cleanupErrors.join('; ')}`);
  if (operationError) throw operationError;
  if (cleanupErrors.length) throw new Error(cleanupErrors.join('; '));
}

main().then(() => process.exit(0)).catch((error) => {
  console.error(`quick replies probe failed: ${error instanceof Error ? error.stack || error.message : String(error)}`);
  process.exit(1);
});
