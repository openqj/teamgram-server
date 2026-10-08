# Payment Provider

APIFull sends payment operations to `PaymentProviderEndpoint`. Stars top-up form discovery uses `operation: "get_stars_form"`; settlement uses `operation: "settle_stars"`. Both include the authenticated user and an active catalog offer. The provider must not choose a different offer or beneficiary. Form discovery returns a signed `form` with predicate `payments_paymentFormStars` and the matching `offer_id`, `stars`, `store_product`, `currency`, and `amount`; APIFull checks known catalog terms and requires a non-zero form ID.

The request includes an `Idempotency-Key` derived from the user and request key. Retries for that key must return the same transaction and response body. APIFull also serializes requests for the same user and request key locally.

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
