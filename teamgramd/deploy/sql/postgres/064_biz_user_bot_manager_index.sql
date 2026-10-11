-- PostgreSQL index for bots.getAdminedBots manager hierarchy lookups.
CREATE INDEX IF NOT EXISTS bots_manager_idx ON bots (manager_bot_id, bot_id);
