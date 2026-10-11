package persist

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/teamgram/proto/mtproto"
)

// LoadPremiumPromo returns the deployment-provisioned premium catalogue. An
// absent row is distinct from an empty or malformed catalogue so callers can
// fail closed without manufacturing prices.
func LoadPremiumPromo(ctx context.Context) (*mtproto.Help_PremiumPromo, bool, error) {
	store, ok := Default.(*postgresStore)
	if !ok || store == nil || store.db == nil {
		return nil, false, errors.New("apifull: premium promo requires PostgreSQL")
	}
	var raw []byte
	err := store.db.QueryRowContext(ctx, `SELECT promo FROM apifull_premium_promo WHERE id = 1`).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if len(raw) == 0 || !json.Valid(raw) {
		return nil, true, errors.New("apifull: invalid premium promo json")
	}
	var promo mtproto.Help_PremiumPromo
	if err = json.Unmarshal(raw, &promo); err != nil {
		return nil, true, err
	}
	return &promo, true, nil
}

// StorePremiumPromo provisions or replaces the singleton catalogue row. It is
// intended for deployment tooling and focused integration tests, not a client
// RPC mutation.
func StorePremiumPromo(ctx context.Context, promo *mtproto.Help_PremiumPromo) error {
	store, ok := Default.(*postgresStore)
	if !ok || store == nil || store.db == nil {
		return errors.New("apifull: premium promo requires PostgreSQL")
	}
	if promo == nil {
		return errors.New("apifull: premium promo is required")
	}
	raw, err := json.Marshal(promo)
	if err != nil {
		return err
	}
	if !json.Valid(raw) {
		return errors.New("apifull: invalid premium promo json")
	}
	_, err = store.db.ExecContext(ctx, `INSERT INTO apifull_premium_promo (id, promo, updated_at)
		VALUES (1, $1::jsonb, CURRENT_TIMESTAMP)
		ON CONFLICT (id) DO UPDATE SET promo = EXCLUDED.promo, updated_at = CURRENT_TIMESTAMP`, string(raw))
	return err
}

func DeletePremiumPromo(ctx context.Context) error {
	store, ok := Default.(*postgresStore)
	if !ok || store == nil || store.db == nil {
		return errors.New("apifull: premium promo requires PostgreSQL")
	}
	_, err := store.db.ExecContext(ctx, `DELETE FROM apifull_premium_promo WHERE id = 1`)
	return err
}
