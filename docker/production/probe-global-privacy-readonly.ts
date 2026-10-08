import { createHash } from 'node:crypto';
import { execFileSync } from 'node:child_process';
import path from 'node:path';
import { createRequire } from 'node:module';

const gramjsDir = process.env.TEAMGRAM_GRAMJS_DIR || '/Users/mac/open/test/telegram-tt-master';
const dcId = Number(process.env.TG_DC_ID || '2');
const userId = '136907714';
const globalAny = globalThis as typeof globalThis & { self?: any; addEventListener?: () => void };
globalAny.self ??= globalThis;
globalAny.addEventListener ??= () => {};
globalAny.self.addEventListener ??= globalAny.addEventListener;

function sql(query: string): string {
  return execFileSync('docker', ['exec', 'teamgram-postgres', 'psql', '-v', 'ON_ERROR_STOP=1', '-At', '-q', '-F', '\t', '-U', 'teamgram', '-d', 'teamgram', '-c', query], {
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

function settingsSnapshot(): { value: any; hash: string } {
  const value = sql(`SELECT json_build_object(
    'archive_and_mute_new_noncontact_peers',archive_and_mute_new_noncontact_peers,
    'keep_archived_unmuted',keep_archived_unmuted,
    'keep_archived_folders',keep_archived_folders,
    'hide_read_marks',hide_read_marks,
    'new_noncontact_peers_require_premium',new_noncontact_peers_require_premium,
    'display_gifts_button',display_gifts_button,
    'noncontact_peers_paid_stars',noncontact_peers_paid_stars,
    'disallowed_gifts',disallowed_gifts)
    FROM user_global_privacy_settings WHERE user_id=${userId}`);
  if (!value) throw new Error(`no production global privacy settings row found for ${userId}`);
  return { value: JSON.parse(value), hash: createHash('sha256').update(value).digest('hex') };
}

const requireModule = createRequire(import.meta.url);
const TelegramClient = requireModule(path.join(gramjsDir, 'src/lib/gramjs/client/TelegramClient.ts')).default;
const CallbackSession = requireModule(path.join(gramjsDir, 'src/lib/gramjs/sessions/CallbackSession.ts')).default;
const Api = requireModule(path.join(gramjsDir, 'src/lib/gramjs/tl/index.ts')).Api;
const Connection = requireModule(path.join(gramjsDir, 'src/lib/gramjs/network/connection/TCPObfuscated.ts')).ConnectionTCPObfuscated;
const quietLogger = { debug() {}, info() {}, warn() {}, error() {} };

function rpcName(value: any): string { return value?.className || value?.constructor?.name || typeof value; }

function makeClient(authKeyHex: string): any {
  const session = new CallbackSession({ mainDcId: dcId, keys: { [dcId]: authKeyHex } }, () => {});
  const load = session.load.bind(session);
  session.load = async () => {
    await load();
    (session as any)._serverAddress = process.env.TG_DC_HOST || '127.0.0.1';
    (session as any)._port = Number(process.env.TG_DC_PORT || '11443');
  };
  return new TelegramClient(session, 1, '0123456789abcdef0123456789abcdef', {
    connection: Connection, fallbackConnection: Connection,
    shouldAllowHttpTransport: false, shouldForceHttpTransport: false,
    timeout: 20, requestRetries: 1, connectionRetries: 1,
    connectionRetriesToFallback: 0, autoReconnect: false, baseLogger: quietLogger,
  });
}

function assertBoolean(settings: any, field: string, expected: unknown, key: string) {
  if (Boolean(settings?.[field]) !== Boolean(Number(expected))) {
    throw new Error(`global privacy field mismatch ${field}: got=${settings?.[field]} expected=${expected}`);
  }
}

async function main() {
  const before = settingsSnapshot();
  const client = makeClient(loadAuthKey());
  try {
    (client as any)._borrowExportedSender = async () => undefined;
    (client as any)._updateLoop = async () => undefined;
    await client.connect();
    const me = await client.getMe();
    if (String(me?.id) !== userId) throw new Error('auth key user mismatch');

    const settings = await client.invoke(new Api.account.GetGlobalPrivacySettings());
    if (rpcName(settings) !== 'GlobalPrivacySettings') throw new Error(`getGlobalPrivacySettings returned ${rpcName(settings)}`);
    const mappings = [
      ['archiveAndMuteNewNoncontactPeers', 'archive_and_mute_new_noncontact_peers'],
      ['keepArchivedUnmuted', 'keep_archived_unmuted'],
      ['keepArchivedFolders', 'keep_archived_folders'],
      ['hideReadMarks', 'hide_read_marks'],
      ['newNoncontactPeersRequirePremium', 'new_noncontact_peers_require_premium'],
      ['displayGiftsButton', 'display_gifts_button'],
    ];
    for (const [field, key] of mappings) assertBoolean(settings, field, before.value[key], key);
    if (before.value.noncontact_peers_paid_stars === null ? settings?.noncontactPeersPaidStars !== undefined
      : String(settings?.noncontactPeersPaidStars) !== String(before.value.noncontact_peers_paid_stars)) {
      throw new Error(`noncontactPeersPaidStars mismatch: got=${settings?.noncontactPeersPaidStars} expected=${before.value.noncontact_peers_paid_stars}`);
    }
    const gifts = settings?.disallowedGifts;
    if (before.value.disallowed_gifts === null ? gifts !== undefined
      : JSON.stringify(gifts) !== JSON.stringify(JSON.parse(before.value.disallowed_gifts))) {
      throw new Error('disallowedGifts mismatch against production PostgreSQL');
    }

    const after = settingsSnapshot();
    if (before.hash !== after.hash) throw new Error('global privacy settings changed during read-only probe');
    console.log(JSON.stringify({ userId, result: rpcName(settings), fieldsChecked: mappings.length + 2, settings: before.value, postgresSha256: after.hash, writes: 0 }));
  } finally {
    client.destroy();
  }
}

main().then(() => process.exit(0)).catch((error) => {
  console.error(`global privacy production probe failed: ${error instanceof Error ? error.stack || error.message : String(error)}`);
  process.exit(1);
});
