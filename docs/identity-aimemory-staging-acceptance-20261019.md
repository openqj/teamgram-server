# Identity/AIMemory staging acceptance

Date: 2026-10-19 (local acceptance run)

## Scope

This run exercised the current Identity, AIMemory, and Teamgram bridge code on
one Docker Desktop host. The stack used PostgreSQL 18, Redis 7, two Identity
API processes, two AIMemory API processes, and two temporary Teamgram HTTP
gateway processes sharing the same replay Redis. The Teamgram gateway
processes ran inside the existing Teamgram container, so this is a cross
process/shared-state check, not a multi-host failure-domain check.

No production credentials, private keys, or JWTs are included in this record.

## Acceptance results

| Check | Result | Evidence |
| --- | --- | --- |
| Identity A/B health and JWKS | PASS | Both returned `200`; JWKS matched the shared signing key. |
| Same encrypted MTProto proof on two gateways | PASS | Gateway A returned `200`; gateway B replay returned `401`. |
| Identity A stopped | PASS | A health failed; B stayed `200`; a fresh proof through B returned `200`. |
| Identity A restarted | PASS | A health returned `200`; a fresh proof through A returned `200`. |
| AIMemory A stopped | PASS | A became unreachable; B `/v1/me` stayed `200`; both returned `200` after recovery. |
| Identity Redis stopped | PASS | Both Identity health endpoints returned `503`; both recovered after Redis and DNS recovery. |
| Identity PostgreSQL stopped | PASS | Both Identity health endpoints returned `503`; both recovered after PostgreSQL accepted connections. |
| JWKS rotation | PASS | JWKS exposed old and new `kid`; AIMemory verified both old-key and new-key JWTs with `200`. |

The proof replay claim was performed against the shared Redis before Identity
token exchange, so the second gateway could not consume the same proof even
when it used a different Identity API instance.

## Bounded load results

Each rate ran for 10 seconds with a client concurrency cap of 128. Identity
requests were round-robin across both instances; AIMemory `/v1/me` requests
were also round-robin. Every request in the corrected run returned `200`.

| Target | Offered RPS | Requests | p50 ms | p95 ms | p99 ms | Max ms |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Identity token exchange | 50 | 500 | 5.240 | 16.605 | 41.370 | 191.692 |
| Identity token exchange | 100 | 1,000 | 4.661 | 8.664 | 15.380 | 27.951 |
| Identity token exchange | 200 | 2,000 | 7.050 | 21.396 | 111.935 | 223.210 |
| Identity token exchange | 500 | 5,000 | 168.052 | 2,171.593 | 3,976.604 | 8,626.947 |
| AIMemory `/v1/me` | 50 | 500 | 8.149 | 61.389 | 177.028 | 250.909 |
| AIMemory `/v1/me` | 100 | 1,000 | 8.564 | 21.790 | 175.883 | 264.377 |
| AIMemory `/v1/me` | 200 | 2,000 | 7.329 | 1,145.656 | 2,042.282 | 3,153.536 |
| AIMemory `/v1/me` | 500 | 5,000 | 570.511 | 3,105.149 | 5,334.618 | 11,669.451 |

These are local measurements only. The AIMemory image was linux/amd64 on an
ARM64 Docker Desktop host, and the run did not include a production load
balancer, TLS termination, external SMTP/OIDC, or multi-host networking. The
200+ RPS tail latency therefore remains a capacity gate rather than a passed
production SLO.

## Observed operational issue

An initial unbounded client run hit the configured AIMemory rate limits and
created excessive client-side connection wait. It was discarded. The bounded
rerun used explicit staging limits and a concurrency cap. During the run one
Identity process briefly reported database-unavailable health; restarting that
process cleared its connection-pool state. The database itself remained
reachable. The cause was not isolated, so connection-pool and request queue
metrics must be collected in the next capacity run.

## Release decision

The shared-state and recovery gates pass on this host. Production canary is
not authorized by this local run. Before a 5% -> 25% -> 100% canary, repeat
the same matrix on at least two hosts behind the real load balancer, add
connection-pool/queue metrics, execute backup-restore and external provider
checks, and reduce the 200/500 RPS tail through measured worker or replica
scaling.
