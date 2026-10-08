-- PostgreSQL 18 cryptographic primitives used by production probes and
-- guarded persistence readbacks. Kept as a separate migration so an already
-- initialized development database receives the extension as well.
CREATE EXTENSION IF NOT EXISTS pgcrypto;
