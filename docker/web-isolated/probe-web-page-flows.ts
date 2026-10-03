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

function rpcName(value: any) {
  return value?.className || value?.constructor?.name || typeof value;
}

function hasType(value: any, name: string) {
  return rpcName(value).toLowerCase().includes(name.toLowerCase());
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
    connectionRetries: 1,
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
      firstAndLastNames: async () => ['Isolated', 'Web Page'],
      qrCode: async () => {},
      onError: (error: Error) => { throw error; },
      webAuthTokenFailed: () => {},
      onPasskeyOption: () => {},
      initialMethod: 'phoneNumber',
    });
    requireCondition((await client.getMe())?.id !== undefined, `login failed for ${FIXTURE.username}`);
    return client;
  } catch (error) {
    client.destroy();
    throw error;
  }
}

function webPageFromPreview(preview: any) {
  requireCondition(hasType(preview, 'messages.WebPagePreview'), `preview return type = ${rpcName(preview)}`);
  requireCondition(hasType(preview?.media, 'messageMediaWebPage'), `preview media type = ${rpcName(preview?.media)}`);
  requireCondition(hasType(preview?.media?.webpage, 'webPage'), `preview webpage type = ${rpcName(preview?.media?.webpage)}`);
  return preview.media.webpage;
}

function webPageFromResponse(response: any, label: string) {
  requireCondition(hasType(response, 'messages.WebPage'), `${label} return type = ${rpcName(response)}`);
  requireCondition(hasType(response?.webpage, 'webPage'), `${label} webpage type = ${rpcName(response?.webpage)}`);
  return response.webpage;
}

async function expectURLInvalid(action: () => Promise<unknown>) {
  try {
    await action();
  } catch (error) {
    requireCondition(String(error).includes('URL_INVALID'), `private URL returned ${String(error)}, want URL_INVALID`);
    return;
  }
  throw new Error('private URL unexpectedly succeeded');
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
  const client = await login(gram);
  const tag = `web-page-${Date.now()}-${process.pid}`;
  const canonicalURL = `https://example.com/${tag}`;

  try {
    const preview = await client.invoke(new Api.messages.GetWebPagePreview({
      message: `Preview ${canonicalURL}#fragment`,
    }));
    const previewPage = webPageFromPreview(preview);
    requireCondition(previewPage.url === canonicalURL, `preview URL = ${previewPage.url}, want ${canonicalURL}`);
    requireCondition(previewPage.displayUrl === `example.com/${tag}`, `preview display URL = ${previewPage.displayUrl}`);
    requireCondition(previewPage.siteName === 'example.com', `preview site name = ${previewPage.siteName}`);
    requireCondition(previewPage.id !== undefined && String(previewPage.id) !== '0', 'preview webpage lacks an ID');
    requireCondition(Number(previewPage.hash) !== 0, 'preview webpage lacks a hash');
    console.log(`PASS messages.getWebPagePreview URL/type: ${rpcName(preview)} -> ${rpcName(previewPage)}`);

    const full = await client.invoke(new Api.messages.GetWebPage({
      url: `HTTPS://EXAMPLE.COM:443/${tag}#ignored`,
      hash: 0,
    }));
    const fullPage = webPageFromResponse(full, 'messages.getWebPage full');
    requireCondition(fullPage.url === canonicalURL, `getWebPage URL = ${fullPage.url}, want ${canonicalURL}`);
    requireCondition(String(fullPage.id) === String(previewPage.id), 'getWebPage ID does not match preview');
    requireCondition(Number(fullPage.hash) === Number(previewPage.hash), 'getWebPage hash does not match preview');
    console.log(`PASS messages.getWebPage URL/type: ${rpcName(full)} -> ${rpcName(fullPage)}`);

    const notModified = await client.invoke(new Api.messages.GetWebPage({
      url: canonicalURL,
      hash: Number(previewPage.hash),
    }));
    const notModifiedPage = webPageFromResponse(notModified, 'messages.getWebPage not-modified');
    requireCondition(hasType(notModifiedPage, 'webPageNotModified'), `hash response type = ${rpcName(notModifiedPage)}`);
    console.log(`PASS messages.getWebPage hash/notModified: ${rpcName(notModifiedPage)}`);

    await expectURLInvalid(() => client.invoke(new Api.messages.GetWebPage({
      url: 'http://127.0.0.1/private',
      hash: 0,
    })));
    console.log('PASS messages.getWebPage rejects private URL: URL_INVALID');

    const privatePreview = await client.invoke(new Api.messages.GetWebPagePreview({
      message: 'Private http://localhost/private',
    }));
    requireCondition(hasType(privatePreview, 'messages.WebPagePreview'), `private preview return type = ${rpcName(privatePreview)}`);
    requireCondition(hasType(privatePreview?.media, 'messageMediaEmpty'), `private preview media type = ${rpcName(privatePreview?.media)}`);
    console.log(`PASS messages.getWebPagePreview rejects private URL: ${rpcName(privatePreview?.media)}`);
  } finally {
    client.destroy();
  }
}

void run()
  .then(() => process.exit(0))
  .catch((error) => {
    console.error(error instanceof Error && error.stack ? error.stack : String(error));
    process.exit(1);
  });
