import path from 'node:path';
import { createRequire } from 'node:module';

const required = ['TEAMGRAM_GRAMJS_DIR', 'AUTH_B64', 'USER_ID', 'PREMIUM_INVOICE_SLUG'];
for (const key of required) {
  if (!process.env[key]) throw new Error(`${key} is required`);
}

const gramjsDir = process.env.TEAMGRAM_GRAMJS_DIR!;
const dcId = Number(process.env.TG_DC_ID || '2');
const dcHost = process.env.TG_DC_HOST || '127.0.0.1';
const dcPort = Number(process.env.TG_DC_PORT || '11443');
const apiId = Number(process.env.TG_API_ID || '1');
const apiHash = process.env.TG_API_HASH || '0123456789abcdef0123456789abcdef';
const userId = String(process.env.USER_ID);
const invoiceSlug = process.env.PREMIUM_INVOICE_SLUG!;

const globalAny = globalThis as typeof globalThis & { self?: any; addEventListener?: () => void };
globalAny.self ??= globalThis;
globalAny.addEventListener ??= () => {};
globalAny.self.addEventListener ??= globalAny.addEventListener;

const requireModule = createRequire(import.meta.url);
const TelegramClient = requireModule(path.join(gramjsDir, 'src/lib/gramjs/client/TelegramClient.ts')).default;
const CallbackSession = requireModule(path.join(gramjsDir, 'src/lib/gramjs/sessions/CallbackSession.ts')).default;
const Api = requireModule(path.join(gramjsDir, 'src/lib/gramjs/tl/index.ts')).Api;
const ConnectionTCPObfuscated = requireModule(
  path.join(gramjsDir, 'src/lib/gramjs/network/connection/TCPObfuscated.ts'),
).ConnectionTCPObfuscated;
const quietLogger = { debug() {}, info() {}, warn() {}, error() {} };

function typeName(value: any): string {
  return value?.className || value?.constructor?.name || typeof value;
}

function makeClient(authKeyBase64: string): any {
  const authKeyHex = Buffer.from(authKeyBase64, 'base64').toString('hex');
  const session = new CallbackSession({ mainDcId: dcId, keys: { [dcId]: authKeyHex } }, () => {});
  const load = session.load.bind(session);
  session.load = async () => {
    await load();
    (session as any)._serverAddress = dcHost;
    (session as any)._port = dcPort;
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
  (client as any)._borrowExportedSender = async () => undefined;
  (client as any)._updateLoop = async () => undefined;
  return client;
}

function assertType(value: any, ...names: string[]): void {
  if (!names.includes(typeName(value))) {
    throw new Error(`expected ${names.join(' or ')}, got ${typeName(value)}`);
  }
}

async function run() {
  const client = makeClient(process.env.AUTH_B64!);
  try {
    await client.connect();
    const me = await client.getMe();
    if (String(me?.id) !== userId) throw new Error('the authenticated account does not match USER_ID');

    const promo = await client.invoke(new Api.help.GetPremiumPromo({}));
    assertType(promo, 'help.PremiumPromo', 'HelpPremiumPromo');
    if (!Array.isArray(promo.periodOptions) || promo.periodOptions.length === 0) {
      throw new Error('help.getPremiumPromo returned no subscription options');
    }

    const purpose = new Api.InputStorePaymentPremiumSubscription({});
    const canPurchase = await client.invoke(new Api.payments.CanPurchaseStore({ purpose }));
    assertType(canPurchase, 'BoolTrue', 'BoolFalse', 'boolTrue', 'boolFalse', 'Boolean');

    const invoice = new Api.InputInvoiceSlug({ slug: invoiceSlug });
    const form = await client.invoke(new Api.payments.GetPaymentForm({ invoice }));
    assertType(form, 'payments.PaymentForm', 'PaymentForm');
    if (BigInt(form.formId) <= 0n) throw new Error('payments.getPaymentForm returned an invalid form id');

    const info = new Api.PaymentRequestedInfo({});
    const validated = await client.invoke(new Api.payments.ValidateRequestedInfo({
      invoice,
      info,
    }));
    assertType(validated, 'payments.ValidatedRequestedInfo', 'ValidatedRequestedInfo');

    const credentials = new Api.InputPaymentCredentials({
      data: new Api.DataJSON({ data: JSON.stringify({ provider: 'local-fixture' }) }),
    });
    const request = new Api.payments.SendPaymentForm({
      formId: form.formId,
      invoice,
      requestedInfoId: validated.id,
      credentials,
    });
    const first = await client.invoke(request);
    assertType(first, 'payments.PaymentResult', 'PaymentResult');
    const replay = await client.invoke(request);
    assertType(replay, 'payments.PaymentResult', 'PaymentResult');

    console.log(JSON.stringify({
      userId,
      promoType: typeName(promo),
      periodMonths: promo.periodOptions.map((option: any) => Number(option.months)),
      canPurchaseStore: typeName(canPurchase),
      formType: typeName(form),
      formId: String(form.formId),
      validatedType: typeName(validated),
      firstResult: typeName(first),
      replayResult: typeName(replay),
    }));
  } finally {
    client.destroy();
  }
}

run().then(() => process.exit(0)).catch((error) => {
  console.error(`premium purchase probe failed: ${error instanceof Error ? error.stack || error.message : String(error)}`);
  process.exit(1);
});
