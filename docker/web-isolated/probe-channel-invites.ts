import path from 'node:path';
import { createRequire } from 'node:module';
import process from 'node:process';

const API_ID = 1;
const API_HASH = '0123456789abcdef0123456789abcdef';
const FIXTURES = [
  { phone: '12025550101', username: 'isolated_test_01' },
  { phone: '12025550102', username: 'isolated_test_02' },
  { phone: '12025550103', username: 'isolated_test_03' },
  { phone: '12025550104', username: 'isolated_test_04' },
];

type Actor = {
  client: any;
  id: bigint | number;
  username: string;
};

const quietLogger = { debug() {}, info() {}, warn() {}, error() {} };

function requireCondition(condition: unknown, message: string): asserts condition {
  if (!condition) throw new Error(message);
}

function rpcName(value: any) {
  return value?.className || value?.constructor?.name || typeof value;
}

function chatList(value: any): any[] {
  return [
    ...(Array.isArray(value?.chats) ? value.chats : []),
    ...(Array.isArray(value?.updates?.chats) ? value.updates.chats : []),
  ];
}

function inputChannel(Api: any, channel: any) {
  requireCondition(channel?.id !== undefined && channel?.accessHash !== undefined, 'channel has no usable access hash');
  return new Api.InputChannel({ channelId: channel.id, accessHash: channel.accessHash });
}

function inputPeerChannel(Api: any, channel: any) {
  requireCondition(channel?.id !== undefined && channel?.accessHash !== undefined, 'channel has no usable peer access hash');
  return new Api.InputPeerChannel({ channelId: channel.id, accessHash: channel.accessHash });
}

function inputUser(Api: any, user: any) {
  requireCondition(user?.id !== undefined && user?.accessHash !== undefined, 'user has no usable access hash');
  return new Api.InputUser({ userId: user.id, accessHash: user.accessHash });
}

function wrongAccessHash(accessHash: bigint | number) {
  const value = BigInt(accessHash);
  return value === 0n ? 1n : value + 1n;
}

function inviteHash(link: string) {
  const marker = '/+';
  const index = link.lastIndexOf(marker);
  requireCondition(index >= 0, `invite link has no hash: ${link}`);
  const hash = link.slice(index + marker.length);
  requireCondition(hash.length === 20, `invite hash has length ${hash.length}, want 20`);
  return hash;
}

function exportedInvite(value: any) {
  const invite = value?.invite ?? value?.newInvite ?? value;
  requireCondition(typeof invite?.link === 'string', `response ${rpcName(value)} has no exported invite`);
  return invite;
}

function containsInvite(value: any, link: string) {
  return Array.isArray(value?.invites) && value.invites.some((invite: any) => {
    const candidate = invite?.link || invite?.invite?.link || invite?.newInvite?.link;
    return candidate === link;
  });
}

function containsImporter(value: any, userID: bigint | number) {
  return Array.isArray(value?.importers)
    && value.importers.some((importer: any) => String(importer?.userId) === String(userID));
}

function containsAdmin(value: any, userID: bigint | number) {
  return Array.isArray(value?.admins)
    && value.admins.some((admin: any) => String(admin?.adminId) === String(userID));
}

function isUpdates(value: any) {
  return rpcName(value).toLowerCase().includes('updates');
}

async function expectRpcError(name: string, code: string, action: () => Promise<unknown>) {
  try {
    await action();
  } catch (error) {
    requireCondition(String(error).includes(code), `${name} returned ${String(error)}, want ${code}`);
    console.log(`PASS ${name}: ${code}`);
    return;
  }
  throw new Error(`${name} unexpectedly succeeded`);
}

async function invokeWithTimeout(actor: Actor, label: string, request: any, timeoutMs = 15_000) {
  const started = Date.now();
  let timer: ReturnType<typeof setTimeout> | undefined;
  console.log(`BEGIN ${label}`);
  try {
    const timeout = new Promise<never>((_, reject) => {
      timer = setTimeout(() => reject(new Error(`${label} timed out after ${timeoutMs}ms`)), timeoutMs);
    });
    const result = await Promise.race([actor.client.invoke(request), timeout]);
    console.log(`END ${label} (${Date.now() - started}ms): ${rpcName(result)}`);
    return result;
  } catch (error) {
    console.error(`FAIL ${label} (${Date.now() - started}ms): ${String(error)}`);
    throw error;
  } finally {
    if (timer) clearTimeout(timer);
  }
}

async function loadGramJs(root: string) {
  const requireModule = createRequire(import.meta.url);
  const clientModule = requireModule(path.join(root, 'src/lib/gramjs/client/TelegramClient.ts'));
  const sessionModule = requireModule(path.join(root, 'src/lib/gramjs/sessions/Memory.ts'));
  const tlModule = requireModule(path.join(root, 'src/lib/gramjs/tl/index.ts'));
  const connectionModule = requireModule(path.join(root, 'src/lib/gramjs/network/connection/TCPObfuscated.ts'));
  return {
    TelegramClient: clientModule.default,
    MemorySession: sessionModule.default,
    Api: tlModule.Api,
    Connection: connectionModule.ConnectionTCPObfuscated,
  };
}

async function login(gram: any, fixture: typeof FIXTURES[number]): Promise<Actor> {
  const client = new gram.TelegramClient(new gram.MemorySession(), API_ID, API_HASH, {
    connection: gram.Connection,
    fallbackConnection: gram.Connection,
    shouldAllowHttpTransport: false,
    shouldForceHttpTransport: false,
    timeout: 15,
    requestRetries: 2,
    connectionRetries: 2,
    connectionRetriesToFallback: 0,
    retryDelay: 250,
    autoReconnect: false,
    baseLogger: quietLogger,
  });
  try {
    await client.start({
      phoneNumber: fixture.phone,
      phoneCode: async () => '12345',
      password: async () => '',
      firstAndLastNames: async () => ['Isolated', 'Invite'],
      qrCode: async () => {},
      onError: (error: Error) => { throw error; },
      webAuthTokenFailed: () => {},
      onPasskeyOption: () => {},
      initialMethod: 'phoneNumber',
    });
    const me = await client.getMe();
    requireCondition(me?.id !== undefined, `login failed for ${fixture.username}`);
    return { client, id: me.id, username: fixture.username };
  } catch (error) {
    client.destroy();
    throw error;
  }
}

async function searchUser(Api: any, actor: Actor, username: string) {
  const result = await actor.client.invoke(new Api.contacts.Search({ q: username, limit: 10 }));
  const user = result?.users?.find((candidate: any) => candidate?.username === username);
  requireCondition(user, `contacts.search did not find ${username}`);
  return user;
}

async function getImporters(Api: any, actor: Actor, peer: any, options: { link?: string; requested?: boolean }) {
  const request: any = {
    peer,
    offsetDate: 0,
    offsetUser: new Api.InputUserEmpty(),
    limit: 50,
  };
  if (options.link !== undefined) request.link = options.link;
  if (options.requested) request.requested = true;
  return actor.client.invoke(new Api.messages.GetChatInviteImporters(request));
}

async function run() {
  requireCondition(process.env.TG_DC_HOST === '127.0.0.1', 'TG_DC_HOST must target the isolated gateway');
  requireCondition(process.env.TG_DC_PORT === '31443', 'TG_DC_PORT must target the isolated gateway');
  requireCondition(process.env.TEAMGRAM_GRAMJS_DIR, 'TEAMGRAM_GRAMJS_DIR is required');

  const globalAny = globalThis as typeof globalThis & { self?: any; addEventListener?: () => void };
  globalAny.self ??= globalThis;
  globalAny.addEventListener ??= () => {};
  globalAny.self.addEventListener ??= globalAny.addEventListener;

  const gram = await loadGramJs(process.env.TEAMGRAM_GRAMJS_DIR);
  const { Api } = gram;
  let actors: Actor[] = [];
  let createdChannel: any;
  const createdInviteLinks = new Set<string>();

  const rememberInvite = (value: any) => {
    const invite = exportedInvite(value);
    createdInviteLinks.add(invite.link);
    return invite;
  };

  try {
    actors = await Promise.all(FIXTURES.map((fixture) => login(gram, fixture)));
    const [owner, member, requester, requesterTwo] = actors;
    requireCondition(owner && member && requester && requesterTwo, 'fixture login returned an incomplete actor set');

    const tag = `channel_invites_${Date.now()}_${process.pid}`;
    const created = await owner.client.invoke(new Api.channels.CreateChannel({
      megagroup: true,
      title: `${tag} channel`,
      about: 'isolated APIFull channel invite probe',
    }));
    createdChannel = chatList(created).find((channel) => channel?.title === `${tag} channel`);
    requireCondition(createdChannel, 'channels.createChannel did not return the new channel');
    const ownerChannel = inputChannel(Api, createdChannel);
    const ownerPeer = inputPeerChannel(Api, createdChannel);
    console.log(`PASS channel creation: ${rpcName(created)}`);

    const memberUser = await searchUser(Api, owner, member.username);
    const requesterUser = await searchUser(Api, owner, requester.username);

    const normalExport = await owner.client.invoke(new Api.messages.ExportChatInvite({
      peer: ownerPeer,
      usageLimit: 3,
      title: `${tag} normal`,
    }));
    const normalInvite = rememberInvite(normalExport);
    const normalHash = inviteHash(normalInvite.link);
    requireCondition(normalInvite.requestNeeded !== true, 'normal export unexpectedly requires approval');
    console.log(`PASS admin invite export: ${normalInvite.link}`);

    const listed = await owner.client.invoke(new Api.messages.GetExportedChatInvites({
      peer: ownerPeer,
      adminId: new Api.InputUserSelf(),
      limit: 50,
    }));
    requireCondition(containsInvite(listed, normalInvite.link), 'active invite list omitted normal invite');
    console.log(`PASS exported invite list: ${rpcName(listed)}`);

    const fetched = await owner.client.invoke(new Api.messages.GetExportedChatInvite({
      peer: ownerPeer,
      link: normalInvite.link,
    }));
    requireCondition(exportedInvite(fetched).link === normalInvite.link, 'getExportedChatInvite returned the wrong link');
    console.log(`PASS exported invite get: ${rpcName(fetched)}`);

    const edited = await owner.client.invoke(new Api.messages.EditExportedChatInvite({
      peer: ownerPeer,
      link: normalInvite.link,
      usageLimit: 5,
      title: `${tag} normal edited`,
      requestNeeded: false,
    }));
    const editedInvite = rememberInvite(edited);
    requireCondition(editedInvite.title === `${tag} normal edited`, 'editExportedChatInvite did not update the title');
    requireCondition(editedInvite.usageLimit === 5, 'editExportedChatInvite did not update the usage limit');
    console.log(`PASS exported invite edit: ${rpcName(edited)}`);

    const checkedNormal = await member.client.invoke(new Api.messages.CheckChatInvite({ hash: normalHash }));
    requireCondition(checkedNormal?.requestNeeded !== true, 'normal invite check unexpectedly requires approval');
    const normalJoin = await member.client.invoke(new Api.messages.ImportChatInvite({ hash: normalHash }));
    requireCondition(rpcName(normalJoin).toLowerCase().includes('chatinvitejoinresult'), 'normal invite import did not return join result');
    console.log(`PASS normal invite check and import: ${rpcName(normalJoin)}`);

    const memberPeer = inputPeerChannel(Api, createdChannel);
    const normalImporters = await getImporters(Api, owner, ownerPeer, { link: normalInvite.link });
    requireCondition(containsImporter(normalImporters, member.id), 'normal importer list omitted the joined member');
    console.log(`PASS normal importer listing: ${rpcName(normalImporters)}`);

    const adminsBeforePromotion = await owner.client.invoke(new Api.messages.GetAdminsWithInvites({ peer: ownerPeer }));
    requireCondition(containsAdmin(adminsBeforePromotion, owner.id), 'admin listing omitted the channel creator');
    console.log(`PASS admin aggregation: ${rpcName(adminsBeforePromotion)}`);

    await expectRpcError('non-admin export rejection', 'CHAT_ADMIN_REQUIRED', () => member.client.invoke(
      new Api.messages.ExportChatInvite({ peer: memberPeer, title: `${tag} member denied` }),
    ));
    await expectRpcError('non-admin list rejection', 'CHAT_ADMIN_REQUIRED', () => member.client.invoke(
      new Api.messages.GetExportedChatInvites({ peer: memberPeer, adminId: new Api.InputUserSelf(), limit: 50 }),
    ));
    await expectRpcError('non-admin get rejection', 'CHAT_ADMIN_REQUIRED', () => member.client.invoke(
      new Api.messages.GetExportedChatInvite({ peer: memberPeer, link: normalInvite.link }),
    ));
    await expectRpcError('non-admin importer rejection', 'CHAT_ADMIN_REQUIRED', () => getImporters(
      Api, member, memberPeer, { link: normalInvite.link },
    ));

    const requestExport = await owner.client.invoke(new Api.messages.ExportChatInvite({
      peer: ownerPeer,
      requestNeeded: true,
      title: `${tag} request`,
    }));
    const requestInvite = rememberInvite(requestExport);
    const requestHash = inviteHash(requestInvite.link);
    const checkedRequest = await requester.client.invoke(new Api.messages.CheckChatInvite({ hash: requestHash }));
    requireCondition(checkedRequest?.requestNeeded === true, 'request invite check did not preserve request_needed');
    await expectRpcError('request invite import acknowledgement', 'INVITE_REQUEST_SENT', () => requester.client.invoke(
      new Api.messages.ImportChatInvite({ hash: requestHash }),
    ));
    const pending = await getImporters(Api, owner, ownerPeer, { link: requestInvite.link, requested: true });
    requireCondition(containsImporter(pending, requester.id), 'request importer list omitted requester');
    console.log(`PASS request-needed check, import, and importer listing: ${rpcName(pending)}`);

    await expectRpcError('non-admin request rejection', 'CHAT_ADMIN_REQUIRED', () => member.client.invoke(
      new Api.messages.HideChatJoinRequest({
        peer: memberPeer,
        userId: inputUser(Api, requesterUser),
      }),
    ));
    const singleRejected = await owner.client.invoke(new Api.messages.HideChatJoinRequest({
      peer: ownerPeer,
      userId: inputUser(Api, requesterUser),
    }));
    requireCondition(isUpdates(singleRejected), 'single request rejection did not return updates');
    const afterSingleReject = await getImporters(Api, owner, ownerPeer, { link: requestInvite.link, requested: true });
    requireCondition(!containsImporter(afterSingleReject, requester.id), 'single request rejection left a pending importer');
    console.log(`PASS single request rejection: ${rpcName(singleRejected)}`);

    await expectRpcError('request retry acknowledgement', 'INVITE_REQUEST_SENT', () => requester.client.invoke(
      new Api.messages.ImportChatInvite({ hash: requestHash }),
    ));
    const bulkRejected = await owner.client.invoke(new Api.messages.HideAllChatJoinRequests({
      peer: ownerPeer,
      link: requestInvite.link,
    }));
    requireCondition(isUpdates(bulkRejected), 'bulk request rejection did not return updates');
    const afterBulkReject = await getImporters(Api, owner, ownerPeer, { link: requestInvite.link, requested: true });
    requireCondition(!containsImporter(afterBulkReject, requester.id), 'bulk request rejection left a pending importer');
    console.log(`PASS bulk request rejection: ${rpcName(bulkRejected)}`);

    const bulkExport = await owner.client.invoke(new Api.messages.ExportChatInvite({
      peer: ownerPeer,
      requestNeeded: true,
      title: `${tag} bulk`,
    }));
    const bulkInvite = rememberInvite(bulkExport);
    const bulkHash = inviteHash(bulkInvite.link);
    await expectRpcError('bulk requester one acknowledgement', 'INVITE_REQUEST_SENT', () => requester.client.invoke(
      new Api.messages.ImportChatInvite({ hash: bulkHash }),
    ));
    await expectRpcError('bulk requester two acknowledgement', 'INVITE_REQUEST_SENT', () => requesterTwo.client.invoke(
      new Api.messages.ImportChatInvite({ hash: bulkHash }),
    ));
    const bulkPending = await getImporters(Api, owner, ownerPeer, { link: bulkInvite.link, requested: true });
    requireCondition(
      containsImporter(bulkPending, requester.id) && containsImporter(bulkPending, requesterTwo.id),
      'bulk importer listing omitted one of the pending requesters',
    );
    const bulkApproved = await owner.client.invoke(new Api.messages.HideAllChatJoinRequests({
      peer: ownerPeer,
      link: bulkInvite.link,
      approved: true,
    }));
    requireCondition(isUpdates(bulkApproved), 'bulk request approval did not return updates');
    const bulkRemaining = await getImporters(Api, owner, ownerPeer, { link: bulkInvite.link, requested: true });
    requireCondition(bulkRemaining?.importers?.length === 0, 'bulk approval left pending requesters');
    const approvedImporters = await getImporters(Api, owner, ownerPeer, { link: bulkInvite.link });
    requireCondition(
      containsImporter(approvedImporters, requester.id) && containsImporter(approvedImporters, requesterTwo.id),
      'approved importer listing omitted one of the approved members',
    );
    console.log(`PASS bulk request approval: ${rpcName(bulkApproved)}`);

    const already = await requester.client.invoke(new Api.messages.CheckChatInvite({ hash: bulkHash }));
    requireCondition(rpcName(already).toLowerCase().includes('already'), 'approved requester did not see ChatInviteAlready');
    console.log(`PASS approved invite check: ${rpcName(already)}`);

    const promoted = await owner.client.invoke(new Api.channels.EditAdmin({
      channel: ownerChannel,
      userId: inputUser(Api, memberUser),
      adminRights: new Api.ChatAdminRights({ inviteUsers: true }),
      rank: 'invite admin',
    }));
    requireCondition(isUpdates(promoted), 'channels.editAdmin did not return updates');
    const adminExport = await member.client.invoke(new Api.messages.ExportChatInvite({
      peer: memberPeer,
      title: `${tag} admin`,
    }));
    const adminInvite = rememberInvite(adminExport);
    requireCondition(String(adminInvite.adminId) === String(member.id), 'admin invite was attributed to the wrong admin');
    const memberListed = await member.client.invoke(new Api.messages.GetExportedChatInvites({
      peer: memberPeer,
      adminId: new Api.InputUserSelf(),
      limit: 50,
    }));
    requireCondition(containsInvite(memberListed, adminInvite.link), 'admin invite list omitted the admin invite');
    console.log(`PASS delegated admin invite export and list: ${rpcName(memberListed)}`);

    const revokedExport = await owner.client.invoke(new Api.messages.ExportChatInvite({
      peer: ownerPeer,
      title: `${tag} revoke`,
    }));
    const revokedInvite = rememberInvite(revokedExport);
    const revoked = await owner.client.invoke(new Api.messages.EditExportedChatInvite({
      peer: ownerPeer,
      link: revokedInvite.link,
      revoked: true,
    }));
    requireCondition(exportedInvite(revoked).revoked === true, 'invite revoke did not set revoked');
    const revokedList = await owner.client.invoke(new Api.messages.GetExportedChatInvites({
      peer: ownerPeer,
      adminId: new Api.InputUserSelf(),
      revoked: true,
      limit: 50,
    }));
    requireCondition(containsInvite(revokedList, revokedInvite.link), 'revoked invite list omitted revoked invite');
    await expectRpcError('non-admin revoke rejection', 'CHAT_ADMIN_REQUIRED', () => requester.client.invoke(
      new Api.messages.EditExportedChatInvite({ peer: ownerPeer, link: revokedInvite.link, revoked: true }),
    ));
    const deletedRevoked = await owner.client.invoke(new Api.messages.DeleteRevokedExportedChatInvites({
      peer: ownerPeer,
      adminId: new Api.InputUserSelf(),
    }));
    requireCondition(deletedRevoked === true || rpcName(deletedRevoked).toLowerCase().includes('true'), 'revoked invite cleanup did not return true');
    const revokedAfterDelete = await owner.client.invoke(new Api.messages.GetExportedChatInvites({
      peer: ownerPeer,
      adminId: new Api.InputUserSelf(),
      revoked: true,
      limit: 50,
    }));
    requireCondition(!containsInvite(revokedAfterDelete, revokedInvite.link), 'revoked invite cleanup left the invite');
    createdInviteLinks.delete(revokedInvite.link);
    console.log(`PASS revoke and revoked-invite cleanup: ${rpcName(deletedRevoked)}`);

    const deletableExport = await owner.client.invoke(new Api.messages.ExportChatInvite({
      peer: ownerPeer,
      title: `${tag} delete`,
    }));
    const deletableInvite = rememberInvite(deletableExport);
    const deleted = await owner.client.invoke(new Api.messages.DeleteExportedChatInvite({
      peer: ownerPeer,
      link: deletableInvite.link,
    }));
    requireCondition(deleted === true || rpcName(deleted).toLowerCase().includes('true'), 'active invite deletion did not return true');
    await expectRpcError('deleted invite lookup', 'INVITE_HASH_INVALID', () => owner.client.invoke(
      new Api.messages.GetExportedChatInvite({ peer: ownerPeer, link: deletableInvite.link }),
    ));
    createdInviteLinks.delete(deletableInvite.link);
    console.log(`PASS active invite deletion: ${rpcName(deleted)}`);

    const toggledOn = await invokeWithTimeout(owner, 'channels.toggleJoinRequest enabled', new Api.channels.ToggleJoinRequest({
      channel: ownerChannel,
      applyToInvites: true,
      enabled: true,
    }));
    requireCondition(isUpdates(toggledOn), 'channels.toggleJoinRequest did not return updates');
    const toggledOff = await invokeWithTimeout(owner, 'channels.toggleJoinRequest disabled', new Api.channels.ToggleJoinRequest({
      channel: ownerChannel,
      applyToInvites: true,
      enabled: false,
    }));
    requireCondition(isUpdates(toggledOff), 'channels.toggleJoinRequest disable did not return updates');
    await expectRpcError('non-admin join-request toggle rejection', 'CHAT_ADMIN_REQUIRED', () => invokeWithTimeout(
      requester,
      'channels.toggleJoinRequest non-admin',
      new Api.channels.ToggleJoinRequest({ channel: ownerChannel, enabled: true }),
    ));

    const joinToSendOn = await invokeWithTimeout(owner, 'channels.toggleJoinToSend enabled', new Api.channels.ToggleJoinToSend({
      channel: ownerChannel,
      enabled: true,
    }));
    requireCondition(isUpdates(joinToSendOn), 'channels.toggleJoinToSend did not return updates');
    const joinToSendOff = await invokeWithTimeout(owner, 'channels.toggleJoinToSend disabled', new Api.channels.ToggleJoinToSend({
      channel: ownerChannel,
      enabled: false,
    }));
    requireCondition(isUpdates(joinToSendOff), 'channels.toggleJoinToSend disable did not return updates');
    await expectRpcError('non-admin join-to-send rejection', 'CHAT_ADMIN_REQUIRED', () => invokeWithTimeout(
      requester,
      'channels.toggleJoinToSend non-admin',
      new Api.channels.ToggleJoinToSend({ channel: ownerChannel, enabled: true }),
    ));
    console.log('PASS channel join-request and join-to-send toggles');

    const badPeer = new Api.InputPeerChannel({
      channelId: createdChannel.id,
      accessHash: wrongAccessHash(createdChannel.accessHash),
    });
    const badChannel = new Api.InputChannel({
      channelId: createdChannel.id,
      accessHash: wrongAccessHash(createdChannel.accessHash),
    });
    await expectRpcError('wrong access-hash invite list rejection', 'CHANNEL_INVALID', () => owner.client.invoke(
      new Api.messages.GetExportedChatInvites({ peer: badPeer, adminId: new Api.InputUserSelf(), limit: 50 }),
    ));
    await expectRpcError('wrong access-hash toggle rejection', 'CHANNEL_INVALID', () => invokeWithTimeout(
      owner,
      'channels.toggleJoinRequest wrong access hash',
      new Api.channels.ToggleJoinRequest({ channel: badChannel, enabled: true }),
    ));
    console.log('PASS wrong channel access-hash authorization');
  } finally {
    const owner = actors[0];
    if (createdChannel && owner) {
      let invitesForCleanup = [...createdInviteLinks];
      try {
        const active = await owner.client.invoke(new Api.messages.GetExportedChatInvites({
          peer: inputPeerChannel(Api, createdChannel),
          adminId: new Api.InputUserSelf(),
          limit: 100,
        }));
        invitesForCleanup = [...new Set([
          ...invitesForCleanup,
          ...(Array.isArray(active?.invites) ? active.invites.map((invite: any) => invite?.link) : []),
        ].filter((link): link is string => typeof link === 'string' && link !== ''))];
      } catch (error) {
        console.error(`invite cleanup listing failed: ${String(error)}`);
      }
      for (const link of invitesForCleanup) {
        try {
          await invokeWithTimeout(
            owner,
            `cleanup messages.deleteExportedChatInvite ${link}`,
            new Api.messages.DeleteExportedChatInvite({
              peer: inputPeerChannel(Api, createdChannel),
              link,
            }),
            5_000,
          );
        } catch (error) {
          console.error(`invite cleanup failed for ${link}: ${String(error)}`);
        }
      }
      try {
        await invokeWithTimeout(
          owner,
          'cleanup channels.deleteChannel',
          new Api.channels.DeleteChannel({ channel: inputChannel(Api, createdChannel) }),
          5_000,
        );
      } catch (error) {
        console.error(`channel invite probe cleanup failed: ${String(error)}`);
      }
    }
    for (const actor of actors) actor.client.destroy();
  }
}

void run()
  .then(() => process.exit(0))
  .catch((error) => {
    console.error(`channel invite probe fatal: ${error instanceof Error && error.stack ? error.stack : String(error)}`);
    process.exit(1);
  });
