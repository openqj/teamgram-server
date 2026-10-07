import { execFileSync } from 'node:child_process';
import path from 'node:path';
import { createRequire } from 'node:module';

const userId = 136907714;
const gramjsDir = process.env.TEAMGRAM_GRAMJS_DIR || '/Users/mac/open/test/telegram-tt-master';
const dcId = Number(process.env.TG_DC_ID || '2');
const dcHost = process.env.TG_DC_HOST || '127.0.0.1';
const dcPort = Number(process.env.TG_DC_PORT || '11443');

const globalAny = globalThis as typeof globalThis & { self?: any; addEventListener?: () => void };
globalAny.self ??= globalThis;
globalAny.addEventListener ??= () => {};
globalAny.self.addEventListener ??= globalAny.addEventListener;

function mysql(query: string): string {
  return execFileSync('docker', [
    'exec', 'mysql', 'mysql', '-N', '-s', '-uteamgram', '-pteamgram', '-Dteamgram', '-e', query,
  ], { encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'] }).trim();
}

function requiredValue(query: string, name: string): string {
  const value = mysql(query);
  if (!value) throw new Error(`${name} is absent from production MySQL`);
  return value;
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

function makeClient(authKeyB64: string): any {
  const session = new CallbackSession({
    mainDcId: dcId,
    keys: { [dcId]: Buffer.from(authKeyB64, 'base64').toString('hex') },
  }, () => {});
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
  const authKey = requiredValue(`
    SELECT k.body
    FROM auth_users u
    JOIN auth_keys k USING (auth_key_id)
    JOIN auth_key_infos i USING (auth_key_id)
    WHERE u.user_id=${userId} AND u.state=0 AND u.deleted=0
      AND k.deleted=0 AND i.deleted=0 AND i.auth_key_type=0
    ORDER BY u.date_active DESC,u.id DESC
    LIMIT 1
  `, 'active production auth key');
  const [rawCallId, rawAccessHash, rawParticipantCount, rawParticipantsHash] = requiredValue(`
    SELECT id, access_hash, JSON_LENGTH(participants), SHA2(participants, 256)
    FROM apifull_group_call
    WHERE creator_user_id=${userId} AND channel_id=0
    ORDER BY id
    LIMIT 1
  `, 'owned production group call').split('\t');
  const callId = BigInt(rawCallId);
  const accessHash = BigInt(rawAccessHash);
  const participantCount = Number(rawParticipantCount);
  if (!Number.isSafeInteger(participantCount) || participantCount < 1) {
    throw new Error('production group call has no valid persisted participants');
  }

  const client = makeClient(authKey);
  try {
    await client.connect();
    const me = await client.getMe();
    if (String(me?.id) !== String(userId)) throw new Error('auth key user mismatch');
    const inputCall = new Api.InputGroupCall({ id: callId, accessHash });
    const result = await client.invoke(new Api.phone.GetGroupCall({
      call: inputCall,
      limit: 100,
    }));
    const participants = Array.isArray(result?.participants) ? result.participants : [];
    const actualName = rpcName(result);
    const call = result?.call;
    const selfParticipant = participants.find((item: any) => item?.self);
    if (!actualName.endsWith('GroupCall')
      || String(call?.id) !== String(callId)
      || String(call?.accessHash) !== String(accessHash)
      || Number(call?.participantsCount) !== participantCount
      || participants.length !== participantCount
      || String(selfParticipant?.peer?.userId) !== String(userId)) {
      throw new Error(`group call response mismatch: type=${actualName}, callId=${String(call?.id)}, participants=${participants.length}, db=${participantCount}`);
    }
    const roster = await client.invoke(new Api.phone.GetGroupParticipants({
      call: inputCall,
      ids: [],
      sources: [],
      offset: '',
      limit: 100,
    }));
    const rosterParticipants = Array.isArray(roster?.participants) ? roster.participants : [];
    const rosterSelf = rosterParticipants.find((item: any) => item?.self);
    const rosterType = rpcName(roster);
    if (!rosterType.endsWith('GroupParticipants')
      || Number(roster?.count) !== participantCount
      || Number(roster?.version) !== participantCount
      || rosterParticipants.length !== participantCount
      || String(rosterSelf?.peer?.userId) !== String(userId)) {
      throw new Error(`group participant response mismatch: type=${rosterType}, count=${String(roster?.count)}, participants=${rosterParticipants.length}, db=${participantCount}`);
    }
    const checkedSources = await client.invoke(new Api.phone.CheckGroupCall({
      call: inputCall,
      sources: [userId, userId + 1],
    }));
    if (!Array.isArray(checkedSources)
      || checkedSources.length !== 1
      || Number(checkedSources[0]) !== userId) {
      throw new Error(`group call source check mismatch: ${JSON.stringify(checkedSources)}`);
    }
    const callStateAfter = requiredValue(`
      SELECT id, access_hash, JSON_LENGTH(participants), SHA2(participants, 256)
      FROM apifull_group_call
      WHERE id=${callId} AND creator_user_id=${userId}
      LIMIT 1
    `, 'unchanged production group call');
    const callStateBefore = [rawCallId, rawAccessHash, rawParticipantCount, rawParticipantsHash].join('\t');
    if (callStateAfter !== callStateBefore) {
      throw new Error('read-only group call probes changed the persisted call row');
    }
    console.log(JSON.stringify({
      userId: String(me.id),
      transport: `DC${dcId} WebSocket -> gateway -> session -> BFF -> MySQL`,
      resultType: actualName,
      callType: rpcName(call),
      callId: String(call.id),
      participants: participants.length,
      mysqlParticipants: participantCount,
      selfParticipant: true,
      getGroupParticipants: {
        type: rosterType,
        count: Number(roster.count),
        version: Number(roster.version),
        participants: rosterParticipants.length,
        selfParticipant: true,
      },
      checkGroupCall: { sourcesRequested: 2, sourcesPresent: checkedSources.length, matchedSelfSource: true },
      persistedCallUnchanged: true,
      writes: 0,
    }));
  } finally {
    client.destroy();
  }
}

main().then(() => process.exit(0)).catch((error) => {
  console.error(`group call production probe failed: ${error instanceof Error ? error.stack || error.message : String(error)}`);
  process.exit(1);
});
