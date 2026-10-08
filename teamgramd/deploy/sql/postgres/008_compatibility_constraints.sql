-- PostgreSQL 18 compatibility constraints for legacy Teamgram aggregates.
--
-- These keys preserve the idempotency boundaries of the source schema. The
-- project has not launched, so a fresh database is expected to contain no
-- duplicate rows when this migration is applied.

-- APIFull's process-shared KV store is part of the PostgreSQL deployment
-- boundary so read-only application processes can start without issuing DDL.
CREATE TABLE IF NOT EXISTS apifull_kv (
    k TEXT PRIMARY KEY,
    v TEXT NOT NULL
);

ALTER TABLE phone_books
    DROP CONSTRAINT IF EXISTS phone_books_user_phone_key;

CREATE UNIQUE INDEX IF NOT EXISTS phone_books_auth_client_key
    ON phone_books (auth_key_id, client_id);

CREATE UNIQUE INDEX IF NOT EXISTS auth_users_auth_key_user_key
    ON auth_users (auth_key_id, user_id);

CREATE UNIQUE INDEX IF NOT EXISTS photo_sizes_photo_size_type_key
    ON photo_sizes (photo_size_id, size_type);

CREATE UNIQUE INDEX IF NOT EXISTS video_sizes_video_size_type_key
    ON video_sizes (video_size_id, size_type);

CREATE UNIQUE INDEX IF NOT EXISTS hash_tags_user_tag_message_key
    ON hash_tags (user_id, hash_tag, hash_tag_message_id);
