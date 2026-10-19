-- Stars top-ups use CNY minor units on the Telegram wire contract.
-- One Star is one yuan, so amount is stars * 100 (fen). Keep prior rows for
-- receipt/history lookups but publish only this canonical StoreKit catalog.
UPDATE apifull_stars_offer
SET active = 0
WHERE kind = 'topup';

INSERT INTO apifull_stars_offer
    (kind, stars, store_product, currency, amount, extended, active)
VALUES
    ('topup', 1,      'org.telegram.telegramStars.topup.x1',      'CNY', 100,      0, 1),
    ('topup', 15,     'org.telegram.telegramStars.topup.x15',     'CNY', 1500,     0, 1),
    ('topup', 25,     'org.telegram.telegramStars.topup.x25',     'CNY', 2500,     0, 1),
    ('topup', 50,     'org.telegram.telegramStars.topup.x50',     'CNY', 5000,     0, 1),
    ('topup', 75,     'org.telegram.telegramStars.topup.x75',     'CNY', 7500,     0, 1),
    ('topup', 100,    'org.telegram.telegramStars.topup.x100',    'CNY', 10000,    0, 1),
    ('topup', 150,    'org.telegram.telegramStars.topup.x150',    'CNY', 15000,    0, 1),
    ('topup', 250,    'org.telegram.telegramStars.topup.x250',    'CNY', 25000,    0, 1),
    ('topup', 350,    'org.telegram.telegramStars.topup.x350',    'CNY', 35000,    0, 1),
    ('topup', 500,    'org.telegram.telegramStars.topup.x500',    'CNY', 50000,    0, 1),
    ('topup', 750,    'org.telegram.telegramStars.topup.x750',    'CNY', 75000,    0, 1),
    ('topup', 1000,   'org.telegram.telegramStars.topup.x1000',   'CNY', 100000,   0, 1),
    ('topup', 1500,   'org.telegram.telegramStars.topup.x1500',   'CNY', 150000,   0, 1),
    ('topup', 2500,   'org.telegram.telegramStars.topup.x2500',   'CNY', 250000,   0, 1),
    ('topup', 5000,   'org.telegram.telegramStars.topup.x5000',   'CNY', 500000,   0, 1),
    ('topup', 10000,  'org.telegram.telegramStars.topup.x10000',  'CNY', 1000000,  0, 1),
    ('topup', 25000,  'org.telegram.telegramStars.topup.x25000',  'CNY', 2500000,  0, 1),
    ('topup', 35000,  'org.telegram.telegramStars.topup.x35000',  'CNY', 3500000,  0, 1),
    ('topup', 50000,  'org.telegram.telegramStars.topup.x50000',  'CNY', 5000000,  0, 1),
    ('topup', 100000, 'org.telegram.telegramStars.topup.x100000', 'CNY', 10000000, 0, 1),
    ('topup', 150000, 'org.telegram.telegramStars.topup.x150000', 'CNY', 15000000, 0, 1)
ON CONFLICT (kind, stars, store_product, currency, amount) DO UPDATE
SET extended = EXCLUDED.extended,
    active = EXCLUDED.active;
