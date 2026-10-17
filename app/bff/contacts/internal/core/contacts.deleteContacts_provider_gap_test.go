package core

import (
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestContactsDeleteContactsRejectsNilUserProviderResult(t *testing.T) {
	const (
		selfID = int64(42)
		peerID = int64(84)
	)
	userClient := &deleteContactsUserClientStub{}
	syncClient := &deleteContactsSyncClientStub{}
	core := newDeleteContactsTestCore(selfID, userClient, &deleteContactsMessageClientStub{}, syncClient)

	result, err := core.ContactsDeleteContacts(&mtproto.TLContactsDeleteContacts{Id: []*mtproto.InputUser{
		{PredicateName: mtproto.Predicate_inputUser, UserId: peerID},
	}})
	if result != nil || !errors.Is(err, mtproto.ErrInternalServerError) {
		t.Fatalf("nil user provider result = (%v, %v), want (nil, INTERNAL_SERVER_ERROR)", result, err)
	}
	if syncClient.request != nil {
		t.Fatalf("sync request = %v after malformed lookup, want none", syncClient.request)
	}
}
