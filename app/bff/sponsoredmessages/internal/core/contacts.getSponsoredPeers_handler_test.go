package core

import (
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
)

func TestContactsGetSponsoredPeersFailsClosedWithoutProvider(t *testing.T) {
	core := &SponsoredMessagesCore{MD: &metadata.RpcMetadata{UserId: 42}}
	got, err := core.ContactsGetSponsoredPeers(&mtproto.TLContactsGetSponsoredPeers{Q: "isolated_test"})
	if got != nil || !errors.Is(err, mtproto.ErrMethodNotImpl) {
		t.Fatalf("ContactsGetSponsoredPeers() = (%v, %v), want nil and METHOD_NOT_IMPL", got, err)
	}
}

func TestContactsGetSponsoredPeersRequiresAuthentication(t *testing.T) {
	core := &SponsoredMessagesCore{}
	got, err := core.ContactsGetSponsoredPeers(&mtproto.TLContactsGetSponsoredPeers{})
	if got != nil || err != mtproto.ErrAuthKeyUnregistered {
		t.Fatalf("ContactsGetSponsoredPeers() = (%v, %v), want AUTH_KEY_UNREGISTERED", got, err)
	}
}
