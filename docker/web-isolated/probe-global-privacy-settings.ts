import path from 'node:path';
import { createRequire } from 'node:module';
import process from 'node:process';

const API_ID = 1;
const API_HASH = '0123456789abcdef0123456789abcdef';
const FIXTURE = { phone: '12025550101', username: 'isolated_test_01' };
const quietLogger = { debug() {}, info() {}, warn() {}, error() {} };

function requireCondition(condition: unknown, message: string): asserts condition {
  if (!condition) throw new Error(message);
}

function rpcName(value: unknown) {
  return (value as { className?: string; constructor?: { name?: string } })?.className
    || (value as { constructor?: { name?: string } })?.constructor?.name
    || typeof value;
}

function bool(value: unknown): boolean {
  return value === true;
}

function normalize(settings: any) {
  const gifts = settings?.disallowedGifts;
  return {
    archiveAndMuteNewNoncontactPeers: bool(settings?.archiveAndMuteNewNoncontactPeers),
    keepArchivedUnmuted: bool(settings?.keepArchivedUnmuted),
    keepArchivedFolders: bool(settings?.keepArchivedFolders),
    hideReadMarks: bool(settings?.hideReadMarks),
    newNoncontactPeersRequirePremium: bool(settings?.newNoncontactPeersRequirePremium),
    displayGiftsButton: bool(settings?.displayGiftsButton),
    noncontactPeersPaidStars: settings?.noncontactPeersPaidStars === undefined
      ? null
      : String(settings.noncontactPeersPaidStars),
    disallowedGifts: gifts === undefined ? null : {
      disallowUnlimitedStargifts: bool(gifts.disallowUnlimitedStargifts),
      disallowLimitedStargifts: bool(gifts.disallowLimitedStargifts),
      disallowUniqueStargifts: bool(gifts.disallowUniqueStargifts),
      disallowPremiumGifts: bool(gifts.disallowPremiumGifts),
      disallowStargiftsFromChannels: bool(gifts.disallowStargiftsFromChannels),
    },
  };
}

function settingsInput(Api: any, normalized: ReturnType<typeof normalize>) {
  return new Api.GlobalPrivacySettings({
    ...(normalized.archiveAndMuteNewNoncontactPeers ? { archiveAndMuteNewNoncontactPeers: true } : {}),
    ...(normalized.keepArchivedUnmuted ? { keepArchivedUnmuted: true } : {}),
    ...(normalized.keepArchivedFolders ? { keepArchivedFolders: true } : {}),
    ...(normalized.hideReadMarks ? { hideReadMarks: true } : {}),
    ...(normalized.newNoncontactPeersRequirePremium ? { newNoncontactPeersRequirePremium: true } : {}),
    ...(normalized.displayGiftsButton ? { displayGiftsButton: true } : {}),
    ...(normalized.noncontactPeersPaidStars === null
      ? {}
      : { noncontactPeersPaidStars: BigInt(normalized.noncontactPeersPaidStars) }),
    ...(normalized.disallowedGifts === null ? {} : {
      disallowedGifts: new Api.DisallowedGiftsSettings({
        ...(normalized.disallowedGifts.disallowUnlimitedStargifts ? { disallowUnlimitedStargifts: true } : {}),
        ...(normalized.disallowedGifts.disallowLimitedStargifts ? { disallowLimitedStargifts: true } : {}),
        ...(normalized.disallowedGifts.disallowUniqueStargifts ? { disallowUniqueStargifts: true } : {}),
        ...(normalized.disallowedGifts.disallowPremiumGifts ? { disallowPremiumGifts: true } : {}),
        ...(normalized.disallowedGifts.disallowStargiftsFromChannels ? { disallowStargiftsFromChannels: true } : {}),
      }),
    }),
  });
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

async function login(gram: any) {
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
      phoneNumber: FIXTURE.phone,
      phoneCode: async () => '12345',
      password: async () => '',
      firstAndLastNames: async () => ['Isolated', 'Privacy'],
      qrCode: async () => {},
      onError: (error: Error) => { throw error; },
      webAuthTokenFailed: () => {},
      onPasskeyOption: () => {},
      initialMethod: 'phoneNumber',
    });
    const me = await client.getMe();
    requireCondition(me?.id !== undefined, `login failed for ${FIXTURE.username}`);
    return { client, id: me.id };
  } catch (error) {
    client.destroy();
    throw error;
  }
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
  const actor = await login(gram);
  let original: ReturnType<typeof normalize> | undefined;
  try {
    const before = await actor.client.invoke(new Api.account.GetGlobalPrivacySettings());
    requireCondition(before, `getGlobalPrivacySettings returned ${rpcName(before)}`);
    original = normalize(before);

    const candidate = {
      archiveAndMuteNewNoncontactPeers: true,
      keepArchivedUnmuted: true,
      keepArchivedFolders: true,
      hideReadMarks: true,
      newNoncontactPeersRequirePremium: true,
      displayGiftsButton: true,
      noncontactPeersPaidStars: '123',
      disallowedGifts: {
        disallowUnlimitedStargifts: true,
        disallowLimitedStargifts: false,
        disallowUniqueStargifts: true,
        disallowPremiumGifts: false,
        disallowStargiftsFromChannels: true,
      },
    } as ReturnType<typeof normalize>;
    const saved = await actor.client.invoke(new Api.account.SetGlobalPrivacySettings({
      settings: settingsInput(Api, candidate),
    }));
    requireCondition(saved, `setGlobalPrivacySettings returned ${rpcName(saved)}`);
    const after = normalize(await actor.client.invoke(new Api.account.GetGlobalPrivacySettings()));
    requireCondition(JSON.stringify(after) === JSON.stringify(candidate), `readback mismatch: ${JSON.stringify(after)}`);
    console.log(`PASS account global privacy set/get user=${String(actor.id)} ${JSON.stringify(after)}`);
  } finally {
    try {
      if (original !== undefined) {
        await actor.client.invoke(new Api.account.SetGlobalPrivacySettings({
          settings: settingsInput(Api, original),
        }));
      }
    } finally {
      actor.client.destroy();
    }
  }
}

void run()
  .then(() => process.exit(0))
  .catch((error) => {
    console.error(`global privacy probe fatal: ${error instanceof Error && error.stack ? error.stack : String(error)}`);
    process.exit(1);
  });
