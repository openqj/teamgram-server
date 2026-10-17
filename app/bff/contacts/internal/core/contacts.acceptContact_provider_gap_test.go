package core

import (
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestContactsAcceptContactRejectsNilUserProviderResult(t *testing.T) {
	userClient := &acceptContactUserClientStub{}
	core := newAcceptContactTestCore(42, userClient)

	result, err := core.ContactsAcceptContact(acceptContactTestRequest(84))
	if result != nil || !errors.Is(err, mtproto.ErrInternalServerError) {
		t.Fatalf("nil user provider result = (%v, %v), want (nil, INTERNAL_SERVER_ERROR)", result, err)
	}
	if userClient.addCalls != 0 {
		t.Fatalf("UserAddContact called %d times after malformed lookup, want 0", userClient.addCalls)
	}
}
