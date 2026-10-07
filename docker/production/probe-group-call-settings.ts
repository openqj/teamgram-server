import { execFileSync } from 'node:child_process';
import path from 'node:path';
import { createRequire } from 'node:module';

const gramjsDir = process.env.TEAMGRAM_GRAMJS_DIR || '/Users/mac/open/test/telegram-tt-master';
const dcId = Number(process.env.TG_DC_ID || '2');
const userId = '136907713';
const globalAny = globalThis as typeof globalThis & { self?: any; addEventListener?: () => void };
globalAny.self ??= globalThis;
globalAny.addEventListener ??= () => {};
globalAny.self.addEventListener ??= globalAny.addEventListener;

const requireModule = createRequire(import.meta.url);
const TelegramClient = requireModule(path.join(gramjsDir, 'src/lib/gramjs/client/TelegramClient.ts')).default;
const CallbackSession = requireModule(path.join(gramjsDir, 'src/lib/gramjs/sessions/CallbackSession.ts')).default;
const Api = requireModule(path.join(gramjsDir, 'src/lib/gramjs/tl/index.ts')).Api;
const Connection = requireModule(path.join(gramjsDir, 'src/lib/gramjs/network/connection/TCPObfuscated.ts')).ConnectionTCPObfuscated;
const quietLogger = { debug() {}, info() {}, warn() {}, error() {} };

function sql(query: string): string {
  return execFileSync('docker', ['exec', 'mysql', 'mysql', '-N', '-s', '-uteamgram', '-pteamgram', '-Dteamgram', '-e', query], {
    encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'],
  }).trim();
}

function loadAuthKey(): string {
  const body = sql(`SELECT k.body FROM auth_users u JOIN auth_keys k USING(auth_key_id)
    JOIN auth_key_infos i USING(auth_key_id) WHERE u.user_id=${userId} AND u.state=0 AND u.deleted=0
    AND k.deleted=0 AND i.deleted=0 AND i.auth_key_type=0 ORDER BY u.date_active DESC,u.id DESC LIMIT 1`);
  if (!body) throw new Error(`no active production auth key found for user ${userId}`);
  return Buffer.from(body, 'base64').toString('hex');
}

function rpcName(value: any): string { return value?.className || value?.constructor?.name || typeof value; }
function isType(value: any, name: string): boolean { const actual = rpcName(value); return actual === name || actual.endsWith(`.${name}`); }
function isTrue(value: any): boolean { return value === true || rpcName(value).toLowerCase().includes('true'); }

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
  const bytes = new TextEncoder().encode(value);
  if (bytes.length >= 254) throw new Error('probe string unexpectedly large');
  const out = new Uint8Array((bytes.length + 1 + 3) & ~3);
  out[0] = bytes.length;
  out.set(bytes, 1);
  return out;
}

function concat(...parts: Uint8Array[]): Uint8Array {
  const out = new Uint8Array(parts.reduce((n, part) => n + part.length, 0));
  let offset = 0;
  for (const part of parts) { out.set(part, offset); offset += part.length; }
  return out;
}

function rawRequest(className: string, constructorId: number, payload: Uint8Array): any {
  return { className, classType: 'request', CONSTRUCTOR_ID: constructorId,
    getBytes: () => payload, readResult: (reader: any) => reader.tgReadObject() };
}

function toggleSettingsRequest(call: any): any {
  const flags = 1 | 4 | 8;
  return rawRequest('phone.ToggleGroupCallSettings', 0x974392f2, concat(
    int32(0x974392f2), int32(flags), call.getBytes(), int32(0x997275b5), int32(0xbc799737), int64(10n),
  ));
}

function enableMessagesRequest(call: any): any {
  return rawRequest('phone.ToggleGroupCallSettings', 0x974392f2, concat(
    int32(0x974392f2), int32(4), call.getBytes(), int32(0x997275b5),
  ));
}

function toggleRecordRequest(call: any, start: boolean, video: boolean, title?: string, videoPortrait?: boolean): any {
  let flags = 0;
  if (start) flags |= 1;
  if (title !== undefined) flags |= 2;
  if (video || videoPortrait) flags |= 4;
  const parts = [int32(0xf128c708), int32(flags), call.getBytes()];
  if (title !== undefined) parts.push(tlString(title));
  if (video || videoPortrait) parts.push(int32(videoPortrait ? 0x997275b5 : 0xbc799737));
  return rawRequest('phone.ToggleGroupCallRecord', 0xf128c708, concat(...parts));
}

function editParticipantRequest(call: any, participant: any): any {
  const flags = 1 | 4 | 8;
  return rawRequest('phone.EditGroupCallParticipant', 0xa5273abf, concat(
    int32(0xa5273abf), int32(flags), call.getBytes(), participant.getBytes(),
    int32(0xbc799737), int32(0x997275b5), int32(0xbc799737),
  ));
}

function sendGroupCallMessageRequest(call: any, randomId: bigint, message: any): any {
  return rawRequest('phone.SendGroupCallMessage', 0xb1d11410, concat(
    int32(0xb1d11410), int32(0), call.getBytes(), int64(randomId), message.getBytes(),
  ));
}

function deleteGroupCallMessagesRequest(call: any, messageId: number): any {
  return rawRequest('phone.DeleteGroupCallMessages', 0xf64f54f7, concat(
    int32(0xf64f54f7), int32(0), call.getBytes(), int32(0x1cb5c415), int32(1), int32(messageId),
  ));
}

function deleteGroupCallParticipantMessagesRequest(call: any, participant: any): any {
  return rawRequest('phone.DeleteGroupCallParticipantMessages', 0x1dbfeca0, concat(
    int32(0x1dbfeca0), int32(0), call.getBytes(), participant.getBytes(),
  ));
}

function makeClient(authKeyHex: string): any {
  const session = new CallbackSession({ mainDcId: dcId, keys: { [dcId]: authKeyHex } }, () => {});
  const load = session.load.bind(session);
  session.load = async () => { await load(); (session as any)._serverAddress = process.env.TG_DC_HOST || '127.0.0.1'; (session as any)._port = Number(process.env.TG_DC_PORT || '11443'); };
  return new TelegramClient(session, 1, '0123456789abcdef0123456789abcdef', {
    connection: Connection, fallbackConnection: Connection, shouldAllowHttpTransport: false,
    shouldForceHttpTransport: false, timeout: 20, requestRetries: 1, connectionRetries: 1,
    connectionRetriesToFallback: 0, autoReconnect: false, baseLogger: quietLogger,
  });
}

function callFromUpdates(value: any): any {
  const call = Array.isArray(value?.updates) ? value.updates.map((item: any) => item?.call)
    .find((item: any) => item?.id !== undefined && item?.accessHash !== undefined) : undefined;
  if (!call) throw new Error(`phone.createGroupCall returned ${rpcName(value)} without a call`);
  return call;
}

function dbSettings(callId: bigint): string {
  return sql(`SELECT join_muted,messages_enabled,COALESCE(send_paid_messages_stars,''),record_active,record_video,record_title,record_video_portrait
    FROM apifull_group_call_settings WHERE call_id=${callId}`);
}

function dbParticipant(callId: bigint): string {
  return sql(`SELECT muted,COALESCE(volume,''),raise_hand,video_stopped,video_paused,presentation_paused,presentation_active
    FROM apifull_group_call_participant WHERE call_id=${callId} AND user_id=${userId}`);
}

function dbMessage(callId: bigint): string {
  return sql(`SELECT id,random_id,message,deleted FROM apifull_group_call_message WHERE call_id=${callId} ORDER BY id DESC LIMIT 1`);
}

async function main() {
  const client = makeClient(loadAuthKey());
  let inputCall: any;
  let callId = 0n;
  let discarded = false;
  try {
    (client as any)._borrowExportedSender = async () => undefined;
    (client as any)._updateLoop = async () => undefined;
    await client.connect();
    const me = await client.getMe();
    if (String(me?.id) !== userId) throw new Error('auth key user mismatch');

    const created = await client.invoke(new Api.phone.CreateGroupCall({
      peer: new Api.InputPeerSelf(), randomId: Number(BigInt(Date.now()) % 2147483647n),
      title: `production settings ${Date.now()}`,
    }));
    const call = callFromUpdates(created);
    callId = BigInt(call.id);
    inputCall = new Api.InputGroupCall({ id: call.id, accessHash: call.accessHash });

    const settingsUpdate = await client.invoke(toggleSettingsRequest(inputCall));
    if (!isType(settingsUpdate, 'Updates')) throw new Error(`toggleGroupCallSettings returned ${rpcName(settingsUpdate)}`);
    const settingsRead = await client.invoke(new Api.phone.GetGroupCall({ call: inputCall, limit: 20 }));
    const settingCall = settingsRead?.call;
    if (!settingCall?.joinMuted || settingCall?.messagesEnabled || String(settingCall?.sendPaidMessagesStars) !== '10') {
      throw new Error(`group-call settings readback mismatch: ${rpcName(settingsRead)} ${JSON.stringify(settingCall)}`);
    }
    const settingsRow = dbSettings(callId);
    if (settingsRow !== '1\t0\t10\t0\t0\t\t0') throw new Error(`settings DB readback mismatch: ${settingsRow}`);

    const recordStart = await client.invoke(toggleRecordRequest(inputCall, true, true, 'production recording control', true));
    if (!isType(recordStart, 'Updates')) throw new Error(`toggleGroupCallRecord start returned ${rpcName(recordStart)}`);
    const recordRead = await client.invoke(new Api.phone.GetGroupCall({ call: inputCall, limit: 20 }));
    if (recordRead?.call?.recordVideoActive !== true) throw new Error('recordVideoActive did not read back');
    const recordRow = dbSettings(callId);
    if (recordRow !== '1\t0\t10\t1\t1\tproduction recording control\t1') throw new Error(`record DB readback mismatch: ${recordRow}`);

    await client.invoke(new Api.phone.JoinGroupCall({ call: inputCall, joinAs: new Api.InputPeerSelf(), muted: true, videoStopped: true, params: new Api.DataJSON({ data: '{}' }) }));
    const participantUpdate = await client.invoke(editParticipantRequest(inputCall, new Api.InputPeerSelf()));
    if (!isType(participantUpdate, 'Updates')) throw new Error(`editGroupCallParticipant returned ${rpcName(participantUpdate)}`);
    const participantRow = dbParticipant(callId);
    if (participantRow !== '0\t\t1\t0\t0\t0\t0') throw new Error(`participant DB readback mismatch: ${participantRow}`);

    await client.invoke(enableMessagesRequest(inputCall));
    const randomId = BigInt(Date.now()) * 1000n + BigInt(process.pid % 1000);
    const sendMessage = await client.invoke(sendGroupCallMessageRequest(
      inputCall,
      randomId,
      new Api.TextWithEntities({ text: 'production group call message', entities: [] }),
    ));
    if (!isType(sendMessage, 'Updates')) throw new Error(`sendGroupCallMessage returned ${rpcName(sendMessage)}`);
    const messageRow = dbMessage(callId);
    const messageFields = messageRow.split('\t');
    let storedMessage: any;
    try { storedMessage = JSON.parse(messageFields[2] || ''); } catch { storedMessage = undefined; }
    if (!messageFields[0] || messageFields[1] !== randomId.toString() || storedMessage?.text !== 'production group call message' || messageFields[3] !== '0') {
      throw new Error(`group-call message DB readback mismatch: ${messageRow}`);
    }
    const messageId = Number(messageFields[0]);
    const deleteMessage = await client.invoke(deleteGroupCallMessagesRequest(inputCall, messageId));
    if (!isType(deleteMessage, 'Updates')) throw new Error(`deleteGroupCallMessages returned ${rpcName(deleteMessage)}`);
    const deleteParticipantMessages = await client.invoke(deleteGroupCallParticipantMessagesRequest(inputCall, new Api.InputPeerSelf()));
    if (!isType(deleteParticipantMessages, 'Updates')) throw new Error(`deleteGroupCallParticipantMessages returned ${rpcName(deleteParticipantMessages)}`);
    const deletedMessageRow = dbMessage(callId);
    if (deletedMessageRow.split('\t')[3] !== '1') throw new Error(`group-call message was not marked deleted: ${deletedMessageRow}`);

    const recordStop = await client.invoke(toggleRecordRequest(inputCall, false, false));
    if (!isType(recordStop, 'Updates')) throw new Error(`toggleGroupCallRecord stop returned ${rpcName(recordStop)}`);
    await client.invoke(new Api.phone.DiscardGroupCall({ call: inputCall }));
    discarded = true;
    try { await client.invoke(new Api.phone.GetGroupCall({ call: inputCall, limit: 20 })); throw new Error('discarded call remained readable'); }
    catch (error) { if (!String(error).includes('GROUPCALL_INVALID')) throw error; }

    const remaining = sql(`SELECT (SELECT COUNT(*) FROM apifull_group_call WHERE id=${callId}) +
      (SELECT COUNT(*) FROM apifull_group_call_settings WHERE call_id=${callId}) +
      (SELECT COUNT(*) FROM apifull_group_call_participant WHERE call_id=${callId})`);
    if (remaining !== '0') throw new Error(`group-call cleanup left rows: ${remaining}`);
    console.log(JSON.stringify({ userId: String(me.id), callId: String(callId), settingsUpdate: rpcName(settingsUpdate), recordStart: rpcName(recordStart), participantUpdate: rpcName(participantUpdate), sendMessage: rpcName(sendMessage), deleteMessage: rpcName(deleteMessage), deleteParticipantMessages: rpcName(deleteParticipantMessages), recordStop: rpcName(recordStop), cleanupRows: Number(remaining), mediaRecorder: 'not-present' }));
  } finally {
    if (inputCall && !discarded) { try { await client.invoke(new Api.phone.DiscardGroupCall({ call: inputCall })); } catch {} }
    client.destroy();
  }
}

main().then(() => process.exit(0)).catch((error) => { console.error(`group-call settings production probe failed: ${error instanceof Error ? error.stack || error.message : String(error)}`); process.exit(1); });
