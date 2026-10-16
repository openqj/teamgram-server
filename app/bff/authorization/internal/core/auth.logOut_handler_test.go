package core

import (
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestAuthLogOutRejectsNilRequest(t *testing.T) {
	c := newProviderGapCore()
	c.MD.UserId = 42

	result, err := c.AuthLogOut(nil)
	if result != nil || !errors.Is(err, mtproto.ErrInputRequestInvalid) {
		t.Fatalf("auth.logOut = (%v, %v), want INPUT_REQUEST_INVALID", result, err)
	}
}
