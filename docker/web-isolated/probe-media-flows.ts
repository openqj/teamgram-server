import path from 'node:path';
import { createRequire } from 'node:module';
import process from 'node:process';
import { createHash } from 'node:crypto';
import { readFile } from 'node:fs/promises';

const API_ID = 1;
const API_HASH = '0123456789abcdef0123456789abcdef';
const FIXTURES = [
  { phone: '12025550101', username: 'isolated_test_01' },
  { phone: '12025550102', username: 'isolated_test_02' },
];

const quietLogger = { debug() {}, info() {}, warn() {}, error() {} };
let randomIdCounter = 0n;

function rpcName(value: any) {
  return value?.className || value?.constructor?.name || typeof value;
}

function requireCondition(condition: unknown, message: string): asserts condition {
  if (!condition) throw new Error(message);
}

function randomId() {
  randomIdCounter += 1n;
  return BigInt(Date.now()) * 1_000_000n + randomIdCounter;
}

function inputUser(Api: any, user: any) {
  requireCondition(user?.id !== undefined && user?.accessHash !== undefined, 'user has no access hash');
  return new Api.InputUser({ userId: user.id, accessHash: user.accessHash });
}

function inputPeerUser(Api: any, user: any) {
  requireCondition(user?.id !== undefined && user?.accessHash !== undefined, 'peer has no access hash');
  return new Api.InputPeerUser({ userId: user.id, accessHash: user.accessHash });
}

function messageList(value: any) {
  return [
    ...(Array.isArray(value?.messages) ? value.messages : []),
    ...(Array.isArray(value?.newMessages) ? value.newMessages : []),
  ];
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
    WebSocketConnection: connectionModule.ConnectionTCPObfuscated,
  };
}

async function login(gram: any, fixture: typeof FIXTURES[number]) {
  const client = new gram.TelegramClient(new gram.MemorySession(), API_ID, API_HASH, {
    connection: gram.WebSocketConnection,
    fallbackConnection: gram.WebSocketConnection,
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
    firstAndLastNames: async () => ['Isolated', 'Media'],
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

const pngBytes = Uint8Array.from(Buffer.from(
  'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=',
  'base64',
));
const documentBytes = new TextEncoder().encode('teamgram isolated media probe');
const directDocumentBytes = new TextEncoder().encode('teamgram direct sendMedia upload probe');
const multipartDocumentBytes = Uint8Array.from(
  Array.from({ length: 128 * 1024 + 23 }, (_value, index) => index % 251),
);

function file(name: string, bytes: Uint8Array, type: string) {
  return new File([bytes], name, { type });
}

function documentLocation(Api: any, document: any) {
  requireCondition(document?.id !== undefined && document?.accessHash !== undefined, 'document has no download location');
  return new Api.InputDocumentFileLocation({
    id: document.id,
    accessHash: document.accessHash,
    fileReference: document.fileReference || new Uint8Array(),
    thumbSize: '',
  });
}

function photoLocation(Api: any, photo: any) {
  requireCondition(photo?.id !== undefined && photo?.accessHash !== undefined, 'photo has no download location');
  const size = photo.sizes?.find((candidate: any) => candidate?.type && candidate.type !== 'i');
  requireCondition(size, 'photo has no downloadable size');
  return new Api.InputPhotoFileLocation({
    id: photo.id,
    accessHash: photo.accessHash,
    fileReference: photo.fileReference || new Uint8Array(),
    thumbSize: size.type,
  });
}

function requireBytes(actual: Uint8Array | undefined, expected: Uint8Array, name: string) {
  requireCondition(actual && Buffer.from(actual).equals(Buffer.from(expected)), `${name} bytes differ from the upload`);
}

async function run() {
  requireCondition(process.env.TG_DC_HOST === '127.0.0.1', 'TG_DC_HOST must target isolated gateway');
  requireCondition(process.env.TG_DC_PORT === '31443', 'TG_DC_PORT must target isolated gateway');
  const globalAny = globalThis as typeof globalThis & { self?: any; addEventListener?: () => void };
  globalAny.self ??= globalThis;
  globalAny.addEventListener ??= () => {};
  globalAny.self.addEventListener ??= globalAny.addEventListener;
  const gram = await loadGramJs(process.env.TEAMGRAM_GRAMJS_DIR!);
  const { Api } = gram;
  const alice = await login(gram, FIXTURES[0]);
  const bob = await login(gram, FIXTURES[1]);
  const bobUser = await searchUser(Api, alice, bob.username);
  const bobPeer = inputPeerUser(Api, bobUser);
  const results: { name: string; status: string; detail: string }[] = [];
  const cleanup: (() => Promise<void>)[] = [];

  async function step(name: string, action: () => Promise<any>) {
    try {
      const value = await action();
      const detail = rpcName(value);
      results.push({ name, status: 'PASS', detail });
      console.log(`PASS ${name}: ${detail}`);
      return value;
    } catch (error) {
      const detail = String(error);
      results.push({ name, status: 'FAIL', detail });
      console.log(`FAIL ${name}: ${detail}`);
      return undefined;
    }
  }

  async function blockedStep(name: string, action: () => Promise<any>) {
    try {
      const value = await action();
      const detail = `${rpcName(value)} (expected METHOD_NOT_IMPL)`;
      results.push({ name, status: 'FAIL', detail });
      console.log(`FAIL ${name}: ${detail}`);
      return undefined;
    } catch (error) {
      const detail = String(error);
      if (!detail.includes('METHOD_NOT_IMPL')) {
        results.push({ name, status: 'FAIL', detail });
        console.log(`FAIL ${name}: ${detail}`);
        return undefined;
      }
      results.push({ name, status: 'BLOCKED', detail });
      console.log(`BLOCKED ${name}: ${detail}`);
      return undefined;
    }
  }

  let uploadedDocument: any;
  let uploadedPhoto: any;
  let mediaMessageId: number | undefined;
  let directMediaMessageId: number | undefined;
  let multipartMediaMessageId: number | undefined;
  let photoMediaMessageId: number | undefined;
  let videoMediaMessageId: number | undefined;
  let avatarPhoto: any;

  async function saveFilePart(stepName: string, name: string, bytes: Uint8Array) {
    const fileId = randomId();
    const saved = await step(stepName, () => alice.client.invoke(new Api.upload.SaveFilePart({
      fileId,
      filePart: 0,
      bytes,
    })));
    return saved ? new Api.InputFile({ id: fileId, parts: 1, name, md5Checksum: '' }) : undefined;
  }

  const documentInput = await saveFilePart('upload.saveFilePart document', 'isolated-media.txt', documentBytes);

  if (documentInput) {
    const mediaResult = await step('messages.uploadMedia document', async () => {
      const result = await alice.client.invoke(new Api.messages.UploadMedia({
        peer: bobPeer,
        media: new Api.InputMediaUploadedDocument({
          file: documentInput,
          mimeType: 'text/plain',
          attributes: [new Api.DocumentAttributeFilename({ fileName: 'isolated-media.txt' })],
        }),
      }));
      requireCondition(result?.document, 'messages.uploadMedia did not return a document');
      return result;
    });
    uploadedDocument = mediaResult?.document;
    if (uploadedDocument) {
      const location = documentLocation(Api, uploadedDocument);
      await step('upload.getFile document bytes', async () => {
        const downloaded = await alice.client.invoke(new Api.upload.GetFile({
          location,
          offset: 0n,
          limit: 1024,
        }));
        requireBytes(downloaded?.bytes, documentBytes, 'upload.getFile document');
        return downloaded;
      });
      await step('upload.getFileHashes document bytes', async () => {
        const hashes = await alice.client.invoke(new Api.upload.GetFileHashes({
          location,
          offset: 0n,
        }));
        requireCondition(Array.isArray(hashes) && hashes.length === 1, 'upload.getFileHashes returned no document hash');
        requireCondition(hashes[0].limit === documentBytes.length, 'upload.getFileHashes returned the wrong document hash range');
        requireCondition(
          Buffer.from(hashes[0].hash).equals(createHash('sha256').update(documentBytes).digest()),
          'upload.getFileHashes returned the wrong document hash',
        );
        return hashes;
      });
    }
  }

  const sentDocument = uploadedDocument && await step('messages.sendMedia document', () => alice.client.invoke(new Api.messages.SendMedia({
    peer: bobPeer,
    media: new Api.InputMediaDocument({
      id: new Api.InputDocument({
        id: uploadedDocument.id,
        accessHash: uploadedDocument.accessHash,
        fileReference: uploadedDocument.fileReference || new Uint8Array(),
      }),
    }),
    message: 'isolated media probe',
    randomId: randomId(),
  })));
  if (sentDocument) {
    const bobAlicePeer = inputPeerUser(Api, await searchUser(Api, bob, alice.username));
    const mediaMessage = await step('messages.getHistory media delivery', async () => {
      const history = await bob.client.invoke(new Api.messages.GetHistory({
        peer: bobAlicePeer,
        offsetId: 0,
        offsetDate: 0,
        addOffset: 0,
        limit: 20,
        maxId: 0,
        minId: 0,
        hash: 0n,
      }));
      const message = messageList(history).find((candidate: any) => candidate?.message === 'isolated media probe');
      requireCondition(message?.media?.document, 'recipient history did not include the sent document');
      return message;
    });
    if (mediaMessage) {
      mediaMessageId = mediaMessage.id;
      cleanup.push(() => alice.client.invoke(new Api.messages.DeleteMessages({ revoke: true, id: [mediaMessageId!] })));
      await step('upload.getFile recipient document bytes', async () => {
        const downloaded = await bob.client.invoke(new Api.upload.GetFile({
          location: documentLocation(Api, mediaMessage.media.document),
          offset: 0n,
          limit: 1024,
        }));
        requireBytes(downloaded?.bytes, documentBytes, 'upload.getFile recipient document');
        return downloaded;
      });
    }
  }

  const unknownDocumentCaption = `isolated unknown document probe ${randomId()}`;
  await step('messages.sendMedia rejects unknown document', async () => {
    try {
      await alice.client.invoke(new Api.messages.SendMedia({
        peer: bobPeer,
        media: new Api.InputMediaDocument({
          id: new Api.InputDocument({
            id: randomId(),
            accessHash: randomId(),
            fileReference: new Uint8Array(),
          }),
        }),
        message: unknownDocumentCaption,
        randomId: randomId(),
      }));
    } catch {
      return true;
    }
    throw new Error('messages.sendMedia accepted an unknown document');
  });
  try {
    const bobAlicePeer = inputPeerUser(Api, await searchUser(Api, bob, alice.username));
    const invalidHistory = await bob.client.invoke(new Api.messages.GetHistory({
      peer: bobAlicePeer,
      offsetId: 0,
      offsetDate: 0,
      addOffset: 0,
      limit: 20,
      maxId: 0,
      minId: 0,
      hash: 0n,
    }));
    const invalidMessage = messageList(invalidHistory).find((candidate: any) => candidate?.message === unknownDocumentCaption);
    if (invalidMessage?.id) {
      cleanup.push(() => alice.client.invoke(new Api.messages.DeleteMessages({ revoke: true, id: [invalidMessage.id] })));
    }
  } catch (error) {
    console.log(`CLEANUP unknown document ${String(error)}`);
  }

  const directDocumentInput = await saveFilePart('upload.saveFilePart direct document', 'isolated-direct-media.txt', directDocumentBytes);
  const sentDirectDocument = directDocumentInput && await step('messages.sendMedia uploaded document', () => alice.client.invoke(new Api.messages.SendMedia({
    peer: bobPeer,
    media: new Api.InputMediaUploadedDocument({
      file: directDocumentInput,
      mimeType: 'text/plain',
      attributes: [new Api.DocumentAttributeFilename({ fileName: 'isolated-direct-media.txt' })],
    }),
    message: 'isolated direct media probe',
    randomId: randomId(),
  })));
  if (sentDirectDocument) {
    const bobAlicePeer = inputPeerUser(Api, await searchUser(Api, bob, alice.username));
    const directMediaMessage = await step('messages.getHistory uploaded media delivery', async () => {
      const history = await bob.client.invoke(new Api.messages.GetHistory({
        peer: bobAlicePeer,
        offsetId: 0,
        offsetDate: 0,
        addOffset: 0,
        limit: 20,
        maxId: 0,
        minId: 0,
        hash: 0n,
      }));
      const message = messageList(history).find((candidate: any) => candidate?.message === 'isolated direct media probe');
      requireCondition(message?.media?.document, 'recipient history did not include the directly uploaded document');
      return message;
    });
    if (directMediaMessage) {
      directMediaMessageId = directMediaMessage.id;
      cleanup.push(() => alice.client.invoke(new Api.messages.DeleteMessages({ revoke: true, id: [directMediaMessageId!] })));
      await step('upload.getFile recipient uploaded document bytes', async () => {
        const downloaded = await bob.client.invoke(new Api.upload.GetFile({
          location: documentLocation(Api, directMediaMessage.media.document),
          offset: 0n,
          limit: 1024,
        }));
        requireBytes(downloaded?.bytes, directDocumentBytes, 'upload.getFile recipient uploaded document');
        return downloaded;
      });
    }
  }

  const multipartFileId = randomId();
  const multipartPartSize = 128 * 1024;
  const multipartFirstPart = await step('upload.saveFilePart multipart 0', () => alice.client.invoke(new Api.upload.SaveFilePart({
    fileId: multipartFileId,
    filePart: 0,
    bytes: multipartDocumentBytes.slice(0, multipartPartSize),
  })));
  const multipartLastPart = await step('upload.saveFilePart multipart 1', () => alice.client.invoke(new Api.upload.SaveFilePart({
    fileId: multipartFileId,
    filePart: 1,
    bytes: multipartDocumentBytes.slice(multipartPartSize),
  })));
  const multipartDocumentInput = multipartFirstPart && multipartLastPart && new Api.InputFile({
    id: multipartFileId,
    parts: 2,
    name: 'isolated-multipart-media.bin',
    md5Checksum: createHash('md5').update(multipartDocumentBytes).digest('hex'),
  });
  const sentMultipartDocument = multipartDocumentInput && await step('messages.sendMedia multipart uploaded document', () => alice.client.invoke(new Api.messages.SendMedia({
    peer: bobPeer,
    media: new Api.InputMediaUploadedDocument({
      file: multipartDocumentInput,
      mimeType: 'application/octet-stream',
      attributes: [new Api.DocumentAttributeFilename({ fileName: 'isolated-multipart-media.bin' })],
    }),
    message: 'isolated multipart media probe',
    randomId: randomId(),
  })));
  if (sentMultipartDocument) {
    const bobAlicePeer = inputPeerUser(Api, await searchUser(Api, bob, alice.username));
    const multipartMediaMessage = await step('messages.getHistory multipart media delivery', async () => {
      const history = await bob.client.invoke(new Api.messages.GetHistory({
        peer: bobAlicePeer,
        offsetId: 0,
        offsetDate: 0,
        addOffset: 0,
        limit: 20,
        maxId: 0,
        minId: 0,
        hash: 0n,
      }));
      const message = messageList(history).find((candidate: any) => candidate?.message === 'isolated multipart media probe');
      requireCondition(message?.media?.document, 'recipient history did not include the multipart document');
      return message;
    });
    if (multipartMediaMessage) {
      multipartMediaMessageId = multipartMediaMessage.id;
      cleanup.push(() => alice.client.invoke(new Api.messages.DeleteMessages({ revoke: true, id: [multipartMediaMessageId!] })));
      await step('upload.getFile recipient multipart document bytes', async () => {
        const downloaded = await bob.client.invoke(new Api.upload.GetFile({
          location: documentLocation(Api, multipartMediaMessage.media.document),
          offset: 0n,
          limit: multipartDocumentBytes.length + 1024,
        }));
        requireBytes(downloaded?.bytes, multipartDocumentBytes, 'upload.getFile recipient multipart document');
        return downloaded;
      });
    }
  }

  const photoMediaInput = await saveFilePart('upload.saveFilePart photo media', 'isolated-photo-media.png', pngBytes);
  const sentPhotoMedia = photoMediaInput && await step('messages.sendMedia uploaded photo', () => alice.client.invoke(new Api.messages.SendMedia({
    peer: bobPeer,
    media: new Api.InputMediaUploadedPhoto({ file: photoMediaInput }),
    message: 'isolated photo media probe',
    randomId: randomId(),
  })));
  if (sentPhotoMedia) {
    const bobAlicePeer = inputPeerUser(Api, await searchUser(Api, bob, alice.username));
    const photoMediaMessage = await step('messages.getHistory photo media delivery', async () => {
      const history = await bob.client.invoke(new Api.messages.GetHistory({
        peer: bobAlicePeer,
        offsetId: 0,
        offsetDate: 0,
        addOffset: 0,
        limit: 20,
        maxId: 0,
        minId: 0,
        hash: 0n,
      }));
      const message = messageList(history).find((candidate: any) => candidate?.message === 'isolated photo media probe');
      requireCondition(message?.media?.photo, 'recipient history did not include the uploaded photo');
      return message;
    });
    if (photoMediaMessage) {
      photoMediaMessageId = photoMediaMessage.id;
      cleanup.push(() => alice.client.invoke(new Api.messages.DeleteMessages({ revoke: true, id: [photoMediaMessageId!] })));
      await step('upload.getFile recipient photo media bytes', async () => {
        const downloaded = await bob.client.invoke(new Api.upload.GetFile({
          location: photoLocation(Api, photoMediaMessage.media.photo),
          offset: 0n,
          limit: 1024,
        }));
        requireCondition(downloaded?.bytes?.length > 0, 'upload.getFile recipient photo returned no bytes');
        return downloaded;
      });
    }
  }

  const videoBytes = new Uint8Array(await readFile(path.join(process.env.TEAMGRAM_GRAMJS_DIR!, 'public/nojs.mp4')));
  const videoMediaInput = await saveFilePart('upload.saveFilePart video media', 'isolated-video-media.mp4', videoBytes);
  const sentVideoMedia = videoMediaInput && await step('messages.sendMedia uploaded video', () => alice.client.invoke(new Api.messages.SendMedia({
    peer: bobPeer,
    media: new Api.InputMediaUploadedDocument({
      file: videoMediaInput,
      mimeType: 'video/mp4',
      attributes: [
        new Api.DocumentAttributeFilename({ fileName: 'isolated-video-media.mp4' }),
        new Api.DocumentAttributeVideo({ duration: 1, w: 128, h: 128, supportsStreaming: true }),
      ],
    }),
    message: 'isolated video media probe',
    randomId: randomId(),
  })));
  if (sentVideoMedia) {
    const bobAlicePeer = inputPeerUser(Api, await searchUser(Api, bob, alice.username));
    const videoMediaMessage = await step('messages.getHistory video media delivery', async () => {
      const history = await bob.client.invoke(new Api.messages.GetHistory({
        peer: bobAlicePeer,
        offsetId: 0,
        offsetDate: 0,
        addOffset: 0,
        limit: 20,
        maxId: 0,
        minId: 0,
        hash: 0n,
      }));
      const message = messageList(history).find((candidate: any) => candidate?.message === 'isolated video media probe');
      requireCondition(message?.media?.document, 'recipient history did not include the uploaded video');
      return message;
    });
    if (videoMediaMessage) {
      videoMediaMessageId = videoMediaMessage.id;
      cleanup.push(() => alice.client.invoke(new Api.messages.DeleteMessages({ revoke: true, id: [videoMediaMessageId!] })));
      await step('upload.getFile recipient video media bytes', async () => {
        const downloaded = await bob.client.invoke(new Api.upload.GetFile({
          location: documentLocation(Api, videoMediaMessage.media.document),
          offset: 0n,
          limit: videoBytes.length + 1024,
        }));
        requireBytes(downloaded?.bytes, videoBytes, 'upload.getFile recipient video');
        return downloaded;
      });
    }
  }

  const photoInput = await saveFilePart('upload.saveFilePart profile photo', 'isolated-avatar.png', pngBytes);
  if (photoInput) {
    const photoResult = await step('photos.uploadProfilePhoto', () => alice.client.invoke(new Api.photos.UploadProfilePhoto({ file: photoInput })));
    avatarPhoto = photoResult?.photo;
    uploadedPhoto = avatarPhoto;
    if (avatarPhoto) {
      await step('photos.getUserPhotos', () => alice.client.invoke(new Api.photos.GetUserPhotos({
        userId: inputUser(Api, alice.me),
        offset: 0,
        maxId: 0n,
        limit: 20,
      })));
      cleanup.push(() => alice.client.invoke(new Api.photos.DeletePhotos({ id: [new Api.InputPhoto({
        id: avatarPhoto.id,
        accessHash: avatarPhoto.accessHash,
        fileReference: avatarPhoto.fileReference || new Uint8Array(),
      })] })));
    }
  }

  await blockedStep('messages.getAllStickers (catalog unavailable)', () => alice.client.invoke(new Api.messages.GetAllStickers({ hash: 0n })));
  await blockedStep('messages.getStickers emoji (catalog unavailable)', () => alice.client.invoke(new Api.messages.GetStickers({ emoticon: '👍', hash: 0n })));
  await blockedStep('messages.getStickerSet animated emoji (catalog unavailable)', () => alice.client.invoke(new Api.messages.GetStickerSet({
    stickerset: new Api.InputStickerSetAnimatedEmoji(),
    hash: 0,
  })));
  await blockedStep('messages.getRecentStickers (provider unavailable)', () => alice.client.invoke(new Api.messages.GetRecentStickers({ hash: 0n })));
  await blockedStep('messages.getFavedStickers (provider unavailable)', () => alice.client.invoke(new Api.messages.GetFavedStickers({ hash: 0n })));
  await blockedStep('messages.getEmojiKeywordsLanguages (catalog unavailable)', () => alice.client.invoke(new Api.messages.GetEmojiKeywordsLanguages({ langCodes: ['en'] })));
  await step('messages.getEmojiKeywordsDifference', () => alice.client.invoke(new Api.messages.GetEmojiKeywordsDifference({ langCode: 'en', fromVersion: 0 })));
  await step('messages.getEmojiGroups', () => alice.client.invoke(new Api.messages.GetEmojiGroups({ hash: 0 })));
  await step('messages.getAvailableReactions', () => alice.client.invoke(new Api.messages.GetAvailableReactions({ hash: 0 })));

  const notifyUsers = new Api.InputNotifyUsers();
  await step('account.getNotifySettings users', () => alice.client.invoke(new Api.account.GetNotifySettings({ peer: notifyUsers })));
  await step('account.updateNotifySettings users', () => alice.client.invoke(new Api.account.UpdateNotifySettings({
    peer: notifyUsers,
    settings: new Api.InputPeerNotifySettings({ showPreviews: true, silent: false, muteUntil: 0 }),
  })));
  await step('account.getNotifyExceptions', () => alice.client.invoke(new Api.account.GetNotifyExceptions({ compareSound: true })));
  await step('account.getContactSignUpNotification', () => alice.client.invoke(new Api.account.GetContactSignUpNotification()));
  await step('account.setContactSignUpNotification', () => alice.client.invoke(new Api.account.SetContactSignUpNotification({ silent: false })));
  await step('account.registerDevice', () => alice.client.invoke(new Api.account.RegisterDevice({
    tokenType: 10,
    token: `isolated-media-probe-${Date.now()}`,
    appSandbox: true,
    secret: new Uint8Array(),
    otherUids: [],
  })));

  for (const action of cleanup.reverse()) {
    try { await action(); } catch (error) { console.log(`CLEANUP ${String(error)}`); }
  }
  if (uploadedPhoto) {
    await step('photos.updateProfilePhoto reset', () => alice.client.invoke(new Api.photos.UpdateProfilePhoto({ id: new Api.InputPhotoEmpty() })));
  }
  await step('account.resetNotifySettings', () => alice.client.invoke(new Api.account.ResetNotifySettings()));
  alice.client.destroy();
  bob.client.destroy();
  console.log(JSON.stringify(results, null, 2));
  if (results.some((result) => result.status === 'FAIL')) {
    throw new Error('media flow probe failed');
  }
}

void run()
  .then(() => process.exit(0))
  .catch((error) => {
    console.error(`probe fatal: ${error instanceof Error && error.stack ? error.stack : String(error)}`);
    process.exit(1);
  });
