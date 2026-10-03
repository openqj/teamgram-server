package dao

import (
	"errors"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/twofa"
)

func (d *Dao) LoadPasswordState(userID int64) (twofa.PasswordState, error) {
	if d.passwordStoreErr != nil {
		return twofa.PasswordState{}, d.passwordStoreErr
	}
	if d.passwordStore == nil {
		return twofa.PasswordState{}, errors.New("2FA proof store is unavailable")
	}
	return twofa.LoadPasswordState(d.passwordStore, userID)
}

func (d *Dao) CheckSessionPasswordNeeded(userID int64) (bool, error) {
	state, err := d.LoadPasswordState(userID)
	return len(state.Secret) > 0, err
}

func (d *Dao) CheckPassword(userID int64, in *mtproto.InputCheckPasswordSRP) error {
	state, err := d.LoadPasswordState(userID)
	if err != nil {
		return err
	}
	if len(state.Secret) == 0 {
		return mtproto.ErrPasswordHashInvalid
	}
	return twofa.VerifyPassword(d.passwordStore, userID, state, in)
}
