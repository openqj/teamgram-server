package dao

import (
	"errors"

	"github.com/teamgram/teamgram-server/pkg/twofa"
)

func (d *Dao) CheckSessionPasswordNeeded(userID int64) (bool, error) {
	if d.passwordStoreErr != nil {
		return false, d.passwordStoreErr
	}
	if d.passwordStore == nil {
		return false, errors.New("2FA proof store is unavailable")
	}
	state, err := twofa.LoadPasswordState(d.passwordStore, userID)
	return len(state.Secret) > 0, err
}
