import { execFileSync } from 'node:child_process';
import path from 'node:path';
import { createRequire } from 'node:module';

for (const key of ['TEAMGRAM_GRAMJS_DIR', 'AUTH_B64', 'USER_ID']) {
  if (!process.env[key]) throw new Error(`${key} is required`);
}

const gramjsDir = process.env.TEAMGRAM_GRAMJS_DIR!;
const dcId = Number(process.env.TG_DC_ID || '2');
const dcHost = process.env.TG_DC_HOST || '127.0.0.1';
const dcPort = Number(process.env.TG_DC_PORT || '11443');
const apiId = Number(process.env.TG_API_ID || '1');
const apiHash = process.env.TG_API_HASH || '0123456789abcdef0123456789abcdef';
const userId = BigInt(process.env.USER_ID!);
const tag = `codex-unique-theme-${Date.now()}-${process.pid}`;

const globalAny = globalThis as typeof globalThis & { self?: any; addEventListener?: () => void };
globalAny.self ??= globalThis;
globalAny.addEventListener ??= () => {};
globalAny.self.addEventListener ??= globalAny.addEventListener;

const requireModule = createRequire(import.meta.url);
const TelegramClient = requireModule(path.join(gramjsDir, 'src/lib/gramjs/client/TelegramClient.ts')).default;
const CallbackSession = requireModule(path.join(gramjsDir, 'src/lib/gramjs/sessions/CallbackSession.ts')).default;
const Api = requireModule(path.join(gramjsDir, 'src/lib/gramjs/tl/index.ts')).Api;
const serializeBytes = requireModule(path.join(gramjsDir, 'src/lib/gramjs/tl/index.ts')).serializeBytes;
const Connection = requireModule(path.join(gramjsDir, 'src/lib/gramjs/network/connection/TCPObfuscated.ts')).ConnectionTCPObfuscated;
const quietLogger = { debug() {}, info() {}, warn() {}, error() {} };

class RawRequest {
  classType = 'request';
  constructor(
    public readonly CONSTRUCTOR_ID: number,
    private readonly payload: Uint8Array,
    public readonly className: string,
  ) {}

  getBytes(): Uint8Array {
    const constructor = Buffer.alloc(4);
    constructor.writeUInt32LE(this.CONSTRUCTOR_ID >>> 0);
    return Buffer.concat([constructor, Buffer.from(this.payload)]);
  }

  readResult(reader: any): any {
    return reader.tgReadObject();
  }
}

function int32(value: number): Uint8Array {
  const bytes = Buffer.alloc(4);
  bytes.writeInt32LE(value);
  return bytes;
}

function int64(value: bigint): Uint8Array {
  const bytes = Buffer.alloc(8);
  bytes.writeBigInt64LE(value);
  return bytes;
}

function rawGetUniqueGiftChatThemes(): RawRequest {
  return new RawRequest(0xe42ce9c9, Buffer.concat([
    Buffer.from(serializeBytes('')),
    Buffer.from(int32(10)),
    Buffer.from(int64(0n)),
  ]), 'account.getUniqueGiftChatThemes');
}

function sql(query: string): string {
  return execFileSync('docker', [
    'exec', 'teamgram-postgres', 'psql', '-v', 'ON_ERROR_STOP=1', '-At', '-q',
    '-U', 'teamgram', '-d', 'teamgram', '-c', query,
  ], { encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] }).trim();
}

function quote(value: string): string {
  return `'${value.replaceAll("'", "''")}'`;
}

function makeClient(authKeyBase64: string): any {
  const authKeyHex = Buffer.from(authKeyBase64, 'base64').toString('hex');
  const session = new CallbackSession({ mainDcId: dcId, keys: { [dcId]: authKeyHex } }, () => {});
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

function typeName(value: any): string {
  return value?.className || value?.constructor?.name || typeof value;
}

function seed(): bigint {
  const id = sql(`
    INSERT INTO apifull_gift (from_user, to_user, slug, stars, saved)
    VALUES (${userId}, ${userId}, ${quote(tag)}, 250, 1)
    RETURNING id;
  `);
  if (!/^\d+$/.test(id)) throw new Error(`gift seed did not return an id: ${id}`);
  const giftId = BigInt(id);
  sql(`INSERT INTO apifull_gift_chat_theme (gift_id) VALUES (${giftId}) ON CONFLICT (gift_id) DO UPDATE SET created_at=CURRENT_TIMESTAMP;`);
  return giftId;
}

function cleanup(giftId: bigint | undefined): number {
  if (giftId === undefined) return 0;
  return Number(sql(`
    WITH deleted AS (
      DELETE FROM apifull_gift WHERE id=${giftId} AND slug=${quote(tag)} RETURNING id
    ) SELECT count(*) FROM deleted;
  `));
}

async function run(): Promise<void> {
  const giftId = seed();
  const client = makeClient(process.env.AUTH_B64!);
  let deleted = 0;
  try {
    await client.connect();
    const me = await client.getMe();
    if (String(me?.id) !== userId.toString()) throw new Error('the authenticated account does not match USER_ID');
    const result = await client.invoke(rawGetUniqueGiftChatThemes());
    if (typeName(result) !== 'account.ChatThemes' && typeName(result) !== 'ChatThemes') {
      throw new Error(`expected account.ChatThemes, got ${typeName(result)}`);
    }
    const themes = Array.isArray(result.themes) ? result.themes : [];
    const theme = themes.find((candidate: any) => String(candidate?.gift?.id) === giftId.toString());
    if (!theme || typeName(theme) !== 'ChatThemeUniqueGift') {
      throw new Error(`seeded gift theme was not returned: ${themes.map(typeName).join(',')}`);
    }
    if (!theme.gift || (typeName(theme.gift) !== 'StarGift' && typeName(theme.gift) !== 'StarGiftUnique')) {
      throw new Error(`unexpected gift constructor: ${typeName(theme.gift)}`);
    }
    if (!theme.gift.sticker || (typeName(theme.gift.sticker) !== 'DocumentEmpty' && typeName(theme.gift.sticker) !== 'Document')) {
      throw new Error(`unexpected sticker constructor: ${typeName(theme.gift.sticker)}`);
    }
    console.log(JSON.stringify({
      transport: `DC${dcId} TCPObfuscated -> gateway -> session -> APIFull/PostgreSQL`,
      userId: userId.toString(),
      giftId: giftId.toString(),
      constructor: typeName(result),
      themeConstructor: typeName(theme),
      giftConstructor: typeName(theme.gift),
      stickerConstructor: typeName(theme.gift.sticker),
      hash: String(result.hash ?? ''),
    }));
  } finally {
    client.destroy();
    deleted = cleanup(giftId);
  }
  if (deleted !== 1) throw new Error(`expected one seeded gift cleanup, deleted ${deleted}`);
}

run().then(() => process.exit(0)).catch((error) => {
  console.error(`unique gift chat themes probe failed: ${error instanceof Error ? error.stack || error.message : String(error)}`);
  process.exit(1);
});
