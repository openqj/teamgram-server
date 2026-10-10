package dao

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/twofa"
)

// CheckPassword verifies the one-time SRP proof against the deployment-owned
// PostgreSQL proof store.  Creator transfer must never proceed without it.
func (d *Dao) CheckPassword(userID int64, proof *mtproto.InputCheckPasswordSRP) error {
	if d == nil || d.PostgresDSN == "" || userID <= 0 || proof == nil {
		return mtproto.ErrPasswordHashInvalid
	}
	store, err := twofa.OpenPostgresProofStore(d.PostgresDSN)
	if err != nil {
		return err
	}
	state, err := twofa.LoadPasswordState(store, userID)
	if err != nil {
		return err
	}
	if len(state.Secret) == 0 {
		return mtproto.ErrPasswordHashInvalid
	}
	if err = twofa.VerifyPassword(store, userID, state, proof); err != nil {
		return err
	}
	return nil
}
