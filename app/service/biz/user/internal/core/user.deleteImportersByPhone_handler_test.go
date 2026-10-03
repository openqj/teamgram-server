package core

import (
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

func TestUserDeleteImportersByPhoneRequiresCallerMetadata(t *testing.T) {
	got, err := (&UserCore{}).UserDeleteImportersByPhone(&user.TLUserDeleteImportersByPhone{Phone: "12025550104"})
	if got != nil || !errors.Is(err, mtproto.ErrAuthKeyUnregistered) {
		t.Fatalf("UserDeleteImportersByPhone() = (%v, %v), want AUTH_KEY_UNREGISTERED", got, err)
	}
}
