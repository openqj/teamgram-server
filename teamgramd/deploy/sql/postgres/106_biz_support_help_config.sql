-- Keep help.getSupport usable when the embedded BFF configuration omits Help.
-- Deployment-specific values can replace this row without changing the peer seed.
INSERT INTO apifull_kv (k, v)
VALUES (
    'configuration:help',
    '{"supportUserId":900000000016,"supportUserAccessHash":910000000016,"supportUserFirstName":"Volunteer Support"}'
)
ON CONFLICT (k) DO NOTHING;
