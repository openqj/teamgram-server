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

type getContactsUserClientStub struct {
	userclient.UserClient
	contacts     *userpb.Vector_ContactData
	contactErr   error
	users        *userpb.Vector_ImmutableUser
	usersErr     error
	usersRequest *userpb.TLUserGetMutableUsers
}

func (s *getContactsUserClientStub) UserGetContactList(context.Context, *userpb.TLUserGetContactList) (*userpb.Vector_ContactData, error) {
	return s.contacts, s.contactErr
}

func (s *getContactsUserClientStub) UserGetMutableUsers(_ context.Context, in *userpb.TLUserGetMutableUsers) (*userpb.Vector_ImmutableUser, error) {
	s.usersRequest = in
	return s.users, s.usersErr
}

func newGetContactsTestCore(client userclient.UserClient) *ContactsCore {
	ctx := context.Background()
	return &ContactsCore{
		ctx:    ctx,
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{UserClient: client}},
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: 42},
	}
}

func getContactsImmutableUser(id int64) *mtproto.ImmutableUser {
	return &mtproto.ImmutableUser{User: &mtproto.UserData{Id: id, FirstName: "user"}}
}

func TestContactsGetContactsHydratesContactAndUserEntities(t *testing.T) {
	client := &getContactsUserClientStub{
		contacts: &userpb.Vector_ContactData{Datas: []*mtproto.ContactData{{ContactUserId: 9, MutualContact: true}}},
		users:    &userpb.Vector_ImmutableUser{Datas: []*mtproto.ImmutableUser{getContactsImmutableUser(42), getContactsImmutableUser(9)}},
	}
	got, err := newGetContactsTestCore(client).ContactsGetContacts(&mtproto.TLContactsGetContacts{Hash: 1})
	if err != nil || got == nil || len(got.GetContacts()) != 1 || len(got.GetUsers()) != 1 {
		t.Fatalf("ContactsGetContacts() = (%+v, %v), want one contact and one hydrated user", got, err)
	}
	if !mtproto.FromBool(got.GetContacts()[0].GetMutual()) || got.GetContacts()[0].GetUserId() != 9 || got.GetUsers()[0].GetId() != 9 {
		t.Fatalf("contacts result = %+v, want mutual contact/user 9", got)
	}
	if client.usersRequest == nil || len(client.usersRequest.GetId()) != 2 || client.usersRequest.GetId()[0] != 42 || client.usersRequest.GetId()[1] != 9 {
		t.Fatalf("mutable-user request = %v, want [42 9]", client.usersRequest.GetId())
	}
}

func TestContactsGetContactsReturnsNotModifiedForSortedMatchingHash(t *testing.T) {
	client := &getContactsUserClientStub{contacts: &userpb.Vector_ContactData{Datas: []*mtproto.ContactData{
		{ContactUserId: 9},
		{ContactUserId: 2},
	}}}
	got, err := newGetContactsTestCore(client).ContactsGetContacts(&mtproto.TLContactsGetContacts{
		Hash: calculateContactsHash([]int64{2, 9}),
	})
	if err != nil || got == nil || got.To_ContactsContactsNotModified() == nil {
		t.Fatalf("matching hash result = (%+v, %v), want contacts.contactsNotModified", got, err)
	}
	if client.usersRequest != nil {
		t.Fatalf("matching hash performed mutable-user lookup: %+v", client.usersRequest)
	}
}

func TestContactsGetContactsReturnsEmptyWithoutContactLookup(t *testing.T) {
	client := &getContactsUserClientStub{contacts: &userpb.Vector_ContactData{}}
	got, err := newGetContactsTestCore(client).ContactsGetContacts(&mtproto.TLContactsGetContacts{})
	if err != nil || got == nil || len(got.GetContacts()) != 0 || len(got.GetUsers()) != 0 {
		t.Fatalf("empty ContactsGetContacts() = (%+v, %v), want empty result", got, err)
	}
	if client.usersRequest != nil {
		t.Fatalf("mutable-user lookup request = %+v, want no lookup", client.usersRequest)
	}
}

func TestContactsGetContactsPropagatesAndRejectsMalformedDependencies(t *testing.T) {
	contactErr := errors.New("contact list unavailable")
	client := &getContactsUserClientStub{contactErr: contactErr}
	if got, err := newGetContactsTestCore(client).ContactsGetContacts(&mtproto.TLContactsGetContacts{}); got != nil || err != contactErr {
		t.Fatalf("contact-list error = (%+v, %v), want %v", got, err, contactErr)
	}

	usersErr := errors.New("user lookup unavailable")
	client = &getContactsUserClientStub{
		contacts: &userpb.Vector_ContactData{Datas: []*mtproto.ContactData{{ContactUserId: 9}}},
		usersErr: usersErr,
	}
	if got, err := newGetContactsTestCore(client).ContactsGetContacts(&mtproto.TLContactsGetContacts{}); got != nil || err != usersErr {
		t.Fatalf("mutable-user error = (%+v, %v), want %v", got, err, usersErr)
	}

	for name, client := range map[string]userclient.UserClient{
		"nil contact response": &getContactsUserClientStub{},
		"nil contact row":      &getContactsUserClientStub{contacts: &userpb.Vector_ContactData{Datas: []*mtproto.ContactData{nil}}},
		"nil user response": &getContactsUserClientStub{
			contacts: &userpb.Vector_ContactData{Datas: []*mtproto.ContactData{{ContactUserId: 9}}},
		},
		"missing user": &getContactsUserClientStub{
			contacts: &userpb.Vector_ContactData{Datas: []*mtproto.ContactData{{ContactUserId: 9}}},
			users:    &userpb.Vector_ImmutableUser{Datas: []*mtproto.ImmutableUser{getContactsImmutableUser(42)}},
		},
		"nil user row": &getContactsUserClientStub{
			contacts: &userpb.Vector_ContactData{Datas: []*mtproto.ContactData{{ContactUserId: 9}}},
			users:    &userpb.Vector_ImmutableUser{Datas: []*mtproto.ImmutableUser{getContactsImmutableUser(42), nil}},
		},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := newGetContactsTestCore(client).ContactsGetContacts(&mtproto.TLContactsGetContacts{})
			if got != nil || !errors.Is(err, mtproto.ErrInternalServerError) && !errors.Is(err, mtproto.ErrContactIdInvalid) {
				t.Fatalf("malformed dependency = (%+v, %v), want fail-closed error", got, err)
			}
		})
	}

	if got, err := (&ContactsCore{MD: &metadata.RpcMetadata{UserId: 42}}).ContactsGetContacts(&mtproto.TLContactsGetContacts{}); got != nil || !errors.Is(err, mtproto.ErrInternalServerError) {
		t.Fatalf("missing DAO = (%+v, %v), want INTERNAL_SERVER_ERROR", got, err)
	}
	if got, err := (&ContactsCore{}).ContactsGetContacts(&mtproto.TLContactsGetContacts{}); got != nil || !errors.Is(err, mtproto.ErrAuthKeyUnregistered) {
		t.Fatalf("unauthenticated = (%+v, %v), want AUTH_KEY_UNREGISTERED", got, err)
	}
	if got, err := newGetContactsTestCore(client).ContactsGetContacts(nil); got != nil || !errors.Is(err, mtproto.ErrInputRequestInvalid) {
		t.Fatalf("nil request = (%+v, %v), want INPUT_REQUEST_INVALID", got, err)
	}
}
