-- Persist canonical Telegram document SHA-256 values for messages.getDocumentByHash.
ALTER TABLE documents
    ADD COLUMN IF NOT EXISTS sha256 BYTEA NOT NULL DEFAULT ''::bytea;

CREATE INDEX IF NOT EXISTS documents_sha256_lookup_idx
    ON documents (sha256, file_size, mime_type, id)
    WHERE deleted = FALSE AND octet_length(sha256) = 32;
