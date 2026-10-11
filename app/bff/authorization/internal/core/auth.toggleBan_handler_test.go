package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
)

func TestAuthToggleBanRejectsMissingAuthentication(t *testing.T) {
	var c *AuthorizationCore
	result, err := c.AuthToggleBan(nil)
	if result != nil || !errors.Is(err, mtproto.ErrAuthKeyUnregistered) {
		t.Fatalf("AuthToggleBan() = (%v, %v), want auth key error", result, err)
	}

	c = New(context.Background(), nil)
	c.MD = nil
	result, err = c.AuthToggleBan(nil)
	if result != nil || !errors.Is(err, mtproto.ErrAuthKeyUnregistered) {
		t.Fatalf("AuthToggleBan() with nil metadata = (%v, %v), want auth key error", result, err)
	}
}

func TestAuthToggleBanRejectsNilRequestForAdmin(t *testing.T) {
	c := New(context.Background(), nil)
	c.MD = &metadata.RpcMetadata{IsAdmin: true}

	result, err := c.AuthToggleBan(nil)
	if result != nil || !errors.Is(err, mtproto.ErrInputRequestInvalid) {
		t.Fatalf("AuthToggleBan() = (%v, %v), want input request error", result, err)
	}
}
