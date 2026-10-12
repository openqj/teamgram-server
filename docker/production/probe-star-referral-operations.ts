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
const userId = Number(process.env.USER_ID);
const botId = Number(process.env.STAR_REF_BOT_ID || '900000000010');
const botAccessHash = BigInt(process.env.STAR_REF_BOT_ACCESS_HASH || '910000000010');

const globalAny = globalThis as typeof globalThis & { self?: any; addEventListener?: () => void };
globalAny.self ??= globalThis;
globalAny.addEventListener ??= () => {};
globalAny.self.addEventListener ??= globalAny.addEventListener;

const requireModule = createRequire(import.meta.url);
const TelegramClient = requireModule(path.join(gramjsDir, 'src/lib/gramjs/client/TelegramClient.ts')).default;
const CallbackSession = requireModule(path.join(gramjsDir, 'src/lib/gramjs/sessions/CallbackSession.ts')).default;
const Api = requireModule(path.join(gramjsDir, 'src/lib/gramjs/tl/index.ts')).Api;
const Connection = requireModule(path.join(gramjsDir, 'src/lib/gramjs/network/connection/TCPObfuscated.ts')).ConnectionTCPObfuscated;
const serializeBytes = requireModule(path.join(gramjsDir, 'src/lib/gramjs/tl/index.ts')).serializeBytes;
const quietLogger = { debug() {}, info() {}, warn() {}, error() {} };

class ReferralRequestCompat {
  static classType = 'request';
  classType = 'request';
  className: string;
  CONSTRUCTOR_ID: number;
  private parts: Uint8Array[];

  constructor(constructorId: number, className: string, parts: Uint8Array[]) {
    this.CONSTRUCTOR_ID = constructorId;
    this.className = className;
    this.parts = parts;
  }

  getBytes(): Uint8Array {
    const constructor = Buffer.alloc(4);
    constructor.writeUInt32LE(this.CONSTRUCTOR_ID >>> 0);
    return Buffer.concat([constructor, ...this.parts.map((part) => Buffer.from(part))]);
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

function referralRequest(name: string, peer: any, bot: any): any {
  switch (name) {
    case 'getSuggestedStarRefBots':
      return new ReferralRequestCompat(0x0d6b48f7, 'payments.getSuggestedStarRefBots', [
        int32(0), peer.getBytes(), serializeBytes(''), int32(10),
      ]);
    case 'connectStarRefBot':
      return new ReferralRequestCompat(0x7ed5348a, 'payments.connectStarRefBot', [peer.getBytes(), bot.getBytes()]);
    case 'getConnectedStarRefBots':
      return new ReferralRequestCompat(0x5869a553, 'payments.getConnectedStarRefBots', [
        int32(0), peer.getBytes(), int32(10),
      ]);
    case 'getConnectedStarRefBot':
      return new ReferralRequestCompat(0xb7d998f0, 'payments.getConnectedStarRefBot', [peer.getBytes(), bot.getBytes()]);
    default:
      throw new Error(`unsupported referral request ${name}`);
  }
}

function editReferralRequest(peer: any, link: string): any {
  return new ReferralRequestCompat(0xe4fca4a3, 'payments.editConnectedStarRefBot', [
    int32(1), peer.getBytes(), serializeBytes(link),
  ]);
}

function typeName(value: any): string {
  return value?.className || value?.constructor?.name || typeof value;
}

function sql(query: string): string {
  return execFileSync('docker', [
    'exec', 'teamgram-postgres', 'psql', '-v', 'ON_ERROR_STOP=1', '-At', '-q', '-F', '\t',
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

function assertType(value: any, ...names: string[]): void {
  if (!names.includes(typeName(value))) {
    throw new Error(`expected ${names.join(' or ')}, got ${typeName(value)}`);
  }
}

function seed(): void {
  // The service bot is reserved for isolated provider acceptance. Remove only
  // rows for this owner/bot pair, then publish a deterministic active program.
  sql(`BEGIN;
    DELETE FROM apifull_star_ref_connection WHERE owner_user_id=${userId} AND bot_user_id=${botId};
    DELETE FROM apifull_star_ref_program WHERE bot_user_id=${botId};
    INSERT INTO apifull_star_ref_program
      (bot_user_id, commission_permille, duration_months, end_date,
       daily_revenue_amount, daily_revenue_nanos, active, updated_at)
    VALUES (${botId}, 125, 12, NULL, 250, 0, TRUE, CURRENT_TIMESTAMP);
    COMMIT;`);
}

function cleanup(): void {
  if (process.env.KEEP_ROWS === '1') return;
  sql(`BEGIN;
    DELETE FROM apifull_star_ref_connection WHERE owner_user_id=${userId} AND bot_user_id=${botId};
    DELETE FROM apifull_star_ref_program WHERE bot_user_id=${botId};
    COMMIT;`);
}

async function run() {
  seed();
  const client = makeClient(process.env.AUTH_B64!);
  try {
    await client.connect();
    const me = await client.getMe();
    if (Number(me?.id) !== userId) throw new Error('the authenticated account does not match USER_ID');
    const peer = new Api.InputPeerSelf();
    const bot = new Api.InputUser({ userId: BigInt(botId), accessHash: botAccessHash });

    const suggested = await client.invoke(referralRequest('getSuggestedStarRefBots', peer, bot));
    assertType(suggested, 'payments.SuggestedStarRefBots', 'SuggestedStarRefBots');
    if (Number(suggested.count) < 1 || !Array.isArray(suggested.suggestedBots) || suggested.suggestedBots.length < 1) {
      throw new Error('suggested referral catalogue did not return the seeded program');
    }

    const connected = await client.invoke(referralRequest('connectStarRefBot', peer, bot));
    assertType(connected, 'payments.ConnectedStarRefBots', 'ConnectedStarRefBots');
    if (Number(connected.count) !== 1 || !connected.connectedBots?.[0]?.url) {
      throw new Error('connectStarRefBot did not return one connected bot with a URL');
    }
    const link = String(connected.connectedBots[0].url);

    const listed = await client.invoke(referralRequest('getConnectedStarRefBots', peer, bot));
    assertType(listed, 'payments.ConnectedStarRefBots', 'ConnectedStarRefBots');
    if (Number(listed.count) !== 1 || listed.connectedBots?.[0]?.url !== link) {
      throw new Error('getConnectedStarRefBots did not return the connected row');
    }

    const single = await client.invoke(referralRequest('getConnectedStarRefBot', peer, bot));
    assertType(single, 'payments.ConnectedStarRefBots', 'ConnectedStarRefBots');
    if (Number(single.count) !== 1 || single.connectedBots?.[0]?.url !== link) {
      throw new Error('getConnectedStarRefBot did not return the connected row');
    }

    const edited = await client.invoke(editReferralRequest(peer, link));
    assertType(edited, 'payments.ConnectedStarRefBots', 'ConnectedStarRefBots');
    if (Number(edited.count) !== 1 || edited.connectedBots?.[0]?.revoked !== true) {
      throw new Error('editConnectedStarRefBot did not persist revoked=true');
    }

    console.log(JSON.stringify({
      transport: `DC${dcId} TCPObfuscated -> gateway -> session -> APIFull/PostgreSQL`,
      userId,
      botId,
      methods: {
        getSuggestedStarRefBots: typeName(suggested),
        connectStarRefBot: typeName(connected),
        getConnectedStarRefBots: typeName(listed),
        getConnectedStarRefBot: typeName(single),
        editConnectedStarRefBot: typeName(edited),
      },
      link,
      seededConnectionRows: sql(`SELECT count(*) FROM apifull_star_ref_connection WHERE owner_user_id=${userId} AND bot_user_id=${botId}`),
    }));
  } finally {
    client.destroy();
    cleanup();
  }
}

run().then(() => process.exit(0)).catch((error) => {
  try { cleanup(); } catch { /* best effort cleanup */ }
  console.error(`Star referral probe failed: ${error instanceof Error ? error.stack || error.message : String(error)}`);
  process.exit(1);
});
