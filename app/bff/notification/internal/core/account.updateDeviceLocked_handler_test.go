package core

import (
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
)

func TestAccountUpdateDeviceLockedRejectsUnauthenticatedAndNilRequest(t *testing.T) {
	core := &NotificationCore{}
	if _, err := core.AccountUpdateDeviceLocked(nil); !errors.Is(err, mtproto.ErrAuthKeyUnregistered) {
		t.Fatalf("nil core metadata error = %v, want AUTH_KEY_UNREGISTERED", err)
	}

	core.MD = &metadata.RpcMetadata{UserId: 972341}
	if _, err := core.AccountUpdateDeviceLocked(nil); !errors.Is(err, mtproto.ErrInputRequestInvalid) {
		t.Fatalf("nil request error = %v, want INPUT_REQUEST_INVALID", err)
	}
}
