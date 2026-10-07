import path from 'node:path';
import { createRequire } from 'node:module';

for (const key of ['TEAMGRAM_GRAMJS_DIR', 'AUTH_B64']) {
  if (!process.env[key]) throw new Error(`${key} is required`);
}

const gramjsDir = process.env.TEAMGRAM_GRAMJS_DIR!;
const dcId = Number(process.env.TG_DC_ID || '2');
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

function rpcName(value: any): string {
  return value?.className || value?.constructor?.name || typeof value;
}

function tlInt(value: number): Uint8Array {
  const bytes = new Uint8Array(4);
  new DataView(bytes.buffer).setInt32(0, value, true);
  return bytes;
}

class GetSearchResultsCalendarRequest {
  CONSTRUCTOR_ID = 0x6aa3f6bd;
  SUBCLASS_OF_ID = 0;
  className = 'messages.GetSearchResultsCalendar';
  classType = 'request';

  constructor(private readonly peer: any, private readonly filter: any) {}

  getBytes() {
    return Uint8Array.from(Buffer.concat([
      Buffer.from(tlInt(this.CONSTRUCTOR_ID)),
      Buffer.from(tlInt(0)),
      Buffer.from(this.peer.getBytes()),
      Buffer.from(this.filter.getBytes()),
      Buffer.from(tlInt(0)),
      Buffer.from(tlInt(0)),
    ]));
  }

  readResult(reader: any) {
    return reader.tgReadObject();
  }
}

async function main() {
  const authKeyHex = Buffer.from(process.env.AUTH_B64!, 'base64').toString('hex');
  const session = new CallbackSession({ mainDcId: dcId, keys: { [dcId]: authKeyHex } }, () => {});
  const load = session.load.bind(session);
  session.load = async () => {
    await load();
    (session as any)._serverAddress = process.env.TG_DC_HOST || '127.0.0.1';
    (session as any)._port = Number(process.env.TG_DC_PORT || '11443');
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
  try {
    await client.connect();
    await client.getMe();
    const dialogs = await client.invoke(new Api.messages.GetDialogs({
      offsetDate: 0,
      offsetId: 0,
      offsetPeer: new Api.InputPeerEmpty(),
      limit: 50,
      hash: 0n,
    }));
    const users = new Map((dialogs?.users || []).map((user: any) => [String(user?.id), user]));
    const candidates: Array<{ peer: any; kind: string }> = [{ peer: new Api.InputPeerSelf(), kind: 'self' }];
    const seen = new Set(['self']);
    for (const dialog of Array.isArray(dialogs?.dialogs) ? dialogs.dialogs : []) {
      const dialogPeer = dialog?.peer;
      if (rpcName(dialogPeer).endsWith('PeerUser')) {
        const user = users.get(String(dialogPeer.userId)) as any;
        if (user?.accessHash === undefined) continue;
        const key = `user:${String(user.id)}`;
        if (seen.has(key)) continue;
        seen.add(key);
        candidates.push({
          peer: new Api.InputPeerUser({ userId: user.id, accessHash: user.accessHash }),
          kind: 'user',
        });
      } else if (rpcName(dialogPeer).endsWith('PeerChat')) {
        const key = `chat:${String(dialogPeer.chatId)}`;
        if (seen.has(key)) continue;
        seen.add(key);
        candidates.push({ peer: new Api.InputPeerChat({ chatId: dialogPeer.chatId }), kind: 'chat' });
      }
    }
    const filter = new Api.InputMessagesFilterPhotoVideo();
    let emptySamples = 0;
    for (const candidate of candidates.slice(0, 12)) {
      const search = await client.invoke(new Api.messages.Search({
        peer: candidate.peer,
        q: '',
        filter,
        minDate: 0,
        maxDate: 0,
        offsetId: 0,
        addOffset: 0,
        limit: 50,
        maxId: 0,
        minId: 0,
        hash: 0n,
      }));
      const calendar = await client.invoke(new GetSearchResultsCalendarRequest(candidate.peer, filter));
      const searchMessages = Array.isArray(search?.messages) ? search.messages : [];
      const periods = Array.isArray(calendar?.periods) ? calendar.periods : [];
      const representatives = Array.isArray(calendar?.messages) ? calendar.messages : [];
      const periodCount = periods.reduce((sum: number, item: any) => sum + Number(item?.count || 0), 0);

      if (!rpcName(calendar).toLowerCase().includes('searchresultscalendar')) {
        throw new Error(`unexpected calendar constructor: ${rpcName(calendar)}`);
      }
      if (Number(calendar?.count) !== searchMessages.length || periodCount !== Number(calendar?.count)) {
        throw new Error(`search/calendar count mismatch: search=${searchMessages.length}, calendar=${calendar?.count}, periods=${periodCount}`);
      }
      if (representatives.length !== periods.length) {
        throw new Error(`calendar representatives=${representatives.length}, periods=${periods.length}`);
      }
      emptySamples++;
      if (searchMessages.length === 0) continue;

      console.log(JSON.stringify({
        searchType: rpcName(search),
        searchCount: searchMessages.length,
        calendarType: rpcName(calendar),
        calendarCount: Number(calendar.count),
        periodCount: periods.length,
        representativeCount: representatives.length,
        peerKind: candidate.kind,
        precedingEmptySamples: emptySamples - 1,
      }));
      return;
    }

    console.log(JSON.stringify({
      testedPeers: emptySamples,
      result: 'valid_empty_media_search',
    }));
  } finally {
    client.destroy();
  }
}

main().catch((error) => {
  console.error(error);
  process.exit(1);
}).then(() => {
  process.exit(0);
});
