import path from 'node:path';
import { createRequire } from 'node:module';
import { authSession, loadFixtureAuthKey } from './fixture-auth';

const gramjsDir = process.env.TEAMGRAM_GRAMJS_DIR || '/Users/mac/open/test/telegram-tt-master';
const username = process.env.BOTFATHER_ACTOR || 'isolated_test_07';
const phone = process.env.BOTFATHER_PHONE || '12025550107';
const dcId = Number(process.env.TG_DC_ID || '2');
const dcHost = process.env.TG_DC_HOST || '127.0.0.1';
const dcPort = Number(process.env.TG_DC_PORT || '11443');
const apiId = Number(process.env.TG_API_ID || '1');
const apiHash = process.env.TG_API_HASH || '0123456789abcdef0123456789abcdef';
const botFatherId = 900000000010n;
const botFatherAccessHash = 910000000010n;

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

function className(value: any): string {
  return value?.className || value?.constructor?.name || typeof value;
}

function requireCondition(condition: unknown, message: string): asserts condition {
  if (!condition) throw new Error(message);
}

function inputPeer(): any {
  return new Api.InputPeerUser({ userId: botFatherId, accessHash: botFatherAccessHash });
}

function historyMessages(value: any): any[] {
  return Array.isArray(value?.messages) ? value.messages : [];
}

function isBotFatherMessage(message: any): boolean {
  return String(message?.fromId?.userId ?? '') === String(botFatherId)
    || (message?.out !== true && String(message?.peerId?.userId ?? '') === String(botFatherId));
}

async function createClient(): Promise<any> {
  const authKey = loadFixtureAuthKey(username);
  const session = authKey
    ? authSession({ CallbackSession }, authKey, dcId, dcHost, dcPort)
    : new (requireModule(path.join(gramjsDir, 'src/lib/gramjs/sessions/Memory.ts')).default)();
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
  (client as any)._borrowExportedSender = async () => undefined;
  (client as any)._updateLoop = async () => undefined;
  if (authKey) {
    await client.connect();
  } else {
    await client.start({
      phoneNumber: phone,
      phoneCode: async () => '12345',
      password: async () => '',
      firstAndLastNames: async () => ['BotFather', 'Probe'],
      qrCode: async () => {},
      onError: (error: Error) => { throw error; },
      webAuthTokenFailed: () => {},
      onPasskeyOption: () => {},
      initialMethod: 'phoneNumber',
    });
  }
  return client;
}

async function waitForReply(client: any, afterId: number, label: string): Promise<any> {
  const deadline = Date.now() + 10_000;
  let inspected = false;
  while (Date.now() < deadline) {
    const result = await client.invoke(new Api.messages.GetHistory({
      peer: inputPeer(), offsetId: 0, offsetDate: 0, addOffset: 0,
      limit: 50, maxId: 0, minId: 0, hash: 0n,
    }));
    const reply = historyMessages(result)
      .filter((message) => isBotFatherMessage(message) && Number(message.id) > afterId)
      .sort((a, b) => Number(a.id) - Number(b.id))[0];
    if (!inspected) {
      inspected = true;
      const samples = historyMessages(result).slice(0, 4).map((sample) => `id=${String(sample?.id)} fromId=${String(sample?.fromId?.userId)} peerId=${String(sample?.peerId?.userId)} out=${String(sample?.out)} text=${String(sample?.message)}`);
      console.log(`DEBUG history samples ${samples.join(' || ')}`);
    }
    if (reply) return reply;
    await new Promise((resolve) => setTimeout(resolve, 200));
  }
  throw new Error(`${label}: timed out waiting for BotFather reply after message ${afterId}`);
}

async function latestIncomingId(client: any): Promise<number> {
  const result = await client.invoke(new Api.messages.GetHistory({
    peer: inputPeer(), offsetId: 0, offsetDate: 0, addOffset: 0,
    limit: 50, maxId: 0, minId: 0, hash: 0n,
  }));
  return historyMessages(result)
    .filter(isBotFatherMessage)
    .reduce((max, message) => Math.max(max, Number(message.id)), 0);
}

async function send(client: any, text: string): Promise<string> {
  const before = await latestIncomingId(client);
  await client.invoke(new Api.messages.SendMessage({
    peer: inputPeer(), message: text,
    randomId: BigInt(Date.now()) * 1_000_000n + BigInt(Math.floor(Math.random() * 1_000_000)),
  }));
  const reply = await waitForReply(client, before, text);
  const replyText = String(reply.message || '');
  console.log(`PASS ${text} -> ${replyText.replaceAll('\n', ' | ')}`);
  return replyText;
}

async function main() {
  const client = await createClient();
  try {
    const me = await client.getMe();
    requireCondition(BigInt(me?.id || 0) > 0n, 'fixture login returned no user');

    await send(client, '/cancel');
    const bots = await send(client, '/mybots');
    requireCondition(bots.includes('@bobobot'), '/mybots did not include the existing bot');

    const suffix = `${Date.now()}`.slice(-8);
    const botUsername = `probe_${suffix}bot`;
    await send(client, '/newbot');
    await send(client, `Probe Bot ${suffix}`);
    const created = await send(client, botUsername);
    requireCondition(created.includes(botUsername), 'new bot reply did not include its username');
    const token = created.match(/\b\d+:[A-Za-z0-9_-]+/)?.[0];
    requireCondition(token, 'new bot reply did not include a token');

    const list = await send(client, '/mybots');
    requireCondition(list.includes(`@${botUsername}`), 'created bot missing from /mybots');
    const tokenReply = await send(client, `/token @${botUsername}`);
    requireCondition(tokenReply.includes(token), '/token did not return the created token');

    await send(client, `/setname @${botUsername}`);
    await send(client, `Renamed ${suffix}`);
    await send(client, `/setabouttext @${botUsername}`);
    await send(client, 'Probe about');
    await send(client, `/setdescription @${botUsername}`);
    await send(client, 'Probe description');
    await send(client, `/setcommands @${botUsername}`);
    await send(client, 'start - Start the bot\nhelp - Show help');
    await send(client, `/setprivacy @${botUsername} disable`);
    await send(client, `/setjoingroups @${botUsername} enable`);
    await send(client, `/setinline @${botUsername} Search here`);
    await send(client, `/setinlinegeo @${botUsername} enable`);
    await send(client, `/setmenubutton @${botUsername}`);
    await send(client, 'Open app\nhttps://mini.example/probe');

    await send(client, `/newapp @${botUsername}`);
    await send(client, 'Probe Mini App');
    await send(client, 'probe_app');
    await send(client, 'Probe mini app description');
    await send(client, 'https://mini.example/probe-app');
    const apps = await send(client, '/myapps');
    requireCondition(apps.includes('probe_app'), '/myapps did not list the created app');
    await send(client, `/editapp @${botUsername}`);
    await send(client, 'Edited Mini App\nprobe_app\nEdited description\nhttps://mini.example/probe-edited');
    await send(client, `/setdomain @${botUsername}`);
    await send(client, 'example.com');
    await send(client, `/deleteapp @${botUsername}`);
    await send(client, `/revoke @${botUsername}`);
    console.log(`PASS BotFather full flow (${className(me)} ${username})`);
  } finally {
    await client.disconnect();
  }
}

main().catch((error) => {
  console.error(error);
  process.exitCode = 1;
});
