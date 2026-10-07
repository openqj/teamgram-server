import path from 'node:path';
import { createRequire } from 'node:module';

type Client = any;

for (const key of ['TEAMGRAM_GRAMJS_DIR', 'A_AUTH_B64', 'B_AUTH_B64', 'A_USER_ID', 'B_USER_ID', 'A_ACCESS_HASH', 'B_ACCESS_HASH']) {
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
const A_ID = BigInt(process.env.A_USER_ID!);
const B_ID = BigInt(process.env.B_USER_ID!);
const A_HASH = BigInt(process.env.A_ACCESS_HASH!);
const B_HASH = BigInt(process.env.B_ACCESS_HASH!);

const globalAny = globalThis as typeof globalThis & { self?: any; addEventListener?: () => void };
globalAny.self ??= globalThis;
globalAny.addEventListener ??= () => {};
globalAny.self.addEventListener ??= globalAny.addEventListener;

function rpcName(value: any): string { return value?.className || value?.constructor?.name || typeof value; }
function isTrue(value: any): boolean { return value === true || rpcName(value).toLowerCase().includes('true'); }
function inputUser(id: bigint, accessHash: bigint): any { return new Api.InputUser({ userId: id, accessHash }); }
function inputPeer(id: bigint, accessHash: bigint): any { return new Api.InputPeerUser({ userId: id, accessHash }); }
function notifyPeer(peer: any): any { return new Api.InputNotifyPeer({ peer }); }
function dialogPeer(peer: any): any { return new Api.InputDialogPeer({ peer }); }

class GetBirthdaysRequest {
  CONSTRUCTOR_ID = 0xdaeda864;
  SUBCLASS_OF_ID = 0;
  className = 'contacts.getBirthdays';
  classType = 'request';

  getBytes() {
    const bytes = new Uint8Array(4);
    new DataView(bytes.buffer).setUint32(0, this.CONSTRUCTOR_ID, true);
    return bytes;
  }

  readResult(reader: any) {
    return reader.tgReadObject();
  }
}

function makeClient(auth: string): Client {
  const session = new CallbackSession({ mainDcId: dcId, keys: { [dcId]: Buffer.from(auth, 'base64').toString('hex') } }, () => {});
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

function notifyInput(settings: any): any {
  return new Api.InputPeerNotifySettings({
    muteUntil: Number(settings?.muteUntil || 0),
    showPreviews: settings?.showPreviews !== false,
    silent: settings?.silent === true,
    sound: new Api.NotificationSoundDefault(),
  });
}

function draftOf(result: any): any {
  return result?.dialogs?.[0]?.draft;
}

async function waitForMessage(client: Client, peer: any, text: string): Promise<any> {
  let result: any;
  for (let attempt = 0; attempt < 12; attempt += 1) {
    result = await client.invoke(new Api.messages.GetHistory({
      peer, offsetId: 0, offsetDate: 0, addOffset: 0, limit: 20, maxId: 0, minId: 0, hash: 0n,
    }));
    const message = Array.isArray(result?.messages)
      ? result.messages.find((item: any) => item?.message === text)
      : undefined;
    if (message) return message;
    await new Promise((resolve) => setTimeout(resolve, 250));
  }
  throw new Error(`message not visible after retries: ${text}`);
}

async function main() {
  const a = makeClient(process.env.A_AUTH_B64!);
  const b = makeClient(process.env.B_AUTH_B64!);
  const peer = inputPeer(B_ID, B_HASH);
  const results: Record<string, unknown> = {};
  let ttlBefore: number | undefined;
  let contactAdded = false;
  let pinChanged = false;
  let messagePinned = false;
  let outboxMessageId = 0;
  let registeredDeviceToken = '';
  try {
    await a.connect();
    await b.connect();
    const meA = await a.getMe();
    const meB = await b.getMe();
    if (String(meA?.id) !== String(A_ID) || String(meB?.id) !== String(B_ID)) throw new Error('auth key user mismatch');

    if (process.env.PROBE_REPORT_INTAKE === '1') {
      const reportTag = process.env.PROBE_REPORT_TAG || `prod-report-intake-${Date.now()}-${process.pid}`;
      const report = await a.invoke(new Api.account.ReportPeer({
        peer,
        reason: new Api.InputReportReasonOther(),
        message: reportTag,
      }));
      if (!isTrue(report)) throw new Error(`account.reportPeer returned ${rpcName(report)}`);
      results.reportPeer = { result: rpcName(report), tag: reportTag };
    }

    const ttl = await a.invoke(new Api.account.GetAccountTTL());
    ttlBefore = Number(ttl?.days);
    const ttlCandidate = ttlBefore === 30 ? 90 : 30;
    const ttlSet = await a.invoke(new Api.account.SetAccountTTL({ ttl: new Api.AccountDaysTTL({ days: ttlCandidate }) }));
    const ttlRead = await a.invoke(new Api.account.GetAccountTTL());
    if (Number(ttlRead?.days) !== ttlCandidate) throw new Error('account TTL did not read back');
    const ttlRestore = await a.invoke(new Api.account.SetAccountTTL({ ttl: new Api.AccountDaysTTL({ days: ttlBefore }) }));
    const ttlFinal = await a.invoke(new Api.account.GetAccountTTL());
    if (Number(ttlFinal?.days) !== ttlBefore) throw new Error('account TTL restore did not read back');
    results.ttl = { get: rpcName(ttl), set: rpcName(ttlSet), read: rpcName(ttlRead), restore: rpcName(ttlRestore) };

    const notify = await a.invoke(new Api.account.GetNotifySettings({ peer: notifyPeer(peer) }));
    const notifySet = await a.invoke(new Api.account.UpdateNotifySettings({
      peer: notifyPeer(peer),
      settings: new Api.InputPeerNotifySettings({ muteUntil: 2147483647, showPreviews: false, silent: true, sound: new Api.NotificationSoundDefault() }),
    }));
    const notifyRead = await a.invoke(new Api.account.GetNotifySettings({ peer: notifyPeer(peer) }));
    if (Number(notifyRead?.muteUntil) !== 2147483647 || notifyRead?.showPreviews !== false || notifyRead?.silent !== true) {
      throw new Error('notify settings did not read back');
    }
    const notifyRestore = await a.invoke(new Api.account.UpdateNotifySettings({ peer: notifyPeer(peer), settings: notifyInput(notify) }));
    const notifyFinal = await a.invoke(new Api.account.GetNotifySettings({ peer: notifyPeer(peer) }));
    if (Number(notifyFinal?.muteUntil || 0) !== Number(notify?.muteUntil || 0) || notifyFinal?.silent !== notify?.silent) {
      throw new Error(`notify settings restore did not read back: before=${JSON.stringify(notify)} final=${JSON.stringify(notifyFinal)}`);
    }
    results.notify = { get: rpcName(notify), set: rpcName(notifySet), read: rpcName(notifyRead), restore: rpcName(notifyRestore), final: rpcName(notifyFinal) };

    const contactBefore = await a.invoke(new Api.contacts.GetContacts({ hash: 0n }));
    const hadContact = Array.isArray(contactBefore?.contacts)
      && contactBefore.contacts.some((item: any) => String(item?.userId) === String(B_ID));
    if (!hadContact) {
      const contact = await a.invoke(new Api.contacts.AddContact({
        id: inputUser(B_ID, B_HASH), firstName: 'Production', lastName: 'Probe', phone: '', addPhonePrivacyException: false,
      }));
      if (!rpcName(contact).toLowerCase().includes('updates')) throw new Error(`contacts.addContact returned ${rpcName(contact)}`);
      contactAdded = true;
      const contactRead = await a.invoke(new Api.contacts.GetContacts({ hash: 0n }));
      if (!contactRead?.contacts?.some((item: any) => String(item?.userId) === String(B_ID))) throw new Error('contact did not read back');
      const deleted = await a.invoke(new Api.contacts.DeleteContacts({ id: [inputUser(B_ID, B_HASH)] }));
      if (!rpcName(deleted).toLowerCase().includes('updates')) throw new Error(`contacts.deleteContacts returned ${rpcName(deleted)}`);
      contactAdded = false;
      results.contact = { add: rpcName(contact), delete: rpcName(deleted) };
    } else {
      results.contact = { skipped: 'pre-existing contact' };
    }

    const birthdays = await a.invoke(new GetBirthdaysRequest());
    results.birthdays = {
      result: rpcName(birthdays),
      contactCount: Array.isArray(birthdays?.contacts) ? birthdays.contacts.length : 0,
      userCount: Array.isArray(birthdays?.users) ? birthdays.users.length : 0,
    };

    registeredDeviceToken = `prod-notification-${Date.now()}-${process.pid}`;
    const registeredDevice = await a.invoke(new Api.account.RegisterDevice({
      tokenType: 10,
      token: registeredDeviceToken,
      appSandbox: true,
      secret: new Uint8Array(),
      otherUids: [],
    }));
    if (!isTrue(registeredDevice)) throw new Error(`account.registerDevice returned ${rpcName(registeredDevice)}`);
    const unregisteredDevice = await a.invoke(new Api.account.UnregisterDevice({
      tokenType: 10,
      token: registeredDeviceToken,
      otherUids: [],
    }));
    if (!isTrue(unregisteredDevice)) throw new Error(`account.unregisterDevice returned ${rpcName(unregisteredDevice)}`);
    registeredDeviceToken = '';
    results.notificationDevice = { register: rpcName(registeredDevice), unregister: rpcName(unregisteredDevice) };

    const outboxTag = `prod-outbox-read-${Date.now()}-${process.pid}`;
    const outboxSent = await a.invoke(new Api.messages.SendMessage({
      peer,
      message: outboxTag,
      randomId: BigInt(Date.now()) * 1000n + 17n,
    }));
    const outboxUpdate = Array.isArray(outboxSent?.updates)
      ? outboxSent.updates.find((item: any) => item?.message?.id !== undefined)
      : undefined;
    if (outboxUpdate?.message?.id === undefined) {
      throw new Error(`outbox probe send returned ${rpcName(outboxSent)} without message id`);
    }
    outboxMessageId = Number(outboxUpdate.message.id);
    const outboxPeerMessage = await waitForMessage(b, inputPeer(A_ID, A_HASH), outboxTag);
    const outboxReadByPeer = await b.invoke(new Api.messages.GetHistory({
      peer: inputPeer(A_ID, A_HASH), offsetId: 0, offsetDate: 0,
      addOffset: 0, limit: 20, maxId: 0, minId: 0, hash: 0n,
    }));
    const outboxRead = await b.invoke(new Api.messages.ReadHistory({
      peer: inputPeer(A_ID, A_HASH), maxId: Number(outboxPeerMessage.id),
    }));
    const outboxDate = await a.invoke(new Api.messages.GetOutboxReadDate({
      peer,
      msgId: outboxMessageId,
    }));
    if (rpcName(outboxDate).toLowerCase() !== 'outboxreaddate' || !Number(outboxDate?.date)) {
      throw new Error(`messages.getOutboxReadDate returned ${rpcName(outboxDate)}`);
    }
    results.outboxReadDate = {
      send: rpcName(outboxSent),
      recipientHistory: rpcName(outboxReadByPeer),
      recipientRead: rpcName(outboxRead),
      result: rpcName(outboxDate),
      messageId: outboxMessageId,
    };

    const pinMessage = await a.invoke(new Api.messages.UpdatePinnedMessage({
      peer,
      id: outboxMessageId,
      pmOneside: true,
      silent: true,
    }));
    if (!rpcName(pinMessage).toLowerCase().includes('updates')) {
      throw new Error(`messages.updatePinnedMessage returned ${rpcName(pinMessage)}`);
    }
    messagePinned = true;
    const pinnedHistory = await a.invoke(new Api.messages.GetHistory({
      peer, offsetId: 0, offsetDate: 0, addOffset: 0, limit: 20, maxId: 0, minId: 0, hash: 0n,
    }));
    const pinnedProbeMessage = Array.isArray(pinnedHistory?.messages)
      ? pinnedHistory.messages.find((item: any) => Number(item?.id) === outboxMessageId)
      : undefined;
    if (pinnedProbeMessage?.pinned !== true) throw new Error('pinned probe message did not read back');
    const unpinAll = await a.invoke(new Api.messages.UnpinAllMessages({ peer }));
    if (!rpcName(unpinAll).toLowerCase().includes('affectedhistory')) {
      throw new Error(`messages.unpinAllMessages returned ${rpcName(unpinAll)}`);
    }
    const unpinnedHistory = await a.invoke(new Api.messages.GetHistory({
      peer, offsetId: 0, offsetDate: 0, addOffset: 0, limit: 20, maxId: 0, minId: 0, hash: 0n,
    }));
    const unpinnedProbeMessage = Array.isArray(unpinnedHistory?.messages)
      ? unpinnedHistory.messages.find((item: any) => Number(item?.id) === outboxMessageId)
      : undefined;
    if (unpinnedProbeMessage?.pinned === true) throw new Error('messages.unpinAllMessages did not clear pin');
    messagePinned = false;
    results.unpinAllMessages = {
      pin: rpcName(pinMessage),
      pinnedRead: rpcName(pinnedHistory),
      result: rpcName(unpinAll),
      unpinnedRead: rpcName(unpinnedHistory),
    };

    const draftBefore = await a.invoke(new Api.messages.GetPeerDialogs({ peers: [dialogPeer(peer)] }));
    const tag = `prod-reversible-${Date.now()}-${process.pid}`;
    const draftText = `${tag} draft`;
    const saved = await a.invoke(new Api.messages.SaveDraft({ peer, message: draftText }));
    const draftRead = await a.invoke(new Api.messages.GetPeerDialogs({ peers: [dialogPeer(peer)] }));
    if (draftOf(draftRead)?.message !== draftText) throw new Error('draft did not read back');
    const cleared = await a.invoke(new Api.messages.SaveDraft({ peer, message: '' }));
    const draftFinal = await a.invoke(new Api.messages.GetPeerDialogs({ peers: [dialogPeer(peer)] }));
    if (draftOf(draftFinal)?.message) throw new Error('draft clear did not read back');
    if (draftOf(draftBefore)?.message) {
      const restore = await a.invoke(new Api.messages.SaveDraft({ peer, message: draftOf(draftBefore).message, noWebpage: draftOf(draftBefore).noWebpage === true }));
      if (!isTrue(restore)) throw new Error('pre-existing draft restore failed');
    }
    results.draft = { before: rpcName(draftBefore), save: rpcName(saved), read: rpcName(draftRead), clear: rpcName(cleared), final: rpcName(draftFinal) };

    const pinnedBefore = await a.invoke(new Api.messages.GetPinnedDialogs({ folderId: 0 }));
    const alreadyPinned = Array.isArray(pinnedBefore?.dialogs) && pinnedBefore.dialogs.some((item: any) => String(item?.peer?.userId) === String(B_ID));
    if (!alreadyPinned) {
      const pin = await a.invoke(new Api.messages.ToggleDialogPin({ pinned: true, peer: dialogPeer(peer) }));
      if (!isTrue(pin)) throw new Error(`toggleDialogPin returned ${rpcName(pin)}`);
      pinChanged = true;
    }
    const pinnedRead = await a.invoke(new Api.messages.GetPinnedDialogs({ folderId: 0 }));
    if (!pinnedRead?.dialogs?.some((item: any) => String(item?.peer?.userId) === String(B_ID))) throw new Error('pinned dialog did not read back');
    if (pinChanged) {
      const unpin = await a.invoke(new Api.messages.ToggleDialogPin({ pinned: false, peer: dialogPeer(peer) }));
      if (!isTrue(unpin)) throw new Error(`unpin returned ${rpcName(unpin)}`);
    }
    results.pin = { before: rpcName(pinnedBefore), read: rpcName(pinnedRead), changed: pinChanged };
    console.log(JSON.stringify(results));
  } finally {
    if (contactAdded) { try { await a.invoke(new Api.contacts.DeleteContacts({ id: [inputUser(B_ID, B_HASH)] })); } catch {} }
    if (registeredDeviceToken) {
      try { await a.invoke(new Api.account.UnregisterDevice({ tokenType: 10, token: registeredDeviceToken, otherUids: [] })); } catch {}
    }
    if (pinChanged) { try { await a.invoke(new Api.messages.ToggleDialogPin({ pinned: false, peer: dialogPeer(peer) })); } catch {} }
    if (messagePinned && outboxMessageId > 0) {
      try { await a.invoke(new Api.messages.UpdatePinnedMessage({ peer, id: outboxMessageId, unpin: true, pmOneside: true, silent: true })); } catch {}
    }
    if (outboxMessageId > 0) { try { await a.invoke(new Api.messages.DeleteMessages({ revoke: true, id: [outboxMessageId] })); } catch {} }
    if (ttlBefore !== undefined) { try { await a.invoke(new Api.account.SetAccountTTL({ ttl: new Api.AccountDaysTTL({ days: ttlBefore }) })); } catch {} }
    a.destroy();
    b.destroy();
  }
}

main()
  .then(() => process.exit(0))
  .catch((error) => {
    console.error(`production reversible probe failed: ${error instanceof Error ? error.stack || error.message : String(error)}`);
    process.exit(1);
  });
