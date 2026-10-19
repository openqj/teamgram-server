-- Align the acceptance catalog with the StoreKit identifiers used by iOS.
-- Keep the old rows for historical references, but make only the canonical
-- identifiers active so the catalog remains one offer per Stars tier.
WITH product_map(stars, old_product, new_product) AS (
  VALUES
    (100::bigint, 'teamgram-stars-topup-100', 'org.telegram.telegramStars.topup.x100'),
    (150::bigint, 'teamgram-stars-topup-150', 'org.telegram.telegramStars.topup.x150'),
    (250::bigint, 'teamgram-stars-topup-250', 'org.telegram.telegramStars.topup.x250'),
    (350::bigint, 'teamgram-stars-topup-350', 'org.telegram.telegramStars.topup.x350'),
    (500::bigint, 'teamgram-stars-topup-500', 'org.telegram.telegramStars.topup.x500'),
    (750::bigint, 'teamgram-stars-topup-750', 'org.telegram.telegramStars.topup.x750'),
    (1000::bigint, 'teamgram-stars-topup-1000', 'org.telegram.telegramStars.topup.x1000'),
    (1500::bigint, 'teamgram-stars-topup-1500', 'org.telegram.telegramStars.topup.x1500'),
    (2500::bigint, 'teamgram-stars-topup-2500', 'org.telegram.telegramStars.topup.x2500'),
    (5000::bigint, 'teamgram-stars-topup-5000', 'org.telegram.telegramStars.topup.x5000'),
    (10000::bigint, 'teamgram-stars-topup-10000', 'org.telegram.telegramStars.topup.x10000'),
    (25000::bigint, 'teamgram-stars-topup-25000', 'org.telegram.telegramStars.topup.x25000'),
    (50000::bigint, 'teamgram-stars-topup-50000', 'org.telegram.telegramStars.topup.x50000'),
    (100000::bigint, 'teamgram-stars-topup-100000', 'org.telegram.telegramStars.topup.x100000'),
    (150000::bigint, 'teamgram-stars-topup-150000', 'org.telegram.telegramStars.topup.x150000')
), canonical_rows AS (
  INSERT INTO apifull_stars_offer
    (kind, stars, store_product, currency, amount, extended, active)
  SELECT offer.kind, offer.stars, product_map.new_product, offer.currency,
         offer.amount, offer.extended, 1
  FROM apifull_stars_offer AS offer
  JOIN product_map
    ON product_map.stars = offer.stars
   AND product_map.old_product = offer.store_product
  WHERE offer.kind = 'topup'
  ON CONFLICT (kind, stars, store_product, currency, amount) DO UPDATE
    SET extended = EXCLUDED.extended,
        active = 1
  RETURNING 1
)
UPDATE apifull_stars_offer AS offer
SET active = 0
FROM product_map
WHERE offer.kind = 'topup'
  AND offer.store_product = product_map.old_product;
