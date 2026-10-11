package core

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/sponsoredmessages/internal/svc"
)

func TestAccountToggleSponsoredMessagesValidation(t *testing.T) {
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
			want: nil,
		},
		{
			name: "disable",
			in:   &mtproto.TLAccountToggleSponsoredMessages{Enabled: mtproto.BoolFalse},
			want: nil,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			core := &SponsoredMessagesCore{MD: &metadata.RpcMetadata{UserId: 42}}
			got, err := core.AccountToggleSponsoredMessages(tc.in)
			if tc.want == nil {
				if got != mtproto.BoolTrue || err != nil {
					t.Fatalf("AccountToggleSponsoredMessages() = (%v, %v), want boolTrue", got, err)
				}
				return
			}
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

func TestMessagesGetSponsoredMessagesHonorsDisabledPreference(t *testing.T) {
	uid := time.Now().UnixNano()
	core := New(context.Background(), &svc.ServiceContext{})
	core.MD = &metadata.RpcMetadata{UserId: uid}
	// A configured provider must not be reached when the caller has disabled
	// sponsored messages; the empty constructor is returned before HTTP I/O.
	core.svcCtx.Config.SponsoredProviderEndpoint = "https://provider.invalid/sponsored"
	if got, err := core.AccountToggleSponsoredMessages(&mtproto.TLAccountToggleSponsoredMessages{Enabled: mtproto.BoolFalse}); got != mtproto.BoolTrue || err != nil {
		t.Fatalf("disable sponsored messages = (%v, %v)", got, err)
	}
	peer := mtproto.MakeTLInputPeerSelf(nil).To_InputPeer()
	got, err := core.MessagesGetSponsoredMessages(&mtproto.TLMessagesGetSponsoredMessages{Peer: peer})
	if err != nil || got == nil || got.GetPredicateName() != mtproto.Predicate_messages_sponsoredMessagesEmpty {
		t.Fatalf("disabled sponsored messages = (%v, %v), want canonical empty result", got, err)
	}
}
