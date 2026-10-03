package layer229

import (
	"context"

	"github.com/teamgram/proto/mtproto"
)

// UserDirectory writes Firebase signups through the user service.
type UserDirectory interface {
	CreateNewUser(ctx context.Context, authKeyID int64, phone, country, first, last string) (*mtproto.User, error)
	UpdateName(ctx context.Context, userID int64, first, last string) error
}

var users UserDirectory

// SetDirectory installs the user-service backend. A nil directory clears it.
func SetDirectory(d UserDirectory) {
	stateMu.Lock()
	users = d
	stateMu.Unlock()
}

func currentUsers() UserDirectory {
	stateMu.Lock()
	defer stateMu.Unlock()
	return users
}
