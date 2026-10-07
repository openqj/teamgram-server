import path from 'node:path';
import { createRequire } from 'node:module';

for (const key of ['TEAMGRAM_GRAMJS_DIR', 'A_AUTH_B64', 'A_USER_ID']) {
  if (!process.env[key]) throw new Error(`${key} is required`);
}

const root = process.env.TEAMGRAM_GRAMJS_DIR!;
const dcId = Number(process.env.TG_DC_ID || '2');
const requireModule = createRequire(import.meta.url);
const TelegramClient = requireModule(path.join(root, 'src/lib/gramjs/client/TelegramClient.ts')).default;
const CallbackSession = requireModule(path.join(root, 'src/lib/gramjs/sessions/CallbackSession.ts')).default;
const Connection = requireModule(path.join(root, 'src/lib/gramjs/network/connection/TCPObfuscated.ts')).ConnectionTCPObfuscated;
const quietLogger = { debug() {}, info() {}, warn() {}, error() {} };
const globalAny = globalThis as typeof globalThis & { self?: any; addEventListener?: () => void };
globalAny.self ??= globalThis;
globalAny.addEventListener ??= () => {};
globalAny.self.addEventListener ??= globalAny.addEventListener;

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
    (session as any)._serverAddress = process.env.TG_DC_HOST || '127.0.0.1';
    (session as any)._port = Number(process.env.TG_DC_PORT || '11443');
  };
  return new TelegramClient(session, 1, '0123456789abcdef0123456789abcdef', {
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

function tlBytes(value: Uint8Array): Uint8Array {
  const header = value.length < 254
    ? Uint8Array.from([value.length])
    : Uint8Array.from([254, value.length & 0xff, (value.length >>> 8) & 0xff, (value.length >>> 16) & 0xff]);
  const out = new Uint8Array((header.length + value.length + 3) & ~3);
  out.set(header);
  out.set(value, header.length);
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

function inputDocument(id: bigint, accessHash: bigint): Uint8Array {
  return concat(int32(0x1abfb575), int64(id), int64(accessHash), tlBytes(new Uint8Array()));
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

function getSavedRingtonesRequest(hash: bigint): any {
  return rawRequest('account.GetSavedRingtones', 0xe1902288, concat(
    int32(0xe1902288),
    int64(hash),
  ));
}

function saveRingtoneRequest(id: bigint, accessHash: bigint, unsave: boolean): any {
  return rawRequest('account.SaveRingtone', 0x3dea5b03, concat(
    int32(0x3dea5b03),
    inputDocument(id, accessHash),
    int32(unsave ? 0x997275b5 : 0xbc799737),
  ));
}

function docs(result: any): any[] {
  return Array.isArray(result?.ringtones) ? result.ringtones : [];
}

async function main() {
  const client = makeClient();
  const id = process.env.DOCUMENT_ID
    ? BigInt(process.env.DOCUMENT_ID)
    : BigInt(Date.now()) * 1000n + BigInt(process.pid % 1000);
  const accessHash = process.env.DOCUMENT_ACCESS_HASH
    ? BigInt(process.env.DOCUMENT_ACCESS_HASH)
    : id ^ 0x5a17n;
  const documentSource = process.env.DOCUMENT_ID ? 'existing_documents_row' : 'unique_probe_reference';
  let saved = false;
  try {
    (client as any)._borrowExportedSender = async () => undefined;
    (client as any)._updateLoop = async () => undefined;
    await client.connect();
    const me = await client.getMe();
    if (String(me?.id) !== String(process.env.A_USER_ID)) throw new Error('auth key user mismatch');

    const before = await client.invoke(getSavedRingtonesRequest(0n));
    if (!isType(before, 'SavedRingtones')) throw new Error(`unexpected initial result: ${rpcName(before)}`);
    if (docs(before).some((doc: any) => String(doc?.id) === id.toString())) {
      throw new Error('unique ringtone id unexpectedly existed before probe');
    }

    const save = await client.invoke(saveRingtoneRequest(id, accessHash, false));
    if (!isType(save, 'SavedRingtone')) throw new Error(`account.saveRingtone returned ${rpcName(save)}`);
    saved = true;

    const read = await client.invoke(getSavedRingtonesRequest(0n));
    const found = docs(read).find((doc: any) => String(doc?.id) === id.toString());
    if (!isType(read, 'SavedRingtones') || !found || String(found.accessHash) !== accessHash.toString()) {
      throw new Error(`saved ringtone readback mismatch: ${rpcName(read)} ${JSON.stringify(found)}`);
    }
    const notModified = await client.invoke(getSavedRingtonesRequest(BigInt(read.hash)));
    if (!isType(notModified, 'SavedRingtonesNotModified')) {
      throw new Error(`saved ringtone hash was not honored: ${rpcName(notModified)}`);
    }

    const unsave = await client.invoke(saveRingtoneRequest(id, accessHash, true));
    if (!isTrue(unsave) && !isType(unsave, 'SavedRingtone')) {
      throw new Error(`account.saveRingtone unsave returned ${rpcName(unsave)}`);
    }
    saved = false;
    const after = await client.invoke(getSavedRingtonesRequest(0n));
    if (docs(after).some((doc: any) => String(doc?.id) === id.toString())) {
      throw new Error('saved ringtone remained after unsave');
    }
    console.log(JSON.stringify({
      userId: String(me.id),
      ringtoneId: id.toString(),
      documentSource,
      beforeType: rpcName(before),
      saveType: rpcName(save),
      readType: rpcName(read),
      readDocumentCount: docs(read).length,
      hashProbeType: rpcName(notModified),
      unsaveType: rpcName(unsave),
      afterType: rpcName(after),
      afterContainsProbe: false,
      writes: 1,
    }));
  } finally {
    if (saved) {
      try { await client.invoke(saveRingtoneRequest(id, accessHash, true)); } catch {}
    }
    client.destroy();
  }
}

main().then(() => process.exit(0)).catch((error) => {
  console.error(`saved ringtone roundtrip failed: ${error instanceof Error ? error.stack || error.message : String(error)}`);
  process.exit(1);
});
