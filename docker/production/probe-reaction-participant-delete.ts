import { execFileSync } from 'node:child_process';
import path from 'node:path';
import { createRequire } from 'node:module';

for (const key of ['TEAMGRAM_GRAMJS_DIR']) if (!process.env[key]) throw new Error(`${key} is required`);
const gramjsDir = process.env.TEAMGRAM_GRAMJS_DIR!;
const dcId = Number(process.env.TG_DC_ID || '2');
const aId = BigInt(process.env.A_USER_ID || '136907713');
const bId = BigInt(process.env.B_USER_ID || '136907714');
const quietLogger = { debug() {}, info() {}, warn() {}, error() {} };
const globalAny = globalThis as typeof globalThis & { self?: any; addEventListener?: () => void };
globalAny.self ??= globalThis;
globalAny.addEventListener ??= () => {};
globalAny.self.addEventListener ??= globalAny.addEventListener;

function sql(query: string): string {
  return execFileSync('docker', ['exec', 'mysql', 'mysql', '-N', '-s', '-uteamgram', '-pteamgram', '-Dteamgram', '-e', query], {
    encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'],
  }).trim();
}

function redisSnapshot(keys: string[]): Record<string, { present: boolean; value: string }> {
  const out: Record<string, { present: boolean; value: string }> = {};
  for (const key of keys) {
    const present = execFileSync('docker', ['exec', 'redis', 'redis-cli', 'EXISTS', key], { encoding: 'utf8' }).trim() === '1';
    const value = present ? execFileSync('docker', ['exec', 'redis', 'redis-cli', '--raw', 'GET', key], { encoding: 'utf8' }) : '';
    out[key] = { present, value };
  }
  return out;
}

function restoreRedis(snapshot: Record<string, { present: boolean; value: string }>): void {
  for (const [key, state] of Object.entries(snapshot)) {
    if (state.present) execFileSync('docker', ['exec', 'redis', 'redis-cli', 'SET', key, state.value], { stdio: 'ignore' });
    else execFileSync('docker', ['exec', 'redis', 'redis-cli', 'DEL', key], { stdio: 'ignore' });
  }
}

function authKey(userId: bigint): string {
  const body = sql(`SELECT k.body FROM auth_users u JOIN auth_keys k USING(auth_key_id)
    JOIN auth_key_infos i USING(auth_key_id) WHERE u.user_id=${userId} AND u.state=0 AND u.deleted=0
    AND k.deleted=0 AND i.deleted=0 AND i.auth_key_type=0 ORDER BY u.date_active DESC,u.id DESC LIMIT 1`);
  if (!body) throw new Error(`no active production auth key for ${userId}`);
  return body;
}

function accessHash(userId: bigint): bigint {
  const value = sql(`SELECT access_hash FROM users WHERE id=${userId} AND deleted=0 LIMIT 1`);
  if (!value) throw new Error(`no production access hash for ${userId}`);
  return BigInt(value);
}

const requireModule = createRequire(import.meta.url);
const TelegramClient = requireModule(path.join(gramjsDir, 'src/lib/gramjs/client/TelegramClient.ts')).default;
const CallbackSession = requireModule(path.join(gramjsDir, 'src/lib/gramjs/sessions/CallbackSession.ts')).default;
const Api = requireModule(path.join(gramjsDir, 'src/lib/gramjs/tl/index.ts')).Api;
const Connection = requireModule(path.join(gramjsDir, 'src/lib/gramjs/network/connection/TCPObfuscated.ts')).ConnectionTCPObfuscated;
const rpcName = (value: any): string => value?.className || value?.constructor?.name || typeof value;

function makeClient(authB64: string): any {
  const session = new CallbackSession({ mainDcId: dcId, keys: { [dcId]: Buffer.from(authB64, 'base64').toString('hex') } }, () => {});
  const load = session.load.bind(session);
  session.load = async () => { await load(); (session as any)._serverAddress = process.env.TG_DC_HOST || '127.0.0.1'; (session as any)._port = Number(process.env.TG_DC_PORT || '11443'); };
  const client = new TelegramClient(session, 1, '0123456789abcdef0123456789abcdef', {
    connection: Connection, fallbackConnection: Connection,
    shouldAllowHttpTransport: false, shouldForceHttpTransport: false,
    timeout: 20, requestRetries: 2, connectionRetries: 2,
    connectionRetriesToFallback: 0, retryDelay: 250, autoReconnect: false, baseLogger: quietLogger,
  });
  (client as any)._borrowExportedSender = async () => undefined;
  (client as any)._updateLoop = async () => undefined;
  return client;
}

function history(peer: any): any {
  return new Api.messages.GetHistory({ peer, offsetId: 0, offsetDate: 0, addOffset: 0, limit: 100, maxId: 0, minId: 0, hash: 0n });
}

async function main() {
  const sharedKey = `react:shared:user:${aId < bId ? aId : bId}:${aId < bId ? bId : aId}`;
  const keys = [sharedKey, `react:list:${bId}`, `react:recent:${bId}`, `react:${bId}:send`, `react:${bId}:delOne`, `react:${bId}:delAll`];
  const redisBefore = redisSnapshot(keys);
  const a = makeClient(authKey(aId));
  const b = makeClient(authKey(bId));
  const peerA = new Api.InputPeerUser({ userId: bId, accessHash: accessHash(bId) });
  const peerB = new Api.InputPeerUser({ userId: aId, accessHash: accessHash(aId) });
  const participantB = new Api.InputPeerUser({ userId: bId, accessHash: accessHash(bId) });
  const ids: number[] = [];
  const senderIds: number[] = [];
  const tag = `prod-reaction-delete-${Date.now()}-${process.pid}`;
  try {
    await a.connect(); await b.connect();
    if (String((await a.getMe())?.id) !== String(aId) || String((await b.getMe())?.id) !== String(bId)) throw new Error('auth key user mismatch');
    const sentOne = await a.invoke(new Api.messages.SendMessage({ peer: peerA, message: `${tag}-one`, randomId: BigInt(Date.now()) * 1000n + 1n }));
    const sentTwo = await a.invoke(new Api.messages.SendMessage({ peer: peerA, message: `${tag}-two`, randomId: BigInt(Date.now()) * 1000n + 2n }));
    const findId = async (client: any, peer: any, text: string): Promise<number> => {
      for (let attempt = 0; attempt < 8; attempt++) {
        const page = await client.invoke(history(peer));
        const message = (page?.messages || []).find((item: any) => item?.message === text);
        if (message?.id) return Number(message.id);
        await new Promise((resolve) => setTimeout(resolve, 150));
      }
      throw new Error(`message not visible: ${text}`);
    };
    const oneId = await findId(b, peerB, `${tag}-one`);
    const twoId = await findId(b, peerB, `${tag}-two`);
    senderIds.push(await findId(a, peerA, `${tag}-one`), await findId(a, peerA, `${tag}-two`));
    ids.push(oneId, twoId);
    const setOne = await b.invoke(new Api.messages.SendReaction({ peer: peerB, msgId: oneId, reaction: [new Api.ReactionEmoji({ emoticon: '👍' })] }));
    const setTwo = await b.invoke(new Api.messages.SendReaction({ peer: peerB, msgId: twoId, reaction: [new Api.ReactionEmoji({ emoticon: '🔥' })] }));
    const oneBefore = await b.invoke(new Api.messages.GetMessageReactionsList({ peer: peerB, id: oneId, limit: 20 }));
    if (Number(oneBefore?.count || 0) !== 1) throw new Error(`single reaction was not stored: ${rpcName(oneBefore)} count=${oneBefore?.count}`);
    const deleteOne = await b.invoke(new Api.messages.DeleteParticipantReaction({ peer: peerB, msgId: oneId, participant: participantB }));
    if (!rpcName(deleteOne).toLowerCase().includes('updates')) throw new Error(`deleteParticipantReaction returned ${rpcName(deleteOne)}`);
    const oneAfter = await b.invoke(new Api.messages.GetMessageReactionsList({ peer: peerB, id: oneId, limit: 20 }));
    if (Number(oneAfter?.count || 0) !== 0) throw new Error(`single participant reaction remains: ${oneAfter?.count}`);
    const twoBefore = await b.invoke(new Api.messages.GetMessageReactionsList({ peer: peerB, id: twoId, limit: 20 }));
    if (Number(twoBefore?.count || 0) !== 1) throw new Error(`second reaction was not stored: ${twoBefore?.count}`);
    const deleteAll = await b.invoke(new Api.messages.DeleteParticipantReactions({ peer: peerB, participant: participantB }));
    const deleteAllOk = deleteAll === true || deleteAll?.value === true || deleteAll?.value === 1 || rpcName(deleteAll).toLowerCase().includes('true');
    if (!deleteAllOk) throw new Error(`deleteParticipantReactions returned ${rpcName(deleteAll)} value=${deleteAll?.value ?? deleteAll}`);
    const twoAfter = await b.invoke(new Api.messages.GetMessageReactionsList({ peer: peerB, id: twoId, limit: 20 }));
    if (Number(twoAfter?.count || 0) !== 0) throw new Error(`batch participant reactions remain: ${twoAfter?.count}`);
    console.log(JSON.stringify({ tag, sentOne: rpcName(sentOne), sentTwo: rpcName(sentTwo), setOne: rpcName(setOne), setTwo: rpcName(setTwo), deleteOne: rpcName(deleteOne), deleteAll: rpcName(deleteAll), oneBefore: Number(oneBefore.count || 0), oneAfter: Number(oneAfter.count || 0), twoBefore: Number(twoBefore.count || 0), twoAfter: Number(twoAfter.count || 0), oneId, twoId }));
  } finally {
    for (const id of senderIds) { try { await a.invoke(new Api.messages.DeleteMessages({ revoke: true, id: [id] })); } catch {} }
    for (const id of ids) { try { await b.invoke(new Api.messages.DeleteMessages({ revoke: true, id: [id] })); } catch {} }
    try { restoreRedis(redisBefore); } catch (error) { console.error(`reaction Redis restore failed: ${String(error)}`); }
    a.destroy(); b.destroy();
  }
}

main().then(() => process.exit(0)).catch((error) => {
  console.error(`reaction participant-delete production probe failed: ${error instanceof Error ? error.stack || error.message : String(error)}`);
  process.exit(1);
});
