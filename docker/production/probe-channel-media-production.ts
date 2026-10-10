import { randomBytes } from 'node:crypto';
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

function postgres(query: string): string {
  return execFileSync('docker', [
    'exec', 'teamgram-postgres', 'psql', '-v', 'ON_ERROR_STOP=1', '-At', '-q', '-F', '\t', '-U', 'teamgram', '-d', 'teamgram', '-c', query,
  ], { encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'] }).trim();
}

function requiredValue(query: string, name: string): string {
  const value = postgres(query);
  if (!value) throw new Error(`${name} is absent from production PostgreSQL`);
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

function updateChats(value: any): any[] {
  return [
    ...(Array.isArray(value?.chats) ? value.chats : []),
    ...(Array.isArray(value?.updates?.chats) ? value.updates.chats : []),
  ];
}

function findMessage(value: any, expected: string, seen = new Set<any>()): any | undefined {
  if (!value || typeof value !== 'object' || seen.has(value)) return undefined;
  seen.add(value);
  if (value.message === expected) return value;
  for (const item of Array.isArray(value) ? value : Object.values(value)) {
    const found = findMessage(item, expected, seen);
    if (found) return found;
  }
  return undefined;
}

function hasMessage(value: any, expected: string, seen = new Set<any>()): boolean {
  return findMessage(value, expected, seen) !== undefined;
}

function channelRows(title: string): string {
  return postgres(`SELECT id, access_hash FROM apifull_channel WHERE creator_user_id=${userId} AND title='${title}' ORDER BY id DESC LIMIT 1`);
}

function messageRows(channelId: bigint, text: string): Array<{ messageId: number; groupedId: string; media: string }> {
  const rows = postgres(`
    SELECT message_id,
      COALESCE(content_json::jsonb->>'grouped_id', '0'),
      COALESCE(content_json::jsonb #>> '{media,predicate_name}', '')
    FROM apifull_channel_message
    WHERE channel_id=${channelId} AND message='${text}'
    ORDER BY message_id
  `);
  if (!rows) return [];
  return rows.split('\n').map((row) => {
    const [messageId, groupedId, media] = row.split('\t');
    return { messageId: Number(messageId), groupedId, media };
  });
}

function rowCounts(channelId: bigint): Record<string, number> {
  const checks: Record<string, string> = {
    channel: `SELECT COUNT(*) FROM apifull_channel WHERE id=${channelId}`,
    members: `SELECT COUNT(*) FROM apifull_channel_member WHERE channel_id=${channelId}`,
    messages: `SELECT COUNT(*) FROM apifull_channel_message WHERE channel_id=${channelId}`,
    requests: `SELECT COUNT(*) FROM apifull_channel_message_request WHERE channel_id=${channelId}`,
    sequence: `SELECT COUNT(*) FROM apifull_channel_message_seq WHERE channel_id=${channelId}`,
    events: `SELECT COUNT(*) FROM apifull_channel_event WHERE channel_id=${channelId}`,
    hidden: `SELECT COUNT(*) FROM apifull_channel_message_hidden WHERE channel_id=${channelId}`,
    contentReads: `SELECT COUNT(*) FROM apifull_channel_message_content_read WHERE channel_id=${channelId}`,
    readState: `SELECT COUNT(*) FROM apifull_channel_read_state WHERE channel_id=${channelId}`,
    adminLog: `SELECT COUNT(*) FROM apifull_channel_admin_log WHERE channel_id=${channelId}`,
  };
  return Object.fromEntries(Object.entries(checks).map(([name, query]) => [name, Number(postgres(query))]));
}

function inputContact(firstName: string): any {
  return new Api.InputMediaContact({
    phoneNumber: '+14155550101',
    firstName,
    lastName: 'Production Probe',
    vcard: '',
  });
}

function historyRequest(peer: any): any {
  return new Api.messages.GetHistory({
    peer,
    offsetId: 0,
    offsetDate: 0,
    addOffset: 0,
    limit: 100,
    maxId: 0,
    minId: 0,
    hash: 0n,
  });
}

async function main() {
  const authKey = requiredValue(`
    SELECT encode(k.body, 'base64')
    FROM auth_users u
    JOIN auth_keys k USING (auth_key_id)
    JOIN auth_key_infos i USING (auth_key_id)
    WHERE u.user_id=${userId} AND u.state=0 AND u.deleted=0
      AND k.deleted=0 AND i.deleted=0 AND i.auth_key_type=0
    ORDER BY u.date_active DESC,u.id DESC
    LIMIT 1
  `, 'active production auth key');
  const client = makeClient(authKey);
  const title = `r28-channel-media-${Date.now()}-${randomBytes(4).toString('hex')}`;
  const tag = `${title}-content`;
  let channelId = 0n;
  let accessHash = 0n;
  let failure: unknown;
  const evidence: Record<string, unknown> = {};

  try {
    await client.connect();
    const me = await client.getMe();
    if (String(me?.id) !== String(userId)) throw new Error('production auth key user mismatch');

    const created = await client.invoke(new Api.channels.CreateChannel({
      broadcast: true,
      title,
      about: 'r28 production channel media probe',
    }));
    const channel = updateChats(created).find((item: any) => item?.title === title);
    if (!channel?.id || channel.accessHash === undefined) {
      throw new Error(`channel creation returned ${rpcName(created)} without channel identity`);
    }
    channelId = BigInt(String(channel.id));
    accessHash = BigInt(String(channel.accessHash));
    const inputChannel = new Api.InputChannel({ channelId, accessHash });
    const peer = new Api.InputPeerChannel({ channelId, accessHash });
    const baseRandomId = BigInt(Date.now()) * 1000n + BigInt(randomBytes(2).readUInt16BE(0));

    const singleText = `${tag}-single`;
    const singleRequest = new Api.messages.SendMedia({
      peer,
      media: inputContact(`${tag}-single`),
      message: singleText,
      randomId: baseRandomId,
    });
    const single = await client.invoke(singleRequest);
    if (!rpcName(single).toLowerCase().includes('updates')) throw new Error(`channel sendMedia returned ${rpcName(single)}`);
    const singleRows = messageRows(channelId, singleText);
    if (singleRows.length !== 1 || !singleRows[0].media.toLowerCase().includes('messagemediacontact')) {
      throw new Error(`single media row missing or malformed: ${JSON.stringify(singleRows)}`);
    }
    const singleEventCount = Number(postgres(`SELECT COUNT(*) FROM apifull_channel_event WHERE channel_id=${channelId}`));
    const singleReplay = await client.invoke(singleRequest);
    const singleRowsAfterReplay = messageRows(channelId, singleText);
    const singleEventsAfterReplay = Number(postgres(`SELECT COUNT(*) FROM apifull_channel_event WHERE channel_id=${channelId}`));
    if (!rpcName(singleReplay).toLowerCase().includes('updates')
      || singleRowsAfterReplay.length !== 1
      || singleRowsAfterReplay[0].messageId !== singleRows[0].messageId
      || singleEventsAfterReplay !== singleEventCount) {
      throw new Error(`single media retry changed persisted state: rows=${JSON.stringify(singleRowsAfterReplay)} events=${singleEventsAfterReplay}`);
    }
    const historyAfterSingle = await client.invoke(historyRequest(peer));
    const singleHistoryMessage = findMessage(historyAfterSingle, singleText);
    if (!singleHistoryMessage || !rpcName(singleHistoryMessage.media).toLowerCase().includes('messagemediacontact')) {
      throw new Error(`channel history did not hydrate single media: ${rpcName(historyAfterSingle)}`);
    }
    evidence.single = rpcName(single);
    evidence.singleRetry = rpcName(singleReplay);
    evidence.singleMessageId = singleRows[0].messageId;
    evidence.singleHistoryMedia = rpcName(singleHistoryMessage.media);

    const albumTexts = [`${tag}-album-one`, `${tag}-album-two`];
    const albumRandomIds = [baseRandomId + 1n, baseRandomId + 2n];
    const albumRequest = new Api.messages.SendMultiMedia({
      peer,
      multiMedia: albumTexts.map((message, index) => new Api.InputSingleMedia({
        media: inputContact(`${tag}-album-${index + 1}`),
        message,
        randomId: albumRandomIds[index],
      })),
    });
    const album = await client.invoke(albumRequest);
    if (!rpcName(album).toLowerCase().includes('updates')) throw new Error(`channel sendMultiMedia returned ${rpcName(album)}`);
    const albumRows = albumTexts.map((text) => messageRows(channelId, text)[0]);
    if (albumRows.some((row) => !row || !row.media.toLowerCase().includes('messagemediacontact') || row.groupedId === '0')) {
      throw new Error(`album media/group metadata missing: ${JSON.stringify(albumRows)}`);
    }
    if (new Set(albumRows.map((row) => row.groupedId)).size !== 1) throw new Error('album items do not share grouped_id');
    const eventsBeforeAlbumReplay = Number(postgres(`SELECT COUNT(*) FROM apifull_channel_event WHERE channel_id=${channelId}`));
    const albumReplay = await client.invoke(albumRequest);
    const albumRowsAfterReplay = albumTexts.map((text) => messageRows(channelId, text)[0]);
    const eventsAfterAlbumReplay = Number(postgres(`SELECT COUNT(*) FROM apifull_channel_event WHERE channel_id=${channelId}`));
    if (!rpcName(albumReplay).toLowerCase().includes('updates')
      || albumRowsAfterReplay.some((row, index) => !row || row.messageId !== albumRows[index].messageId || row.groupedId !== albumRows[index].groupedId)
      || eventsAfterAlbumReplay !== eventsBeforeAlbumReplay) {
      throw new Error(`album retry changed persisted state: rows=${JSON.stringify(albumRowsAfterReplay)} events=${eventsAfterAlbumReplay}`);
    }
    const historyAfterAlbum = await client.invoke(historyRequest(peer));
    for (const text of albumTexts) {
      const message = findMessage(historyAfterAlbum, text);
      if (!message || !rpcName(message.media).toLowerCase().includes('messagemediacontact')) {
        throw new Error(`channel history missing media for ${text}`);
      }
      if (String(message.groupedId) !== albumRows[0].groupedId) {
        throw new Error(`history grouped_id mismatch for ${text}: got=${String(message.groupedId)} want=${albumRows[0].groupedId}`);
      }
    }
    const difference = await client.invoke(new Api.updates.GetChannelDifference({
      force: true,
      channel: inputChannel,
      filter: new Api.ChannelMessagesFilterEmpty(),
      pts: 0,
      limit: 100,
    }));
    const differenceAlbum = albumTexts.map((text) => {
      const message = findMessage(difference, text);
      return { text, found: Boolean(message), media: rpcName(message?.media), groupedId: message?.groupedId };
    });
    const differenceMessages = (Array.isArray(difference?.newMessages) ? difference.newMessages : [])
      .filter((message: any) => String(message?.message || '').startsWith(tag))
      .map((message: any) => ({
        text: message.message,
        media: rpcName(message.media),
        rawMedia: message.media,
        json: typeof message.toJSON === 'function' ? message.toJSON() : undefined,
        keys: Object.keys(message),
      }));
    if (!rpcName(difference).toLowerCase().includes('channeldifference')
      || !albumTexts.every((text) => hasMessage(difference, text))
      || differenceAlbum.some((item) => !item.media.toLowerCase().includes('messagemediacontact'))) {
      const historyMessages = albumTexts.map((text) => {
        const message = findMessage(historyAfterAlbum, text);
        return { text, media: rpcName(message?.media), mediaKeys: message?.media ? Object.keys(message.media) : [] };
      });
      throw new Error(`channel difference did not hydrate album media: ${rpcName(difference)} ${JSON.stringify({ differenceAlbum, differenceMessages, historyMessages, content: postgres(`SELECT string_agg(content_json, '|') FROM apifull_channel_event WHERE channel_id=${channelId} AND event_type='new'`) })}`);
    }
    evidence.album = rpcName(album);
    evidence.albumRetry = rpcName(albumReplay);
    evidence.albumMessageIds = albumRows.map((row) => row.messageId);
    evidence.albumGroupedId = albumRows[0].groupedId;
    evidence.albumHistoryMedia = albumTexts.map((text) => rpcName(findMessage(historyAfterAlbum, text)?.media));
    evidence.difference = rpcName(difference);

    const deleted = await client.invoke(new Api.channels.DeleteChannel({ channel: inputChannel }));
    if (!rpcName(deleted).toLowerCase().includes('updates')) throw new Error(`channel delete returned ${rpcName(deleted)}`);
    evidence.delete = rpcName(deleted);
  } catch (error) {
    failure = error;
  }

  if (channelId === 0n) {
    try {
      const row = channelRows(title);
      if (row) {
        const [id, hash] = row.split('\t');
        channelId = BigInt(id);
        accessHash = BigInt(hash);
      }
    } catch (error) {
      evidence.lookupError = String(error);
    }
  }

  if (channelId > 0n) {
    try {
      if (Number(postgres(`SELECT COUNT(*) FROM apifull_channel WHERE id=${channelId}`)) > 0) {
        await client.invoke(new Api.channels.DeleteChannel({
          channel: new Api.InputChannel({ channelId, accessHash }),
        }));
        evidence.cleanup = 'delete retried';
      }
    } catch (error) {
      evidence.cleanupError = String(error);
    }
  }
  client.destroy();

  if (channelId === 0n) throw failure || new Error('production probe did not create a channel');
  const counts = rowCounts(channelId);
  evidence.deletedRows = counts;
  if (Object.values(counts).some((count) => count !== 0)) {
    throw new Error(`temporary production channel cleanup left rows: ${JSON.stringify(evidence)}`);
  }
  if (failure) throw failure;
  console.log(JSON.stringify(evidence));
}

main().then(() => process.exit(0)).catch((error) => {
  console.error(`production channel media probe failed: ${error instanceof Error ? error.stack || error.message : String(error)}`);
  process.exit(1);
});
