import path from 'node:path';
import { createHash, randomBytes } from 'node:crypto';
import { createRequire } from 'node:module';

type Client = any;

for (const key of ['TEAMGRAM_GRAMJS_DIR', 'A_AUTH_B64', 'B_AUTH_B64', 'B2_AUTH_B64', 'A_USER_ID', 'B_USER_ID', 'B_ACCESS_HASH']) {
  if (!process.env[key]) throw new Error(`${key} is required`);
}

const root = process.env.TEAMGRAM_GRAMJS_DIR!;
const dcId = Number(process.env.TG_DC_ID || '2');
const apiId = Number(process.env.TG_API_ID || '1');
const apiHash = process.env.TG_API_HASH || '0123456789abcdef0123456789abcdef';
const aID = BigInt(process.env.A_USER_ID!);
const bID = BigInt(process.env.B_USER_ID!);
const bHash = BigInt(process.env.B_ACCESS_HASH!);

const globalAny = globalThis as typeof globalThis & { self?: any; addEventListener?: () => void };
globalAny.self ??= globalThis;
globalAny.addEventListener ??= () => {};
globalAny.self.addEventListener ??= globalAny.addEventListener;

const requireModule = createRequire(import.meta.url);
const TelegramClient = requireModule(path.join(root, 'src/lib/gramjs/client/TelegramClient.ts')).default;
const CallbackSession = requireModule(path.join(root, 'src/lib/gramjs/sessions/CallbackSession.ts')).default;
const Api = requireModule(path.join(root, 'src/lib/gramjs/tl/index.ts')).Api;
const Connection = requireModule(path.join(root, 'src/lib/gramjs/network/connection/TCPObfuscated.ts')).ConnectionTCPObfuscated;
const IGE = requireModule(path.join(root, 'src/lib/gramjs/crypto/IGE.ts')).IGE;
const quietLogger = { debug() {}, info() {}, warn() {}, error() {} };
const decryptedMessageLayerConstructor = 0x1be31789 | 0;
const decryptedMessageConstructor = 0x91cc4674 | 0;

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

function tlBytes(value: Uint8Array): Uint8Array {
  const length = value.length;
  const header = length < 254
    ? Uint8Array.from([length])
    : Uint8Array.from([254, length & 0xff, (length >>> 8) & 0xff, (length >>> 16) & 0xff]);
  const total = header.length + length;
  const padded = new Uint8Array((total + 3) & ~3);
  padded.set(header);
  padded.set(value, header.length);
  return padded;
}

function concat(...parts: Uint8Array[]): Uint8Array {
  const out = new Uint8Array(parts.reduce((sum, part) => sum + part.length, 0));
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

function requestEncryption(user: any, randomId: number, ga: Uint8Array): any {
  return rawRequest('messages.RequestEncryption', 0xf64daf43, concat(
    int32(0xf64daf43), user.getBytes(), int32(randomId), tlBytes(ga),
  ));
}

function acceptEncryption(peer: any, gb: Uint8Array, fingerprint: bigint): any {
  return rawRequest('messages.AcceptEncryption', 0x3dbc0415, concat(
    int32(0x3dbc0415), peer.getBytes(), tlBytes(gb), int64(fingerprint),
  ));
}

function discardEncryption(chatId: number, deleteHistory = true): any {
  return rawRequest('messages.DiscardEncryption', 0xf393aea0, concat(
    int32(0xf393aea0), int32(deleteHistory ? 1 : 0), int32(chatId),
  ));
}

function readEncryptedHistory(peer: any, maxDate: number): any {
  return rawRequest('messages.ReadEncryptedHistory', 0x7f4b690a, concat(
    int32(0x7f4b690a), peer.getBytes(), int32(maxDate),
  ));
}

function sendEncrypted(peer: any, randomId: bigint, data: Uint8Array): any {
  return rawRequest('messages.SendEncrypted', 0x44fa7a15, concat(
    int32(0x44fa7a15), int32(0), peer.getBytes(), int64(randomId), tlBytes(data),
  ));
}

function bigIntFromBytes(value: Uint8Array): bigint {
  const hex = Buffer.from(value).toString('hex');
  return hex ? BigInt(`0x${hex}`) : 0n;
}

function bytesFromBigInt(value: bigint, size = 0): Uint8Array {
  const hex = value.toString(16).padStart(size * 2, '0');
  const normalized = hex.length % 2 === 0 ? hex : `0${hex}`;
  return Uint8Array.from(Buffer.from(normalized, 'hex'));
}

function modPow(base: bigint, exponent: bigint, modulus: bigint): bigint {
  let result = 1n;
  let current = base % modulus;
  let remaining = exponent;
  while (remaining > 0n) {
    if ((remaining & 1n) === 1n) result = (result * current) % modulus;
    current = (current * current) % modulus;
    remaining >>= 1n;
  }
  return result;
}

function fingerprint(shared: Uint8Array): bigint {
  const digest = createHash('sha1').update(shared).digest().subarray(12, 20);
  return new DataView(digest.buffer, digest.byteOffset, digest.byteLength).getBigInt64(0, true);
}

function sha256(...parts: Uint8Array[]): Uint8Array {
  const hash = createHash('sha256');
  for (const part of parts) hash.update(part);
  return Uint8Array.from(hash.digest());
}

function mtproto2KeyAndIv(authKey: Uint8Array, messageKey: Uint8Array, x: 0 | 8): { key: Uint8Array; iv: Uint8Array } {
  const sha256A = sha256(messageKey, authKey.slice(x, x + 36));
  const sha256B = sha256(authKey.slice(40 + x, 76 + x), messageKey);
  return {
    key: concat(sha256A.slice(0, 8), sha256B.slice(8, 24), sha256A.slice(24, 32)),
    iv: concat(sha256B.slice(0, 8), sha256A.slice(8, 24), sha256B.slice(24, 32)),
  };
}

function secretMessageLayer(randomId: bigint, text: string, inSeqNo: number, outSeqNo: number): Uint8Array {
  return concat(
    int32(decryptedMessageLayerConstructor),
    tlBytes(randomBytes(31)),
    int32(144),
    int32(inSeqNo),
    int32(outSeqNo),
    int32(decryptedMessageConstructor),
    int32(0),
    int64(randomId),
    int32(0),
    tlBytes(Uint8Array.from(Buffer.from(text, 'utf8'))),
  );
}

function encryptSecretMessage(authKey: Uint8Array, creator: boolean, inner: Uint8Array): Uint8Array {
  const dataLength = 4 + inner.length;
  let paddingLength = (16 - (dataLength % 16)) % 16;
  if (paddingLength < 12) paddingLength += 16;
  const plaintext = concat(int32(inner.length), inner, randomBytes(paddingLength));
  const x: 0 | 8 = creator ? 0 : 8;
  const messageKeyLarge = sha256(authKey.slice(88 + x, 120 + x), plaintext);
  const messageKey = messageKeyLarge.slice(8, 24);
  const { key, iv } = mtproto2KeyAndIv(authKey, messageKey, x);
  const encrypted = new IGE(key, iv).encryptIge(plaintext);
  return concat(int64(fingerprint(authKey)), messageKey, encrypted);
}

function readInt32(data: Uint8Array, offset: number): { value: number; next: number } {
  if (offset + 4 > data.length) throw new Error('secret payload truncated int32');
  return { value: new DataView(data.buffer, data.byteOffset + offset, 4).getInt32(0, true), next: offset + 4 };
}

function readInt64(data: Uint8Array, offset: number): { value: bigint; next: number } {
  if (offset + 8 > data.length) throw new Error('secret payload truncated int64');
  return { value: new DataView(data.buffer, data.byteOffset + offset, 8).getBigInt64(0, true), next: offset + 8 };
}

function readTLBytes(data: Uint8Array, offset: number): { value: Uint8Array; next: number } {
  if (offset >= data.length) throw new Error('secret payload truncated bytes length');
  const first = data[offset];
  const headerLength = first === 254 ? 4 : 1;
  if (offset + headerLength > data.length) throw new Error('secret payload truncated bytes header');
  const length = first === 254
    ? data[offset + 1] | (data[offset + 2] << 8) | (data[offset + 3] << 16)
    : first;
  const valueStart = offset + headerLength;
  const valueEnd = valueStart + length;
  const paddedEnd = (valueEnd + 3) & ~3;
  if (paddedEnd > data.length) throw new Error('secret payload truncated bytes value');
  return { value: data.slice(valueStart, valueEnd), next: paddedEnd };
}

function decryptSecretMessage(authKey: Uint8Array, creator: boolean, packet: Uint8Array): { text: string; randomId: bigint; layer: number } {
  if (packet.length < 24 || packet.length % 16 !== 8) throw new Error(`invalid secret packet length ${packet.length}`);
  const expectedFingerprint = fingerprint(authKey);
  const packetFingerprint = new DataView(packet.buffer, packet.byteOffset, 8).getBigInt64(0, true);
  if (packetFingerprint !== expectedFingerprint) throw new Error('secret packet fingerprint mismatch');
  const messageKey = packet.slice(8, 24);
  const encrypted = packet.slice(24);
  const x: 0 | 8 = creator ? 0 : 8;
  const { key, iv } = mtproto2KeyAndIv(authKey, messageKey, x);
  const plaintext = Uint8Array.from(new IGE(key, iv).decryptIge(encrypted));
  const expectedMessageKey = sha256(authKey.slice(88 + x, 120 + x), plaintext).slice(8, 24);
  if (Buffer.compare(Buffer.from(messageKey), Buffer.from(expectedMessageKey)) !== 0) {
    throw new Error('secret packet message key mismatch');
  }
  const length = readInt32(plaintext, 0).value;
  if (length <= 0 || length > plaintext.length - 4) throw new Error('invalid secret payload data length');
  const data = plaintext.slice(4, 4 + length);
  let offset = 0;
  const constructor = readInt32(data, offset);
  offset = constructor.next;
  if (constructor.value !== decryptedMessageLayerConstructor) throw new Error(`unexpected secret layer constructor ${constructor.value}`);
  offset = readTLBytes(data, offset).next;
  const layer = readInt32(data, offset);
  offset = layer.next;
  offset = readInt32(data, offset).next;
  offset = readInt32(data, offset).next;
  const messageConstructor = readInt32(data, offset);
  offset = messageConstructor.next;
  if (messageConstructor.value !== decryptedMessageConstructor) {
    throw new Error(`unexpected secret message constructor ${messageConstructor.value}`);
  }
  const flags = readInt32(data, offset);
  offset = flags.next;
  if (flags.value !== 0) throw new Error(`unsupported secret message flags ${flags.value}`);
  const randomId = readInt64(data, offset);
  offset = randomId.next;
  offset = readInt32(data, offset).next;
  const textBytes = readTLBytes(data, offset).value;
  return { text: Buffer.from(textBytes).toString('utf8'), randomId: randomId.value, layer: layer.value };
}

function makeClient(auth: string): Client {
  const session = new CallbackSession({
    mainDcId: dcId,
    keys: { [dcId]: Buffer.from(auth, 'base64').toString('hex') },
  }, () => {});
  const load = session.load.bind(session);
  session.load = async () => {
    await load();
    (session as any)._serverAddress = process.env.TG_DC_HOST || '127.0.0.1';
    (session as any)._port = Number(process.env.TG_DC_PORT || '11443');
  };
  return new TelegramClient(session, apiId, apiHash, {
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

async function close(client: Client | undefined) {
  if (!client) return;
  try { await client.disconnect(); } catch {}
  try { client.destroy(); } catch {}
}

async function main() {
  let alice: Client | undefined;
  let bob: Client | undefined;
  let bob2: Client | undefined;
  let chatId = 0;
  let accessHash = 0n;
  let discarded = false;
  const deleteHistory = process.env.PROBE_DELETE_HISTORY !== 'false';
  const result: Record<string, unknown> = {};
  try {
    alice = makeClient(process.env.A_AUTH_B64!);
    await alice.connect();
    const meA = await alice.getMe();
    if (BigInt(meA?.id) !== aID) throw new Error('Alice auth key user mismatch');

    const dh = await alice.invoke(new Api.messages.GetDhConfig({ version: 0, randomLength: 32 }));
    const p = bigIntFromBytes(dh.p);
    const g = BigInt(dh.g);
    if (p <= 0n || g <= 1n) throw new Error('invalid DH config');
    const a = bigIntFromBytes(randomBytes(256)) % (p - 2n) + 2n;
    const b = bigIntFromBytes(randomBytes(256)) % (p - 2n) + 2n;
    const b2 = bigIntFromBytes(randomBytes(256)) % (p - 2n) + 2n;
    const ga = modPow(g, a, p);
    const gb = modPow(g, b, p);
    const gb2 = modPow(g, b2, p);
    if (ga <= 1n || ga >= p - 1n || gb <= 1n || gb >= p - 1n || gb2 <= 1n || gb2 >= p - 1n) throw new Error('DH public value outside range');
    const sharedA = bytesFromBigInt(modPow(gb, a, p), 256);
    const sharedB = bytesFromBigInt(modPow(ga, b, p), 256);
    if (Buffer.compare(Buffer.from(sharedA), Buffer.from(sharedB)) !== 0) throw new Error('DH shared key mismatch');
    const sharedA2 = bytesFromBigInt(modPow(gb2, a, p), 256);
    const sharedB2 = bytesFromBigInt(modPow(ga, b2, p), 256);
    if (Buffer.compare(Buffer.from(sharedA2), Buffer.from(sharedB2)) !== 0) throw new Error('second-device DH shared key mismatch');
    const keyFingerprint = fingerprint(sharedA);
    const secondKeyFingerprint = fingerprint(sharedA2);
    result.dh = {
      config: rpcName(dh),
      version: dh.version,
      primeBytes: dh.p.length,
      generator: dh.g,
      gaBytes: ga.toString(16).length / 2,
      gbBytes: gb.toString(16).length / 2,
      sharedKeyEqual: true,
      fingerprint: keyFingerprint.toString(),
      secondDeviceFingerprint: secondKeyFingerprint.toString(),
    };

    const user = new Api.InputUser({ userId: bID, accessHash: bHash });
    const randomId = Math.floor(Date.now() / 1000) & 0x7fffffff;
    const requested = await alice.invoke(requestEncryption(user, randomId, bytesFromBigInt(ga)));
    if (rpcName(requested) !== 'EncryptedChatWaiting') throw new Error(`request returned ${rpcName(requested)}`);
    chatId = Number(requested.id);
    accessHash = BigInt(requested.accessHash);
    result.request = { type: rpcName(requested), chatId };

    // Bob is intentionally connected only after the request is durable, which exercises offline recovery.
    bob = makeClient(process.env.B_AUTH_B64!);
    await bob.connect();
    const meB = await bob.getMe();
    if (BigInt(meB?.id) !== bID) throw new Error('Bob auth key user mismatch');
    const peer = new Api.InputEncryptedChat({ chatId, accessHash });
    const accepted = await bob.invoke(acceptEncryption(peer, bytesFromBigInt(gb), keyFingerprint));
    if (rpcName(accepted) !== 'EncryptedChat' || BigInt(accepted.keyFingerprint) !== keyFingerprint) {
      throw new Error(`accept returned ${rpcName(accepted)} with an unexpected fingerprint`);
    }
    result.accept = { type: rpcName(accepted), fingerprintMatches: true };

    const bobBeforePlaintext = await bob.invoke(new Api.updates.GetState());
    const bobBeforePlaintextQts = Number(bobBeforePlaintext?.qts || 0);
    const bobBeforePlaintextPts = Number(bobBeforePlaintext?.pts || 0);
    const bobBeforePlaintextDate = Number(bobBeforePlaintext?.date || Math.floor(Date.now() / 1000));
    const plaintextRandomId = BigInt(Date.now()) * 1000n + 1n;
    const plaintext = `secret-plaintext-${Date.now()}`;
    const encryptedPlaintext = encryptSecretMessage(
      sharedA,
      true,
      secretMessageLayer(plaintextRandomId, plaintext, 0, 1),
    );
    const sentPlaintext = await alice.invoke(sendEncrypted(peer, plaintextRandomId, encryptedPlaintext));
    if (!rpcName(sentPlaintext).toLowerCase().includes('sentencrypted')) {
      throw new Error(`plaintext send returned ${rpcName(sentPlaintext)}`);
    }
    const plaintextDifference = await bob.invoke(new Api.updates.GetDifference({
      pts: bobBeforePlaintextPts,
      date: bobBeforePlaintextDate,
      qts: bobBeforePlaintextQts,
      qtsLimit: 100,
    }));
    const plaintextMessage = (plaintextDifference?.newEncryptedMessages || []).find((message: any) => (
      Buffer.compare(Buffer.from(message.bytes || []), Buffer.from(encryptedPlaintext)) === 0
    ));
    if (!plaintextMessage) throw new Error(`plaintext message missing from ${rpcName(plaintextDifference)}`);
    const decodedPlaintext = decryptSecretMessage(sharedA, true, Uint8Array.from(plaintextMessage.bytes));
    if (decodedPlaintext.text !== plaintext || decodedPlaintext.randomId !== plaintextRandomId || decodedPlaintext.layer !== 144) {
      throw new Error(`plaintext round trip mismatch: ${JSON.stringify(decodedPlaintext)}`);
    }
    result.plaintext = {
      send: rpcName(sentPlaintext),
      difference: rpcName(plaintextDifference),
      constructor: 'decryptedMessage',
      layer: decodedPlaintext.layer,
      textMatched: true,
      randomIdMatched: true,
      packetRoundTrip: true,
    };

    // A second permanent auth key for the same user represents another
    // device. It may advance its own key epoch, while an older device retry
    // must not replace that newer canonical key.
    bob2 = makeClient(process.env.B2_AUTH_B64!);
    await bob2.connect();
    const meB2 = await bob2.getMe();
    if (BigInt(meB2?.id) !== bID) throw new Error('Bob second-device auth key user mismatch');
    const acceptedOnSecondDevice = await bob2.invoke(acceptEncryption(peer, bytesFromBigInt(gb2), secondKeyFingerprint));
    if (rpcName(acceptedOnSecondDevice) !== 'EncryptedChat' || BigInt(acceptedOnSecondDevice.keyFingerprint) !== secondKeyFingerprint) {
      throw new Error(`second-device accept returned ${rpcName(acceptedOnSecondDevice)} with an unexpected fingerprint`);
    }
    const staleRetry = await bob.invoke(acceptEncryption(peer, bytesFromBigInt(gb), keyFingerprint));
    if (rpcName(staleRetry) !== 'EncryptedChat' || BigInt(staleRetry.keyFingerprint) !== secondKeyFingerprint) {
      throw new Error(`stale-device retry returned ${rpcName(staleRetry)} and rolled back the canonical key`);
    }
    result.acceptSecondDevice = {
      type: rpcName(acceptedOnSecondDevice),
      fingerprintMatches: true,
      staleRetryPreservedNewerKey: true,
    };

    const beforeDifference = await alice.invoke(new Api.updates.GetState());
    const beforeQts = Number(beforeDifference?.qts || 0);
    const beforePts = Number(beforeDifference?.pts || 0);
    const beforeDate = Number(beforeDifference?.date || Math.floor(Date.now() / 1000));
    const crossDeviceRandomId = BigInt(Date.now()) * 1000n + 2n;
    const crossDeviceText = `secret-device-plaintext-${Date.now()}`;
    const crossDevicePayload = encryptSecretMessage(
      sharedA2,
      false,
      secretMessageLayer(crossDeviceRandomId, crossDeviceText, 1, 0),
    );
    const sentFromSecondDevice = await bob2.invoke(sendEncrypted(peer, crossDeviceRandomId, crossDevicePayload));
    if (!rpcName(sentFromSecondDevice).toLowerCase().includes('sentencrypted')) {
      throw new Error(`second-device send returned ${rpcName(sentFromSecondDevice)}`);
    }
    const difference = await alice.invoke(new Api.updates.GetDifference({
      pts: beforePts,
      date: beforeDate,
      qts: beforeQts,
      qtsLimit: 100,
    }));
    const encryptedMessages = difference?.newEncryptedMessages || [];
    const replayed = encryptedMessages.find((message: any) => Buffer.compare(Buffer.from(message.bytes || []), Buffer.from(crossDevicePayload)) === 0);
    if (!replayed) throw new Error(`cross-device message missing from ${rpcName(difference)}`);
    const decodedCrossDevice = decryptSecretMessage(sharedA2, false, Uint8Array.from(replayed.bytes));
    if (decodedCrossDevice.text !== crossDeviceText || decodedCrossDevice.randomId !== crossDeviceRandomId || decodedCrossDevice.layer !== 144) {
      throw new Error(`cross-device plaintext mismatch: ${JSON.stringify(decodedCrossDevice)}`);
    }
    result.crossDevice = {
      send: rpcName(sentFromSecondDevice),
      difference: rpcName(difference),
      qtsBefore: beforeQts,
      qtsAfter: Number(difference?.state?.qts || difference?.intermediateState?.qts || beforeQts),
      payloadMatched: true,
      textMatched: true,
      packetRoundTrip: true,
    };

    await close(alice);
    await close(bob);
    await close(bob2);
    alice = makeClient(process.env.A_AUTH_B64!);
    bob = makeClient(process.env.B_AUTH_B64!);
    await alice.connect();
    await bob.connect();
    const recoveredPeer = new Api.InputEncryptedChat({ chatId, accessHash });
    const bobBeforeRecovered = await bob.invoke(new Api.updates.GetState());
    const bobBeforeRecoveredQts = Number(bobBeforeRecovered?.qts || 0);
    const bobBeforeRecoveredPts = Number(bobBeforeRecovered?.pts || 0);
    const bobBeforeRecoveredDate = Number(bobBeforeRecovered?.date || Math.floor(Date.now() / 1000));
    const recoveredRandomId = BigInt(Date.now()) * 1000n + 3n;
    const recoveredText = `secret-recovered-plaintext-${Date.now()}`;
    const recoveredPayload = encryptSecretMessage(
      sharedA,
      true,
      secretMessageLayer(recoveredRandomId, recoveredText, 0, 1),
    );
    const sent = await alice.invoke(sendEncrypted(recoveredPeer, recoveredRandomId, recoveredPayload));
    if (!rpcName(sent).toLowerCase().includes('sentencrypted')) throw new Error(`send returned ${rpcName(sent)}`);
    const recoveredDifference = await bob.invoke(new Api.updates.GetDifference({
      pts: bobBeforeRecoveredPts,
      date: bobBeforeRecoveredDate,
      qts: bobBeforeRecoveredQts,
      qtsLimit: 100,
    }));
    const recoveredMessage = (recoveredDifference?.newEncryptedMessages || []).find((message: any) => (
      Buffer.compare(Buffer.from(message.bytes || []), Buffer.from(recoveredPayload)) === 0
    ));
    if (!recoveredMessage) throw new Error(`recovered plaintext missing from ${rpcName(recoveredDifference)}`);
    const decodedRecovered = decryptSecretMessage(sharedA, true, Uint8Array.from(recoveredMessage.bytes));
    if (decodedRecovered.text !== recoveredText || decodedRecovered.randomId !== recoveredRandomId || decodedRecovered.layer !== 144) {
      throw new Error(`recovered plaintext mismatch: ${JSON.stringify(decodedRecovered)}`);
    }
    const read = await bob.invoke(readEncryptedHistory(recoveredPeer, Math.floor(Date.now() / 1000)));
    result.recovered = {
      send: rpcName(sent),
      difference: rpcName(recoveredDifference),
      readHistory: rpcName(read),
      textMatched: true,
      packetRoundTrip: true,
    };

    const discardedResult = await alice.invoke(discardEncryption(chatId, deleteHistory));
    if (rpcName(discardedResult).toLowerCase().includes('error')) throw new Error(`discard returned ${rpcName(discardedResult)}`);
    discarded = true;
    result.discard = rpcName(discardedResult);
    console.log(JSON.stringify(result));
  } finally {
    if (!discarded && chatId && accessHash) {
      try {
        if (!alice) alice = makeClient(process.env.A_AUTH_B64!);
        if (!alice.isConnected?.()) await alice.connect();
        await alice.invoke(discardEncryption(chatId, deleteHistory));
      } catch {}
    }
    await close(alice);
    await close(bob);
    await close(bob2);
  }
}

main().catch((error) => {
  console.error(`production secret-chat DH probe failed: ${error instanceof Error ? error.stack || error.message : String(error)}`);
  process.exit(1);
});
