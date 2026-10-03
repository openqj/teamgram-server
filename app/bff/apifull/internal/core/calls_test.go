package core

import (
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
)

func TestCallsUnauthed(t *testing.T) {
	c := &ApiFullCore{}
	checks := []struct {
		name string
		fn   func() error
	}{
		{"PhoneCreateGroupCall", func() error { _, err := c.PhoneCreateGroupCall(nil); return err }},
		{"PhoneCreateConferenceCall7D0444BB", func() error { _, err := c.PhoneCreateConferenceCall7D0444BB(nil); return err }},
		{"MessagesRequestEncryption", func() error { _, err := c.MessagesRequestEncryption(nil); return err }},
	}
	for _, tc := range checks {
		if err := tc.fn(); !errors.Is(err, mtproto.ErrAuthKeyUnregistered) {
			t.Fatalf("%s: got %v, want AUTH_KEY_UNREGISTERED", tc.name, err)
		}
	}
}

func TestCallsCreateNilError(t *testing.T) {
	c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 1}}
	if _, err := c.PhoneCreateGroupCall(nil); err != nil {
		t.Fatalf("create: got %v", err)
	}
}
