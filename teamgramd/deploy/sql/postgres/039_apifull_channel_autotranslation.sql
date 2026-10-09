-- Persist the Layer 229 channel autotranslation flag in the canonical channel row.
ALTER TABLE apifull_channel
    ADD COLUMN IF NOT EXISTS autotranslation SMALLINT NOT NULL DEFAULT 0;
