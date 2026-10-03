package core

import (
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
)

func TestAccountToggleSponsoredMessagesFailsClosed(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   *mtproto.TLAccountToggleSponsoredMessages
		want error
	}{
		{name: "nil request", want: mtproto.ErrInputRequestInvalid},
		{name: "missing enabled", in: &mtproto.TLAccountToggleSponsoredMessages{}, want: mtproto.ErrInputRequestInvalid},
		{
			name: "invalid enabled",
			in: &mtproto.TLAccountToggleSponsoredMessages{
				Enabled: &mtproto.Bool{PredicateName: "unknown"},
			},
			want: mtproto.ErrInputRequestInvalid,
		},
		{
			name: "enable",
			in:   &mtproto.TLAccountToggleSponsoredMessages{Enabled: mtproto.BoolTrue},
			want: mtproto.ErrMethodNotImpl,
		},
		{
			name: "disable",
			in:   &mtproto.TLAccountToggleSponsoredMessages{Enabled: mtproto.BoolFalse},
			want: mtproto.ErrMethodNotImpl,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			core := &SponsoredMessagesCore{MD: &metadata.RpcMetadata{UserId: 42}}
			got, err := core.AccountToggleSponsoredMessages(tc.in)
			if got != nil || !errors.Is(err, tc.want) {
				t.Fatalf("AccountToggleSponsoredMessages() = (%v, %v), want (nil, %v)", got, err, tc.want)
			}
		})
	}
}

func TestAccountToggleSponsoredMessagesRequiresAuthentication(t *testing.T) {
	core := &SponsoredMessagesCore{}
	got, err := core.AccountToggleSponsoredMessages(nil)
	if got != nil || !errors.Is(err, mtproto.ErrAuthKeyUnregistered) {
		t.Fatalf("AccountToggleSponsoredMessages() = (%v, %v), want AUTH_KEY_UNREGISTERED", got, err)
	}
}
