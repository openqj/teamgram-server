# Payment Provider

APIFull sends payment operations to `PaymentProviderEndpoint`. Stars top-up form discovery uses `operation: "get_stars_form"`; settlement uses `operation: "settle_stars"`. Both include the authenticated user and an active catalog offer. The provider must not choose a different offer or beneficiary. Form discovery returns a signed `form` with predicate `payments_paymentFormStars` and the matching `offer_id`, `stars`, `store_product`, `currency`, and `amount`; APIFull checks known catalog terms and requires a non-zero form ID.

The isolated payment-provider fixture also exposes signed provider metadata through `GET /providers` and the authenticated `operation: "get_payment_providers"`. Its deterministic test catalog contains Stripe (card), PayPal (wallet), and Alipay (wallet). `get_stars_form` and `settle_stars` accept an optional fixture-only `provider_id` and echo the selected test channel in the signed JSON response and receipt. This metadata is outside the Telegram `payments.paymentFormStars` constructor; the standard Layer 229 form and `payments.sendStarsForm` wire contract remain unchanged. Production APIFull still uses its configured `PaymentProviderEndpoint` and `PaymentProviderName` as the settlement authority.

Each request includes `X-Teamgram-Payment-Provider-Request-Signature: sha256=<digest>`, an HMAC-SHA256 digest of the exact JSON request body using `PaymentProviderSigningKey`. When `PaymentProviderKey` is configured, APIFull also sends `Authorization: Bearer <key>`. The request includes an `Idempotency-Key` derived from the user and request key. Retries for that key must return the same transaction and response body. APIFull also serializes requests for the same user and request key locally.

Migration `113_apifull_stars_rmb_pricing` publishes the canonical CNY top-up catalog. It starts at `org.telegram.telegramStars.topup.x1`; every offer represents one yuan per Star and stores `amount = stars * 100` in Telegram's currency minor units. When `TEAMGRAM_SUB2API_INTERNAL_BASE_URL`, `TEAMGRAM_SUB2API_SERVICE_TOKEN`, `TEAMGRAM_IDENTITY_BASE_URL`, `TEAMGRAM_IDENTITY_TEAMGRAM_SERVICE_TOKEN`, and `TEAMGRAM_IDENTITY_INSTANCE_ID` are configured, a settled top-up is also added to the mapped Sub2API account. The bridge sends the Stars count as the balance amount and uses a transaction-derived idempotency key, so client retries do not double-credit Sub2API.

The local acceptance fixture at `docker/production/payment-provider-fixture` implements signed Premium and bot-invoice form/validation/settlement responses, deterministic Premium gift-code options, and deterministic `get_stars_form` / `settle_stars` responses. Its Stars form mirrors the submitted active offer, and settlement responses are cached by operation plus `request_key` so a retry cannot mint a second transaction or cross product boundaries. The PostgreSQL control plane canonicalizes stored JSON so a provider restart returns the same response bytes and signature. Form and validation responses may include product metadata; only Premium and Stars settlements create provider entitlement rows. Bot invoice settlements retain their receipt and transaction without creating an account entitlement. The fixture is test-only and does not provide production authority for revenue, paid messages, gift-code settlement, or giveaways. Those operations remain external provider dependencies and must fail closed when no signed provider is configured.

The settlement provider response is JSON and must include:

```json
{
  "verified": true,
  "user_id": 123,
  "beneficiary_user_id": 123,
  "request_key": "stars:123:456:...",
  "fingerprint": "...",
  "form_id": 456,
  "product_type": "stars_topup",
  "offer_id": 7,
  "stars": 100,
  "store_product": "",
  "transaction_id": "provider-transaction-id",
  "currency": "USD",
  "amount": 199,
  "title": "Stars top-up",
  "receipt": {"provider": "..."}
}
```

The provider must sign the exact response body with HMAC-SHA256 using `PaymentProviderSigningKey` and return the lowercase hexadecimal digest in `X-Teamgram-Payment-Signature` as `sha256=<digest>`. APIFull rejects mismatched user, beneficiary, form, request key, fingerprint, product, offer, Stars quantity, known price terms, missing transaction ID, or invalid signature. Form-discovery responses use the same HMAC header and must return a typed `payments.paymentFormStars` with the matching offer metadata.

On acceptance, APIFull atomically writes the payment request, receipt, payment ledger entry, Stars transaction, and balance update. A duplicate request returns the settled result without another provider call or Stars credit. Missing endpoint or signing key fails closed. Production settlement requires the provider endpoint, credentials, matching active catalog offers, and a real transaction acceptance run.

The isolated provider-fixture seed can model a provider-confirmed Stars credit
for a bot that must spend Stars in a Bot API probe. Set `STARS_CREDITS_JSON` to
an array such as
`[{"user_id":900000000001,"stars":1000,"transaction_id":"fixture-stars-credit-1"}]`.
The seed writes one idempotent `payment:credit:<transaction_id>` ledger row per
user and transaction, then increases the local Stars balance. It is fixture
data only; production balances must come from the configured payment provider.

Stars refunds use `operation: "refund_stars_charge"` and carry the authenticated caller, target user, charge ID, original provider and transaction ID, and credited Stars amount. The provider response must be signed and include `verified`, matching caller/target/charge/provider/Stars fields, a non-empty `refund_transaction_id`, and a non-empty receipt. APIFull commits the refund and debits the Stars ledger in one PostgreSQL transaction; a committed replay returns `updates` without another provider call. `docker/production/probe-stars-refund-provider.ts` exercises this contract against the isolated fixture. Store receipt assignment probes only verify the signed Layer 229 assignment envelope and replay behavior; Apple App Store Server API, Google Play Developer API, receipt revocation, and store refunds remain external provider responsibilities and require production credentials.

Read-only catalog and account operations use the same signed response header. The
request `operation` values are `get_stars_revenue_stats`,
`get_stars_revenue_withdrawal_url`, `get_stars_revenue_ads_account_url`,
`export_invoice`, `get_premium_gift_code_options`,
`get_stars_giveaway_options`, `get_giveaway_info`, and `check_gift_code`. Every response must carry
`verified: true` and the authenticated `user_id`; APIFull rejects a mismatched
user, malformed TL predicate, negative amount/date, or an unsigned response.
Exported invoice, withdrawal, and ads URLs must be HTTPS. Gift-code and
giveaway options are provider-owned catalog data and are not treated as an
entitlement until a separate mutation workflow commits the corresponding
transaction.

The Premium BFF uses the same endpoint with a signed request envelope for
store operations. Requests contain `request_id`, `operation`, `user_id`, and a
JSON `payload`; `X-Teamgram-Payment-Provider-Request-Signature` authenticates
the exact body and `Idempotency-Key` is the deterministic `request_id`. The
provider signs the exact response body in
`X-Teamgram-Payment-Provider-Signature` and returns:

```json
{
  "request_id": "...",
  "operation": "payments.assignAppStoreTransaction",
  "verified": true,
  "user_id": 123,
  "result": { "accepted": true, "updates": { "predicate_name": "updates" } }
}
```

`payments.assignAppStoreTransaction` and
`payments.assignPlayMarketTransaction` require an accepted result containing
canonical Layer 229 `Updates`; `payments.canPurchaseStore` and
`payments.canPurchasePremium` return `{ "allowed": true|false }`; and
`help.getPremiumPromo` returns `{ "promo": ... }` with a canonical
`help.premiumPromo`. Missing or invalid provider configuration, mismatched
users, failed verification, malformed TL values, and provider errors fail
closed. Assignment retries use the same idempotency key and must not grant an
entitlement twice.

## Refund and recovery

Providers deliver refund and revoke notifications to
`POST /payment/provider/webhook`. The body is signed byte-for-byte with the
dedicated `TEAMGRAM_PAYMENT_PROVIDER_WEBHOOK_SIGNING_KEY` using
`X-Teamgram-Payment-Provider-Webhook-Signature: sha256=<digest>` (the existing
provider response signature header is accepted during rollout). A notification
has this shape:

```json
{
  "event_id": "provider-event-id",
  "event_type": "payment.refunded",
  "provider": "external",
  "user_id": 123,
  "request_key": "premium:123:offer:1",
  "transaction_id": "provider-transaction-id",
  "product": "premium_subscription",
  "months": 1,
  "stars": 0,
  "occurred_at": 1760000000,
  "reason": "chargeback"
}
```

The HTTP endpoint acknowledges a verified, schema-valid event only after it is
durably inserted in PostgreSQL and returns `202`. Reusing an event ID with a
different payload is a conflict. APIFull workers claim events with a lease,
apply the local ledger reversal exactly once, revoke the matching Premium
transaction or debit the matching Stars credit, and retry failed user-service
calls after a restart. Bot invoices are recorded as reversed but never create
an account entitlement.

The provider also supports the signed `get_payment_status` operation. The
request contains `user_id`, `provider`, `request_key`, `transaction_id`, and
`product`; the signed response contains `verified`, matching identity fields,
and `status` (`settled`, `refunded`, `revoked`, `pending`, or `not_found`). A
reconciliation worker polls settled local receipts, treats `pending` and
`not_found` as retryable, keeps settled rows on a periodic poll so a later
refund is still detected, and routes `refunded`/`revoked` through the same
webhook state machine. Provider outages never manufacture a local refund.

`provider` is the canonical provider identity stored with the local receipt.
For Premium and Stars, it defaults to `external` and can be changed with
`TEAMGRAM_PAYMENT_PROVIDER_NAME`; settlement, status polling, and webhook
payloads must use the same value. Bot invoice events keep the provider name
from the invoice itself.
