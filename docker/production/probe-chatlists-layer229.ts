import { execFileSync } from 'node:child_process';
import path from 'node:path';
import { createRequire } from 'node:module';

for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) {
  delete process.env[key];
}

const gramjsDir = process.env.TEAMGRAM_GRAMJS_DIR || '/Users/mac/open/test/telegram-tt-master';
const postgresContainer = process.env.TEAMGRAM_POSTGRES_CONTAINER || 'teamgram-postgres';
const dcId = Number(process.env.TG_DC_ID || '2');
const dcHost = process.env.TG_DC_HOST || '127.0.0.1';
const dcPort = Number(process.env.TG_DC_PORT || '11443');
const aID = BigInt(process.env.A_USER_ID || '1');
const bID = BigInt(process.env.B_USER_ID || '2');
const apiId = Number(process.env.TG_API_ID || '1');
const apiHash = process.env.TG_API_HASH || '0123456789abcdef0123456789abcdef';
const reportPath = process.env.CHATLISTS_REPORT || `/tmp/teamgram-chatlists-layer229-${Date.now()}.json`;
const quietLogger = { debug() {}, info() {}, warn() {}, error() {} };

if (dcId !== 2 || dcHost !== '127.0.0.1' || dcPort !== 11443) {
  throw new Error('probe requires DC2 TCPObfuscated at 127.0.0.1:11443');
}
if (aID <= 0n || bID <= 0n || aID === bID) throw new Error('fixture users must be distinct positive IDs');

const globalAny = globalThis as typeof globalThis & { self?: any; addEventListener?: () => void };
globalAny.self ??= globalThis;
globalAny.addEventListener ??= () => {};
globalAny.self.addEventListener ??= globalAny.addEventListener;

const requireModule = createRequire(import.meta.url);
const TelegramClient = requireModule(path.join(gramjsDir, 'src/lib/gramjs/client/TelegramClient.ts')).default;
const CallbackSession = requireModule(path.join(gramjsDir, 'src/lib/gramjs/sessions/CallbackSession.ts')).default;
const Api = requireModule(path.join(gramjsDir, 'src/lib/gramjs/tl/index.ts')).Api;
const Connection = requireModule(path.join(gramjsDir, 'src/lib/gramjs/network/connection/TCPObfuscated.ts')).ConnectionTCPObfuscated;

function postgres(query: string): string {
  return execFileSync('docker', [
    'exec', postgresContainer, 'psql', '-Atq', '-v', 'ON_ERROR_STOP=1',
    '-U', 'teamgram', '-d', 'teamgram', '-c', query,
  ], { encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] }).trim();
}

function authKey(userID: bigint, envName: string): string {
  const supplied = process.env[envName]?.trim();
  if (supplied) return supplied;
  const result = postgres(`
    SELECT encode(k.body, 'base64')
    FROM auth_users au
    JOIN auth_keys k USING (auth_key_id)
    JOIN auth_key_infos i USING (auth_key_id)
    WHERE au.user_id=${userID} AND au.state=0 AND NOT au.deleted
      AND NOT k.deleted AND NOT i.deleted AND i.auth_key_type=0
    ORDER BY au.date_active DESC, au.id DESC LIMIT 1`);
  if (!result) throw new Error(`no active production auth key for user ${userID}`);
  return result;
}

function accessHash(userID: bigint, envName: string): bigint {
  const supplied = process.env[envName]?.trim();
  if (supplied) return BigInt(supplied);
  const result = postgres(`SELECT access_hash FROM users WHERE id=${userID} LIMIT 1`);
  if (!result) throw new Error(`no access hash for user ${userID}`);
  return BigInt(result);
}

function rpcName(value: any): string {
  return value?.className || value?.constructor?.name || typeof value;
}

function isTrue(value: any): boolean {
  return value === true || rpcName(value).toLowerCase().includes('true');
}

function hasConstructor(value: any, suffix: string): boolean {
  return rpcName(value) === suffix || rpcName(value).endsWith(`.${suffix}`);
}

function check(condition: unknown, message: string): asserts condition {
  if (!condition) throw new Error(message);
}

function stateCount(userID: bigint): number {
  return Number(postgres(`SELECT count(*) FROM apifull_chatlist_state WHERE user_id=${userID}`));
}

function inviteCount(userID: bigint): number {
  return Number(postgres(`SELECT count(*) FROM apifull_chatlist_invite WHERE owner_user_id=${userID}`));
}

function filterCount(userID: bigint, filterID: number): number {
  return Number(postgres(`SELECT count(*) FROM dialog_filters WHERE user_id=${userID} AND dialog_filter_id=${filterID}`));
}

function makeClient(auth: string): any {
  check(Buffer.from(auth, 'base64').length === 256, 'auth key must be 256 bytes');
  const session = new CallbackSession({
    mainDcId: dcId,
    keys: { [dcId]: Buffer.from(auth, 'base64').toString('hex') },
  }, () => {});
  const load = session.load.bind(session);
  session.load = async () => {
    await load();
    (session as any)._serverAddress = dcHost;
    (session as any)._port = dcPort;
  };
  const client = new TelegramClient(session, apiId, apiHash, {
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
  const initWith = client._initWith.bind(client);
  client._initWith = (query: any) => {
    const original = initWith(query);
    check(rpcName(original) === 'InvokeWithLayer' && rpcName(original.query) === 'InitConnection', 'invalid init wrapper');
    return new Api.InvokeWithLayer({ layer: 229, query: original.query });
  };
  (client as any)._borrowExportedSender = async () => undefined;
  (client as any)._updateLoop = async () => undefined;
  return client;
}

function filterIDs(value: any): number[] {
  return (Array.isArray(value?.filters) ? value.filters : [])
    .map((filter: any) => Number(filter?.id))
    .filter((id: number) => Number.isInteger(id));
}

function filterByID(value: any, id: number): any {
  return (Array.isArray(value?.filters) ? value.filters : [])
    .find((filter: any) => Number(filter?.id) === id);
}

function inputChatlist(id: number): any {
  return new Api.InputChatlistDialogFilter({ filterId: id });
}

function intBytes(value: number): Uint8Array {
  const bytes = new Uint8Array(4);
  new DataView(bytes.buffer).setUint32(0, value >>> 0, true);
  return bytes;
}

function rawRequest(constructorID: number, className: string, parts: Uint8Array[]): any {
  return {
    CONSTRUCTOR_ID: constructorID,
    SUBCLASS_OF_ID: 0,
    className,
    classType: 'request',
    getBytes: () => Uint8Array.from(Buffer.concat([
      Buffer.from(intBytes(constructorID)),
      ...parts.map((part) => Buffer.from(part)),
    ])),
    readResult: (reader: any) => reader.tgReadObject(),
  };
}

function getChatlistUpdates(id: number): any {
  return rawRequest(0x89419521, 'chatlists.GetChatlistUpdates', [inputChatlist(id).getBytes()]);
}

function joinChatlistUpdates(id: number, peers: any[]): any {
  return rawRequest(0xe089f8f5, 'chatlists.JoinChatlistUpdates', [
    inputChatlist(id).getBytes(),
    intBytes(0x1cb5c415), intBytes(peers.length),
    ...peers.map((peer) => peer.getBytes()),
  ]);
}

function hideChatlistUpdates(id: number): any {
  return rawRequest(0x66e486fb, 'chatlists.HideChatlistUpdates', [inputChatlist(id).getBytes()]);
}

function updateFilterIDs(value: any): number[] {
  return (Array.isArray(value?.updates) ? value.updates : [])
    .filter((update: any) => rpcName(update).endsWith('UpdateDialogFilter'))
    .map((update: any) => Number(update?.id))
    .filter((id: number) => Number.isInteger(id));
}

async function main(): Promise<void> {
  const authA = authKey(aID, 'A_AUTH_B64');
  const authB = authKey(bID, 'B_AUTH_B64');
  const aHash = accessHash(aID, 'A_ACCESS_HASH');
  const bHash = accessHash(bID, 'B_ACCESS_HASH');
  const before = {
    aState: stateCount(aID),
    bState: stateCount(bID),
    aInvites: inviteCount(aID),
    bInvites: inviteCount(bID),
  };
  check(before.aState === 0 && before.bState === 0 && before.aInvites === 0 && before.bInvites === 0,
    `fixture chatlist state is not empty: ${JSON.stringify(before)}`);
  const alice = makeClient(authA);
  const bob = makeClient(authB);
  let filterID = 0;
  let joinedFilterID = 0;
  let slug = '';
  let deletedInvite = false;
  const operation: Record<string, unknown> = {};
  let cleanup: Record<string, unknown> = { ok: false };
  try {
    await Promise.all([alice.connect(), bob.connect()]);
    const meA = await alice.getMe();
    const meB = await bob.getMe();
    check(String(meA?.id) === aID.toString() && String(meB?.id) === bID.toString(), 'auth key user mismatch');
    const [filtersA, filtersB] = await Promise.all([
      alice.invoke(new Api.messages.GetDialogFilters()),
      bob.invoke(new Api.messages.GetDialogFilters()),
    ]);
    const used = new Set([...filterIDs(filtersA), ...filterIDs(filtersB)]);
    for (let id = 2; id <= 10; id += 1) {
      if (!used.has(id) && filterCount(aID, id) === 0 && filterCount(bID, id) === 0) {
        filterID = id;
        break;
      }
    }
    check(filterID > 0, 'no unused chatlist filter id in Layer 229 range');
    const aPeer = new Api.InputPeerUser({ userId: aID, accessHash: aHash });
    const bPeer = new Api.InputPeerUser({ userId: bID, accessHash: bHash });
    const title = `chatlists-${Date.now()}-${process.pid}`;
    const renamed = `${title}-edited`;
    const filter = new Api.DialogFilterChatlist({
      id: filterID,
      title: new Api.TextWithEntities({ text: title, entities: [] }),
      pinnedPeers: [],
      includePeers: [aPeer],
    });
    check(isTrue(await alice.invoke(new Api.messages.UpdateDialogFilter({ id: filterID, filter }))), 'owner filter create failed');
    check(filterByID(await alice.invoke(new Api.messages.GetDialogFilters()), filterID)?.className === 'DialogFilterChatlist', 'owner filter readback failed');

    const exported = await alice.invoke(new Api.chatlists.ExportChatlistInvite({
      chatlist: inputChatlist(filterID), title, peers: [aPeer],
    }));
    check(hasConstructor(exported, 'ExportedChatlistInvite') && exported.invite?.url, `export failed: ${rpcName(exported)}`);
    slug = String(exported.invite.url);
    const listed = await alice.invoke(new Api.chatlists.GetExportedInvites({ chatlist: inputChatlist(filterID) }));
    check(hasConstructor(listed, 'ExportedInvites') && listed.invites?.some((invite: any) => invite.url === slug), 'get exported invites missing slug');
    const edited = await alice.invoke(new Api.chatlists.EditExportedInvite({ chatlist: inputChatlist(filterID), slug, title: renamed }));
    check(hasConstructor(edited, 'ExportedChatlistInvite') && edited.title === renamed, 'edit exported invite readback failed');

    const checked = await bob.invoke(new Api.chatlists.CheckChatlistInvite({ slug }));
    check(hasConstructor(checked, 'ChatlistInvite') && checked.peers?.length === 1, `check invite returned ${rpcName(checked)}`);
    const joined = await bob.invoke(new Api.chatlists.JoinChatlistInvite({ slug, peers: [aPeer] }));
    joinedFilterID = updateFilterIDs(joined)[0] || 0;
    check(rpcName(joined).toLowerCase().includes('updates') && joinedFilterID >= 2, 'join invite omitted updateDialogFilter');
    const chatlist = inputChatlist(joinedFilterID);
    const updates = await bob.invoke(getChatlistUpdates(joinedFilterID));
    check(hasConstructor(updates, 'ChatlistUpdates') && (updates.missingPeers || []).length === 0, 'get chatlist updates was not empty after join');
    const joinedUpdates = await bob.invoke(joinChatlistUpdates(joinedFilterID, [aPeer]));
    check(rpcName(joinedUpdates).toLowerCase().includes('updates'), 'join chatlist updates did not return Updates');
    check(isTrue(await bob.invoke(hideChatlistUpdates(joinedFilterID))), 'hide chatlist updates failed');
    const hidden = await bob.invoke(getChatlistUpdates(joinedFilterID));
    check(hasConstructor(hidden, 'ChatlistUpdates') && (hidden.missingPeers || []).length === 0, 'hidden chatlist updates not empty');
    const suggestions = await bob.invoke(new Api.chatlists.GetLeaveChatlistSuggestions({ chatlist }));
    check(Array.isArray(suggestions) && suggestions.length === 1, 'leave suggestions missing joined peer');
    const left = await bob.invoke(new Api.chatlists.LeaveChatlist({ chatlist, peers: [aPeer] }));
    check(rpcName(left).toLowerCase().includes('updates'), 'leave chatlist did not return Updates');
    check(isTrue(await alice.invoke(new Api.chatlists.DeleteExportedInvite({ chatlist: inputChatlist(filterID), slug }))), 'delete exported invite failed');
    deletedInvite = true;
    const afterDelete = await alice.invoke(new Api.chatlists.GetExportedInvites({ chatlist: inputChatlist(filterID) }));
    check(hasConstructor(afterDelete, 'ExportedInvites') && afterDelete.invites?.length === 0, 'deleted invite still listed');
    operation.layer = 229;
    operation.transport = 'DC2 TCPObfuscated -> session -> APIFull -> Dialog/PostgreSQL';
    operation.users = { owner: aID.toString(), joiner: bID.toString() };
    operation.filterID = filterID;
    operation.joinedFilterID = joinedFilterID;
    operation.slug = slug;
    operation.methods = {
      export: rpcName(exported), getExported: rpcName(listed), edit: rpcName(edited),
      check: rpcName(checked), join: rpcName(joined), getUpdates: rpcName(updates),
      joinUpdates: rpcName(joinedUpdates), hideUpdates: 'BoolTrue',
      hiddenGetUpdates: rpcName(hidden), suggestions: 'VectorPeer', leave: rpcName(left), delete: 'BoolTrue',
    };
  } finally {
    const errors: string[] = [];
    try {
      if (slug && !deletedInvite) await alice.invoke(new Api.chatlists.DeleteExportedInvite({ chatlist: inputChatlist(filterID), slug }));
    } catch (error) { errors.push(`invite cleanup: ${String(error)}`); }
    try { if (joinedFilterID > 0) await bob.invoke(new Api.messages.UpdateDialogFilter({ id: joinedFilterID })); } catch (error) { errors.push(`join filter cleanup: ${String(error)}`); }
    try { if (filterID > 0) await alice.invoke(new Api.messages.UpdateDialogFilter({ id: filterID })); } catch (error) { errors.push(`owner filter cleanup: ${String(error)}`); }
    try { if (filterID > 0) await postgres(`DELETE FROM dialog_filters WHERE user_id=${aID} AND dialog_filter_id=${filterID}`); } catch (error) { errors.push(`owner filter row cleanup: ${String(error)}`); }
    try { if (joinedFilterID > 0) await postgres(`DELETE FROM dialog_filters WHERE user_id=${bID} AND dialog_filter_id=${joinedFilterID}`); } catch (error) { errors.push(`join filter row cleanup: ${String(error)}`); }
    try { if (before.aState === 0) await postgres(`DELETE FROM apifull_chatlist_state WHERE user_id=${aID}`); } catch (error) { errors.push(`owner state cleanup: ${String(error)}`); }
    try { if (before.bState === 0) await postgres(`DELETE FROM apifull_chatlist_state WHERE user_id=${bID}`); } catch (error) { errors.push(`joiner state cleanup: ${String(error)}`); }
    try { if (before.aInvites === 0) await postgres(`DELETE FROM apifull_chatlist_invite WHERE owner_user_id=${aID}`); } catch (error) { errors.push(`invite row cleanup: ${String(error)}`); }
    const after = {
      aState: stateCount(aID), bState: stateCount(bID), aInvites: inviteCount(aID), bInvites: inviteCount(bID),
      ownerFilterRows: filterID > 0 ? filterCount(aID, filterID) : 0,
      joinerFilterRows: joinedFilterID > 0 ? filterCount(bID, joinedFilterID) : 0,
      errors,
    };
    cleanup = { ok: errors.length === 0 && after.aState === 0 && after.bState === 0 && after.aInvites === 0 && after.bInvites === 0 && after.ownerFilterRows === 0 && after.joinerFilterRows === 0, after };
    try { alice.destroy(); } catch {}
    try { bob.destroy(); } catch {}
    if (!cleanup.ok) throw new Error(`cleanup failed: ${JSON.stringify(cleanup)}`);
  }
  const report = { schemaVersion: 1, probe: 'chatlists.layer229.session', ok: true, before, operation, cleanup };
  execFileSync('mkdir', ['-p', path.dirname(reportPath)]);
  execFileSync('sh', ['-c', `umask 077; printf '%s\\n' "$1" > "$2"`, 'write-report', JSON.stringify(report, null, 2), reportPath]);
  process.stdout.write(`${JSON.stringify({ ...report, reportPath })}\n`);
}

main().catch((error) => {
  const report = { schemaVersion: 1, probe: 'chatlists.layer229.session', ok: false, error: error instanceof Error ? error.message : String(error), reportPath };
  try { execFileSync('sh', ['-c', `umask 077; printf '%s\\n' "$1" > "$2"`, 'write-report', JSON.stringify(report, null, 2), reportPath]); } catch {}
  console.error(JSON.stringify(report));
  process.exit(1);
});
