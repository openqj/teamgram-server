-- Seed the Stars gift catalog used by payments.getStarsGiftOptions.
-- Gift pricing is denominated in XTR and is intentionally separate from the
-- top-up catalog because the two methods expose different product kinds.
INSERT INTO apifull_stars_offer
    (kind, stars, store_product, currency, amount, extended, active)
VALUES
    ('gift', 50,   '', 'XTR', 50,   0, 1),
    ('gift', 100,  '', 'XTR', 100,  0, 1),
    ('gift', 250,  '', 'XTR', 250,  0, 1),
    ('gift', 500,  '', 'XTR', 500,  0, 1),
    ('gift', 1000, '', 'XTR', 1000, 0, 1),
    ('gift', 2500, '', 'XTR', 2500, 0, 1),
    ('gift', 5000, '', 'XTR', 5000, 1, 1),
    ('gift', 10000, '', 'XTR', 10000, 1, 1)
ON CONFLICT (kind, stars, store_product, currency, amount) DO UPDATE
SET extended = EXCLUDED.extended,
    active = 1;
