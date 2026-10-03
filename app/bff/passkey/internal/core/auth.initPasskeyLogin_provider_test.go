package core

import (
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
)

func TestAuthInitPasskeyLoginFailsClosedWithoutProvider(t *testing.T) {
	c := &PasskeyCore{MD: &metadata.RpcMetadata{UserId: 1}}
	result, err := c.AuthInitPasskeyLogin(&mtproto.TLAuthInitPasskeyLogin{
		ApiId:   17349,
		ApiHash: "344583e45741c457fe1862106095a5eb",
	})
	if result != nil || !errors.Is(err, mtproto.ErrMethodNotImpl) {
		t.Fatalf("AuthInitPasskeyLogin() = (%#v, %v), want (nil, METHOD_NOT_IMPL)", result, err)
	}
}
