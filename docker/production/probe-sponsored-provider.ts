import { createRequire } from 'node:module';
import path from 'node:path';

for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) {
  delete process.env[key];
}

for (const key of ['TEAMGRAM_GRAMJS_DIR', 'AUTH_B64', 'USER_ID']) {
  if (!process.env[key]) throw new Error(`${key} is required`);
}

const gramjsDir = process.env.TEAMGRAM_GRAMJS_DIR!;
const dcId = Number(process.env.TG_DC_ID || '2');
const dcHost = process.env.TG_DC_HOST || '127.0.0.1';
const dcPort = Number(process.env.TG_DC_PORT || '11443');
const apiId = Number(process.env.TG_API_ID || '1');
const apiHash = process.env.TG_API_HASH || '0123456789abcdef0123456789abcdef';
const userId = BigInt(process.env.USER_ID!);
const query = process.env.SPONSORED_QUERY || 'fixture';
const expectedPeerUserId = BigInt(process.env.SPONSORED_PROVIDER_PEER_USER_ID || '900000000001');

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

function isClass(value: any, name: string): boolean {
  const actual = className(value);
  return actual === name || actual.endsWith(`.${name}`);
}

function bytesKey(value: any): string {
  if (value == null) return '';
  return Buffer.from(value instanceof Uint8Array ? value : value.value || value).toString('base64');
}

function makeClient(): any {
  const authKeyHex = Buffer.from(process.env.AUTH_B64!, 'base64').toString('hex');
  const session = new CallbackSession({ mainDcId: dcId, keys: { [dcId]: authKeyHex } }, () => {});
  const load = session.load.bind(session);
  session.load = async () => {
    await load();
    (session as any)._serverAddress = dcHost;
    (session as any)._port = dcPort;
  };
  const client = new TelegramClient(session, apiId, apiHash, {
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
  (client as any)._borrowExportedSender = async () => undefined;
  (client as any)._updateLoop = async () => undefined;
  return client;
}

async function main(): Promise<void> {
  const client = makeClient();
  try {
    await client.connect();
    const me = await client.getMe();
    if (BigInt(me?.id) !== userId) throw new Error(`authenticated user mismatch: ${String(me?.id)}`);

    const peers = await client.invoke(new Api.contacts.GetSponsoredPeers({ q: query }));
    const peersReplay = await client.invoke(new Api.contacts.GetSponsoredPeers({ q: query }));
    if (!isClass(peers, 'SponsoredPeers') || !isClass(peersReplay, 'SponsoredPeers')) {
      throw new Error(`contacts.getSponsoredPeers returned ${className(peers)}/${className(peersReplay)}`);
    }
    if (!Array.isArray(peers.peers) || peers.peers.length !== 1 || peers.peers.length !== peersReplay.peers.length) {
      throw new Error('contacts.getSponsoredPeers did not return one deterministic fixture peer');
    }
    const sponsoredPeer = peers.peers[0];
    const peer = sponsoredPeer?.peer;
    if (!isClass(sponsoredPeer, 'SponsoredPeer') || !isClass(peer, 'PeerUser') || BigInt(peer.userId) !== expectedPeerUserId) {
      throw new Error(`unexpected sponsored peer ${className(sponsoredPeer)}/${className(peer)}/${String(peer?.userId)}`);
    }
    if (bytesKey(sponsoredPeer.randomId) !== bytesKey(peersReplay.peers[0].randomId)) {
      throw new Error('contacts.getSponsoredPeers replay changed random_id');
    }

    const messages = await client.invoke(new Api.messages.GetSponsoredMessages({ peer: new Api.InputPeerSelf() }));
    const messagesReplay = await client.invoke(new Api.messages.GetSponsoredMessages({ peer: new Api.InputPeerSelf() }));
    if (!isClass(messages, 'SponsoredMessages') || !isClass(messagesReplay, 'SponsoredMessages')) {
      throw new Error(`messages.getSponsoredMessages returned ${className(messages)}/${className(messagesReplay)}`);
    }
    if (!Array.isArray(messages.messages) || messages.messages.length !== 1 || messages.messages.length !== messagesReplay.messages.length) {
      throw new Error('messages.getSponsoredMessages did not return one deterministic fixture message');
    }
    const sponsoredMessage = messages.messages[0];
    if (!isClass(sponsoredMessage, 'SponsoredMessage') || !String(sponsoredMessage.message).includes(String(userId))) {
      throw new Error(`sponsored message is not bound to user ${userId}`);
    }
    if (bytesKey(sponsoredMessage.randomId) !== bytesKey(messagesReplay.messages[0].randomId)) {
      throw new Error('messages.getSponsoredMessages replay changed random_id');
    }

    console.log(JSON.stringify({
      event: 'sponsored_provider_fixture_rpc',
      transport: `DC${dcId} TCPObfuscated -> gateway -> session -> signed sponsored provider fixture`,
      userId: userId.toString(),
      query,
      contactsGetSponsoredPeers: {
        constructor: className(peers),
        peerConstructor: className(sponsoredPeer),
        peerUserId: String(peer.userId),
        randomId: bytesKey(sponsoredPeer.randomId),
      },
      messagesGetSponsoredMessages: {
        constructor: className(messages),
        messageConstructor: className(sponsoredMessage),
        randomId: bytesKey(sponsoredMessage.randomId),
        text: String(sponsoredMessage.message),
      },
      replay: { contacts: className(peersReplay), messages: className(messagesReplay) },
      cleanup: 'stateless provider fixture; no rows created',
    }));
  } finally {
    client.destroy();
  }
}

main().then(() => process.exit(0)).catch((error) => {
  console.error(`Sponsored provider probe failed: ${error instanceof Error ? error.stack || error.message : String(error)}`);
  process.exit(1);
});
