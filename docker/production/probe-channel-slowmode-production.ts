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

function createdChats(value: any): any[] {
  return [
    ...(Array.isArray(value?.chats) ? value.chats : []),
    ...(Array.isArray(value?.updates?.chats) ? value.updates.chats : []),
  ];
}

class ToggleSlowModeRequest {
  CONSTRUCTOR_ID = 0xedd49ef0;
  SUBCLASS_OF_ID = 0;
  className = 'channels.ToggleSlowMode';
  classType = 'request';

  constructor(private readonly channel: any, private readonly seconds: number) {}

  getBytes(): Uint8Array {
    const constructor = new Uint8Array(4);
    const seconds = new Uint8Array(4);
    new DataView(constructor.buffer).setUint32(0, this.CONSTRUCTOR_ID, true);
    new DataView(seconds.buffer).setInt32(0, this.seconds, true);
    return Uint8Array.from(Buffer.concat([
      Buffer.from(constructor),
      Buffer.from(this.channel.getBytes()),
      Buffer.from(seconds),
    ]));
  }

  readResult(reader: any): any {
    return reader.tgReadObject();
  }
}

function readSlowmode(channelId: bigint): number {
  const value = postgres(`SELECT slowmode_seconds FROM apifull_channel WHERE id=${channelId}`);
  if (value === '') throw new Error('temporary channel disappeared before cleanup');
  return Number(value);
}

function readChannelFlag(channelId: bigint, column: 'participants_hidden' | 'hidden_prehistory'): boolean {
  return postgres(`SELECT ${column} FROM apifull_channel WHERE id=${channelId}`) === '1';
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
  const title = `r20-slowmode-${Date.now()}-${randomBytes(4).toString('hex')}`;
  let channelId = 0n;
  let accessHash = 0n;
  let failure: unknown;
  let evidence: Record<string, unknown> = {};

  try {
    await client.connect();
    const me = await client.getMe();
    if (String(me?.id) !== String(userId)) throw new Error('production auth key user mismatch');
    const created = await client.invoke(new Api.channels.CreateChannel({
      broadcast: true,
      title,
      about: 'r20 production reversible slowmode probe',
    }));
    const channel = createdChats(created).find((item: any) => item?.title === title);
    if (!channel?.id || channel.accessHash === undefined) {
      throw new Error(`channel creation returned ${rpcName(created)} without channel identity`);
    }
    channelId = BigInt(String(channel.id));
    accessHash = BigInt(String(channel.accessHash));
    const input = new Api.InputChannel({ channelId, accessHash });

    const participantsEnabled = await client.invoke(new Api.channels.ToggleParticipantsHidden({ channel: input, enabled: true }));
    if (!rpcName(participantsEnabled).toLowerCase().includes('updates')) {
      throw new Error(`hide participants returned ${rpcName(participantsEnabled)}`);
    }
    let full = await client.invoke(new Api.channels.GetFullChannel({ channel: input }));
    if (!readChannelFlag(channelId, 'participants_hidden') || full?.fullChat?.participantsHidden !== true) {
      throw new Error('participant visibility enable readback mismatch');
    }
    const participantsDisabled = await client.invoke(new Api.channels.ToggleParticipantsHidden({ channel: input, enabled: false }));
    if (!rpcName(participantsDisabled).toLowerCase().includes('updates')) {
      throw new Error(`show participants returned ${rpcName(participantsDisabled)}`);
    }
    full = await client.invoke(new Api.channels.GetFullChannel({ channel: input }));
    if (readChannelFlag(channelId, 'participants_hidden') || full?.fullChat?.participantsHidden === true) {
      throw new Error('participant visibility reset readback mismatch');
    }

    const prehistoryEnabled = await client.invoke(new Api.channels.TogglePreHistoryHidden({ channel: input, enabled: true }));
    if (!rpcName(prehistoryEnabled).toLowerCase().includes('updates')) {
      throw new Error(`hide prehistory returned ${rpcName(prehistoryEnabled)}`);
    }
    full = await client.invoke(new Api.channels.GetFullChannel({ channel: input }));
    if (!readChannelFlag(channelId, 'hidden_prehistory') || full?.fullChat?.hiddenPrehistory !== true) {
      throw new Error('prehistory enable readback mismatch');
    }
    const prehistoryDisabled = await client.invoke(new Api.channels.TogglePreHistoryHidden({ channel: input, enabled: false }));
    if (!rpcName(prehistoryDisabled).toLowerCase().includes('updates')) {
      throw new Error(`show prehistory returned ${rpcName(prehistoryDisabled)}`);
    }
    full = await client.invoke(new Api.channels.GetFullChannel({ channel: input }));
    if (readChannelFlag(channelId, 'hidden_prehistory') || full?.fullChat?.hiddenPrehistory === true) {
      throw new Error('prehistory reset readback mismatch');
    }

    const enabled = await client.invoke(new ToggleSlowModeRequest(input, 17));
    if (!rpcName(enabled).toLowerCase().includes('updates')) {
      throw new Error(`enable slowmode returned ${rpcName(enabled)}`);
    }
    const storedEnabled = readSlowmode(channelId);
    const fullEnabled = await client.invoke(new Api.channels.GetFullChannel({ channel: input }));
    const apiEnabled = Number(fullEnabled?.fullChat?.slowmodeSeconds ?? -1);
    if (storedEnabled !== 17 || apiEnabled !== 17) {
      throw new Error(`slowmode enable readback mismatch: postgres=${storedEnabled}, api=${apiEnabled}`);
    }

    const disabled = await client.invoke(new ToggleSlowModeRequest(input, 0));
    if (!rpcName(disabled).toLowerCase().includes('updates')) {
      throw new Error(`disable slowmode returned ${rpcName(disabled)}`);
    }
    const storedDisabled = readSlowmode(channelId);
    const fullDisabled = await client.invoke(new Api.channels.GetFullChannel({ channel: input }));
    const apiDisabled = Number(fullDisabled?.fullChat?.slowmodeSeconds ?? 0);
    if (storedDisabled !== 0 || apiDisabled !== 0) {
      throw new Error(`slowmode reset readback mismatch: postgres=${storedDisabled}, api=${apiDisabled}`);
    }
    evidence = {
      create: rpcName(created),
      toggleParticipantsHidden: [rpcName(participantsEnabled), rpcName(participantsDisabled)],
      togglePreHistoryHidden: [rpcName(prehistoryEnabled), rpcName(prehistoryDisabled)],
      enable: rpcName(enabled),
      enabledValue: { api: apiEnabled, postgres: storedEnabled },
      disable: rpcName(disabled),
      disabledValue: { api: apiDisabled, postgres: storedDisabled },
      channelId: channelId.toString(),
    };
  } catch (error) {
    failure = error;
  }

  if (channelId === 0n) {
    try {
      const row = postgres(`SELECT id, access_hash FROM apifull_channel WHERE creator_user_id=${userId} AND title='${title}' LIMIT 1`);
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
      }
    } catch (error) {
      evidence.cleanupError = String(error);
    }
  }
  client.destroy();

  if (channelId === 0n) throw failure || new Error('production probe did not create a channel');
  const leftover = Number(postgres(`SELECT COUNT(*) FROM apifull_channel WHERE id=${channelId}`));
  evidence.channelRowsAfterDelete = leftover;
  if (leftover !== 0) throw new Error(`temporary channel cleanup failed: ${JSON.stringify(evidence)}`);
  if (failure) throw failure;
  console.log(JSON.stringify(evidence));
}

main().then(() => process.exit(0)).catch((error) => {
  console.error(`production channel slowmode probe failed: ${error instanceof Error ? error.stack || error.message : String(error)}`);
  process.exit(1);
});
