import { createRequire } from 'node:module';
import { execFileSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import path from 'node:path';

for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) {
  delete process.env[key];
}

for (const key of ['TEAMGRAM_GRAMJS_DIR', 'A_AUTH_B64', 'B_AUTH_B64', 'A_USER_ID', 'B_USER_ID']) {
  if (!process.env[key]) throw new Error(`${key} is required`);
}

const gramjsDir = process.env.TEAMGRAM_GRAMJS_DIR!;
const dcId = Number(process.env.TG_DC_ID || '2');
const dcHost = process.env.TG_DC_HOST || '127.0.0.1';
const dcPort = Number(process.env.TG_DC_PORT || '11443');
const userId = BigInt(process.env.A_USER_ID!);
const botId = BigInt(process.env.B_USER_ID!);
const botAccessHash = BigInt(process.env.BOT_ACCESS_HASH || '910000000001');
const userAccessHash = BigInt(process.env.USER_ACCESS_HASH || '900000000000000005');

const globalAny = globalThis as typeof globalThis & { self?: any; addEventListener?: () => void };
globalAny.self ??= globalThis;
globalAny.addEventListener ??= () => {};
globalAny.self.addEventListener ??= globalAny.addEventListener;

const requireModule = createRequire(import.meta.url);
const TelegramClient = requireModule(path.join(gramjsDir, 'src/lib/gramjs/client/TelegramClient.ts')).default;
const CallbackSession = requireModule(path.join(gramjsDir, 'src/lib/gramjs/sessions/CallbackSession.ts')).default;
const Api = requireModule(path.join(gramjsDir, 'src/lib/gramjs/tl/index.ts')).Api;
const serializeBytes = requireModule(path.join(gramjsDir, 'src/lib/gramjs/tl/index.ts')).serializeBytes;
const Connection = requireModule(path.join(gramjsDir, 'src/lib/gramjs/network/connection/TCPObfuscated.ts')).ConnectionTCPObfuscated;
const quietLogger = { debug() {}, info() {}, warn() {}, error() {} };

function rpcName(value: any): string {
  return value?.className || value?.constructor?.name || typeof value;
}

function makeClient(authB64: string): any {
  const keyHex = Buffer.from(authB64, 'base64').toString('hex');
  const session = new CallbackSession({ mainDcId: dcId, keys: { [dcId]: keyHex } }, () => {});
  const load = session.load.bind(session);
  session.load = async () => {
    await load();
    (session as any)._serverAddress = dcHost;
    (session as any)._port = dcPort;
  };
  const client = new TelegramClient(session, 1, '0123456789abcdef0123456789abcdef', {
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

function concatBytes(...parts: Uint8Array[]): Uint8Array {
  return Uint8Array.from(Buffer.concat(parts.map((part) => Buffer.from(part))));
}

function tlInt(value: number): Uint8Array {
  const bytes = Buffer.alloc(4);
  bytes.writeUInt32LE(value >>> 0);
  return bytes;
}

function tlLong(value: bigint): Uint8Array {
  const bytes = Buffer.alloc(8);
  bytes.writeBigInt64LE(BigInt.asIntN(64, value));
  return bytes;
}

function tlString(value: string): Uint8Array {
  return Uint8Array.from(serializeBytes(value));
}

class RawRequest {
  SUBCLASS_OF_ID = 0;
  classType = 'request';

  constructor(
    public readonly CONSTRUCTOR_ID: number,
    private readonly payload: Uint8Array,
    public readonly className: string,
  ) {}

  getBytes(): Uint8Array {
    return concatBytes(tlInt(this.CONSTRUCTOR_ID), this.payload);
  }

  readResult(reader: any): any {
    return reader.tgReadObject();
  }
}

function inputBot(): any {
  return new Api.InputUser({ userId: botId, accessHash: botAccessHash });
}

function inputPeerUser(userId: bigint, accessHash: bigint): any {
  return new Api.InputPeerUser({ userId, accessHash });
}

function selfPeer(): any {
  return new Api.InputPeerSelf();
}

function inputInlineMessageID(): any {
  return new Api.InputBotInlineMessageID({ dcId, id: 9001n, accessHash: 9002n });
}

function inputInlineResult(): any {
  return new Api.InputBotInlineResult({
    id: 'gap-result',
    type: 'article',
    sendMessage: new Api.InputBotInlineMessageText({ message: 'gap' }),
  });
}

function rawEditInlineBotMessage(): RawRequest {
  const flags = 1 << 11;
  return new RawRequest(
    0xa423bb51,
    concatBytes(tlInt(flags), inputInlineMessageID().getBytes(), tlString('edited by gap probe')),
    'messages.editInlineBotMessage',
  );
}

function rawSetBotShippingResults(queryId: bigint): RawRequest {
  return new RawRequest(0xe5f672fa, concatBytes(tlInt(0), tlLong(queryId)), 'messages.setBotShippingResults');
}

function rawSetBotPrecheckoutResults(queryId: bigint): RawRequest {
  return new RawRequest(0x09c2dd95, concatBytes(tlInt(1 << 1), tlLong(queryId)), 'messages.setBotPrecheckoutResults');
}

type PaymentProbeSeed = {
  shippingQueryId: bigint;
  precheckoutQueryId: bigint;
  botUserId: bigint;
  buyerUserId: bigint;
  payloadFingerprints: string[];
};

type BotOperationSeed = {
  operation: string;
  userId: bigint;
  payloadFingerprint: string;
};

function sqlLiteral(value: string): string {
  return `'${value.replaceAll("'", "''")}'`;
}

function postgres(query: string): string {
  const container = process.env.TEAMGRAM_POSTGRES_CONTAINER || 'teamgram-postgres';
  return execFileSync('docker', [
    'exec', container, 'psql', '-v', 'ON_ERROR_STOP=1', '-At', '-q', '-U', 'teamgram', '-d', 'teamgram', '-c', query,
  ], { encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] }).trim();
}

function seedPaymentQueries(botUserId: bigint, buyerUserId: bigint): PaymentProbeSeed {
  const tag = `bot-message-gap-${Date.now()}-${process.pid}-${Math.floor(Math.random() * 1_000_000)}`;
  const shippingQueryId = BigInt(Date.now()) * 1000n + BigInt(Math.floor(Math.random() * 1000));
  const precheckoutQueryId = shippingQueryId + 1n;
  const shippingPayload = JSON.stringify({ probe_tag: tag, kind: 'shipping', invoice: 'gap-probe' });
  const precheckoutPayload = JSON.stringify({ probe_tag: tag, kind: 'precheckout', invoice: 'gap-probe' });
  const shippingFingerprint = createHash('sha256').update(shippingPayload).digest('hex');
  const precheckoutFingerprint = createHash('sha256').update(precheckoutPayload).digest('hex');
  postgres(`
    BEGIN;
    INSERT INTO apifull_bot_payment_query
      (query_id, bot_user_id, buyer_user_id, stage, payload, currency, amount, shipping_options, expires_at)
    VALUES
      (${shippingQueryId}, ${botUserId}, ${buyerUserId}, 'shipping_pending',
       convert_to(${sqlLiteral(shippingPayload)}, 'UTF8'), 'USD', 1299, '[]'::jsonb,
       CURRENT_TIMESTAMP + INTERVAL '10 minutes'),
      (${precheckoutQueryId}, ${botUserId}, ${buyerUserId}, 'precheckout_pending',
       convert_to(${sqlLiteral(precheckoutPayload)}, 'UTF8'), 'USD', 1299, '[]'::jsonb,
       CURRENT_TIMESTAMP + INTERVAL '10 minutes');
    COMMIT;
  `);
  return {
    shippingQueryId,
    precheckoutQueryId,
    botUserId,
    buyerUserId,
    payloadFingerprints: [shippingFingerprint, precheckoutFingerprint],
  };
}

function seedBotOperations(userCallerId: bigint, botUserId: bigint): BotOperationSeed[] {
  // callBusinessProvider marshals map keys in lexical order. Keep the object
  // insertion order aligned with encoding/json so the fixture fingerprint is
  // the exact bytes sent by APIFull.
  const bot = {
    predicate_name: 'inputUser',
    constructor: -233744186,
    user_id: Number(botUserId),
    access_hash: Number(botAccessHash),
  };
  const params = { predicate_name: 'dataJSON', constructor: 2104790276, data: '{}' };
  const paramsJSON = JSON.stringify(params);
  const webViewScope = `bot:${botUserId}:method:gapStatus:sha256:${createHash('sha256').update(paramsJSON).digest('hex')}`;
  const inlineMessageID = {
    predicate_name: 'inputBotInlineMessageID',
    constructor: -1995686519,
    dc_id: dcId,
    id_INT64: 9001,
    access_hash: 9002,
  };
  const inlineResult = {
    predicate_name: 'inputBotInlineResult',
    constructor: -2000710887,
    id: 'gap-result',
    type: 'article',
    send_message: {
      predicate_name: 'inputBotInlineMessageText',
      constructor: 1036876423,
      message: 'gap',
    },
  };
  const rows: Array<{ operation: string; userId: bigint; payload: string; result: string }> = [
    {
      operation: 'bots.checkDownloadFileParams',
      userId: userCallerId,
      payload: JSON.stringify({ bot, file_name: 'gap.txt', url: 'https://mini.example/gap.txt' }),
      result: JSON.stringify({ value: true }),
    },
    {
      operation: 'bots.getBotRecommendations',
      userId: userCallerId,
      payload: JSON.stringify({ bot_id: Number(botUserId), caller_user_id: Number(userCallerId) }),
      result: JSON.stringify({ users: [], predicate_name: 'users_users' }),
    },
    {
      operation: 'messages.getPaidReactionPrivacy',
      userId: userCallerId,
      payload: JSON.stringify({}),
      result: JSON.stringify({ chats: [], date: 1, predicate_name: 'updates', seq: 0, updates: [], users: [] }),
    },
    {
      operation: 'messages.togglePaidReactionPrivacy',
      userId: userCallerId,
      payload: JSON.stringify({
        enabled: null,
        msg_id: 3,
        peer: {
          predicate_name: 'inputPeerUser',
          constructor: -571955892,
          user_id: Number(botUserId),
          access_hash: Number(botAccessHash),
        },
        privacy: { predicate_name: 'paidReactionPrivacyAnonymous', constructor: 520887001 },
      }),
      result: JSON.stringify({ value: true }),
    },
    {
      operation: 'bots.getPopularAppBots',
      userId: userCallerId,
      payload: JSON.stringify({ limit: 20, offset: '' }),
      result: JSON.stringify({ users: [], predicate_name: 'bots_popularAppBots' }),
    },
    {
      operation: 'bots.invokeWebViewCustomMethod',
      userId: userCallerId,
      payload: JSON.stringify({ bot, custom_method: 'gapStatus', params, scope_key: webViewScope }),
      result: JSON.stringify({ data: '{"ok":true}', constructor: 2104790276, predicate_name: 'dataJSON' }),
    },
    {
      operation: 'messages.requestChatJoinWebView',
      userId: userCallerId,
      payload: JSON.stringify({ platform: 'web', query_id: 991001, scope_key: 'query:991001', theme_params: null }),
      result: JSON.stringify({ url: 'https://mini.example/join', predicate_name: 'webViewResultUrl' }),
    },
    {
      operation: 'messages.editInlineBotMessage',
      userId: userCallerId,
      payload: JSON.stringify({
        entities: null,
        id: inlineMessageID,
        invert_media: false,
        media: null,
        message: { value: 'edited by gap probe' },
        no_webpage: false,
        reply_markup: null,
        rich_message: null,
      }),
      result: JSON.stringify({ value: true }),
    },
    {
      operation: 'messages.setBotGuestChatResult',
      userId: userCallerId,
      payload: JSON.stringify({ query_id: 991002, result: inlineResult }),
      result: JSON.stringify({ dc_id: 2, id_INT64: 9001, access_hash: 9002, predicate_name: 'inputBotInlineMessageID' }),
    },
    {
      operation: 'bots.sendCustomRequest',
      userId: botUserId,
      payload: JSON.stringify({ custom_method: 'gapStatus', params }),
      result: JSON.stringify({ data: '{"ok":true}', constructor: 2104790276, predicate_name: 'dataJSON' }),
    },
    {
      operation: 'bots.answerWebhookJSONQuery',
      userId: botUserId,
      payload: JSON.stringify({ data: { predicate_name: 'dataJSON', constructor: 2104790276, data: '{"ok":true}' }, query_id: 991003 }),
      result: JSON.stringify({ value: true }),
    },
    {
      operation: 'bots.setCustomVerification',
      userId: botUserId,
      payload: JSON.stringify({
        bot: null,
        custom_description: { value: 'gap verifier' },
        enabled: true,
        peer: { predicate_name: 'inputPeerSelf', constructor: 2107670217 },
      }),
      result: JSON.stringify({ value: true }),
    },
    {
      operation: 'bots.setJoinChatResults',
      userId: botUserId,
      payload: JSON.stringify({ query_id: 991004, result: { predicate_name: 'joinChatBotResultApproved', constructor: -1374344599 } }),
      result: JSON.stringify({ value: true }),
    },
    {
      operation: 'bots.updateStarRefProgram',
      userId: botUserId,
      payload: JSON.stringify({ bot, commission_permille: 125, duration_months: { value: 3 } }),
      result: JSON.stringify({
        predicate_name: 'starRefProgram',
        constructor: -586389774,
        bot_id: Number(botUserId),
        commission_permille: 125,
        duration_months: { value: 3 },
      }),
    },
  ];
  const values = rows.map((row) => `(
    ${sqlLiteral(row.operation)},
    ${row.userId},
    digest(convert_to(${sqlLiteral(row.payload)}, 'UTF8'), 'sha256'),
    ${sqlLiteral(row.result)}::jsonb,
    CURRENT_TIMESTAMP + INTERVAL '10 minutes'
  )`).join(',');
  postgres(`
    INSERT INTO teamgram_business_bot_operation
      (operation, user_id, request_fingerprint, result, expires_at)
    VALUES ${values}
    ON CONFLICT (operation, user_id, request_fingerprint) DO UPDATE
      SET result = EXCLUDED.result,
          expires_at = EXCLUDED.expires_at,
          updated_at = CURRENT_TIMESTAMP;
  `);
  return rows.map((row) => ({
    operation: row.operation,
    userId: row.userId,
    payloadFingerprint: createHash('sha256').update(row.payload).digest('hex'),
  }));
}

function cleanupBotOperations(seed: BotOperationSeed[]): number {
  if (seed.length === 0) return 0;
  const clauses = seed.map((row) => `(${sqlLiteral(row.operation)}, ${row.userId}, ${sqlLiteral(row.payloadFingerprint)})`).join(', ');
  return Number(postgres(`
    WITH deleted AS (
      DELETE FROM teamgram_business_bot_operation AS operation
       WHERE (operation.operation, operation.user_id, encode(operation.request_fingerprint, 'hex')) IN (${clauses})
      RETURNING operation.operation
    )
    SELECT count(*) FROM deleted;
  `));
}

function cleanupPaymentQueries(seed: PaymentProbeSeed): { queriesDeleted: number; outboxRemaining: number } {
  const fingerprintList = seed.payloadFingerprints.map(sqlLiteral).join(', ');
  const deleted = Number(postgres(`
    WITH deleted AS (
      DELETE FROM apifull_bot_payment_query
       WHERE bot_user_id = ${seed.botUserId}
         AND buyer_user_id = ${seed.buyerUserId}
         AND encode(digest(payload, 'sha256'), 'hex') IN (${fingerprintList})
      RETURNING query_id
    )
    SELECT count(*) FROM deleted;
  `));
  const outboxRemaining = Number(postgres(`
    SELECT count(*)
      FROM apifull_bot_payment_outbox
     WHERE query_id IN (${seed.shippingQueryId}, ${seed.precheckoutQueryId});
  `));
  if (outboxRemaining !== 0) {
    throw new Error(`payment probe cleanup left ${outboxRemaining} outbox rows`);
  }
  return { queriesDeleted: deleted, outboxRemaining };
}

function rawSetBotGuestChatResult(queryId: bigint): RawRequest {
  return new RawRequest(0xb8f106e3, concatBytes(tlLong(queryId), inputInlineResult().getBytes()), 'messages.setBotGuestChatResult');
}

function rawTogglePaidReactionPrivacy(): RawRequest {
  return new RawRequest(
    0x435885b5,
    concatBytes(inputPeerUser(botId, botAccessHash).getBytes(), tlInt(3), new Api.PaidReactionPrivacyAnonymous().getBytes()),
    'messages.togglePaidReactionPrivacy',
  );
}

function rawSendCustomRequest(): RawRequest {
  return new RawRequest(0xaa2769ed, concatBytes(tlString('gapStatus'), new Api.DataJSON({ data: '{}' }).getBytes()), 'bots.sendCustomRequest');
}

function rawAnswerWebhookJSONQuery(queryId: bigint): RawRequest {
  return new RawRequest(0xe6213f4d, concatBytes(tlLong(queryId), new Api.DataJSON({ data: '{"ok":true}' }).getBytes()), 'bots.answerWebhookJSONQuery');
}

function rawUpdateStarRefProgram(): RawRequest {
  return new RawRequest(0x778b5ab3, concatBytes(tlInt(1), inputBot().getBytes(), tlInt(125), tlInt(3)), 'bots.updateStarRefProgram');
}

function rawSetCustomVerification(): RawRequest {
  return new RawRequest(0x8b89dfbd, concatBytes(tlInt((1 << 1) | (1 << 2)), selfPeer().getBytes(), tlString('gap verifier')), 'bots.setCustomVerification');
}

function rawSetJoinChatResults(queryId: bigint): RawRequest {
  return new RawRequest(0xe71a4810, concatBytes(tlLong(queryId), new Api.JoinChatBotResultApproved().getBytes()), 'bots.setJoinChatResults');
}

async function invoke(client: any, name: string, request: any, results: Record<string, unknown>): Promise<void> {
  try {
    const result = await client.invoke(request);
    results[name] = {
      constructor: rpcName(result),
      value: result?.value,
      url: result?.url,
      queryId: result?.queryId?.toString?.(),
      date: result?.date,
      seq: result?.seq,
    };
  } catch (error) {
    results[name] = { error: error instanceof Error ? error.message : String(error) };
  }
}

async function main() {
  const userClient = makeClient(process.env.A_AUTH_B64!);
  const botClient = makeClient(process.env.B_AUTH_B64!);
  const results: Record<string, unknown> = {};
  let paymentSeed: PaymentProbeSeed | undefined;
  let botOperationsSeed: BotOperationSeed[] | undefined;
  let paymentCleanup: { queriesDeleted: number; outboxRemaining: number } | undefined;
  let paymentCleanupError: string | undefined;
  let botOperationsCleanup: number | undefined;
  let botOperationsCleanupError: string | undefined;
  try {
    paymentSeed = seedPaymentQueries(botId, userId);
    botOperationsSeed = seedBotOperations(userId, botId);
    await userClient.connect();
    const user = await userClient.getMe();
    if (String(user?.id) !== userId.toString()) throw new Error(`user auth mismatch: ${String(user?.id)}`);

    await invoke(userClient, 'bots.checkDownloadFileParams', new Api.bots.CheckDownloadFileParams({ bot: inputBot(), fileName: 'gap.txt', url: 'https://mini.example/gap.txt' }), results);
    await invoke(userClient, 'bots.getBotRecommendations', new Api.bots.GetBotRecommendations({ bot: inputBot() }), results);
    await invoke(userClient, 'messages.getPaidReactionPrivacy', new Api.messages.GetPaidReactionPrivacy(), results);
    await invoke(userClient, 'messages.togglePaidReactionPrivacy', rawTogglePaidReactionPrivacy(), results);
    await invoke(userClient, 'bots.getPopularAppBots', new Api.bots.GetPopularAppBots({ offset: '', limit: 20 }), results);
    await invoke(userClient, 'bots.invokeWebViewCustomMethod', new Api.bots.InvokeWebViewCustomMethod({ bot: inputBot(), customMethod: 'gapStatus', params: new Api.DataJSON({ data: '{}' }) }), results);
    await invoke(userClient, 'messages.requestChatJoinWebView', new Api.messages.RequestChatJoinWebView({ queryId: 991001n, platform: 'web' }), results);
    await invoke(userClient, 'messages.editInlineBotMessage', rawEditInlineBotMessage(), results);
    await invoke(userClient, 'messages.setBotGuestChatResult', rawSetBotGuestChatResult(991002n), results);

    await botClient.connect();
    const bot = await botClient.getMe();
    if (String(bot?.id) !== botId.toString()) throw new Error(`bot auth mismatch: ${String(bot?.id)}`);
    await invoke(botClient, 'bots.sendCustomRequest', rawSendCustomRequest(), results);
    await invoke(botClient, 'bots.answerWebhookJSONQuery', rawAnswerWebhookJSONQuery(991003n), results);
    await invoke(botClient, 'bots.setCustomVerification', rawSetCustomVerification(), results);
    await invoke(botClient, 'bots.setJoinChatResults', rawSetJoinChatResults(991004n), results);
    await invoke(botClient, 'bots.updateStarRefProgram', rawUpdateStarRefProgram(), results);
    await invoke(botClient, 'messages.setBotShippingResults', rawSetBotShippingResults(paymentSeed.shippingQueryId), results);
    await invoke(botClient, 'messages.setBotPrecheckoutResults', rawSetBotPrecheckoutResults(paymentSeed.precheckoutQueryId), results);
  } finally {
    // Disconnect leaves the completed sender queue intact and avoids GramJS's
    // destroy-before-send-loop race. The bounded process exit below closes any
    // remaining runtime handles after database cleanup has completed.
    userClient.disconnect();
    botClient.disconnect();
    if (botOperationsSeed) {
      try {
        botOperationsCleanup = cleanupBotOperations(botOperationsSeed);
      } catch (error) {
        botOperationsCleanupError = error instanceof Error ? error.message : String(error);
      }
    }
    if (paymentSeed) {
      try {
        paymentCleanup = cleanupPaymentQueries(paymentSeed);
      } catch (error) {
        paymentCleanupError = error instanceof Error ? error.message : String(error);
      }
    }
  }
  console.log(JSON.stringify({
    transport: `DC${dcId} TCPObfuscated -> gateway -> session -> APIFull/provider/PostgreSQL`,
    userId: userId.toString(),
    botId: botId.toString(),
    paymentSeed: paymentSeed ? {
      shippingQueryId: paymentSeed.shippingQueryId.toString(),
      precheckoutQueryId: paymentSeed.precheckoutQueryId.toString(),
      cleanup: paymentCleanup,
      cleanupError: paymentCleanupError,
    } : undefined,
    botOperationsSeed: botOperationsSeed ? {
      operations: botOperationsSeed.length,
      cleanup: botOperationsCleanup,
      cleanupError: botOperationsCleanupError,
    } : undefined,
    results,
  }));
  const exitCode = Object.values(results).some((value: any) => value?.error) || paymentCleanupError || botOperationsCleanupError ? 1 : 0;
  process.exitCode = exitCode;
  // GramJS can leave a sender loop alive briefly after destroy(); the probe has
  // already completed all RPC and cleanup work, so bound process lifetime.
  setTimeout(() => process.exit(exitCode), 250);
}

main().catch((error) => {
  console.error(`bot/message gap probe failed: ${error instanceof Error ? error.stack || error.message : String(error)}`);
  process.exitCode = 1;
  setTimeout(() => process.exit(1), 250);
});
