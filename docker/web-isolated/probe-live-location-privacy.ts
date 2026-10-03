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

function requireCondition(condition: unknown, message: string): asserts condition {
  if (!condition) throw new Error(message);
}

function inputUser(Api: any, user: any) {
  requireCondition(user?.id !== undefined && user?.accessHash !== undefined, 'user has no usable access hash');
  return new Api.InputUser({ userId: user.id, accessHash: user.accessHash });
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
      firstAndLastNames: async () => ['Isolated', 'Location'],
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
  const found = await actor.client.invoke(new Api.contacts.Search({ q: username, limit: 10 }));
  const user = found?.users?.find((candidate: any) => candidate?.username === username);
  requireCondition(user, `contacts.search did not find ${username}`);
  return user;
}

function locatedPeerIDs(updates: any): string[] {
  const ids: string[] = [];
  for (const update of updates?.updates || []) {
    for (const peer of update?.peers || []) {
      if (peer?.peer?.userId !== undefined) ids.push(String(peer.peer.userId));
    }
  }
  return ids;
}

function locatedRequest(Api: any) {
  if (Api.contacts?.GetLocated) return Api.contacts.GetLocated;

  class GetLocated {
    static CONSTRUCTOR_ID = 0xd348bc44;

    static className = 'contacts.GetLocated';

    static classType = 'request';

    CONSTRUCTOR_ID = GetLocated.CONSTRUCTOR_ID;

    className = GetLocated.className;

    classType = GetLocated.classType;

    geoPoint: any;

    selfExpires?: number;

    constructor(args: { geoPoint: any; selfExpires?: number }) {
      this.geoPoint = args.geoPoint;
      this.selfExpires = args.selfExpires;
    }

    getBytes() {
      const constructor = new Uint8Array(4);
      new DataView(constructor.buffer).setUint32(0, GetLocated.CONSTRUCTOR_ID, true);
      const flags = new Uint8Array(4);
      new DataView(flags.buffer).setInt32(0, this.selfExpires === undefined ? 0 : 1, true);
      const expires = this.selfExpires === undefined ? new Uint8Array() : (() => {
        const value = new Uint8Array(4);
        new DataView(value.buffer).setInt32(0, this.selfExpires!, true);
        return value;
      })();
      const geoPoint = this.geoPoint.getBytes();
      const bytes = new Uint8Array(constructor.length + flags.length + geoPoint.length + expires.length);
      bytes.set(constructor, 0);
      bytes.set(flags, constructor.length);
      bytes.set(geoPoint, constructor.length + flags.length);
      bytes.set(expires, constructor.length + flags.length + geoPoint.length);
      return bytes;
    }

    readResult(reader: any) {
      return reader.tgReadObject();
    }
  }

  return GetLocated;
}

function locationRequest(Api: any, selfExpires?: number) {
  const GetLocated = locatedRequest(Api);
  return new GetLocated({
    geoPoint: new Api.InputGeoPoint({ lat: 31.2304, long: 121.4737 }),
    ...(selfExpires === undefined ? {} : { selfExpires }),
  });
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
  let alice: Actor | undefined;
  let bob: Actor | undefined;
  let outsider: Actor | undefined;
  let aliceInputBob: any;
  let bobInputAlice: any;

  try {
    [alice, bob, outsider] = await Promise.all(FIXTURES.map((fixture) => login(gram, fixture)));
    aliceInputBob = inputUser(Api, await searchUser(Api, alice, bob.username));
    bobInputAlice = inputUser(Api, await searchUser(Api, bob, alice.username));

    await alice.client.invoke(new Api.contacts.AddContact({
      id: aliceInputBob,
      firstName: 'Isolated',
      lastName: 'Location Bob',
      phone: FIXTURES[1].phone,
    }));
    await bob.client.invoke(new Api.contacts.AddContact({
      id: bobInputAlice,
      firstName: 'Isolated',
      lastName: 'Location Alice',
      phone: FIXTURES[0].phone,
    }));
    console.log('PASS reciprocal contact setup');

    await bob.client.invoke(locationRequest(Api, Math.floor(Date.now() / 1000) + 300));
    const mutualView = await alice.client.invoke(locationRequest(Api));
    requireCondition(locatedPeerIDs(mutualView).includes(String(bob.id)), 'mutual contact could not see Bob live location');
    console.log('PASS mutual contact location visibility');

    const outsiderView = await outsider.client.invoke(locationRequest(Api));
    requireCondition(!locatedPeerIDs(outsiderView).includes(String(bob.id)), 'non-contact received Bob live location');
    console.log('PASS non-contact location rejection');
  } finally {
    if (bob) {
      try {
        await bob.client.invoke(locationRequest(Api, Math.floor(Date.now() / 1000) - 1));
      } catch (error) {
        console.error(`live location cleanup failed: ${String(error)}`);
      }
    }
    if (alice && aliceInputBob) {
      try {
        await alice.client.invoke(new Api.contacts.DeleteContacts({ id: [aliceInputBob] }));
      } catch (error) {
        console.error(`Alice contact cleanup failed: ${String(error)}`);
      }
    }
    if (bob && bobInputAlice) {
      try {
        await bob.client.invoke(new Api.contacts.DeleteContacts({ id: [bobInputAlice] }));
      } catch (error) {
        console.error(`Bob contact cleanup failed: ${String(error)}`);
      }
    }
    for (const actor of [alice, bob, outsider]) actor?.client.destroy();
  }
}

void run()
  .then(() => process.exit(0))
  .catch((error) => {
    console.error(`live location privacy probe fatal: ${error instanceof Error && error.stack ? error.stack : String(error)}`);
    process.exit(1);
  });
