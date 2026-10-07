import path from 'node:path';
import { createRequire } from 'node:module';
import process from 'node:process';

const API_ID = 1;
const API_HASH = '0123456789abcdef0123456789abcdef';
const FIXTURES = [
  { phone: '12025550101', username: 'isolated_test_01' },
  { phone: '12025550102', username: 'isolated_test_02' },
  { phone: '12025550103', username: 'isolated_test_03' },
];

type Actor = {
  client: any;
  id: bigint | number;
  username: string;
};

const quietLogger = { debug() {}, info() {}, warn() {}, error() {} };
let randomIdCounter = 0n;

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

function messageList(value: any): any[] {
  return [
    ...(Array.isArray(value?.messages) ? value.messages : []),
    ...(Array.isArray(value?.newMessages) ? value.newMessages : []),
  ];
}

function randomId() {
  randomIdCounter += 1n;
  return BigInt(Date.now()) * 1_000_000n + randomIdCounter;
}

function inputUser(Api: any, user: any) {
  requireCondition(user?.id !== undefined && user?.accessHash !== undefined, 'user has no usable access hash');
  return new Api.InputUser({ userId: user.id, accessHash: user.accessHash });
}

function inputPeerChat(Api: any, chat: any) {
  requireCondition(chat?.id !== undefined, 'chat has no ID');
  return new Api.InputPeerChat({ chatId: chat.id });
}

function bytesEqual(actual: unknown, expected: Uint8Array) {
  return actual !== undefined && Buffer.from(actual as Uint8Array).equals(Buffer.from(expected));
}

function findPollUpdate(value: any, pollID: bigint | number, label: string) {
  const update = (Array.isArray(value?.updates) ? value.updates : []).find(
    (candidate: any) => rpcName(candidate).toLowerCase().includes('updatemessagepoll')
      && String(candidate?.pollId) === String(pollID),
  );
  requireCondition(update?.poll && update?.results, `${label} did not return updateMessagePoll`);
  return update;
}

function pollRow(results: any, option: Uint8Array, label: string) {
  const row = Array.isArray(results?.results)
    ? results.results.find((candidate: any) => bytesEqual(candidate?.option, option))
    : undefined;
  requireCondition(row, `${label} omitted poll option ${Buffer.from(option).toString('hex')}`);
  return row;
}

function assertPollResults(
  results: any,
  chosenOption: Uint8Array,
  options: Uint8Array[],
  label: string,
) {
  requireCondition(Number(results?.totalVoters) === 2, `${label} totalVoters = ${String(results?.totalVoters)}, want 2`);
  requireCondition(Array.isArray(results?.results) && results.results.length === options.length, `${label} did not return every answer`);
  for (const [index, option] of options.entries()) {
    const row = pollRow(results, option, label);
    const expectedVoters = index < 2 ? 1 : 0;
    const voters = row?.voters === undefined ? 0 : Number(row.voters);
    requireCondition(voters === expectedVoters, `${label} option ${index} voters = ${String(row?.voters)}, want ${expectedVoters}`);
    requireCondition(Boolean(row?.chosen) === bytesEqual(option, chosenOption), `${label} option ${index} chosen state is wrong`);
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
      firstAndLastNames: async () => ['Isolated', 'Poll'],
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

async function history(Api: any, actor: Actor, peer: any) {
  return actor.client.invoke(new Api.messages.GetHistory({
    peer,
    offsetId: 0,
    offsetDate: 0,
    addOffset: 0,
    limit: 20,
    maxId: 0,
    minId: 0,
    hash: 0n,
  }));
}

async function waitForPoll(Api: any, actor: Actor, peer: any, caption: string, attempts = 50) {
  for (let attempt = 0; attempt < attempts; attempt += 1) {
    const value = await history(Api, actor, peer);
    const message = messageList(value).find(
      (candidate: any) => candidate?.message === caption && candidate?.media?.poll,
    );
    if (message) return message;
    await new Promise((resolve) => setTimeout(resolve, 300));
  }
  throw new Error('member history did not return the poll message');
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
  let owner: Actor | undefined;
  let groupID: bigint | number | undefined;

  try {
    actors = await Promise.all(FIXTURES.map((fixture) => login(gram, fixture)));
    const [ownerActor, member, outsider] = actors;
    requireCondition(ownerActor && member && outsider, 'fixture login returned an incomplete actor set');
    owner = ownerActor;

    const memberUser = await searchUser(Api, owner, member.username);
    const tag = `poll_flow_${Date.now()}_${process.pid}`;
    const title = `${tag} group`;
    const created = await owner.client.invoke(new Api.messages.CreateChat({
      users: [inputUser(Api, memberUser)],
      title,
    }));
    const group = chatList(created).find((chat) => chat?.title === title);
    requireCondition(group?.id !== undefined, 'messages.createChat did not return the new group');
    groupID = group.id;
    const peer = inputPeerChat(Api, group);
    console.log(`PASS group creation: ${rpcName(created)}`);

    const caption = `${tag} poll`;
    const options = [Buffer.from([0]), Buffer.from([1]), Buffer.from([2])];
    const poll = new Api.Poll({
      id: 0n,
      publicVoters: true,
      question: new Api.TextWithEntities({ text: `${tag} question`, entities: [] }),
      answers: [
        new Api.PollAnswer({ text: new Api.TextWithEntities({ text: 'One', entities: [] }), option: options[0] }),
        new Api.PollAnswer({ text: new Api.TextWithEntities({ text: 'Two', entities: [] }), option: options[1] }),
        new Api.PollAnswer({ text: new Api.TextWithEntities({ text: 'Three', entities: [] }), option: options[2] }),
      ],
      hash: 0n,
    });
    const sent = await owner.client.invoke(new Api.messages.SendMedia({
      peer,
      media: new Api.InputMediaPoll({ poll }),
      message: caption,
      randomId: randomId(),
    }));
    requireCondition(rpcName(sent).toLowerCase().includes('updates'), 'messages.sendMedia did not return updates');
    console.log(`PASS poll send: ${rpcName(sent)}`);

    const ownerMessage = await waitForPoll(Api, owner, peer, caption);
    const memberMessage = await waitForPoll(Api, member, peer, caption);
    requireCondition(typeof ownerMessage.id === 'number', 'owner poll message did not have an ID');
    requireCondition(typeof memberMessage.id === 'number', 'member poll message did not have an ID');
    const storedPoll = memberMessage.media.poll;
    requireCondition(
      storedPoll?.id !== undefined && storedPoll?.hash !== undefined
        && BigInt(storedPoll.id) !== 0n && BigInt(storedPoll.hash) !== 0n,
      'stored poll has no server ID or hash',
    );
    requireCondition(
      String(ownerMessage.media?.poll?.id) === String(storedPoll.id)
        && String(ownerMessage.media?.poll?.hash) === String(storedPoll.hash),
      'owner and member poll records disagree on ID or hash',
    );
    console.log(`PASS owner and member poll read: ${rpcName(memberMessage)}`);

    const ownerVote = await owner.client.invoke(new Api.messages.SendVote({
      peer,
      msgId: ownerMessage.id,
      options: [options[0]],
    }));
    findPollUpdate(ownerVote, storedPoll.id, 'owner vote');
    const memberVote = await member.client.invoke(new Api.messages.SendVote({
      peer,
      msgId: memberMessage.id,
      options: [options[1]],
    }));
    findPollUpdate(memberVote, storedPoll.id, 'member vote');
    console.log('PASS owner and member votes');

    const ownerResults = await owner.client.invoke(new Api.messages.GetPollResults({
      peer,
      msgId: ownerMessage.id,
      pollHash: storedPoll.hash,
    }));
    assertPollResults(findPollUpdate(ownerResults, storedPoll.id, 'owner poll results').results, options[0], options, 'owner poll results');
    const memberResults = await member.client.invoke(new Api.messages.GetPollResults({
      peer,
      msgId: memberMessage.id,
      pollHash: storedPoll.hash,
    }));
    assertPollResults(findPollUpdate(memberResults, storedPoll.id, 'member poll results').results, options[1], options, 'member poll results');
    console.log('PASS owner and member poll results');

    const votes = await owner.client.invoke(new Api.messages.GetPollVotes({
      peer,
      id: ownerMessage.id,
      limit: 20,
    }));
    requireCondition(Number(votes?.count) === 2 && Array.isArray(votes?.votes) && votes.votes.length === 2, 'messages.getPollVotes did not return two votes');
    const voterIDs = new Set<string>();
    for (const vote of votes.votes) {
      requireCondition(rpcName(vote?.peer).toLowerCase().includes('peeruser'), 'messages.getPollVotes returned a non-user voter');
      requireCondition(vote?.peer?.userId !== undefined, 'messages.getPollVotes voter lacks user ID');
      voterIDs.add(String(vote.peer.userId));
    }
    requireCondition(voterIDs.size === 2 && voterIDs.has(String(owner.id)) && voterIDs.has(String(member.id)), 'messages.getPollVotes returned the wrong voter IDs');
    console.log('PASS public voter list');

    const views = await member.client.invoke(new Api.messages.GetMessagesViews({
      peer,
      id: [memberMessage.id],
      increment: false,
    }));
    requireCondition(Array.isArray(views?.views) && views.views.length === 1, 'messages.getMessagesViews did not return the poll message view');
    console.log(`PASS poll message views: ${rpcName(views)}`);

    await expectRpcError('duplicate poll option rejection', 'POLL_OPTION_DUPLICATE', () => owner.client.invoke(
      new Api.messages.SendVote({ peer, msgId: ownerMessage.id, options: [options[0], options[0]] }),
    ));
    await expectRpcError('invalid poll option rejection', 'POLL_OPTION_INVALID', () => owner.client.invoke(
      new Api.messages.SendVote({ peer, msgId: ownerMessage.id, options: [Buffer.from([99])] }),
    ));
    await expectRpcError('outsider vote rejection', 'USER_NOT_PARTICIPANT', () => outsider.client.invoke(
      new Api.messages.SendVote({ peer, msgId: ownerMessage.id, options: [options[2]] }),
    ));
    await expectRpcError('outsider poll result rejection', 'USER_NOT_PARTICIPANT', () => outsider.client.invoke(
      new Api.messages.GetPollResults({ peer, msgId: ownerMessage.id, pollHash: storedPoll.hash }),
    ));
  } finally {
    if (groupID !== undefined && owner) {
      try {
        await owner.client.invoke(new Api.messages.DeleteChat({ chatId: groupID }));
        console.log('PASS group cleanup');
      } catch (error) {
        console.error(`group cleanup failed: ${String(error)}`);
      }
    }
    for (const actor of actors) actor.client.destroy();
  }
}

void run()
  .then(() => process.exit(0))
  .catch((error) => {
    console.error(`poll probe fatal: ${error instanceof Error && error.stack ? error.stack : String(error)}`);
    process.exit(1);
  });
