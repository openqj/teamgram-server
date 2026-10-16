import path from 'node:path';
import { createRequire } from 'node:module';

const requireModule = createRequire(import.meta.url);
const TelegramClient = requireModule(path.join(process.env.TEAMGRAM_GRAMJS_DIR!, 'src/lib/gramjs/client/TelegramClient.ts')).default;
const CallbackSession = requireModule(path.join(process.env.TEAMGRAM_GRAMJS_DIR!, 'src/lib/gramjs/sessions/CallbackSession.ts')).default;
const Api = requireModule(path.join(process.env.TEAMGRAM_GRAMJS_DIR!, 'src/lib/gramjs/tl/index.ts')).Api;
const Connection = requireModule(path.join(process.env.TEAMGRAM_GRAMJS_DIR!, 'src/lib/gramjs/network/connection/TCPObfuscated.ts')).ConnectionTCPObfuscated;

const quietLogger = { debug() {}, info() {}, warn() {}, error() {} };
const globalAny = globalThis as typeof globalThis & { self?: any; addEventListener?: () => void };
globalAny.self ??= globalThis;
globalAny.addEventListener ??= () => {};
globalAny.self.addEventListener ??= globalAny.addEventListener;
const session = new CallbackSession(
  { mainDcId: 2, keys: { 2: Buffer.from(process.env.A_AUTH_B64!, 'base64').toString('hex') } },
  () => {},
);
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
  requestRetries: 2,
  connectionRetries: 2,
  connectionRetriesToFallback: 0,
  retryDelay: 250,
  autoReconnect: false,
  baseLogger: quietLogger,
});

const chatList = (value: any): any[] => [
  ...(Array.isArray(value?.chats) ? value.chats : []),
  ...(Array.isArray(value?.updates?.chats) ? value.updates.chats : []),
];

async function main() {
  await client.connect();
  const me = await client.getMe();
  if (String(me?.id) !== String(process.env.A_USER_ID)) throw new Error('auth key user mismatch');
  const tag = `bot-api-${Date.now()}-${process.pid}`;
  const created = await client.invoke(new Api.channels.CreateChannel({
    broadcast: false,
    megagroup: true,
    title: tag,
    about: 'temporary Bot API acceptance group',
  }));
  const channel = chatList(created).find((item: any) => item?.title === tag);
  if (!channel?.id || channel.accessHash === undefined) throw new Error('channel creation returned no channel');
  const inputChannel = new Api.InputChannel({ channelId: channel.id, accessHash: channel.accessHash });
  const botResult = await client.invoke(new Api.contacts.ResolveUsername({ username: 'web_e2e_bot' }));
  const bot = botResult?.users?.find((item: any) => String(item?.id) === '900000000001');
  if (!bot?.accessHash) throw new Error('test bot could not be resolved');
  const inputBot = new Api.InputUser({ userId: bot.id, accessHash: bot.accessHash });
  await client.invoke(new Api.channels.InviteToChannel({ channel: inputChannel, users: [inputBot] }));
  await client.invoke(new Api.channels.EditAdmin({
    channel: inputChannel,
    userId: inputBot,
    adminRights: new Api.ChatAdminRights({
      changeInfo: true,
      postMessages: true,
      editMessages: true,
      deleteMessages: true,
      banUsers: true,
      inviteUsers: true,
      pinMessages: true,
      addAdmins: true,
      manageTopics: true,
      manageCall: true,
      other: true,
    }),
    rank: 'bot admin',
  }));
  await client.invoke(new Api.channels.ToggleForum({ channel: inputChannel, enabled: true, tabs: true }));
  console.log(JSON.stringify({ channelId: String(channel.id), chatId: String(-1000000000000n - BigInt(channel.id)), title: tag }));
}

main().finally(() => client.destroy());
