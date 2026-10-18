import { execFileSync } from 'node:child_process';
import path from 'node:path';
import { createRequire } from 'node:module';

const gramjsDir = process.env.TEAMGRAM_GRAMJS_DIR || '/Users/mac/open/test/telegram-tt-master';
const postgresContainer = process.env.TEAMGRAM_POSTGRES_CONTAINER || 'teamgram-postgres';
const dcId = Number(process.env.TG_DC_ID || '2');
const dcHost = process.env.TG_DC_HOST || '127.0.0.1';
const dcPort = Number(process.env.TG_DC_PORT || '11443');
const aID = BigInt(process.env.A_USER_ID || '7');
const bID = BigInt(process.env.B_USER_ID || '8');
const apiId = Number(process.env.TG_API_ID || '1');
const apiHash = process.env.TG_API_HASH || '0123456789abcdef0123456789abcdef';

if (dcHost !== '127.0.0.1' || dcPort !== 11443 || dcId !== 2) {
  throw new Error('probe requires DC2 TCPObfuscated at 127.0.0.1:11443');
}
if (!/^\d+$/.test(aID.toString()) || !/^\d+$/.test(bID.toString())) {
  throw new Error('fixture user IDs must be positive integers');
}

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
    FROM auth_users u
    JOIN auth_keys k USING (auth_key_id)
    JOIN auth_key_infos i USING (auth_key_id)
    WHERE u.user_id=${userID} AND u.state=0 AND NOT u.deleted
      AND NOT k.deleted AND NOT i.deleted AND i.auth_key_type=0
    ORDER BY u.date_active DESC,u.id DESC LIMIT 1`);
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

function tlInt(value: number): Uint8Array {
  const bytes = new Uint8Array(4);
  new DataView(bytes.buffer).setUint32(0, value >>> 0, true);
  return bytes;
}

function rawRequest(constructorId: number, className: string, ...parts: Uint8Array[]): any {
  return {
    CONSTRUCTOR_ID: constructorId,
    SUBCLASS_OF_ID: 0,
    className,
    classType: 'request',
    getBytes: () => Uint8Array.from(Buffer.concat([
      Buffer.from(tlInt(constructorId)),
      ...parts.map((part) => Buffer.from(part)),
    ])),
    readResult: (reader: any) => reader.tgReadObject(),
  };
}

function isDialogFilters(value: any): boolean {
  const name = rpcName(value);
  return name === 'DialogFilters' || name.endsWith('.DialogFilters');
}

function dialogFiltersShape(value: any): Record<string, unknown> {
  return {
    rpcName: rpcName(value),
    keys: value && typeof value === 'object' ? Object.keys(value) : [],
    filters: Array.isArray(value?.filters)
      ? value.filters.map((item: any) => ({
          rpcName: rpcName(item),
          keys: item && typeof item === 'object' ? Object.keys(item) : [],
          id: item?.id,
          title: rpcName(item?.title),
          includePeers: Array.isArray(item?.includePeers) ? item.includePeers.length : null,
          excludePeers: Array.isArray(item?.excludePeers) ? item.excludePeers.length : null,
          pinnedPeers: Array.isArray(item?.pinnedPeers) ? item.pinnedPeers.length : null,
        }))
      : null,
    tagsEnabled: value?.tagsEnabled,
  };
}

function isTrue(value: any): boolean {
  return value === true || Boolean(value?.valueOf?.() === true) || rpcName(value).toLowerCase().includes('true');
}

function requireCondition(condition: unknown, message: string): asserts condition {
  if (!condition) throw new Error(message);
}

function makeClient(auth: string): any {
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
    requireCondition(rpcName(original) === 'InvokeWithLayer' && rpcName(original.query) === 'InitConnection', 'INVALID_INIT_CONNECTION_WRAPPER');
    return new Api.InvokeWithLayer({ layer: 229, query: original.query });
  };
  (client as any)._borrowExportedSender = async () => undefined;
  (client as any)._updateLoop = async () => undefined;
  return client;
}

function dialogFilter(id: number, title: string, peer: any): any {
  return new Api.DialogFilter({
    id,
    title: new Api.TextWithEntities({ text: title, entities: [] }),
    excludeMuted: true,
    excludeArchived: true,
    pinnedPeers: [],
    includePeers: [peer],
    excludePeers: [],
  });
}

function filterIDs(value: any): number[] {
  return (Array.isArray(value?.filters) ? value.filters : [])
    .map((item: any) => Number(item?.id))
    .filter((id: number) => Number.isInteger(id) && id >= 2);
}

function filterByID(value: any, id: number): any {
  return (Array.isArray(value?.filters) ? value.filters : []).find((item: any) => Number(item?.id) === id);
}

function savedDialogRow(userID: bigint): string {
  return postgres(`
    SELECT json_build_object('pinned',pinned,'top_message',top_message,
      'read_max_id',read_max_id,'deleted',deleted)::text
    FROM saved_dialogs
    WHERE user_id=${userID} AND peer_type=2 AND peer_id=${userID}
    LIMIT 1`);
}

function restoreSavedDialog(userID: bigint, snapshot: string): void {
  if (!snapshot) {
    postgres(`DELETE FROM saved_dialogs WHERE user_id=${userID} AND peer_type=2 AND peer_id=${userID}`);
    return;
  }
  const value = JSON.parse(snapshot) as { pinned: number; top_message: number; read_max_id: number; deleted: boolean };
  postgres(`
    UPDATE saved_dialogs
    SET pinned=${Number(value.pinned)}, top_message=${Number(value.top_message)},
      read_max_id=${Number(value.read_max_id)}, deleted=${value.deleted ? 'TRUE' : 'FALSE'}
    WHERE user_id=${userID} AND peer_type=2 AND peer_id=${userID}`);
}

function savedHistory(peer: any): any {
  return new Api.messages.GetSavedHistory({
    peer,
    offsetId: 0,
    offsetDate: 0,
    addOffset: 0,
    limit: 50,
    maxId: 0,
    minId: 0,
    hash: 0n,
  });
}

function savedDialogs(): any {
  return new Api.messages.GetSavedDialogs({
    offsetDate: 0,
    offsetId: 0,
    offsetPeer: new Api.InputPeerEmpty(),
    limit: 50,
    hash: 0n,
  });
}

async function main() {
  const alice = makeClient(authKey(aID, 'A_AUTH_B64'));
  const bob = makeClient(authKey(bID, 'B_AUTH_B64'));
  const bHash = accessHash(bID, 'B_ACCESS_HASH');
  const savedPeer = new Api.InputPeerSelf();
  const bobPeer = new Api.InputPeerUser({ userId: bID, accessHash: bHash });
  const tag = `codex-dialog-saved-${Date.now()}-${process.pid}`;
  const savedText = `${tag} saved`;
  const filterIDsToClean: number[] = [];
  const savedDialogBefore = savedDialogRow(aID);
  let savedMessageId = 0;
  let operation: Record<string, unknown> = {};
  let cleanup: Record<string, unknown> = {};

  try {
    await alice.connect();
    await bob.connect();
    const meA = await alice.getMe();
    const meB = await bob.getMe();
    requireCondition(String(meA?.id) === aID.toString() && String(meB?.id) === bID.toString(), 'AUTH_KEY_USER_MISMATCH');

    const before = await alice.invoke(new Api.messages.GetDialogFilters());
    const beforeBob = await bob.invoke(new Api.messages.GetDialogFilters());
    requireCondition(
      isDialogFilters(before) && isDialogFilters(beforeBob),
      `DIALOG_FILTERS_CONSTRUCTOR_MISMATCH ${JSON.stringify({ before: dialogFiltersShape(before), bob: dialogFiltersShape(beforeBob) })}`,
    );
    let firstID = 1_900_000_000 + (process.pid % 100_000);
    while (postgres(`SELECT count(*) FROM dialog_filters WHERE user_id=${aID} AND dialog_filter_id=${firstID}`) !== '0') firstID += 1;
    const secondID = firstID + 1;
    requireCondition(postgres(`SELECT count(*) FROM dialog_filters WHERE user_id=${aID} AND dialog_filter_id=${secondID}`) === '0', 'FILTER_ID_COLLISION');
    filterIDsToClean.push(firstID, secondID);
    const firstTitle = `${tag} one`;
    const secondTitle = `${tag} two`;

    requireCondition(isTrue(await alice.invoke(new Api.messages.UpdateDialogFilter({
      id: firstID, filter: dialogFilter(firstID, firstTitle, bobPeer),
    }))), 'UPDATE_DIALOG_FILTER_ONE_FAILED');
    requireCondition(isTrue(await alice.invoke(new Api.messages.UpdateDialogFilter({
      id: secondID, filter: dialogFilter(secondID, secondTitle, bobPeer),
    }))), 'UPDATE_DIALOG_FILTER_TWO_FAILED');
    const afterSave = await alice.invoke(new Api.messages.GetDialogFilters());
    requireCondition(filterByID(afterSave, firstID)?.title?.text === firstTitle, 'DIALOG_FILTER_ONE_READBACK_FAILED');
    requireCondition(filterByID(afterSave, secondID)?.title?.text === secondTitle, 'DIALOG_FILTER_TWO_READBACK_FAILED');
    requireCondition(!filterByID(await bob.invoke(new Api.messages.GetDialogFilters()), firstID), 'DIALOG_FILTER_LEAKED_TO_BOB');

    requireCondition(isTrue(await alice.invoke(new Api.messages.UpdateDialogFiltersOrder({ order: [secondID, firstID] }))), 'UPDATE_DIALOG_FILTER_ORDER_FAILED');
    const reordered = await alice.invoke(new Api.messages.GetDialogFilters());
    const order = filterIDs(reordered);
    requireCondition(order.indexOf(secondID) >= 0 && order.indexOf(firstID) >= 0 && order.indexOf(secondID) < order.indexOf(firstID), 'DIALOG_FILTER_ORDER_READBACK_FAILED');

    const sent = await alice.invoke(new Api.messages.SendMessage({
      peer: savedPeer,
      message: savedText,
      randomId: BigInt(Date.now()) * 1000n + 1n,
    }));
    requireCondition(rpcName(sent).toLowerCase().includes('updates'), `SAVED_SEND_CONSTRUCTOR_${rpcName(sent)}`);

    let history: any;
    let savedMessage: any;
    for (let attempt = 0; attempt < 12; attempt += 1) {
      history = await alice.invoke(savedHistory(savedPeer));
      savedMessage = (Array.isArray(history?.messages) ? history.messages : []).find((item: any) => item?.message === savedText);
      if (savedMessage) break;
      await new Promise((resolve) => setTimeout(resolve, 250));
    }
    requireCondition(rpcName(history).toLowerCase().includes('messages'), `SAVED_HISTORY_CONSTRUCTOR_${rpcName(history)}`);
    requireCondition(savedMessage?.id > 0, 'SAVED_HISTORY_MESSAGE_MISSING');
    savedMessageId = Number(savedMessage.id);

    let dialogs: any;
    let dialog: any;
    for (let attempt = 0; attempt < 12; attempt += 1) {
      dialogs = await alice.invoke(savedDialogs());
      requireCondition(rpcName(dialogs).toLowerCase().includes('saveddialog'), `SAVED_DIALOGS_CONSTRUCTOR_${rpcName(dialogs)}`);
      dialog = (Array.isArray(dialogs?.dialogs) ? dialogs.dialogs : []).find((item: any) => Number(item?.topMessage) === savedMessageId);
      if (dialog) break;
      await new Promise((resolve) => setTimeout(resolve, 250));
    }
    requireCondition(dialog, 'SAVED_DIALOGS_TOP_MESSAGE_MISSING');
    requireCondition((Array.isArray(dialogs?.messages) ? dialogs.messages : []).some((item: any) => Number(item?.id) === savedMessageId && item?.message === savedText), 'SAVED_DIALOGS_MESSAGE_HYDRATION_MISSING');

    let byID: any;
    for (let attempt = 0; attempt < 12; attempt += 1) {
      byID = await alice.invoke(rawRequest(
        0x6f6f9c96,
        'messages.GetSavedDialogsByID',
        tlInt(0),
        tlInt(0x1cb5c415),
        tlInt(1),
        new Api.InputPeerSelf().getBytes(),
      ));
      requireCondition(rpcName(byID).toLowerCase().includes('saveddialog'), `SAVED_DIALOGS_BY_ID_CONSTRUCTOR_${rpcName(byID)}`);
      if ((Array.isArray(byID?.dialogs) ? byID.dialogs : []).some((item: any) => Number(item?.topMessage) === savedMessageId)) break;
      await new Promise((resolve) => setTimeout(resolve, 250));
    }
    requireCondition((Array.isArray(byID?.dialogs) ? byID.dialogs : []).some((item: any) => Number(item?.topMessage) === savedMessageId), 'SAVED_DIALOGS_BY_ID_MISSING');

    operation = {
      layer: 229,
      transport: 'DC2 TCPObfuscated -> gateway -> session -> APIFull/PostgreSQL',
      users: [aID.toString(), bID.toString()],
      tag,
      dialogFilters: {
        before: filterIDs(before),
        afterSave: filterIDs(afterSave),
        reordered: order,
        crossAccountIsolation: true,
        constructors: [rpcName(before), rpcName(afterSave), rpcName(reordered)],
      },
      savedMessages: {
        send: rpcName(sent),
        history: rpcName(history),
        dialogs: rpcName(dialogs),
        dialogsByID: rpcName(byID),
        messageId: savedMessageId,
        text: savedText,
      },
    };
  } finally {
    const cleanupErrors: string[] = [];
    for (const id of [...filterIDsToClean].reverse()) {
      try { await alice.invoke(new Api.messages.UpdateDialogFilter({ id })); } catch (error) { cleanupErrors.push(`filter ${id}: ${String(error)}`); }
    }
    if (savedMessageId > 0) {
      try { await alice.invoke(new Api.messages.DeleteMessages({ revoke: true, id: [savedMessageId] })); } catch (error) { cleanupErrors.push(`saved message ${savedMessageId}: ${String(error)}`); }
    }
    try { restoreSavedDialog(aID, savedDialogBefore); } catch (error) { cleanupErrors.push(`saved dialog restore: ${String(error)}`); }
    if (filterIDsToClean.length > 0) {
      try { postgres(`DELETE FROM dialog_filters WHERE user_id=${aID} AND dialog_filter_id IN (${filterIDsToClean.join(',')})`); } catch (error) { cleanupErrors.push(`filter rows: ${String(error)}`); }
    }
    const filterRowsAfter = filterIDsToClean.length > 0
      ? Number(postgres(`SELECT count(*) FROM dialog_filters WHERE user_id=${aID} AND dialog_filter_id IN (${filterIDsToClean.join(',')})`))
      : 0;
    const savedDialogAfter = savedDialogRow(aID);
    cleanup = {
      savedMessageRowsAfter: savedMessageId > 0 ? Number(postgres(`SELECT count(*) FROM messages WHERE user_id=${aID} AND user_message_box_id=${savedMessageId} AND deleted=FALSE`)) : 0,
      savedDialogBefore,
      savedDialogAfter,
      savedDialogRestored: savedDialogAfter === savedDialogBefore,
      filterRowsAfter,
      errors: cleanupErrors,
    };
    alice.destroy();
    bob.destroy();
    if (cleanupErrors.length > 0) throw new Error(cleanupErrors.join('; '));
  }

  console.log(JSON.stringify({ ...operation, cleanup }));
}

main().then(() => process.exit(0)).catch((error) => {
  console.error(`dialog/saved Layer 229 probe failed: ${error instanceof Error ? error.stack || error.message : String(error)}`);
  process.exit(1);
});
