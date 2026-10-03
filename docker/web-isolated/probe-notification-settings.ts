import path from 'node:path';
import { createRequire } from 'node:module';
import process from 'node:process';

const API_ID = 1;
const API_HASH = '0123456789abcdef0123456789abcdef';
const FIXTURES = [
  { phone: '12025550101', username: 'isolated_test_01' },
  { phone: '12025550102', username: 'isolated_test_02' },
];

const quietLogger = { debug() {}, info() {}, warn() {}, error() {} };

function requireCondition(condition: unknown, message: string): asserts condition {
  if (!condition) throw new Error(message);
}

function rpcName(value: any): string {
  return value?.className || value?.constructor?.name || typeof value;
}

function isTrue(value: any): boolean {
  return value === true || rpcName(value).toLowerCase().includes('true');
}

function peerId(value: any): string | undefined {
  const id = value?.userId ?? value?.chatId ?? value?.channelId;
  return id === undefined ? undefined : String(id);
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

async function login(gram: any, fixture: typeof FIXTURES[number]) {
  const client = new gram.TelegramClient(new gram.MemorySession(), API_ID, API_HASH, {
    connection: gram.Connection,
    fallbackConnection: gram.Connection,
    shouldAllowHttpTransport: false,
    shouldForceHttpTransport: false,
    timeout: 15,
    requestRetries: 2,
    connectionRetries: 1,
    connectionRetriesToFallback: 0,
    retryDelay: 250,
    autoReconnect: false,
    baseLogger: quietLogger,
  });
  await client.start({
    phoneNumber: fixture.phone,
    phoneCode: async () => '12345',
    password: async () => '',
    firstAndLastNames: async () => ['Isolated', 'Notify'],
    qrCode: async () => {},
    onError: (error: Error) => { throw error; },
    webAuthTokenFailed: () => {},
    onPasskeyOption: () => {},
    initialMethod: 'phoneNumber',
  });
  const me = await client.getMe();
  requireCondition(me?.id !== undefined, `login failed for ${fixture.username}`);
  return { client, me, ...fixture };
}

async function searchUser(Api: any, actor: any, username: string) {
  const result = await actor.client.invoke(new Api.contacts.Search({ q: username, limit: 10 }));
  const user = result?.users?.find((candidate: any) => candidate?.username === username);
  requireCondition(user, `contacts.search did not find ${username}`);
  return user;
}

function inputPeerUser(Api: any, user: any) {
  requireCondition(user?.id !== undefined && user?.accessHash !== undefined, 'user lacks access hash');
  return new Api.InputPeerUser({ userId: user.id, accessHash: user.accessHash });
}

function inputNotifyPeer(Api: any, peer: any) {
  return new Api.InputNotifyPeer({ peer });
}

function updateForPeer(value: any, userId: any) {
  return (value?.updates || []).find((update: any) => (
    rpcName(update).includes('UpdateNotifySettings')
      && peerId(update?.peer?.peer) === String(userId)
  ));
}

async function run() {
  requireCondition(process.env.TG_DC_HOST === '127.0.0.1', 'probe must target the isolated gateway');
  requireCondition(process.env.TG_DC_PORT === '31443', 'probe must target the isolated gateway');
  requireCondition(process.env.TEAMGRAM_GRAMJS_DIR, 'TEAMGRAM_GRAMJS_DIR is required');

  const globalAny = globalThis as typeof globalThis & { self?: any; addEventListener?: () => void };
  globalAny.self ??= globalThis;
  globalAny.addEventListener ??= () => {};
  globalAny.self.addEventListener ??= globalAny.addEventListener;

  const gram = await loadGramJs(process.env.TEAMGRAM_GRAMJS_DIR);
  const { Api } = gram;
  const alice = await login(gram, FIXTURES[0]);
  const bob = await login(gram, FIXTURES[1]);
  let contactSignUpBeforeEnabled: boolean | undefined;
  let registeredDevice: { tokenType: number; token: string; otherUids: number[] } | undefined;

  try {
    const bobUser = await searchUser(Api, alice, bob.username);
    const aliceUser = await searchUser(Api, bob, alice.username);
    const aliceToBob = inputNotifyPeer(Api, inputPeerUser(Api, bobUser));
    const bobToAlice = inputNotifyPeer(Api, inputPeerUser(Api, aliceUser));

    await alice.client.invoke(new Api.account.ResetNotifySettings());
    await bob.client.invoke(new Api.account.ResetNotifySettings());

    const aggregate = await alice.client.invoke(new Api.account.GetNotifySettings({
      peer: new Api.InputNotifyUsers(),
    }));
    requireCondition(rpcName(aggregate).includes('PeerNotifySettings'), `aggregate get returned ${rpcName(aggregate)}`);

    const saved = await alice.client.invoke(new Api.account.UpdateNotifySettings({
      peer: aliceToBob,
      settings: new Api.InputPeerNotifySettings({
        muteUntil: 2_147_483_647,
        showPreviews: false,
        silent: true,
        sound: new Api.NotificationSoundDefault(),
      }),
    }));
    requireCondition(isTrue(saved), 'account.updateNotifySettings did not return true');

    const loaded = await alice.client.invoke(new Api.account.GetNotifySettings({ peer: aliceToBob }));
    requireCondition(Number(loaded?.muteUntil) === 2_147_483_647, 'saved mute_until was not read back');
    requireCondition(loaded?.showPreviews === false, 'saved show_previews was not read back');
    requireCondition(loaded?.silent === true, 'saved silent was not read back');

    const otherUser = await bob.client.invoke(new Api.account.GetNotifySettings({ peer: bobToAlice }));
    requireCondition(Number(otherUser?.muteUntil ?? 0) !== 2_147_483_647, 'notify settings leaked across users');

    const exceptions = await alice.client.invoke(new Api.account.GetNotifyExceptions({ compareSound: true }));
    requireCondition(updateForPeer(exceptions, bobUser.id), 'getNotifyExceptions omitted the saved user peer');

    const reset = await alice.client.invoke(new Api.account.ResetNotifySettings());
    requireCondition(isTrue(reset), 'account.resetNotifySettings did not return true');
    const afterReset = await alice.client.invoke(new Api.account.GetNotifySettings({ peer: aliceToBob }));
    requireCondition(Number(afterReset?.muteUntil ?? 0) !== 2_147_483_647, 'reset did not clear the user exception');
    requireCondition(afterReset?.silent !== true, 'reset left silent enabled');
    const exceptionsAfterReset = await alice.client.invoke(new Api.account.GetNotifyExceptions({ compareSound: true }));
    requireCondition(!updateForPeer(exceptionsAfterReset, bobUser.id), 'reset left a notify exception');

    const contactSignUpBefore = await alice.client.invoke(new Api.account.GetContactSignUpNotification());
    contactSignUpBeforeEnabled = isTrue(contactSignUpBefore);
    const setContactSignUpSilent = await alice.client.invoke(new Api.account.SetContactSignUpNotification({ silent: true }));
    requireCondition(isTrue(setContactSignUpSilent), 'account.setContactSignUpNotification silent=true did not return true');
    const contactSignUpSilent = await alice.client.invoke(new Api.account.GetContactSignUpNotification());
    requireCondition(!isTrue(contactSignUpSilent), 'contact sign-up notification stayed enabled after silent=true');
    const setContactSignUpEnabled = await alice.client.invoke(new Api.account.SetContactSignUpNotification({ silent: false }));
    requireCondition(isTrue(setContactSignUpEnabled), 'account.setContactSignUpNotification silent=false did not return true');
    const contactSignUpEnabled = await alice.client.invoke(new Api.account.GetContactSignUpNotification());
    requireCondition(isTrue(contactSignUpEnabled), 'contact sign-up notification stayed disabled after silent=false');
    const deviceToken = `isolated-notification-probe-${Date.now()}`;
    const deviceArgs = { tokenType: 10, token: deviceToken, appSandbox: true, secret: new Uint8Array(), otherUids: [] };
    const registered = await alice.client.invoke(new Api.account.RegisterDevice(deviceArgs));
    requireCondition(isTrue(registered), 'account.registerDevice did not return true');
    registeredDevice = {
      tokenType: deviceArgs.tokenType,
      token: deviceArgs.token,
      otherUids: deviceArgs.otherUids,
    };
    const unregistered = await alice.client.invoke(new Api.account.UnregisterDevice({
      tokenType: registeredDevice.tokenType,
      token: registeredDevice.token,
      otherUids: registeredDevice.otherUids,
    }));
    requireCondition(isTrue(unregistered), 'account.unregisterDevice did not return true');
    registeredDevice = undefined;

    const restoredContactSignUp = await alice.client.invoke(new Api.account.SetContactSignUpNotification({
      silent: !contactSignUpBeforeEnabled,
    }));
    requireCondition(isTrue(restoredContactSignUp), 'contact sign-up notification restore did not return true');
    const contactSignUpRestored = await alice.client.invoke(new Api.account.GetContactSignUpNotification());
    requireCondition(
      isTrue(contactSignUpRestored) === contactSignUpBeforeEnabled,
      'contact sign-up notification restore did not read back',
    );
    contactSignUpBeforeEnabled = undefined;

    console.log('notification-settings probe passed');
  } finally {
    if (registeredDevice) {
      try {
        await alice.client.invoke(new Api.account.UnregisterDevice(registeredDevice));
      } catch {}
    }
    if (contactSignUpBeforeEnabled !== undefined) {
      try {
        await alice.client.invoke(new Api.account.SetContactSignUpNotification({ silent: !contactSignUpBeforeEnabled }));
      } catch {}
    }
    try { await alice.client.invoke(new Api.account.ResetNotifySettings()); } catch {}
    try { await bob.client.invoke(new Api.account.ResetNotifySettings()); } catch {}
    alice.client.destroy();
    bob.client.destroy();
  }
}

void run()
  .then(() => process.exit(0))
  .catch((error) => {
    console.error(error instanceof Error && error.stack ? error.stack : String(error));
    process.exit(1);
  });
