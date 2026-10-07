import path from 'node:path';
import { createRequire } from 'node:module';

type Client = any;

const required = [
  'TEAMGRAM_GRAMJS_DIR', 'A_AUTH_B64', 'B_AUTH_B64', 'A_USER_ID', 'B_USER_ID',
  'A_ACCESS_HASH', 'B_ACCESS_HASH',
];
for (const key of required) {
  if (!process.env[key]) throw new Error(`${key} is required`);
}

const gramjsDir = process.env.TEAMGRAM_GRAMJS_DIR!;
const dcId = Number(process.env.TG_DC_ID || '2');
const apiId = Number(process.env.TG_API_ID || '1');
const apiHash = process.env.TG_API_HASH || '0123456789abcdef0123456789abcdef';
const A_ID = BigInt(process.env.A_USER_ID!);
const B_ID = BigInt(process.env.B_USER_ID!);
const A_HASH = BigInt(process.env.A_ACCESS_HASH!);
const B_HASH = BigInt(process.env.B_ACCESS_HASH!);

const globalAny = globalThis as typeof globalThis & { self?: any; addEventListener?: () => void };
globalAny.self ??= globalThis;
globalAny.addEventListener ??= () => {};
globalAny.self.addEventListener ??= globalAny.addEventListener;

const requireModule = createRequire(import.meta.url);
const TelegramClient = requireModule(path.join(gramjsDir, 'src/lib/gramjs/client/TelegramClient.ts')).default;
const CallbackSession = requireModule(path.join(gramjsDir, 'src/lib/gramjs/sessions/CallbackSession.ts')).default;
const Api = requireModule(path.join(gramjsDir, 'src/lib/gramjs/tl/index.ts')).Api;
const ConnectionTCPObfuscated = requireModule(path.join(gramjsDir, 'src/lib/gramjs/network/connection/TCPObfuscated.ts')).ConnectionTCPObfuscated;

const quietLogger = { debug() {}, info() {}, warn() {}, error() {} };

function rpcName(value: any): string {
  return value?.className || value?.constructor?.name || typeof value;
}

function messages(value: any): any[] {
  return [
    ...(Array.isArray(value?.messages) ? value.messages : []),
    ...(Array.isArray(value?.newMessages) ? value.newMessages : []),
  ];
}

function findMessage(value: any, text: string): any {
  return messages(value).find((item) => item?.message === text);
}

function inputUser(id: bigint, accessHash: bigint): any {
  return new Api.InputUser({ userId: id, accessHash });
}

function inputPeerUser(id: bigint, accessHash: bigint): any {
  return new Api.InputPeerUser({ userId: id, accessHash });
}

function inputPeerChannel(channelId: bigint, channelHash: bigint): any {
  return new Api.InputPeerChannel({ channelId, accessHash: channelHash });
}

function inputPeerChat(chatId: bigint | number): any {
  return new Api.InputPeerChat({ chatId });
}

function inputChannel(channelId: bigint, channelHash: bigint): any {
  return new Api.InputChannel({ channelId, accessHash: channelHash });
}

// The checked-in GramJS runtime exposes the Layer 229 type declaration but
// omits this generated constructor. Keep the raw request local to the probe so
// the production audit can still exercise the server method without changing
// the client runtime.
class DeletePollAnswerRequest {
  CONSTRUCTOR_ID = 0xac8505a5;
  SUBCLASS_OF_ID = 0;
  className = 'messages.DeletePollAnswer';
  classType = 'request';

  constructor(
    private readonly peer: any,
    private readonly msgId: number,
    private readonly option: Uint8Array,
  ) {}

  getBytes() {
    const peerBytes = this.peer.getBytes();
    const optionLength = this.option.length;
    if (optionLength >= 254) throw new Error('poll option is unexpectedly large');
    const paddedLength = (1 + optionLength + 3) & ~3;
    const bytes = new Uint8Array(4 + peerBytes.length + 4 + paddedLength);
    const view = new DataView(bytes.buffer);
    view.setUint32(0, this.CONSTRUCTOR_ID, true);
    bytes.set(peerBytes, 4);
    view.setInt32(4 + peerBytes.length, this.msgId, true);
    bytes[8 + peerBytes.length] = optionLength;
    bytes.set(this.option, 9 + peerBytes.length);
    return bytes;
  }

  readResult(reader: any) {
    return reader.tgReadObject();
  }
}

function history(peer: any): any {
  return new Api.messages.GetHistory({
    peer, offsetId: 0, offsetDate: 0, addOffset: 0, limit: 50,
    maxId: 0, minId: 0, hash: 0n,
  });
}

function search(peer: any, q: string): any {
  return new Api.messages.Search({
    peer, q, filter: new Api.InputMessagesFilterEmpty(), minDate: 0, maxDate: 0,
    offsetId: 0, addOffset: 0, limit: 50, maxId: 0, minId: 0, hash: 0n,
  });
}

function scheduledHistory(peer: any): any {
  return new Api.messages.GetScheduledHistory({ peer, hash: 0n });
}

function scheduledID(value: any): number {
  const update = Array.isArray(value?.updates)
    ? value.updates.find((item: any) => item?.message?.id !== undefined)
    : undefined;
  if (update?.message?.id === undefined) throw new Error(`scheduled send returned ${rpcName(value)} without message id`);
  return Number(update.message.id);
}

function removeScheduledID(ids: number[], id: number) {
  const index = ids.indexOf(id);
  if (index >= 0) ids.splice(index, 1);
}

function makeClient(authKeyBase64: string): Client {
  const keyHex = Buffer.from(authKeyBase64, 'base64').toString('hex');
  const session = new CallbackSession({ mainDcId: dcId, keys: { [dcId]: keyHex } }, () => {});
  const load = session.load.bind(session);
  session.load = async () => {
    await load();
    (session as any)._serverAddress = process.env.TG_DC_HOST || '127.0.0.1';
    (session as any)._port = Number(process.env.TG_DC_PORT || '11443');
  };
  const client = new TelegramClient(session, apiId, apiHash, {
    connection: ConnectionTCPObfuscated,
    fallbackConnection: ConnectionTCPObfuscated,
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
  // The acceptance client only uses its authenticated primary DC for these RPCs.
  // GramJS may open an additional media sender without a matching exported
  // media auth key; suppress that unrelated background connection.
  (client as any)._borrowExportedSender = async () => undefined;
  (client as any)._updateLoop = async () => undefined;
  return client;
}

async function waitForMessage(client: Client, request: () => any, text: string): Promise<any> {
  let value: any;
  for (let attempt = 0; attempt < 12; attempt += 1) {
    value = await client.invoke(request());
    const message = findMessage(value, text);
    if (message) return message;
    await new Promise((resolve) => setTimeout(resolve, 250));
  }
  throw new Error(`message not visible after retries: ${text}`);
}

async function main() {
  const a = makeClient(process.env.A_AUTH_B64!);
  const b = makeClient(process.env.B_AUTH_B64!);
  const tag = `prod-chain-${Date.now()}-${process.pid}`;
  const directText = `${tag} direct`;
  const editedText = `${tag} edited`;
  const channelText = `${tag} channel`;
  const channelEditedText = `${tag} channel edited`;
  const results: Record<string, unknown> = { tag };
  let aMessageId = 0;
  let bMessageId = 0;
  let channelMessageId = 0;
  let channelId = 0n;
  let channelHash = 0n;
  let groupId: bigint | number | undefined;
  let groupMessageId = 0;
  const scheduledIds: number[] = [];
  let deliveredScheduledMessageId = 0;
  let reactionWasSet = false;
  try {
    await a.connect();
    await b.connect();
    const meA = await a.getMe();
    const meB = await b.getMe();
    if (String(meA?.id) !== String(A_ID) || String(meB?.id) !== String(B_ID)) {
      throw new Error(`auth key user mismatch: ${String(meA?.id)} ${String(meB?.id)}`);
    }
    results.auth = { a: rpcName(meA), b: rpcName(meB) };

    const config = await a.invoke(new Api.help.GetConfig());
    const nearestDc = await a.invoke(new Api.help.GetNearestDc());
    const countries = await a.invoke(new Api.help.GetCountriesList({ langCode: 'en', hash: 0 }));
    const timezones = await a.invoke(new Api.help.GetTimezonesList({ hash: 0 }));
    const availableReactions = await a.invoke(new Api.messages.GetAvailableReactions({ hash: 0 }));
    const savedGifs = await a.invoke(new Api.messages.GetSavedGifs({ hash: 0n }));
    const dialogs = await a.invoke(new Api.messages.GetDialogs({
      offsetDate: 0, offsetId: 0, offsetPeer: new Api.InputPeerEmpty(), limit: 50, hash: 0n,
    }));
    results.readOnly = {
      config: rpcName(config), nearestDc: rpcName(nearestDc), countries: rpcName(countries),
      timezones: rpcName(timezones), availableReactions: rpcName(availableReactions),
      savedGifs: rpcName(savedGifs), dialogs: rpcName(dialogs),
      dialogCount: Array.isArray(dialogs?.dialogs) ? dialogs.dialogs.length : 0,
    };

    const peerA = inputPeerUser(B_ID, B_HASH);
    const peerB = inputPeerUser(A_ID, A_HASH);
    const sent = await a.invoke(new Api.messages.SendMessage({
      peer: peerA, message: directText, randomId: BigInt(Date.now()) * 1000n + 1n,
    }));
    if (!rpcName(sent).toLowerCase().includes('updates')) throw new Error(`direct send returned ${rpcName(sent)}`);
    const bMessage = await waitForMessage(b, () => history(peerB), directText);
    const aMessage = await waitForMessage(a, () => history(peerA), directText);
    bMessageId = Number(bMessage.id);
    aMessageId = Number(aMessage.id);
    const directSearch = await b.invoke(search(peerB, tag));
    if (!findMessage(directSearch, directText)) throw new Error('direct search did not return the message');
    const globalSearch = await b.invoke(new Api.messages.SearchGlobal({
      q: tag, filter: new Api.InputMessagesFilterEmpty(), minDate: 0, maxDate: 0,
      offsetRate: 0, offsetPeer: new Api.InputPeerEmpty(), offsetId: 0, limit: 50,
    }));
    if (!findMessage(globalSearch, directText)) throw new Error('global search did not return the message');
    const read = await b.invoke(new Api.messages.ReadHistory({ peer: peerB, maxId: bMessageId }));
    const contents = await b.invoke(new Api.messages.ReadMessageContents({ id: [bMessageId] }));
    if (!Number.isInteger(read?.pts) || !Number.isInteger(contents?.pts)) throw new Error('read result missing pts');
    const edited = await a.invoke(new Api.messages.EditMessage({ peer: peerA, id: aMessageId, message: editedText }));
    if (!rpcName(edited).toLowerCase().includes('updates')) throw new Error(`edit returned ${rpcName(edited)}`);
    await waitForMessage(b, () => history(peerB), editedText);
    results.direct = {
      send: rpcName(sent), history: rpcName(bMessage), search: rpcName(directSearch),
      globalSearch: rpcName(globalSearch), readHistory: rpcName(read), readContents: rpcName(contents),
      edit: rpcName(edited), senderMessageId: aMessageId, receiverMessageId: bMessageId,
    };

    const reaction = await b.invoke(new Api.messages.SendReaction({
      peer: peerB, msgId: bMessageId, reaction: [new Api.ReactionEmoji({ emoticon: '👍' })],
    }));
    if (!rpcName(reaction).toLowerCase().includes('updates')) throw new Error(`reaction returned ${rpcName(reaction)}`);
    reactionWasSet = true;
    const reactionList = await b.invoke(new Api.messages.GetMessageReactionsList({ peer: peerB, id: bMessageId, limit: 20 }));
    if (Number(reactionList?.count || 0) < 1) throw new Error('reaction list did not contain the reaction');
    const unreadReactions = await b.invoke(new Api.messages.GetUnreadReactions({
      peer: peerB, offsetId: 0, addOffset: 0, limit: 20, maxId: 0, minId: 0,
    }));
    const readReactions = await b.invoke(new Api.messages.ReadReactions({ peer: peerB }));
    const reactionClear = await b.invoke(new Api.messages.SendReaction({ peer: peerB, msgId: bMessageId, reaction: [] }));
    reactionWasSet = false;
    results.reaction = {
      set: rpcName(reaction), list: rpcName(reactionList), count: reactionList?.count,
      unread: rpcName(unreadReactions), read: rpcName(readReactions), clear: rpcName(reactionClear),
    };

    const groupTitle = `${tag} canonical group`;
    const createdGroup = await a.invoke(new Api.messages.CreateChat({
      users: [inputUser(B_ID, B_HASH)],
      title: groupTitle,
    }));
    let group = (Array.isArray(createdGroup?.chats) ? createdGroup.chats : [])
      .find((chat: any) => chat?.title === groupTitle);
    for (let attempt = 0; !group && attempt < 8; attempt += 1) {
      const dialogs = await a.invoke(new Api.messages.GetDialogs({
        offsetDate: 0, offsetId: 0, offsetPeer: new Api.InputPeerEmpty(), limit: 100, hash: 0n,
      }));
      group = (Array.isArray(dialogs?.chats) ? dialogs.chats : [])
        .find((chat: any) => chat?.title === groupTitle);
      if (!group) await new Promise((resolve) => setTimeout(resolve, 250));
    }
    if (!group || group.id === undefined) throw new Error(`group creation returned ${rpcName(createdGroup)}`);
    groupId = group.id;
    const groupPeer = inputPeerChat(groupId);
    const forumTopics = await b.invoke(new Api.messages.GetForumTopics({
      peer: groupPeer, offsetDate: 0, offsetId: 0, offsetTopic: 0, limit: 50,
    }));
    const forumTopicsByID = await b.invoke(new Api.messages.GetForumTopicsByID({
      peer: groupPeer, topics: [1],
    }));
    const groupText = `${tag} group`;
    const groupSend = await a.invoke(new Api.messages.SendMessage({
      peer: groupPeer, message: groupText, randomId: BigInt(Date.now()) * 1000n + 3n,
    }));
    if (!rpcName(groupSend).toLowerCase().includes('updates')) throw new Error(`group send returned ${rpcName(groupSend)}`);
    const groupHistory = await waitForMessage(b, () => history(groupPeer), groupText);
    groupMessageId = Number(groupHistory.id);
    const groupFull = await b.invoke(new Api.messages.GetFullChat({ chatId: groupId }));
    if (!Array.isArray(groupFull?.users) || !groupFull.users.some((user: any) => String(user?.id) === String(A_ID))) {
      throw new Error('group full chat did not hydrate the sender');
    }
    const groupEditedTitle = `${tag} group edited`;
    const groupEdit = await a.invoke(new Api.messages.EditChatTitle({ chatId: groupId, title: groupEditedTitle }));
    if (!rpcName(groupEdit).toLowerCase().includes('updates')) throw new Error(`group title edit returned ${rpcName(groupEdit)}`);
    const groupReadback = await a.invoke(new Api.messages.GetChats({ id: [groupId] }));
    const groupAfterEdit = (Array.isArray(groupReadback?.chats) ? groupReadback.chats : [])
      .find((chat: any) => String(chat?.id) === String(groupId));
    if (groupAfterEdit?.title !== groupEditedTitle) throw new Error('group title edit did not read back');

    const pollText = `${tag} poll`;
    const pollOptions = [Buffer.from([0]), Buffer.from([1])];
    const poll = new Api.Poll({
      id: 0n,
      publicVoters: true,
      question: new Api.TextWithEntities({ text: `${tag} question`, entities: [] }),
      answers: [
        new Api.PollAnswer({ text: new Api.TextWithEntities({ text: 'One', entities: [] }), option: pollOptions[0] }),
        new Api.PollAnswer({ text: new Api.TextWithEntities({ text: 'Two', entities: [] }), option: pollOptions[1] }),
      ],
      hash: 0n,
    });
    const pollSent = await a.invoke(new Api.messages.SendMedia({
      peer: groupPeer,
      media: new Api.InputMediaPoll({ poll }),
      message: pollText,
      randomId: BigInt(Date.now()) * 1000n + 7n,
    }));
    if (!rpcName(pollSent).toLowerCase().includes('updates')) throw new Error(`poll send returned ${rpcName(pollSent)}`);
    const ownerPoll = await waitForMessage(a, () => history(groupPeer), pollText);
    const memberPoll = await waitForMessage(b, () => history(groupPeer), pollText);
    const storedPoll = memberPoll?.media?.poll;
    if (storedPoll?.id === undefined || storedPoll?.hash === undefined || BigInt(storedPoll.id) === 0n || BigInt(storedPoll.hash) === 0n) {
      throw new Error('poll readback did not contain server id/hash');
    }
    const ownerVote = await a.invoke(new Api.messages.SendVote({ peer: groupPeer, msgId: Number(ownerPoll.id), options: [pollOptions[0]] }));
    const memberVote = await b.invoke(new Api.messages.SendVote({ peer: groupPeer, msgId: Number(memberPoll.id), options: [pollOptions[1]] }));
    const pollResults = await a.invoke(new Api.messages.GetPollResults({ peer: groupPeer, msgId: Number(ownerPoll.id), pollHash: storedPoll.hash }));
    const pollVotes = await a.invoke(new Api.messages.GetPollVotes({ peer: groupPeer, id: Number(ownerPoll.id), limit: 20 }));
    const pollAnswer = await a.invoke(new Api.messages.AddPollAnswer({
      peer: groupPeer, msgId: Number(ownerPoll.id),
      answer: new Api.PollAnswer({ text: new Api.TextWithEntities({ text: 'One', entities: [] }), option: pollOptions[0] }),
    }));
    const unreadPollVotes = await a.invoke(new Api.messages.GetUnreadPollVotes({
      peer: groupPeer, offsetId: 0, addOffset: 0, limit: 20, maxId: 0, minId: 0,
    }));
    const readPollVotes = await a.invoke(new Api.messages.ReadPollVotes({ peer: groupPeer }));
    const memberUnreadPollVotes = await b.invoke(new Api.messages.GetUnreadPollVotes({
      peer: groupPeer, offsetId: 0, addOffset: 0, limit: 20, maxId: 0, minId: 0,
    }));
    const memberHasUnreadPoll = messages(memberUnreadPollVotes).some((item: any) =>
      item?.media?.poll?.question?.text === `${tag} question`);
    if (!memberHasUnreadPoll) throw new Error('poll voter unread list did not include the voted poll');
    const memberReadPollVotes = await b.invoke(new Api.messages.ReadPollVotes({ peer: groupPeer }));
    const memberUnreadAfterRead = await b.invoke(new Api.messages.GetUnreadPollVotes({
      peer: groupPeer, offsetId: 0, addOffset: 0, limit: 20, maxId: 0, minId: 0,
    }));
    if (messages(memberUnreadAfterRead).some((item: any) => item?.media?.poll?.question?.text === `${tag} question`)) {
      throw new Error('read poll votes left the voted poll unread');
    }
    const deletePollAnswer = await a.invoke(new DeletePollAnswerRequest(
      groupPeer,
      Number(ownerPoll.id),
      pollOptions[0],
    ));
    if (!rpcName(ownerVote).toLowerCase().includes('updates') || !rpcName(memberVote).toLowerCase().includes('updates')) {
      throw new Error('poll vote did not return Updates');
    }
    if (!rpcName(pollResults).toLowerCase().includes('updates') || Number(pollVotes?.count || 0) < 2) {
      throw new Error(`poll results/votes incomplete: ${rpcName(pollResults)} ${rpcName(pollVotes)} ${pollVotes?.count}`);
    }
    results.poll = {
      send: rpcName(pollSent), ownerRead: rpcName(ownerPoll), memberRead: rpcName(memberPoll),
      ownerVote: rpcName(ownerVote), memberVote: rpcName(memberVote), results: rpcName(pollResults),
      votes: rpcName(pollVotes), voteCount: pollVotes?.count, addAnswer: rpcName(pollAnswer),
      unreadVotes: rpcName(unreadPollVotes), readVotes: rpcName(readPollVotes),
      memberUnreadVotes: rpcName(memberUnreadPollVotes), memberReadVotes: rpcName(memberReadPollVotes),
      deleteAnswer: rpcName(deletePollAnswer),
    };
    results.group = {
      create: rpcName(createdGroup), forumTopics: rpcName(forumTopics), forumTopicsByID: rpcName(forumTopicsByID),
      send: rpcName(groupSend), history: rpcName(groupHistory),
      full: rpcName(groupFull), editTitle: rpcName(groupEdit), readback: rpcName(groupReadback),
      groupId: String(groupId), messageId: groupMessageId,
    };

    const scheduledText = `${tag} scheduled`;
    const scheduled = await a.invoke(new Api.messages.SendMessage({
      peer: peerA, message: scheduledText, randomId: BigInt(Date.now()) * 1000n + 4n,
      scheduleDate: Math.floor(Date.now() / 1000) + 3600,
    }));
    const firstScheduledId = scheduledID(scheduled);
    scheduledIds.push(firstScheduledId);
    const pendingScheduled = await a.invoke(scheduledHistory(peerA));
    if (!findMessage(pendingScheduled, scheduledText)) throw new Error('scheduled history did not contain the message');
    const scheduledByID = await a.invoke(new Api.messages.GetScheduledMessages({ peer: peerA, id: [firstScheduledId] }));
    if (!findMessage(scheduledByID, scheduledText)) throw new Error('scheduled get-by-id did not contain the message');
    const cancelledText = `${tag} scheduled cancelled`;
    const cancelled = await a.invoke(new Api.messages.SendMessage({
      peer: peerA, message: cancelledText, randomId: BigInt(Date.now()) * 1000n + 5n,
      scheduleDate: Math.floor(Date.now() / 1000) + 3660,
    }));
    const cancelledID = scheduledID(cancelled);
    scheduledIds.push(cancelledID);
    const deletedScheduled = await a.invoke(new Api.messages.DeleteScheduledMessages({ peer: peerA, id: [cancelledID] }));
    removeScheduledID(scheduledIds, cancelledID);
    const afterCancel = await a.invoke(scheduledHistory(peerA));
    if (findMessage(afterCancel, cancelledText)) throw new Error('cancelled scheduled message remained visible');
    const deliverText = `${tag} scheduled delivered`;
    const ready = await a.invoke(new Api.messages.SendMessage({
      peer: peerA, message: deliverText, randomId: BigInt(Date.now()) * 1000n + 6n,
      scheduleDate: Math.floor(Date.now() / 1000) + 3720,
    }));
    const readyID = scheduledID(ready);
    scheduledIds.push(readyID);
    const delivered = await a.invoke(new Api.messages.SendScheduledMessages({ peer: peerA, id: [readyID] }));
    removeScheduledID(scheduledIds, readyID);
    const deliveredMessage = await waitForMessage(b, () => history(peerB), deliverText);
    deliveredScheduledMessageId = Number(deliveredMessage.id);
    if (!rpcName(delivered).toLowerCase().includes('updates')) throw new Error(`scheduled delivery returned ${rpcName(delivered)}`);
    results.scheduled = {
      send: rpcName(scheduled), history: rpcName(pendingScheduled), getByID: rpcName(scheduledByID),
      cancel: rpcName(deletedScheduled), deliver: rpcName(delivered), deliveredHistory: rpcName(deliveredMessage),
    };

    const channelTitle = `${tag} canonical channel`;
    const created = await b.invoke(new Api.channels.CreateChannel({
      broadcast: true,
      title: channelTitle,
      about: `${tag} production channel`,
    }));
    const createdChannel = (Array.isArray(created?.chats) ? created.chats : [])
      .find((chat: any) => chat?.title === channelTitle);
    if (!createdChannel || createdChannel.id === undefined || createdChannel.accessHash === undefined) {
      throw new Error(`canonical channel creation returned ${rpcName(created)}`);
    }
    channelId = BigInt(String(createdChannel.id));
    channelHash = BigInt(String(createdChannel.accessHash));
    const channelInput = inputChannel(channelId, channelHash);
    const invited = await b.invoke(new Api.channels.InviteToChannel({
      channel: channelInput,
      users: [inputUser(A_ID, A_HASH)],
    }));
    if (!rpcName(invited).toLowerCase().includes('invitedusers')) {
      throw new Error(`channel invite returned ${rpcName(invited)}`);
    }
    const channelPeer = inputPeerChannel(channelId, channelHash);
    const channelParticipant = await b.invoke(new Api.channels.GetParticipant({
      channel: channelInput,
      participant: inputPeerUser(A_ID, A_HASH),
    }));
    if (!rpcName(channelParticipant).toLowerCase().includes('channelparticipant')) {
      throw new Error(`channel getParticipant returned ${rpcName(channelParticipant)}`);
    }
    const channelList = await b.invoke(new Api.channels.GetChannels({ id: [channelInput] }));
    if (!rpcName(channelList).toLowerCase().includes('messages.chats') &&
      !rpcName(channelList).toLowerCase().includes('chats')) {
      throw new Error(`channel getChannels returned ${rpcName(channelList)}`);
    }
    const channelFull = await b.invoke(new Api.channels.GetFullChannel({ channel: channelInput }));
    if (!rpcName(channelFull).toLowerCase().includes('chatfull') || !channelFull?.fullChat) {
      throw new Error(`channel getFullChannel returned ${rpcName(channelFull)}`);
    }
    const channelMembers = await b.invoke(new Api.channels.GetParticipants({
      channel: channelInput,
      filter: new Api.ChannelParticipantsRecent(),
      offset: 0,
      limit: 100,
      hash: 0n,
    }));
    if (!rpcName(channelMembers).toLowerCase().includes('channelparticipants') ||
      !Array.isArray(channelMembers?.participants)) {
      throw new Error(`channel getParticipants returned ${rpcName(channelMembers)}`);
    }
    const channel = await b.invoke(new Api.messages.SendMessage({
      peer: channelPeer, message: channelText, randomId: BigInt(Date.now()) * 1000n + 2n,
    }));
    if (!rpcName(channel).toLowerCase().includes('updates')) throw new Error(`channel send returned ${rpcName(channel)}`);
    const channelHistory = await waitForMessage(a, () => history(channelPeer), channelText);
    channelMessageId = Number(channelHistory.id);
    const channelSearch = await a.invoke(search(channelPeer, tag));
    if (!findMessage(channelSearch, channelText)) throw new Error('channel search did not return the message');
    const channelById = await a.invoke(new Api.channels.GetMessages({ channel: channelInput, id: [new Api.InputMessageID({ id: channelMessageId })] }));
    if (!findMessage(channelById, channelText)) throw new Error('channel getMessages did not return the message');
    const channelRead = await a.invoke(new Api.channels.ReadHistory({ channel: channelInput, maxId: channelMessageId }));
    const channelContents = await a.invoke(new Api.channels.ReadMessageContents({ channel: channelInput, id: [channelMessageId] }));
    const channelViews = await a.invoke(new Api.messages.GetMessagesViews({ peer: channelPeer, id: [channelMessageId], increment: false }));
    const channelDifference = await a.invoke(new Api.updates.GetChannelDifference({
      force: true,
      channel: channelInput,
      filter: new Api.ChannelMessagesFilterEmpty(),
      pts: 0,
      limit: 100,
    }));
    if (!rpcName(channelDifference).toLowerCase().includes('channeldifference') ||
      !findMessage(channelDifference, channelText)) {
      throw new Error(`channel difference returned ${rpcName(channelDifference)} without the posted message`);
    }
    if (!(channelRead === true || rpcName(channelRead).toLowerCase().includes('true'))) throw new Error(`channel read returned ${rpcName(channelRead)}`);
    if (!(channelContents === true || rpcName(channelContents).toLowerCase().includes('true'))) throw new Error(`channel content read returned ${rpcName(channelContents)}`);
    const channelEdited = await b.invoke(new Api.messages.EditMessage({
      peer: channelPeer,
      id: channelMessageId,
      message: channelEditedText,
    }));
    if (!rpcName(channelEdited).toLowerCase().includes('updates')) throw new Error(`channel edit returned ${rpcName(channelEdited)}`);
    await waitForMessage(a, () => history(channelPeer), channelEditedText);
    const channelDeleted = await b.invoke(new Api.channels.DeleteMessages({
      channel: channelInput,
      id: [channelMessageId],
    }));
    const channelDeleteName = rpcName(channelDeleted).toLowerCase();
    if (!channelDeleteName.includes('updates') && !channelDeleteName.includes('affectedmessages')) {
      throw new Error(`channel delete returned ${rpcName(channelDeleted)}`);
    }
    let channelAfterDelete: any;
    try {
      channelAfterDelete = await a.invoke(new Api.channels.GetMessages({
        channel: channelInput,
        id: [new Api.InputMessageID({ id: channelMessageId })],
      }));
      if (findMessage(channelAfterDelete, channelEditedText)) throw new Error('deleted channel message remained visible');
    } catch (error) {
      if (!String(error).includes('MESSAGE_ID_INVALID')) throw error;
      channelAfterDelete = { className: 'MESSAGE_ID_INVALID' };
    }
    channelMessageId = 0;
    results.channel = {
      getParticipant: rpcName(channelParticipant), getChannels: rpcName(channelList),
      getFullChannel: rpcName(channelFull),
      getParticipants: rpcName(channelMembers),
      send: rpcName(channel), history: rpcName(channelHistory), search: rpcName(channelSearch),
      getMessages: rpcName(channelById), readHistory: rpcName(channelRead), readContents: rpcName(channelContents),
      views: rpcName(channelViews), difference: rpcName(channelDifference), edit: rpcName(channelEdited),
      delete: rpcName(channelDeleted), afterDelete: rpcName(channelAfterDelete), messageId: Number(channelHistory.id),
      channelId: String(channelId),
    };
  } finally {
    if (reactionWasSet && bMessageId > 0) {
      try { await b.invoke(new Api.messages.SendReaction({ peer: inputPeerUser(A_ID, A_HASH), msgId: bMessageId, reaction: [] })); } catch {}
    }
    if (aMessageId > 0) {
      try { await a.invoke(new Api.messages.DeleteMessages({ revoke: true, id: [aMessageId] })); } catch {}
    }
    if (bMessageId > 0) {
      try { await b.invoke(new Api.messages.DeleteMessages({ revoke: true, id: [bMessageId] })); } catch {}
    }
    if (deliveredScheduledMessageId > 0) {
      try { await b.invoke(new Api.messages.DeleteMessages({ revoke: true, id: [deliveredScheduledMessageId] })); } catch {}
    }
    if (scheduledIds.length > 0) {
      try { await a.invoke(new Api.messages.DeleteScheduledMessages({ peer: inputPeerUser(B_ID, B_HASH), id: scheduledIds })); } catch {}
    }
    if (channelMessageId > 0) {
      try { await b.invoke(new Api.channels.DeleteMessages({ channel: inputChannel(channelId, channelHash), id: [channelMessageId] })); } catch {}
    }
    if (channelId > 0n) {
      try { await b.invoke(new Api.channels.DeleteChannel({ channel: inputChannel(channelId, channelHash) })); } catch {}
    }
    if (groupId !== undefined) {
      try { await a.invoke(new Api.messages.DeleteChat({ chatId: groupId })); } catch {}
    }
    a.destroy();
    b.destroy();
  }
  console.log(JSON.stringify(results));
  process.exit(0);
}

main().catch((error) => {
  console.error(String(error));
  process.exitCode = 1;
});
