package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
)

func TestContactsGetSponsoredPeersReturnsTypedEmpty(t *testing.T) {
	core := &SponsoredMessagesCore{MD: &metadata.RpcMetadata{UserId: 42}}
	got, err := core.ContactsGetSponsoredPeers(&mtproto.TLContactsGetSponsoredPeers{Q: "isolated_test"})
	if err != nil {
		t.Fatalf("ContactsGetSponsoredPeers() error = %v", err)
	}
	if got == nil || got.GetPredicateName() != mtproto.Predicate_contacts_sponsoredPeersEmpty {
		t.Fatalf("ContactsGetSponsoredPeers() = %v, want contacts.sponsoredPeersEmpty", got)
	}
	if len(got.GetPeers()) != 0 || len(got.GetChats()) != 0 || len(got.GetUsers()) != 0 {
		t.Fatalf("ContactsGetSponsoredPeers() returned non-empty inventory: %v", got)
	}
}

func TestContactsGetSponsoredPeersRequiresAuthentication(t *testing.T) {
	core := &SponsoredMessagesCore{}
	got, err := core.ContactsGetSponsoredPeers(&mtproto.TLContactsGetSponsoredPeers{})
	if got != nil || err != mtproto.ErrAuthKeyUnregistered {
		t.Fatalf("ContactsGetSponsoredPeers() = (%v, %v), want AUTH_KEY_UNREGISTERED", got, err)
	}
}
