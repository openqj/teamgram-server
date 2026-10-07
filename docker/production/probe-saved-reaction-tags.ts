import path from 'node:path';
import { createRequire } from 'node:module';

for (const key of ['TEAMGRAM_GRAMJS_DIR', 'A_AUTH_B64', 'A_USER_ID']) {
  if (!process.env[key]) throw new Error(`${key} is required`);
}

const root = process.env.TEAMGRAM_GRAMJS_DIR!;
const dcId = Number(process.env.TG_DC_ID || '2');
const requireModule = createRequire(import.meta.url);
const TelegramClient = requireModule(path.join(root, 'src/lib/gramjs/client/TelegramClient.ts')).default;
const CallbackSession = requireModule(path.join(root, 'src/lib/gramjs/sessions/CallbackSession.ts')).default;
const Api = requireModule(path.join(root, 'src/lib/gramjs/tl/index.ts')).Api;
const Connection = requireModule(path.join(root, 'src/lib/gramjs/network/connection/TCPObfuscated.ts')).ConnectionTCPObfuscated;
const quietLogger = { debug() {}, info() {}, warn() {}, error() {} };
const globalAny = globalThis as typeof globalThis & { self?: any; addEventListener?: () => void };
globalAny.self ??= globalThis;
globalAny.addEventListener ??= () => {};
globalAny.self.addEventListener ??= globalAny.addEventListener;

const rpcName = (value: any): string => value?.className || value?.constructor?.name || typeof value;

function makeClient(): any {
  const session = new CallbackSession({
    mainDcId: dcId,
    keys: { [dcId]: Buffer.from(process.env.A_AUTH_B64!, 'base64').toString('hex') },
  }, () => {});
  const load = session.load.bind(session);
  session.load = async () => {
    await load();
    (session as any)._serverAddress = process.env.TG_DC_HOST || '127.0.0.1';
    (session as any)._port = Number(process.env.TG_DC_PORT || '11443');
  };
  return new TelegramClient(session, 1, '0123456789abcdef0123456789abcdef', {
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
}

function isTrue(value: any): boolean {
  if (value === true) return true;
  if (value instanceof Boolean) return value.valueOf();
  return rpcName(value).toLowerCase().includes('true');
}

function tagFor(result: any, emoticon: string): any | undefined {
  return (result?.tags || []).find((tag: any) => tag?.reaction?.emoticon === emoticon);
}

function reactionNames(result: any): string[] {
  return (result?.reactions || []).map((reaction: any) => reaction?.emoticon || String(reaction?.documentId || ''));
}

async function main() {
  const client = makeClient();
  const emoticon = '🧪';
  const title = `codex-prod-${Date.now()}-${process.pid}`;
  let priorTitle: string | undefined;
  let hadPrior = false;
  try {
    await client.connect();
    const me = await client.getMe();
    if (String(me?.id) !== String(process.env.A_USER_ID)) throw new Error('auth key user mismatch');

    const before = await client.invoke(new Api.messages.GetSavedReactionTags({ hash: 0n }));
    if (rpcName(before) !== 'messages.SavedReactionTags') throw new Error(`unexpected initial result: ${rpcName(before)}`);
    const previous = tagFor(before, emoticon);
    if (previous) {
      hadPrior = true;
      priorTitle = previous.title;
      const removed = await client.invoke(new Api.messages.UpdateSavedReactionTag({
        reaction: new Api.ReactionEmoji({ emoticon }),
      }));
      if (!isTrue(removed)) throw new Error(`could not clear pre-existing probe tag: ${rpcName(removed)}`);
    }

    const saved = await client.invoke(new Api.messages.UpdateSavedReactionTag({
      reaction: new Api.ReactionEmoji({ emoticon }),
      title,
    }));
    if (!isTrue(saved)) throw new Error(`save reaction tag returned ${rpcName(saved)}`);

    const read = await client.invoke(new Api.messages.GetSavedReactionTags({ hash: 0n }));
    const tag = tagFor(read, emoticon);
    if (rpcName(read) !== 'messages.SavedReactionTags' || !tag || tag.title !== title) {
      throw new Error(`saved tag readback mismatch: ${rpcName(read)} ${JSON.stringify(tag)}`);
    }
    const notModified = await client.invoke(new Api.messages.GetSavedReactionTags({ hash: read.hash }));
    if (rpcName(notModified) !== 'messages.SavedReactionTagsNotModified') {
      throw new Error(`hash not-modified returned ${rpcName(notModified)}`);
    }

    const defaults = await client.invoke(new Api.messages.GetDefaultTagReactions({ hash: 0n }));
    if (rpcName(defaults) !== 'messages.Reactions' || !reactionNames(defaults).includes(emoticon)) {
      throw new Error(`default tag reactions mismatch: ${rpcName(defaults)} ${JSON.stringify(reactionNames(defaults))}`);
    }

    const removed = await client.invoke(new Api.messages.UpdateSavedReactionTag({
      reaction: new Api.ReactionEmoji({ emoticon }),
    }));
    if (!isTrue(removed)) throw new Error(`remove reaction tag returned ${rpcName(removed)}`);
    const after = await client.invoke(new Api.messages.GetSavedReactionTags({ hash: 0n }));
    if (tagFor(after, emoticon)) throw new Error('probe reaction tag remained after cleanup');

    console.log(JSON.stringify({
      before: rpcName(before), save: rpcName(saved), read: rpcName(read),
      notModified: rpcName(notModified), defaults: rpcName(defaults),
      removed: rpcName(removed), after: rpcName(after), emoticon,
    }));
  } finally {
    try {
      if (hadPrior && priorTitle !== undefined) {
        await client.invoke(new Api.messages.UpdateSavedReactionTag({
          reaction: new Api.ReactionEmoji({ emoticon }),
          title: priorTitle,
        }));
      } else {
        await client.invoke(new Api.messages.UpdateSavedReactionTag({
          reaction: new Api.ReactionEmoji({ emoticon }),
        }));
      }
    } catch {}
    client.destroy();
  }
}

main().then(() => process.exit(0)).catch((error) => {
  console.error(`saved reaction tags probe failed: ${error instanceof Error ? error.stack || error.message : String(error)}`);
  process.exit(1);
});
