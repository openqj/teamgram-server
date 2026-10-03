package core

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/contacts/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/contacts/internal/svc"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/zeromicro/go-zero/core/logx"
)

type getContactIDsUserClientStub struct {
	userclient.UserClient
	contacts *userpb.Vector_ContactData
	err      error
}

func (s *getContactIDsUserClientStub) UserGetContactList(context.Context, *userpb.TLUserGetContactList) (*userpb.Vector_ContactData, error) {
	return s.contacts, s.err
}

func newGetContactIDsTestCore(client userclient.UserClient) *ContactsCore {
	ctx := context.Background()
	return &ContactsCore{
		ctx:    ctx,
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{UserClient: client}},
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: 42},
	}
}

func contactsHash(ids ...int64) int64 {
	const hashMod = uint64(0x80000000)
	hash := uint64(0)
	for _, id := range ids {
		hash = (hash*20261 + hashMod + uint64(id)) % hashMod
	}
	return int64(hash)
}

func TestContactsGetContactIDsSortsAndReturnsProtocolInts(t *testing.T) {
	client := &getContactIDsUserClientStub{contacts: &userpb.Vector_ContactData{Datas: []*mtproto.ContactData{
		{ContactUserId: 9},
		{ContactUserId: 2},
	}}}
	core := newGetContactIDsTestCore(client)

	got, err := core.ContactsGetContactIDs(&mtproto.TLContactsGetContactIDs{Hash: 0})
	if err != nil || got == nil || !reflect.DeepEqual(got.Datas, []int32{2, 9}) {
		t.Fatalf("ContactsGetContactIDs() = (%+v, %v), want sorted [2 9]", got, err)
	}

	got, err = core.ContactsGetContactIDs(&mtproto.TLContactsGetContactIDs{Hash: contactsHash(2, 9)})
	if err != nil || got == nil || len(got.Datas) != 0 {
		t.Fatalf("matching hash result = (%+v, %v), want empty vector", got, err)
	}
}

func TestContactsGetContactIDsFailsClosedOnInvalidDependencies(t *testing.T) {
	wantErr := errors.New("contact list unavailable")
	client := &getContactIDsUserClientStub{err: wantErr}
	if got, err := newGetContactIDsTestCore(client).ContactsGetContactIDs(&mtproto.TLContactsGetContactIDs{}); got != nil || err != wantErr {
		t.Fatalf("dependency error = (%+v, %v), want (%v)", got, err, wantErr)
	}

	for name, client := range map[string]userclient.UserClient{
		"nil response": &getContactIDsUserClientStub{},
		"nil contact":  &getContactIDsUserClientStub{contacts: &userpb.Vector_ContactData{Datas: []*mtproto.ContactData{nil}}},
		"out of range": &getContactIDsUserClientStub{contacts: &userpb.Vector_ContactData{Datas: []*mtproto.ContactData{{ContactUserId: int64(^uint32(0)>>1) + 1}}}},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := newGetContactIDsTestCore(client).ContactsGetContactIDs(&mtproto.TLContactsGetContactIDs{})
			if got != nil || err == nil {
				t.Fatalf("result = (%+v, %v), want an error", got, err)
			}
			if name == "nil response" && !errors.Is(err, mtproto.ErrInternalServerError) {
				t.Fatalf("nil response error = %v, want INTERNAL_SERVER_ERROR", err)
			}
			if name != "nil response" && !errors.Is(err, mtproto.ErrUserIdInvalid) {
				t.Fatalf("%s error = %v, want USER_ID_INVALID", name, err)
			}
		})
	}

	if got, err := (&ContactsCore{MD: &metadata.RpcMetadata{UserId: 42}}).ContactsGetContactIDs(&mtproto.TLContactsGetContactIDs{}); got != nil || !errors.Is(err, mtproto.ErrInternalServerError) {
		t.Fatalf("missing DAO = (%+v, %v), want INTERNAL_SERVER_ERROR", got, err)
	}
	if got, err := (&ContactsCore{}).ContactsGetContactIDs(&mtproto.TLContactsGetContactIDs{}); got != nil || !errors.Is(err, mtproto.ErrAuthKeyUnregistered) {
		t.Fatalf("unauthenticated = (%+v, %v), want AUTH_KEY_UNREGISTERED", got, err)
	}
	if got, err := newGetContactIDsTestCore(client).ContactsGetContactIDs(nil); got != nil || !errors.Is(err, mtproto.ErrInputRequestInvalid) {
		t.Fatalf("nil request = (%+v, %v), want INPUT_REQUEST_INVALID", got, err)
	}
}
