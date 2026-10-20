import fs from 'node:fs';
import path from 'node:path';
import { createRequire } from 'node:module';
import { authSession, loadFixtureAuthKey } from '../web-isolated/fixture-auth';

/**
 * A three phase probe used by run-backend-restart-critical.sh.
 *
 * The marker and state file are deliberately explicit so a failed after phase
 * can be inspected and cleanup can still remove only rows owned by this run.
 */
const root = process.env.TEAMGRAM_GRAMJS_DIR;
if (!root) throw new Error('TEAMGRAM_GRAMJS_DIR is required');
const kind = process.env.RESTART_PROBE_KIND || '';
const phase = process.env.PROBE_PHASE || '';
const tag = process.env.PROBE_TAG || `backend-restart-${Date.now()}-${process.pid}`;
const stateFile = process.env.PROBE_STATE_FILE || `/tmp/teamgram-${kind}-${tag}.json`;
if (!['channel', 'settings', 'business'].includes(kind)) throw new Error(`unsupported RESTART_PROBE_KIND: ${kind}`);
if (!['before', 'after', 'cleanup'].includes(phase)) throw new Error(`unsupported PROBE_PHASE: ${phase}`);

const dcId = Number(process.env.TG_DC_ID || '2');
const dcHost = process.env.TG_DC_HOST || '127.0.0.1';
const dcPort = Number(process.env.TG_DC_PORT || '11443');
const apiId = Number(process.env.TG_API_ID || '1');
const apiHash = process.env.TG_API_HASH || '0123456789abcdef0123456789abcdef';
const quietLogger = { debug() {}, info() {}, warn() {}, error() {} };
const A_USERNAME = process.env.A_USERNAME || 'isolated_test_01';
const B_USERNAME = process.env.B_USERNAME || 'isolated_test_02';

const globalAny = globalThis as typeof globalThis & { self?: any; addEventListener?: () => void };
globalAny.self ??= globalThis;
globalAny.addEventListener ??= () => {};
globalAny.self.addEventListener ??= globalAny.addEventListener;

const requireModule = createRequire(import.meta.url);
const TelegramClient = requireModule(path.join(root, 'src/lib/gramjs/client/TelegramClient.ts')).default;
const CallbackSession = requireModule(path.join(root, 'src/lib/gramjs/sessions/CallbackSession.ts')).default;
const Api = requireModule(path.join(root, 'src/lib/gramjs/tl/index.ts')).Api;
const Connection = requireModule(path.join(root, 'src/lib/gramjs/network/connection/TCPObfuscated.ts')).ConnectionTCPObfuscated;

function rpcName(value: any): string {
  return value?.className || value?.constructor?.name || typeof value;
}

function requireCondition(value: unknown, message: string): asserts value {
  if (!value) throw new Error(message);
}

function randomId(): bigint {
  return BigInt(Date.now()) * 1_000_000n + BigInt(process.pid);
}

function readState<T extends Record<string, any>>(): T {
  if (!fs.existsSync(stateFile)) {
    if (phase === 'cleanup') return { kind, tag } as T;
    throw new Error(`state file is missing: ${stateFile}`);
  }
  return JSON.parse(fs.readFileSync(stateFile, 'utf8')) as T;
}

function writeState(value: Record<string, any>): void {
  fs.mkdirSync(path.dirname(stateFile), { recursive: true });
  fs.writeFileSync(stateFile, `${JSON.stringify(value, null, 2)}\n`, { encoding: 'utf8', mode: 0o600 });
}

function safeUsername(value: string): string {
  const result = value.replace(/[^A-Za-z0-9_]/g, '').slice(0, 30);
  return result.length >= 5 ? result : `restart${Date.now()}`.slice(0, 30);
}

function stateFields(value: any) {
  return {
    pts: Number(value?.pts ?? 0),
    qts: Number(value?.qts ?? 0),
    seq: Number(value?.seq ?? 0),
    date: Number(value?.date ?? 0),
  };
}

function assertState(value: any, label: string): void {
  const state = stateFields(value);
  requireCondition(state.date > 0, `${label} updates state has no date`);
  for (const [name, field] of Object.entries(state)) {
    // Telegram uses seq=-1 when this account has no global sequence yet.
    const minimum = name === 'seq' ? -1 : 0;
    requireCondition(Number.isInteger(field) && field >= minimum, `${label} updates state has invalid ${name}=${field}`);
  }
}

function history(ApiModule: any, peer: any): any {
  return new ApiModule.messages.GetHistory({
    peer, offsetId: 0, offsetDate: 0, addOffset: 0, limit: 100, maxId: 0, minId: 0, hash: 0n,
  });
}

function messageList(value: any): any[] {
  return [
    ...(Array.isArray(value?.messages) ? value.messages : []),
    ...(Array.isArray(value?.newMessages) ? value.newMessages : []),
  ];
}

function chatList(value: any): any[] {
  return [
    ...(Array.isArray(value?.chats) ? value.chats : []),
    ...(Array.isArray(value?.updates?.chats) ? value.updates.chats : []),
  ];
}

function inputChannel(channel: any): any {
  requireCondition(channel?.id !== undefined && channel?.accessHash !== undefined, 'channel has no access hash');
  return new Api.InputChannel({ channelId: channel.id, accessHash: channel.accessHash });
}

function inputPeerChannel(channel: any): any {
  requireCondition(channel?.id !== undefined && channel?.accessHash !== undefined, 'channel has no peer access hash');
  return new Api.InputPeerChannel({ channelId: channel.id, accessHash: channel.accessHash });
}

function extractMessage(update: any, text: string): any | undefined {
  if (!update || typeof update !== 'object') return undefined;
  if (Array.isArray(update.updates)) {
    for (const nested of update.updates) {
      const found = extractMessage(nested, text);
      if (found) return found;
    }
  }
  if (update.message && typeof update.message === 'object' && update.message.message === text) {
    return { id: Number(update.message.id || update.id || 0), pts: Number(update.pts || 0), date: Number(update.message.date || update.date || 0) };
  }
  if (typeof update.message === 'string' && update.message === text) {
    return { id: Number(update.id || 0), pts: Number(update.pts || 0), date: Number(update.date || 0) };
  }
  return undefined;
}

function updateHandler(): { build: (value: any) => any } {
  return { build: (value: any) => value };
}

async function waitForMessage(events: any[], text: string, timeout = 10_000): Promise<any> {
  const deadline = Date.now() + timeout;
  while (Date.now() < deadline) {
    const event = events.find((item) => item?.text === text);
    if (event) return event;
    await new Promise((resolve) => setTimeout(resolve, 50));
  }
  throw new Error(`recipient update timeout for ${text}; observed=${JSON.stringify(events.slice(-5))}`);
}

type Actor = { client: any; id: bigint; username: string };

async function actor(role: 'A' | 'B', updates = false): Promise<Actor> {
  const username = role === 'A' ? A_USERNAME : B_USERNAME;
  const supplied = process.env[`${role}_AUTH_B64`];
  const auth = supplied || loadFixtureAuthKey(username);
  if (!auth) throw new Error(`no auth key for ${role} (${username}); set ${role}_AUTH_B64`);
  let lastError: unknown;
  for (let attempt = 0; attempt < 24; attempt += 1) {
    const session = authSession({ CallbackSession } as any, auth, dcId, dcHost, dcPort);
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
      sequentialUpdates: true,
      baseLogger: quietLogger,
    });
    (client as any)._borrowExportedSender = async () => undefined;
    if (!updates) (client as any)._updateLoop = async () => undefined;
    try {
      await client.connect();
      const me = await client.getMe();
      requireCondition(me?.id !== undefined, `${role} getMe returned no user`);
      return { client, id: BigInt(me.id), username };
    } catch (error) {
      lastError = error;
      try { client.destroy(); } catch {}
      if (attempt < 23) await new Promise((resolve) => setTimeout(resolve, 1000));
    }
  }
  throw lastError || new Error(`${role} connection failed`);
}

async function searchUser(owner: Actor, username: string): Promise<any> {
  const result = await owner.client.invoke(new Api.contacts.Search({ q: username, limit: 10 }));
  const user = result?.users?.find((candidate: any) => candidate?.username === username);
  requireCondition(user?.id !== undefined && user?.accessHash !== undefined, `contacts.search did not find ${username}`);
  return user;
}

class RawBusinessRequest {
  SUBCLASS_OF_ID = 0;
  classType = 'request';
  constructor(public readonly CONSTRUCTOR_ID: number, private readonly payload: Uint8Array, public readonly className: string) {}
  getBytes() { return new Uint8Array(Buffer.concat([Buffer.from(uint32(this.CONSTRUCTOR_ID)), Buffer.from(this.payload)])); }
  readResult(reader: any) { return reader.tgReadObject(); }
}

function int32(value: number): Uint8Array {
  const bytes = Buffer.alloc(4); bytes.writeInt32LE(value); return bytes;
}

function uint32(value: number): Uint8Array {
  const bytes = Buffer.alloc(4); bytes.writeUInt32LE(value >>> 0); return bytes;
}

function tlString(value: string): Uint8Array {
  const raw = Buffer.from(value, 'utf8');
  const header = raw.length < 254 ? 1 : 4;
  const bytes = Buffer.alloc((header + raw.length + 3) & ~3);
  if (header === 1) bytes[0] = raw.length;
  else { bytes[0] = 254; bytes[1] = raw.length & 255; bytes[2] = (raw.length >>> 8) & 255; bytes[3] = (raw.length >>> 16) & 255; }
  raw.copy(bytes, header);
  return bytes;
}

function rawRequest(constructorId: number, className: string, ...parts: Uint8Array[]): RawBusinessRequest {
  return new RawBusinessRequest(constructorId, new Uint8Array(Buffer.concat(parts.map((part) => Buffer.from(part)))), className);
}

function createBusinessLink(message: string, title: string): RawBusinessRequest {
  return rawRequest(0x8851e68e, 'account.createBusinessChatLink', new Uint8Array(Buffer.concat([
    Buffer.from(uint32(0x11679fa7)), Buffer.from(int32(2)), Buffer.from(tlString(message)), Buffer.from(tlString(title)),
  ])));
}

function getBusinessLinks(): RawBusinessRequest {
  return rawRequest(0x6f70dde1, 'account.getBusinessChatLinks');
}

function deleteBusinessLink(slug: string): RawBusinessRequest {
  return rawRequest(0x60073674, 'account.deleteBusinessChatLink', tlString(slug));
}

function notifyPeer(user: any): any {
  return new Api.InputNotifyPeer({ peer: new Api.InputPeerUser({ userId: user.id, accessHash: user.accessHash }) });
}

function soundSnapshot(sound: any): any {
  const name = rpcName(sound);
  if (name === 'NotificationSoundLocal') return { name, title: sound.title || '', data: sound.data || '' };
  if (name === 'NotificationSoundRingtone') return { name, id: String(sound.id || 0) };
  return { name };
}

function soundFromSnapshot(snapshot: any): any {
  switch (snapshot?.name) {
    case 'NotificationSoundNone': return new Api.NotificationSoundNone();
    case 'NotificationSoundLocal': return new Api.NotificationSoundLocal({ title: snapshot.title || '', data: snapshot.data || '' });
    case 'NotificationSoundRingtone': return new Api.NotificationSoundRingtone({ id: BigInt(snapshot.id || 0) });
    default: return new Api.NotificationSoundDefault();
  }
}

function notifySnapshot(value: any): any {
  return {
    showPreviews: value?.showPreviews === undefined ? undefined : Boolean(value.showPreviews),
    silent: value?.silent === undefined ? undefined : Boolean(value.silent),
    muteUntil: value?.muteUntil === undefined ? undefined : Number(value.muteUntil),
    iosSound: soundSnapshot(value?.iosSound),
    androidSound: soundSnapshot(value?.androidSound),
    otherSound: soundSnapshot(value?.otherSound),
  };
}

function notifyInput(snapshot: any): any {
  const savedSound = [snapshot.otherSound, snapshot.iosSound, snapshot.androidSound]
    .find((candidate: any) => candidate?.name && candidate.name !== 'undefined');
  const input: Record<string, any> = {
    showPreviews: snapshot.showPreviews,
    silent: snapshot.silent,
    muteUntil: snapshot.muteUntil,
  };
  if (savedSound?.name && savedSound.name !== 'undefined') input.sound = soundFromSnapshot(savedSound);
  for (const key of ['showPreviews', 'silent', 'muteUntil']) if (input[key] === undefined) delete input[key];
  return new Api.InputPeerNotifySettings(input);
}

function assertNotifyRestored(before: any, after: any): void {
  for (const key of ['showPreviews', 'silent', 'muteUntil']) {
    requireCondition(before[key] === after[key], `settings restore changed ${key}: ${before[key]} -> ${after[key]}`);
  }
  const beforeSound = [before.otherSound, before.iosSound, before.androidSound]
    .find((candidate: any) => candidate?.name && candidate.name !== 'undefined')?.name || 'undefined';
  const afterSound = [after.otherSound, after.iosSound, after.androidSound]
    .find((candidate: any) => candidate?.name && candidate.name !== 'undefined')?.name || 'undefined';
  requireCondition(beforeSound === afterSound, `settings restore changed sound: ${beforeSound} -> ${afterSound}`);
}

async function runChannel(): Promise<void> {
  if (phase === 'before') {
    const a = await actor('A'); const b = await actor('B');
    const username = safeUsername(`restart_${tag}`);
    const title = `Backend restart ${tag}`;
    const created = await a.client.invoke(new Api.channels.CreateChannel({ broadcast: true, title, about: `probe ${tag}` }));
    const channel = chatList(created).find((item: any) => item?.title === title);
    requireCondition(channel, 'channels.createChannel did not return the tagged channel');
    const ownerChannel = inputChannel(channel);
    const updated = await a.client.invoke(new Api.channels.UpdateUsername({ channel: ownerChannel, username }));
    requireCondition(updated === true || rpcName(updated).toLowerCase().includes('true'), 'channels.updateUsername failed');
    const userB = await searchUser(a, B_USERNAME);
    const invited = await a.client.invoke(new Api.channels.InviteToChannel({ channel: ownerChannel, users: [new Api.InputUser({ userId: userB.id, accessHash: userB.accessHash })] }));
    requireCondition(rpcName(invited).toLowerCase().includes('invitedusers'), `channel invite returned ${rpcName(invited)}`);
    const peer = inputPeerChannel(channel);
    const message = `${tag}:channel:before`;
    const sent = await a.client.invoke(new Api.messages.SendMessage({ peer, message, randomId: randomId() }));
    requireCondition(rpcName(sent).toLowerCase().includes('updates'), `channel send returned ${rpcName(sent)}`);
    const [stateA, stateB] = await Promise.all([a.client.invoke(new Api.updates.GetState()), b.client.invoke(new Api.updates.GetState())]);
    assertState(stateA, 'channel before A'); assertState(stateB, 'channel before B');
    writeState({ kind, tag, username, title, message, stateA: stateFields(stateA), stateB: stateFields(stateB), channelId: String(channel.id) });
    a.client.destroy(); b.client.destroy();
    console.log(JSON.stringify({ phase, kind, tag, username, message, state: { a: stateFields(stateA), b: stateFields(stateB) } }));
    return;
  }
  const saved = readState<any>();
  const a = await actor('A', phase === 'after'); const b = await actor('B', phase === 'after');
  try {
    if (phase === 'cleanup') {
      try {
        const resolved = chatList(await a.client.invoke(new Api.contacts.ResolveUsername({ username: saved.username })))
          .find((item: any) => rpcName(item).toLowerCase().includes('channel'));
        if (resolved) {
          const ownerChannel = inputChannel(resolved);
          const cleared = await a.client.invoke(new Api.channels.UpdateUsername({ channel: ownerChannel, username: '' }));
          requireCondition(cleared === true || rpcName(cleared).toLowerCase().includes('true'), 'channel username cleanup failed');
          await a.client.invoke(new Api.channels.DeleteChannel({ channel: ownerChannel }));
        }
      } finally {
        try { fs.unlinkSync(stateFile); } catch {}
      }
      console.log(JSON.stringify({ phase, kind, tag, deleted: true }));
      return;
    }
    const resolvedA = chatList(await a.client.invoke(new Api.contacts.ResolveUsername({ username: saved.username }))).find((item: any) => rpcName(item).toLowerCase().includes('channel'));
    const resolvedB = chatList(await b.client.invoke(new Api.contacts.ResolveUsername({ username: saved.username }))).find((item: any) => rpcName(item).toLowerCase().includes('channel'));
    requireCondition(resolvedA && resolvedB, 'tagged channel did not resolve after restart');
    const peerA = inputPeerChannel(resolvedA); const peerB = inputPeerChannel(resolvedB);
    const historyA = await a.client.invoke(history(Api, peerA)); const historyB = await b.client.invoke(history(Api, peerB));
    requireCondition(messageList(historyA).some((item: any) => item?.message === saved.message), 'owner history lost tagged channel message');
    requireCondition(messageList(historyB).some((item: any) => item?.message === saved.message), 'member history lost tagged channel message');
    const [beforeA, beforeB] = await Promise.all([a.client.invoke(new Api.updates.GetState()), b.client.invoke(new Api.updates.GetState())]);
    assertState(beforeA, 'channel after A before write'); assertState(beforeB, 'channel after B before write');
    const events: any[] = [];
    if (phase === 'after') b.client.addEventHandler((update: any) => { const found = extractMessage(update, `${tag}:channel:after`); if (found) events.push({ ...found, text: `${tag}:channel:after` }); }, updateHandler());
    await Promise.all([a.client.invoke(new Api.account.UpdateStatus({ offline: false })), b.client.invoke(new Api.account.UpdateStatus({ offline: false }))]);
    const afterText = `${tag}:channel:after`;
    const sent = await a.client.invoke(new Api.messages.SendMessage({ peer: peerA, message: afterText, randomId: randomId() }));
    requireCondition(rpcName(sent).toLowerCase().includes('updates'), `channel after send returned ${rpcName(sent)}`);
    const event = await waitForMessage(events, afterText);
    const afterHistory = await b.client.invoke(history(Api, peerB));
    requireCondition(messageList(afterHistory).some((item: any) => item?.message === afterText), 'member history lost post-restart message');
    const [afterA, afterB] = await Promise.all([a.client.invoke(new Api.updates.GetState()), b.client.invoke(new Api.updates.GetState())]);
    assertState(afterA, 'channel after A'); assertState(afterB, 'channel after B');
    requireCondition(stateFields(afterA).date >= saved.stateA.date && stateFields(afterB).date >= saved.stateB.date, 'channel update date regressed across restart');
    requireCondition(event.id > 0 && event.pts > 0, `member did not receive a canonical channel update: ${JSON.stringify(event)}`);
    writeState({ ...saved, postRestartMessage: afterText, postRestartEvent: event, stateAfter: { a: stateFields(afterA), b: stateFields(afterB) } });
    console.log(JSON.stringify({ phase, kind, tag, channel: saved.username, read: true, fanout: event, stateAfter: { a: stateFields(afterA), b: stateFields(afterB) } }));
  } finally {
    a.client.destroy(); b.client.destroy();
  }
}

async function runSettings(): Promise<void> {
  const a = await actor('A');
  const b = await actor('B');
  try {
    const userB = await searchUser(a, B_USERNAME);
    const peer = notifyPeer(userB);
    if (phase === 'before') {
      const original = await a.client.invoke(new Api.account.GetNotifySettings({ peer }));
      const markerMute = 2_147_483_000 - (tag.length % 1000);
      const saved = notifySnapshot(original);
      const updated = await a.client.invoke(new Api.account.UpdateNotifySettings({ peer, settings: new Api.InputPeerNotifySettings({ muteUntil: markerMute, showPreviews: false, silent: true, sound: new Api.NotificationSoundDefault() }) }));
      requireCondition(updated === true || rpcName(updated).toLowerCase().includes('true'), 'settings marker write failed');
      const readback = await a.client.invoke(new Api.account.GetNotifySettings({ peer }));
      requireCondition(Number(readback?.muteUntil) === markerMute && readback?.silent === true && readback?.showPreviews === false, 'settings marker did not read back');
      const [stateA, stateB] = await Promise.all([a.client.invoke(new Api.updates.GetState()), b.client.invoke(new Api.updates.GetState())]);
      assertState(stateA, 'settings before A'); assertState(stateB, 'settings before B');
      writeState({ kind, tag, peerId: String(userB.id), peerAccessHash: String(userB.accessHash), markerMute, original: saved, stateA: stateFields(stateA), stateB: stateFields(stateB) });
      console.log(JSON.stringify({ phase, kind, tag, markerMute, original: saved }));
      return;
    }
    const saved = readState<any>();
    if (phase === 'cleanup') {
      const restored = await a.client.invoke(new Api.account.UpdateNotifySettings({ peer: new Api.InputNotifyPeer({ peer: new Api.InputPeerUser({ userId: BigInt(saved.peerId), accessHash: BigInt(saved.peerAccessHash) }) }), settings: notifyInput(saved.original) }));
      requireCondition(restored === true || rpcName(restored).toLowerCase().includes('true'), 'settings restore failed');
      const final = await a.client.invoke(new Api.account.GetNotifySettings({ peer: new Api.InputNotifyPeer({ peer: new Api.InputPeerUser({ userId: BigInt(saved.peerId), accessHash: BigInt(saved.peerAccessHash) }) }) }));
      assertNotifyRestored(saved.original, notifySnapshot(final));
      try { fs.unlinkSync(stateFile); } catch {}
      console.log(JSON.stringify({ phase, kind, tag, restored: true }));
      return;
    }
    const ownerPeer = new Api.InputNotifyPeer({ peer: new Api.InputPeerUser({ userId: BigInt(saved.peerId), accessHash: BigInt(saved.peerAccessHash) }) });
    const current = await a.client.invoke(new Api.account.GetNotifySettings({ peer: ownerPeer }));
    if (phase === 'after') {
      requireCondition(Number(current?.muteUntil) === saved.markerMute && current?.silent === true && current?.showPreviews === false, 'settings marker did not survive backend restart');
      const reverse = await b.client.invoke(new Api.account.GetNotifySettings({ peer: new Api.InputNotifyUsers() }));
      requireCondition(Number(reverse?.muteUntil ?? 0) !== saved.markerMute, 'settings marker leaked to the other account');
      const [stateA, stateB] = await Promise.all([a.client.invoke(new Api.updates.GetState()), b.client.invoke(new Api.updates.GetState())]);
      assertState(stateA, 'settings after A'); assertState(stateB, 'settings after B');
      requireCondition(stateFields(stateA).date >= saved.stateA.date && stateFields(stateB).date >= saved.stateB.date, 'settings state date regressed across restart');
      console.log(JSON.stringify({ phase, kind, tag, markerMute: saved.markerMute, restored: false, stateAfter: { a: stateFields(stateA), b: stateFields(stateB) } }));
      return;
    }
    const restored = await a.client.invoke(new Api.account.UpdateNotifySettings({ peer: ownerPeer, settings: notifyInput(saved.original) }));
    requireCondition(restored === true || rpcName(restored).toLowerCase().includes('true'), 'settings restore failed');
    const final = await a.client.invoke(new Api.account.GetNotifySettings({ peer: ownerPeer }));
    requireCondition(Number(final?.muteUntil ?? 0) !== saved.markerMute, 'settings cleanup left marker mute');
    assertNotifyRestored(saved.original, notifySnapshot(final));
    try { fs.unlinkSync(stateFile); } catch {}
    console.log(JSON.stringify({ phase, kind, tag, restored: true }));
  } finally {
    a.client.destroy(); b.client.destroy();
  }
}

async function runBusiness(): Promise<void> {
  const a = await actor('A', phase === 'after');
  const b = await actor('B', phase === 'after');
  try {
    if (phase === 'before') {
      const message = `${tag}:business-link`;
      const created = await a.client.invoke(createBusinessLink(message, `Probe ${tag}`));
      requireCondition(rpcName(created) === 'BusinessChatLink', `business link create returned ${rpcName(created)}`);
      requireCondition(typeof created.link === 'string' && created.link.length > 0, 'business link has no slug');
      const listed = await a.client.invoke(getBusinessLinks());
      requireCondition(Array.isArray(listed.links) && listed.links.some((item: any) => item.link === created.link && item.message === message), 'business link did not read back');
      const [stateA, stateB] = await Promise.all([a.client.invoke(new Api.updates.GetState()), b.client.invoke(new Api.updates.GetState())]);
      assertState(stateA, 'business before A'); assertState(stateB, 'business before B');
      writeState({ kind, tag, slug: created.link, message, stateA: stateFields(stateA), stateB: stateFields(stateB) });
      console.log(JSON.stringify({ phase, kind, tag, slug: created.link }));
      return;
    }
    const saved = readState<any>();
    if (phase === 'cleanup') {
      let slug = saved.slug;
      if (!slug) {
        const links = await a.client.invoke(getBusinessLinks());
        slug = Array.isArray(links.links)
          ? links.links.find((item: any) => item.message === `${tag}:business-link`)?.link
          : undefined;
      }
      requireCondition(slug, 'could not locate tagged business link for cleanup');
      const deleted = await a.client.invoke(deleteBusinessLink(slug));
      requireCondition(deleted === true || rpcName(deleted).toLowerCase().includes('true'), `business link cleanup returned ${rpcName(deleted)}`);
      if (saved.postRestartMessage && saved.recipientId && saved.recipientAccessHash) {
        const peerB = new Api.InputPeerUser({ userId: BigInt(saved.recipientId), accessHash: BigInt(saved.recipientAccessHash) });
        const userA = await searchUser(b, A_USERNAME);
        const peerA = new Api.InputPeerUser({ userId: userA.id, accessHash: userA.accessHash });
        for (const [client, peer] of [[a.client, peerB], [b.client, peerA]] as const) {
          const ids = messageList(await client.invoke(history(Api, peer)))
            .filter((item: any) => item?.message === saved.postRestartMessage && item?.id !== undefined)
            .map((item: any) => Number(item.id));
          if (ids.length) await client.invoke(new Api.messages.DeleteMessages({ revoke: true, id: ids }));
          const remaining = messageList(await client.invoke(history(Api, peer)))
            .filter((item: any) => item?.message === saved.postRestartMessage);
          requireCondition(remaining.length === 0, `business message cleanup left ${remaining.length} tagged messages`);
        }
      }
      try { fs.unlinkSync(stateFile); } catch {}
      console.log(JSON.stringify({ phase, kind, tag, deleted: true }));
      return;
    }
    const links = await a.client.invoke(getBusinessLinks());
    const link = Array.isArray(links.links) ? links.links.find((item: any) => item.link === saved.slug && item.message === saved.message) : undefined;
    requireCondition(link, 'business link was not readable after backend restart');
    const resolved = await b.client.invoke(new Api.account.ResolveBusinessChatLink({ slug: saved.slug }));
    requireCondition(rpcName(resolved) === 'account.ResolvedBusinessChatLinks' && resolved.message === saved.message, 'business resolver did not read the tagged link');
    const full = await a.client.invoke(new Api.users.GetFullUser({ id: new Api.InputUserSelf() }));
    requireCondition((rpcName(full) === 'users.UserFull' || rpcName(full) === 'UserFull') && full?.fullUser, `users.getFullUser returned ${rpcName(full)}`);
    requireCondition(String(full.fullUser.id) === String((await a.client.getMe()).id), 'users.getFullUser hydrated the wrong user');
    const userB = await searchUser(a, B_USERNAME);
    const peerB = new Api.InputPeerUser({ userId: userB.id, accessHash: userB.accessHash });
    const events: any[] = [];
    if (phase === 'after') b.client.addEventHandler((update: any) => { const found = extractMessage(update, `${tag}:business:after`); if (found) events.push({ ...found, text: `${tag}:business:after` }); }, updateHandler());
    await Promise.all([a.client.invoke(new Api.account.UpdateStatus({ offline: false })), b.client.invoke(new Api.account.UpdateStatus({ offline: false }))]);
    const afterText = `${tag}:business:after`;
    writeState({ ...saved, postRestartMessage: afterText, recipientId: String(userB.id), recipientAccessHash: String(userB.accessHash) });
    const sent = await a.client.invoke(new Api.messages.SendMessage({ peer: peerB, message: afterText, randomId: randomId() }));
    requireCondition(rpcName(sent).toLowerCase().includes('updates'), `business recipient send returned ${rpcName(sent)}`);
    const event = await waitForMessage(events, afterText);
    requireCondition(event.id > 0 && event.pts > 0, `business recipient update was not canonical: ${JSON.stringify(event)}`);
    if (phase === 'after') {
      const [stateA, stateB] = await Promise.all([a.client.invoke(new Api.updates.GetState()), b.client.invoke(new Api.updates.GetState())]);
      assertState(stateA, 'business after A'); assertState(stateB, 'business after B');
      requireCondition(stateFields(stateA).date >= saved.stateA.date && stateFields(stateB).date >= saved.stateB.date, 'business state date regressed across restart');
      writeState({ ...saved, postRestartMessage: afterText, recipientId: String(userB.id), recipientAccessHash: String(userB.accessHash), event, stateAfter: { a: stateFields(stateA), b: stateFields(stateB) } });
      console.log(JSON.stringify({ phase, kind, tag, slug: saved.slug, fullUser: rpcName(full), recipientUpdate: event, stateAfter: { a: stateFields(stateA), b: stateFields(stateB) } }));
      return;
    }
    console.log(JSON.stringify({ phase, kind, tag, verified: true }));
  } finally {
    a.client.destroy(); b.client.destroy();
  }
}

async function main(): Promise<void> {
  if (kind === 'channel') await runChannel();
  else if (kind === 'settings') await runSettings();
  else await runBusiness();
}

main().then(() => process.exit(0)).catch((error) => {
  console.error(`backend restart ${kind}/${phase} probe failed: ${error instanceof Error ? error.stack || error.message : String(error)}`);
  process.exit(1);
});
