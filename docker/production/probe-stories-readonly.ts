import { createHash } from 'node:crypto';
import { execFileSync } from 'node:child_process';
import path from 'node:path';
import { createRequire } from 'node:module';

const gramjsDir = process.env.TEAMGRAM_GRAMJS_DIR || '/Users/mac/open/test/telegram-tt-master';
const dcId = Number(process.env.TG_DC_ID || '2');
const dcHost = process.env.TG_DC_HOST || '127.0.0.1';
const dcPort = Number(process.env.TG_DC_PORT || '11443');
const userId = process.env.STORY_USER_ID || '136907714';

const globalAny = globalThis as typeof globalThis & { self?: any; addEventListener?: () => void };
globalAny.self ??= globalThis;
globalAny.addEventListener ??= () => {};
globalAny.self.addEventListener ??= globalAny.addEventListener;

function sql(query: string): string {
  return execFileSync('docker', [
    'exec', 'teamgram-postgres', 'psql', '-v', 'ON_ERROR_STOP=1', '-At', '-q', '-F', '\t', '-U', 'teamgram', '-d', 'teamgram', '-c', query,
  ], { encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'] }).trim();
}

function loadAuthKey(): string {
  const body = sql(`
    SELECT encode(k.body, 'base64')
    FROM auth_users u
    JOIN auth_keys k USING (auth_key_id)
    JOIN auth_key_infos i USING (auth_key_id)
    WHERE u.user_id=${userId} AND u.state=0 AND u.deleted=0
      AND k.deleted=0 AND i.deleted=0 AND i.auth_key_type=0
    ORDER BY u.date_active DESC, u.id DESC
    LIMIT 1
  `);
  if (!body) throw new Error(`no active production auth key found for user ${userId}`);
  return Buffer.from(body, 'base64').toString('hex');
}

function storyRaw(): string {
  // Stories moved from the legacy KV row to the deployment-owned JSONB table.
  // Resolve the relation first so this probe remains usable during a rolling
  // migration where one database still has only the legacy layout.
  const hasStoryTable = sql(`SELECT (to_regclass('public.apifull_story_state') IS NOT NULL)::int`);
  if (hasStoryTable === '1') {
    const state = sql(`SELECT state::text FROM apifull_story_state WHERE user_id=${userId}`);
    if (state) return state;
  }
  return sql(`SELECT v FROM apifull_kv WHERE k='story:${userId}'`);
}

function storySnapshot(): string {
  const value = storyRaw();
  if (!value) throw new Error(`no production story record found for user ${userId}`);
  return createHash('sha256').update(value).digest('hex');
}

function storyViewsBaseline(): string {
  const value = storyRaw();
  if (!value) throw new Error(`no production story record found for user ${userId}`);
  const state = JSON.parse(value) as {
    order?: unknown[];
    items?: Record<string, unknown>;
    viewers?: Record<string, unknown>;
    reactions?: Record<string, unknown>;
  };
  const itemOne = state.items?.['1'] as { views?: unknown } | undefined;
  return [
    state.order?.length || 0,
    Object.keys(state.items || {}).length,
    Object.keys(state.viewers || {}).length,
    Object.keys(state.reactions || {}).length,
    itemOne?.views == null ? 0 : 1,
  ].join('\t');
}

function int32(value: number): Uint8Array {
  const out = new Uint8Array(4);
  new DataView(out.buffer).setInt32(0, value, true);
  return out;
}

function concat(...parts: Uint8Array[]): Uint8Array {
  const out = new Uint8Array(parts.reduce((size, part) => size + part.length, 0));
  let offset = 0;
  for (const part of parts) {
    out.set(part, offset);
    offset += part.length;
  }
  return out;
}

function rawGetStoryReactionsListRequest(storyId: number, limit: number): any {
  const constructorId = 0xb9b2881f;
  return {
    className: 'stories.GetStoryReactionsList',
    classType: 'request',
    CONSTRUCTOR_ID: constructorId,
    getBytes: () => concat(
      int32(constructorId),
      int32(0),
      int32(0x7da07ec9),
      int32(storyId),
      int32(limit),
    ),
    readResult: (reader: any) => reader.tgReadObject(),
  };
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

function makeClient(authKeyHex: string): any {
  const session = new CallbackSession({ mainDcId: dcId, keys: { [dcId]: authKeyHex } }, () => {});
  const load = session.load.bind(session);
  session.load = async () => {
    await load();
    (session as any)._serverAddress = dcHost;
    (session as any)._port = dcPort;
  };
  return new TelegramClient(session, 1, '0123456789abcdef0123456789abcdef', {
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
}

function storyIds(result: any): number[] {
  return (Array.isArray(result?.stories) ? result.stories : [])
    .map((item: any) => Number(item?.id))
    .filter((id: number) => Number.isInteger(id));
}

async function main() {
  const before = storySnapshot();
  const viewsBaseline = storyViewsBaseline();
  if (viewsBaseline !== '8\t8\t0\t0\t0') {
    throw new Error(`unexpected production story-view baseline: ${viewsBaseline}`);
  }
  const client = makeClient(loadAuthKey());
  try {
    (client as any)._borrowExportedSender = async () => undefined;
    (client as any)._updateLoop = async () => undefined;
    await client.connect();
    const me = await client.getMe();
    if (String(me?.id) !== userId) throw new Error(`auth key user mismatch: ${String(me?.id)}`);

    const peer = new Api.InputPeerSelf();
    const archive = await client.invoke(new Api.stories.GetStoriesArchive({
      peer, offsetId: 0, limit: 100,
    }));
    if (!rpcName(archive).toLowerCase().endsWith('stories')) {
      throw new Error(`stories.getStoriesArchive returned ${rpcName(archive)}`);
    }
    const archiveIds = storyIds(archive);
    if (archiveIds.length !== 8 || archiveIds.join(',') !== '1,2,3,4,5,6,7,8') {
      throw new Error(`unexpected archive stories: ${archiveIds.join(',')}`);
    }

    const byId = await client.invoke(new Api.stories.GetStoriesByID({
      peer: new Api.InputPeerSelf(), id: [1, 2, 3],
    }));
    if (!rpcName(byId).toLowerCase().endsWith('stories')) {
      throw new Error(`stories.getStoriesByID returned ${rpcName(byId)}`);
    }
    const byIdIds = storyIds(byId);
    if (byIdIds.length !== 3 || byIdIds.join(',') !== '1,2,3') {
      throw new Error(`unexpected stories by id: ${byIdIds.join(',')}`);
    }

    const storyViews = await client.invoke(new Api.stories.GetStoriesViews({
      peer: new Api.InputPeerSelf(), id: [1, 2, 3],
    }));
    const storyViewItems = Array.isArray(storyViews?.views) ? storyViews.views : [];
    if (!rpcName(storyViews).toLowerCase().endsWith('storyviews')
      || storyViewItems.length !== 3
      || storyViewItems.some((item: any) => Number(item?.viewsCount) !== 0
        || (Array.isArray(item?.recentViewers) && item.recentViewers.length > 0))) {
      throw new Error(`unexpected stories.getStoriesViews result: type=${rpcName(storyViews)}, count=${storyViewItems.length}`);
    }

    const viewsList = await client.invoke(new Api.stories.GetStoryViewsList({
      peer: new Api.InputPeerSelf(), id: 1, offset: '', limit: 100,
    }));
    const listedViews = Array.isArray(viewsList?.views) ? viewsList.views : [];
    if (!rpcName(viewsList).toLowerCase().endsWith('storyviewslist')
      || Number(viewsList?.count) !== 0
      || Number(viewsList?.viewsCount) !== 0
      || Number(viewsList?.reactionsCount) !== 0
      || listedViews.length !== 0) {
      throw new Error(`unexpected stories.getStoryViewsList result: type=${rpcName(viewsList)}, count=${String(viewsList?.count)}, views=${listedViews.length}`);
    }

    const reactionsList = await client.invoke(rawGetStoryReactionsListRequest(1, 100));
    const listedReactions = Array.isArray(reactionsList?.reactions) ? reactionsList.reactions : [];
    if (!rpcName(reactionsList).toLowerCase().endsWith('storyreactionslist')
      || Number(reactionsList?.count) !== 0
      || listedReactions.length !== 0) {
      throw new Error(`unexpected stories.getStoryReactionsList result: type=${rpcName(reactionsList)}, count=${String(reactionsList?.count)}, reactions=${listedReactions.length}`);
    }

    const after = storySnapshot();
    if (before !== after || viewsBaseline !== storyViewsBaseline()) {
      throw new Error('story KV changed during read-only probe');
    }
    console.log(JSON.stringify({
      userId: String(me.id),
      archiveType: rpcName(archive),
      archiveCount: archiveIds.length,
      archiveIds,
      byIdType: rpcName(byId),
      byIdCount: byIdIds.length,
      byIdIds,
      getStoriesViews: {
        type: rpcName(storyViews),
        count: storyViewItems.length,
        viewsCounts: storyViewItems.map((item: any) => Number(item?.viewsCount)),
      },
      getStoryViewsList: {
        type: rpcName(viewsList),
        count: Number(viewsList.count),
        viewsCount: Number(viewsList.viewsCount),
        reactionsCount: Number(viewsList.reactionsCount),
        views: listedViews.length,
      },
      getStoryReactionsList: {
        type: rpcName(reactionsList),
        count: Number(reactionsList.count),
        reactions: listedReactions.length,
      },
      storyKvSha256: after,
      writes: 0,
    }));
  } finally {
    client.destroy();
  }
}

main().then(() => process.exit(0)).catch((error) => {
  console.error(`stories production probe failed: ${error instanceof Error ? error.stack || error.message : String(error)}`);
  process.exit(1);
});
