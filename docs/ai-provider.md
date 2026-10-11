# AI Provider Configuration

The messages and APIFull BFFs support two provider modes:

* The default Teamgram mode uses a signed response envelope. Set
  `TEAMGRAM_AI_PROVIDER_ENDPOINT` and a 32-byte-or-longer
  `TEAMGRAM_AI_PROVIDER_SIGNING_KEY`.
* An OpenAI-compatible endpoint can be used when the endpoint has no Teamgram
  signing key. Set `TEAMGRAM_AI_PROVIDER_ENDPOINT` to the provider base URL
  (for example `https://host.example/v1`), `TEAMGRAM_AI_PROVIDER_KEY` to the
  runtime secret, and optionally `TEAMGRAM_AI_PROVIDER_MODEL`. The adapter
  calls `/chat/completions`, binds `operation`, `user_id`, and the request
  payload in the prompt, requires a JSON-only response, and validates the
  operation-specific result in the BFF before constructing Layer 229 TL.

The API key is read only from process configuration and is never written to
logs or persisted state. HTTPS is required for non-loopback endpoints. The
signed Teamgram mode remains preferred whenever a signing key is configured.
