# Payment Provider

APIFull sends payment operations to `PaymentProviderEndpoint`. Stars top-up form discovery uses `operation: "get_stars_form"`; settlement uses `operation: "settle_stars"`. Both include the authenticated user and an active catalog offer. The provider must not choose a different offer or beneficiary. Form discovery returns a signed `form` with predicate `payments_paymentFormStars` and the matching `offer_id`, `stars`, `store_product`, `currency`, and `amount`; APIFull checks known catalog terms and requires a non-zero form ID.

Each request includes `X-Teamgram-Payment-Provider-Request-Signature: sha256=<digest>`, an HMAC-SHA256 digest of the exact JSON request body using `PaymentProviderSigningKey`. When `PaymentProviderKey` is configured, APIFull also sends `Authorization: Bearer <key>`. The request includes an `Idempotency-Key` derived from the user and request key. Retries for that key must return the same transaction and response body. APIFull also serializes requests for the same user and request key locally.

The local acceptance fixture at `docker/production/payment-provider-fixture` implements signed Premium and bot-invoice form/validation/settlement responses and deterministic `get_stars_form` / `settle_stars` responses. Its Stars form mirrors the submitted active offer, and settlement responses are cached by operation plus `request_key` so a retry cannot mint a second transaction or cross product boundaries. The PostgreSQL control plane canonicalizes stored JSON so a provider restart returns the same response bytes and signature. Form and validation responses may include product metadata; only Premium and Stars settlements create provider entitlement rows. Bot invoice settlements retain their receipt and transaction without creating an account entitlement. The fixture is test-only; it does not provide production authority for revenue, paid messages, gift codes, or giveaways. Those operations remain external provider dependencies and must fail closed when no signed provider is configured.

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
