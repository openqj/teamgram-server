import { execFileSync } from 'node:child_process';
import path from 'node:path';
import { createRequire } from 'node:module';

const gramjsDir = process.env.TEAMGRAM_GRAMJS_DIR || '/Users/mac/open/test/telegram-tt-master';
const dcId = Number(process.env.TG_DC_ID || '2');
const userId = process.env.TOS_USER_ID || '136907714';
const tosKey = `tos:${userId}`;
const tosId = '{"country":""}';
const globalAny = globalThis as typeof globalThis & { self?: any; addEventListener?: () => void };
globalAny.self ??= globalThis;
globalAny.addEventListener ??= () => {};
globalAny.self.addEventListener ??= globalAny.addEventListener;

function sql(query: string): string {
  return execFileSync('docker', ['exec', 'mysql', 'mysql', '-N', '-s', '-uteamgram', '-pteamgram', '-Dteamgram', '-e', query], {
    encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'],
  }).trim();
}

function sqlWrite(query: string): void {
  execFileSync('docker', ['exec', 'mysql', 'mysql', '-N', '-s', '-uteamgram', '-pteamgram', '-Dteamgram', '-e', query], {
    stdio: ['ignore', 'ignore', 'ignore'],
  });
}

function loadAuthKey(): string {
  const body = sql(`SELECT k.body FROM auth_users u JOIN auth_keys k USING(auth_key_id)
    JOIN auth_key_infos i USING(auth_key_id) WHERE u.user_id=${userId} AND u.state=0 AND u.deleted=0
    AND k.deleted=0 AND i.deleted=0 AND i.auth_key_type=0 ORDER BY u.date_active DESC,u.id DESC LIMIT 1`);
  if (!body) throw new Error(`no active production auth key found for user ${userId}`);
  return Buffer.from(body, 'base64').toString('hex');
}

function rowSnapshot(): { present: boolean; value: string } {
  const encoded = sql(`SELECT CONCAT('1:',HEX(v)) FROM apifull_kv WHERE k='${tosKey}'
    UNION ALL SELECT '0:' WHERE NOT EXISTS (SELECT 1 FROM apifull_kv WHERE k='${tosKey}') LIMIT 1`);
  if (!encoded) throw new Error('could not read ToS KV snapshot');
  if (encoded.startsWith('0:')) return { present: false, value: '' };
  const hex = encoded.slice(2);
  return { present: true, value: Buffer.from(hex, 'hex').toString('utf8') };
}

function restore(snapshot: { present: boolean; value: string }): void {
  if (!snapshot.present) {
    sqlWrite(`DELETE FROM apifull_kv WHERE k='${tosKey}'`);
    return;
  }
  const value = snapshot.value.replaceAll('\\', '\\\\').replaceAll("'", "''");
  sqlWrite(`INSERT INTO apifull_kv (k,v) VALUES ('${tosKey}','${value}')
    ON DUPLICATE KEY UPDATE v=VALUES(v)`);
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

async function main() {
  const before = rowSnapshot();
  const client = makeClient(loadAuthKey());
  try {
    (client as any)._borrowExportedSender = async () => undefined;
    (client as any)._updateLoop = async () => undefined;
    await client.connect();
    const me = await client.getMe();
    if (String(me?.id) !== userId) throw new Error('auth key user mismatch');

    const accepted = await client.invoke(new Api.help.AcceptTermsOfService({
      id: new Api.DataJSON({ data: tosId }),
    }));
    const acceptedValue = accepted === true || accepted?.value === true || accepted?.value === 1;
    if (!acceptedValue) {
      throw new Error(`acceptTermsOfService returned ${rpcName(accepted)} value=${accepted?.value ?? accepted}`);
    }
    const afterAccept = rowSnapshot();
    if (!afterAccept.present || afterAccept.value !== tosId) {
      throw new Error(`accepted ToS was not persisted: ${JSON.stringify(afterAccept)}`);
    }

    const update = await client.invoke({
      className: 'help.GetTermsOfServiceUpdate',
      classType: 'request',
      CONSTRUCTOR_ID: 0x2ca51fd1,
      getBytes: () => new Uint8Array([0xd1, 0x1f, 0xa5, 0x2c]),
      readResult: (reader: any) => reader.tgReadObject(),
    });
    if (rpcName(update) !== 'help.TermsOfServiceUpdateEmpty' || update.termsOfService) {
      throw new Error(`accepted ToS did not return empty update: ${rpcName(update)}`);
    }
  } finally {
    try { restore(before); } finally { client.destroy(); }
  }
  const restored = rowSnapshot();
  if (restored.present !== before.present || restored.value !== before.value) {
    throw new Error(`ToS KV restore mismatch: before=${JSON.stringify(before)} after=${JSON.stringify(restored)}`);
  }
  console.log(JSON.stringify({ userId, accepted: true, update: 'help.TermsOfServiceUpdateEmpty', before, restored, writes: 1 }));
}

main().then(() => process.exit(0)).catch((error) => {
  console.error(`terms-of-service production roundtrip failed: ${error instanceof Error ? error.stack || error.message : String(error)}`);
  process.exit(1);
});
