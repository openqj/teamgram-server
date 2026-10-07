import path from 'node:path';
import { createRequire } from 'node:module';

for (const key of ['TEAMGRAM_GRAMJS_DIR', 'A_AUTH_B64', 'B_AUTH_B64', 'A_USER_ID', 'B_USER_ID', 'A_ACCESS_HASH']) {
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
const globalAny = globalThis as typeof globalThis & { self?: any; addEventListener?: () => void };
globalAny.self ??= globalThis;
globalAny.addEventListener ??= () => {};
globalAny.self.addEventListener ??= globalAny.addEventListener;

function name(value: any): string {
  return value?.className || value?.constructor?.name || typeof value;
}

function normalize(settings: any) {
  const gifts = settings?.disallowedGifts;
  return {
    archiveAndMuteNewNoncontactPeers: Boolean(settings?.archiveAndMuteNewNoncontactPeers),
    keepArchivedUnmuted: Boolean(settings?.keepArchivedUnmuted),
    keepArchivedFolders: Boolean(settings?.keepArchivedFolders),
    hideReadMarks: Boolean(settings?.hideReadMarks),
    newNoncontactPeersRequirePremium: Boolean(settings?.newNoncontactPeersRequirePremium),
    displayGiftsButton: Boolean(settings?.displayGiftsButton),
    noncontactPeersPaidStars: settings?.noncontactPeersPaidStars == null ? null : String(settings.noncontactPeersPaidStars),
    disallowedGifts: gifts == null ? null : {
      disallowUnlimitedStargifts: Boolean(gifts.disallowUnlimitedStargifts),
      disallowLimitedStargifts: Boolean(gifts.disallowLimitedStargifts),
      disallowUniqueStargifts: Boolean(gifts.disallowUniqueStargifts),
      disallowPremiumGifts: Boolean(gifts.disallowPremiumGifts),
      disallowStargiftsFromChannels: Boolean(gifts.disallowStargiftsFromChannels),
    },
  };
}

function settingsInput(normalized: ReturnType<typeof normalize>): any {
  return new Api.GlobalPrivacySettings({
    ...(normalized.archiveAndMuteNewNoncontactPeers ? { archiveAndMuteNewNoncontactPeers: true } : {}),
    ...(normalized.keepArchivedUnmuted ? { keepArchivedUnmuted: true } : {}),
    ...(normalized.keepArchivedFolders ? { keepArchivedFolders: true } : {}),
    ...(normalized.hideReadMarks ? { hideReadMarks: true } : {}),
    ...(normalized.newNoncontactPeersRequirePremium ? { newNoncontactPeersRequirePremium: true } : {}),
    ...(normalized.displayGiftsButton ? { displayGiftsButton: true } : {}),
    ...(normalized.noncontactPeersPaidStars === null ? {} : { noncontactPeersPaidStars: BigInt(normalized.noncontactPeersPaidStars) }),
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

class GetIsPremiumRequiredToContactRequest {
  CONSTRUCTOR_ID = 0xa622aa10;
  className = 'users.getIsPremiumRequiredToContact';
  classType = 'request';

  constructor(private readonly users: any[]) {}

  getBytes() {
    const constructor = Buffer.alloc(4);
    constructor.writeUInt32LE(this.CONSTRUCTOR_ID, 0);
    const vector = Buffer.alloc(8);
    vector.writeUInt32LE(0x1cb5c415, 0);
    vector.writeInt32LE(this.users.length, 4);
    return Uint8Array.from(Buffer.concat([
      constructor,
      vector,
      ...this.users.map((user) => Buffer.from(user.getBytes())),
    ]));
  }

  readResult(reader: any) {
    return reader.tgReadObject();
  }
}

function makeClient(auth: string): any {
  const session = new CallbackSession({
    mainDcId: dcId,
    keys: { [dcId]: Buffer.from(auth, 'base64').toString('hex') },
  }, () => {});
  const load = session.load.bind(session);
  session.load = async () => {
    await load();
    (session as any)._serverAddress = process.env.TG_DC_HOST || '127.0.0.1';
    (session as any)._port = Number(process.env.TG_DC_PORT || '11443');
  };
  const client = new TelegramClient(session, 1, '0123456789abcdef0123456789abcdef', {
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
  // This probe supplies only the authenticated primary-DC key. Prevent GramJS
  // from opening an unrelated exported/media sender with no matching key.
  (client as any)._borrowExportedSender = async () => undefined;
  (client as any)._updateLoop = async () => undefined;
  return client;
}

async function main() {
  const owner = makeClient(process.env.A_AUTH_B64!);
  const requester = makeClient(process.env.B_AUTH_B64!);
  let original: ReturnType<typeof normalize> | undefined;
  try {
    await owner.connect();
    await requester.connect();
    const ownerMe = await owner.getMe();
    const requesterMe = await requester.getMe();
    if (String(ownerMe?.id) !== String(process.env.A_USER_ID) || String(requesterMe?.id) !== String(process.env.B_USER_ID)) {
      throw new Error('auth key user mismatch');
    }
    const before = await owner.invoke(new Api.account.GetGlobalPrivacySettings());
    if (!before) throw new Error(`getGlobalPrivacySettings returned ${name(before)}`);
    original = normalize(before);

    const candidate = { ...original, newNoncontactPeersRequirePremium: true, noncontactPeersPaidStars: null };
    const saved = await owner.invoke(new Api.account.SetGlobalPrivacySettings({ settings: settingsInput(candidate) }));
    if (name(saved) !== 'GlobalPrivacySettings') throw new Error(`setGlobalPrivacySettings returned ${name(saved)}`);
    if (JSON.stringify(normalize(await owner.invoke(new Api.account.GetGlobalPrivacySettings()))) !== JSON.stringify(candidate)) {
      throw new Error('global privacy setting did not persist');
    }

    const result = await requester.invoke(new Api.users.GetRequirementsToContact({
      id: [new Api.InputUser({
        userId: BigInt(process.env.A_USER_ID!),
        accessHash: BigInt(process.env.A_ACCESS_HASH!),
      })],
    }));
    const values = Array.isArray(result) ? result : (Array.isArray(result?.requirements) ? result.requirements : []);
    if (values.length !== 1 || name(values[0]) !== 'RequirementToContactPremium') {
      throw new Error(`unexpected result ${name(result)} values=${values.map(name).join(',')}`);
    }
    const premiumResult = await requester.invoke(new GetIsPremiumRequiredToContactRequest([
      new Api.InputUser({
        userId: BigInt(process.env.A_USER_ID!),
        accessHash: BigInt(process.env.A_ACCESS_HASH!),
      }),
    ]) as any);
    const flags = Array.isArray(premiumResult) ? premiumResult : (Array.isArray(premiumResult?.datas) ? premiumResult.datas : []);
    const isTrue = (value: any) => value === true || name(value) === 'BoolTrue';
    if (flags.length !== 1 || !isTrue(flags[0])) {
      throw new Error(`unexpected premium flag ${name(premiumResult)} values=${flags.map(name).join(',')}`);
    }
    console.log(JSON.stringify({
      requirementsResult: name(result), requirements: values.map(name),
      premiumResult: name(premiumResult), premiumFlags: flags.map(name),
      requester: process.env.B_USER_ID, target: process.env.A_USER_ID,
    }));
  } finally {
    try {
      if (original !== undefined) {
        const restored = await owner.invoke(new Api.account.SetGlobalPrivacySettings({ settings: settingsInput(original) }));
        if (name(restored) !== 'GlobalPrivacySettings') throw new Error(`restore returned ${name(restored)}`);
        if (JSON.stringify(normalize(await owner.invoke(new Api.account.GetGlobalPrivacySettings()))) !== JSON.stringify(original)) {
          throw new Error('global privacy settings were not restored');
        }
      }
    } finally {
      owner.destroy();
      requester.destroy();
    }
  }
}

main().then(() => process.exit(0)).catch((error) => {
  console.error(`requirements-to-contact probe failed: ${error instanceof Error ? error.stack || error.message : String(error)}`);
  process.exit(1);
});
