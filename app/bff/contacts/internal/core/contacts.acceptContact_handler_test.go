package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/contacts/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/contacts/internal/svc"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/zeromicro/go-zero/core/logx"
)

type acceptContactUserClientStub struct {
	userclient.UserClient
	users           *mtproto.MutableUsers
	getUsersErr     error
	addContactErr   error
	addContact      *userpb.TLUserAddContact
	addContactReply *mtproto.Bool
	addCalls        int
}

func (s *acceptContactUserClientStub) UserGetMutableUsersV2(_ context.Context, _ *userpb.TLUserGetMutableUsersV2) (*mtproto.MutableUsers, error) {
	return s.users, s.getUsersErr
}

func (s *acceptContactUserClientStub) UserAddContact(_ context.Context, in *userpb.TLUserAddContact) (*mtproto.Bool, error) {
	s.addCalls++
	s.addContact = in
	return s.addContactReply, s.addContactErr
}

func TestContactsAcceptContactPropagatesUserLookupFailure(t *testing.T) {
	const (
		selfID = int64(42)
		peerID = int64(84)
	)
	lookupErr := errors.New("user lookup unavailable")
	userClient := &acceptContactUserClientStub{getUsersErr: lookupErr}
	core := newAcceptContactTestCore(selfID, userClient)

	got, err := core.ContactsAcceptContact(acceptContactTestRequest(peerID))
	if got != nil || err != lookupErr {
		t.Fatalf("ContactsAcceptContact() = (%v, %v), want nil and lookup error", got, err)
	}
	if userClient.addCalls != 0 {
		t.Fatalf("UserAddContact called %d times after lookup failure, want 0", userClient.addCalls)
	}
}

func TestContactsAcceptContactPropagatesContactWriteFailure(t *testing.T) {
	const (
		selfID = int64(42)
		peerID = int64(84)
	)
	writeErr := errors.New("contact write unavailable")
	userClient := &acceptContactUserClientStub{
		users:         acceptContactTestUsers(selfID, peerID),
		addContactErr: writeErr,
	}
	core := newAcceptContactTestCore(selfID, userClient)

	got, err := core.ContactsAcceptContact(acceptContactTestRequest(peerID))
	if got != nil || err != writeErr {
		t.Fatalf("ContactsAcceptContact() = (%v, %v), want nil and write error", got, err)
	}
	if userClient.addCalls != 1 || userClient.addContact == nil || userClient.addContact.Id != peerID {
		t.Fatalf("contact writes = %d, request = %+v; want one write for peer %d", userClient.addCalls, userClient.addContact, peerID)
	}
}

func TestContactsAcceptContactWritesContactAndReturnsUpdates(t *testing.T) {
	const (
		selfID = int64(42)
		peerID = int64(84)
	)
	userClient := &acceptContactUserClientStub{
		users:           acceptContactTestUsers(selfID, peerID),
		addContactReply: mtproto.BoolTrue,
	}
	core := newAcceptContactTestCore(selfID, userClient)

	got, err := core.ContactsAcceptContact(acceptContactTestRequest(peerID))
	if err != nil || got == nil {
		t.Fatalf("ContactsAcceptContact() = (%v, %v), want Updates", got, err)
	}
	if userClient.addCalls != 1 || userClient.addContact == nil {
		t.Fatalf("contact writes = %d, request = %+v; want one write", userClient.addCalls, userClient.addContact)
	}
	if userClient.addContact.UserId != selfID || userClient.addContact.Id != peerID || userClient.addContact.FirstName != "Peer" {
		t.Fatalf("UserAddContact request = %+v, want owner %d and peer %d profile", userClient.addContact, selfID, peerID)
	}
}

func acceptContactTestRequest(peerID int64) *mtproto.TLContactsAcceptContact {
	return &mtproto.TLContactsAcceptContact{Id: mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: peerID}).To_InputUser()}
}

func acceptContactTestUsers(selfID, peerID int64) *mtproto.MutableUsers {
	return &mtproto.MutableUsers{Users: []*mtproto.ImmutableUser{
		mtproto.MakeTLImmutableUser(&mtproto.ImmutableUser{User: &mtproto.UserData{Id: selfID, FirstName: "Owner"}}).To_ImmutableUser(),
		mtproto.MakeTLImmutableUser(&mtproto.ImmutableUser{User: &mtproto.UserData{Id: peerID, FirstName: "Peer"}}).To_ImmutableUser(),
	}}
}

func newAcceptContactTestCore(selfID int64, userClient *acceptContactUserClientStub) *ContactsCore {
	ctx := context.Background()
	return &ContactsCore{
		ctx:    ctx,
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{UserClient: userClient}},
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: selfID},
	}
}
