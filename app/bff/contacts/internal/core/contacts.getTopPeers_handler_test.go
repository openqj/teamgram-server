package core

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/persist"
	"github.com/teamgram/teamgram-server/app/bff/contacts/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/contacts/internal/svc"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/zeromicro/go-zero/core/logx"
)

type topPeersStore struct {
	data map[string]string
}

func (s *topPeersStore) Get(key string) (string, error) {
	return s.data[key], nil
}

func (s *topPeersStore) Set(key, value string) error {
	s.data[key] = value
	return nil
}

type topPeersUserClientStub struct {
	userclient.UserClient
	contacts       *userpb.Vector_ContactData
	contactErr     error
	users          *userpb.Vector_ImmutableUser
	usersErr       error
	mutableRequest *userpb.TLUserGetMutableUsers
}

func (s *topPeersUserClientStub) UserGetContactList(context.Context, *userpb.TLUserGetContactList) (*userpb.Vector_ContactData, error) {
	return s.contacts, s.contactErr
}

func (s *topPeersUserClientStub) UserGetMutableUsers(_ context.Context, in *userpb.TLUserGetMutableUsers) (*userpb.Vector_ImmutableUser, error) {
	s.mutableRequest = in
	return s.users, s.usersErr
}

func newTopPeersTestCore(client userclient.UserClient) *ContactsCore {
	ctx := context.Background()
	return &ContactsCore{
		ctx:    ctx,
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{UserClient: client}},
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: 42},
	}
}

func topPeersImmutableUser(id int64, bot *mtproto.BotData) *mtproto.ImmutableUser {
	return &mtproto.ImmutableUser{User: &mtproto.UserData{
		Id:        id,
		FirstName: "user",
		Bot:       bot,
	}}
}

func withTopPeersStore(t *testing.T) *topPeersStore {
	t.Helper()
	store := &topPeersStore{data: make(map[string]string)}
	previous := persist.Default
	persist.Default = store
	t.Cleanup(func() { persist.Default = previous })
	return store
}

func TestContactsGetTopPeersRequiresAuthentication(t *testing.T) {
	got, err := (&ContactsCore{}).ContactsGetTopPeers(&mtproto.TLContactsGetTopPeers{})
	if got != nil || !errors.Is(err, mtproto.ErrAuthKeyUnregistered) {
		t.Fatalf("ContactsGetTopPeers() = (%+v, %v), want nil result and AUTH_KEY_UNREGISTERED", got, err)
	}
}

func TestContactsGetTopPeersReturnsDisabledState(t *testing.T) {
	store := withTopPeersStore(t)
	if err := store.Set(topPeersKey(42), `{"enabled":false}`); err != nil {
		t.Fatal(err)
	}

	got, err := newTopPeersTestCore(&topPeersUserClientStub{}).ContactsGetTopPeers(&mtproto.TLContactsGetTopPeers{
		Correspondents: true,
	})
	if err != nil || got == nil || got.GetPredicateName() != mtproto.Predicate_contacts_topPeersDisabled {
		t.Fatalf("disabled ContactsGetTopPeers() = (%+v, %v), want contacts.topPeersDisabled", got, err)
	}
	if len(got.GetCategories()) != 0 || len(got.GetUsers()) != 0 {
		t.Fatalf("disabled top peers = %+v, want empty categories/users", got)
	}
}

func TestContactsGetTopPeersHydratesContactPeersAndAppliesState(t *testing.T) {
	withTopPeersStore(t)
	const userID = int64(42)
	client := &topPeersUserClientStub{
		contacts: &userpb.Vector_ContactData{Datas: []*mtproto.ContactData{
			{ContactUserId: 9},
			{ContactUserId: 10},
			{ContactUserId: 11},
			{ContactUserId: 12},
		}},
		users: &userpb.Vector_ImmutableUser{Datas: []*mtproto.ImmutableUser{
			topPeersImmutableUser(userID, nil),
			topPeersImmutableUser(9, nil),
			topPeersImmutableUser(10, &mtproto.BotData{BotInlinePlaceholder: mtproto.MakeFlagsString("query")}),
			topPeersImmutableUser(11, &mtproto.BotData{BotHasMainApp: true}),
			topPeersImmutableUser(12, &mtproto.BotData{BotNochats: true}),
		}},
	}
	core := newTopPeersTestCore(client)
	state := &topPeersState{Hidden: map[string][]string{
		mtproto.Predicate_topPeerCategoryCorrespondents: {"2:9"},
	}}
	if err := saveTopPeersState(userID, state); err != nil {
		t.Fatal(err)
	}

	got, err := core.ContactsGetTopPeers(&mtproto.TLContactsGetTopPeers{
		Correspondents: true,
		BotsPm:         true,
		BotsInline:     true,
		BotsApp:        true,
		Offset:         0,
		Limit:          1,
	})
	if err != nil || got == nil {
		t.Fatalf("ContactsGetTopPeers() = (%+v, %v), want top peers", got, err)
	}
	if client.mutableRequest == nil || len(client.mutableRequest.GetTo()) != 1 || client.mutableRequest.GetTo()[0] != userID || len(client.mutableRequest.GetId()) != 5 {
		t.Fatalf("mutable-user request = %+v, want self plus four contacts", client.mutableRequest)
	}
	if len(got.GetCategories()) != 4 {
		t.Fatalf("categories = %d, want four requested categories", len(got.GetCategories()))
	}

	byCategory := make(map[string]*mtproto.TopPeerCategoryPeers)
	for _, category := range got.GetCategories() {
		byCategory[category.GetCategory().GetPredicateName()] = category
	}
	correspondents := byCategory[mtproto.Predicate_topPeerCategoryCorrespondents]
	if correspondents.GetCount() != 3 || len(correspondents.GetPeers()) != 1 || correspondents.GetPeers()[0].GetPeer().GetUserId() != 10 {
		t.Fatalf("correspondents = %+v, want visible count 3 and paged peer 10", correspondents)
	}
	botsPM := byCategory[mtproto.Predicate_topPeerCategoryBotsPM]
	if botsPM.GetCount() != 2 || len(botsPM.GetPeers()) != 1 || botsPM.GetPeers()[0].GetPeer().GetUserId() != 10 {
		t.Fatalf("bots_pm = %+v, want bot contacts 10/11 with first page 10", botsPM)
	}
	if byCategory[mtproto.Predicate_topPeerCategoryBotsInline].GetPeers()[0].GetPeer().GetUserId() != 10 {
		t.Fatalf("bots_inline = %+v, want inline bot 10", byCategory[mtproto.Predicate_topPeerCategoryBotsInline])
	}
	if byCategory[mtproto.Predicate_topPeerCategoryBotsApp].GetPeers()[0].GetPeer().GetUserId() != 11 {
		t.Fatalf("bots_app = %+v, want app bot 11", byCategory[mtproto.Predicate_topPeerCategoryBotsApp])
	}
	if len(got.GetUsers()) != 2 || got.GetUsers()[0].GetId() != 10 || got.GetUsers()[1].GetId() != 11 {
		t.Fatalf("response users = %+v, want paged users 10 and 11", got.GetUsers())
	}
}

func TestContactsGetTopPeersPropagatesDependencyErrors(t *testing.T) {
	withTopPeersStore(t)
	contactErr := errors.New("contact list unavailable")
	client := &topPeersUserClientStub{contactErr: contactErr}
	if got, err := newTopPeersTestCore(client).ContactsGetTopPeers(&mtproto.TLContactsGetTopPeers{Correspondents: true}); got != nil || err != contactErr {
		t.Fatalf("contact-list error = (%+v, %v), want %v", got, err, contactErr)
	}

	usersErr := errors.New("user lookup unavailable")
	client = &topPeersUserClientStub{
		contacts: &userpb.Vector_ContactData{Datas: []*mtproto.ContactData{{ContactUserId: 9}}},
		usersErr: usersErr,
	}
	if got, err := newTopPeersTestCore(client).ContactsGetTopPeers(&mtproto.TLContactsGetTopPeers{Correspondents: true}); got != nil || err != usersErr {
		t.Fatalf("mutable-user error = (%+v, %v), want %v", got, err, usersErr)
	}

	for name, client := range map[string]userclient.UserClient{
		"nil contact response": &topPeersUserClientStub{},
		"nil contact row":      &topPeersUserClientStub{contacts: &userpb.Vector_ContactData{Datas: []*mtproto.ContactData{nil}}},
		"nil user response": &topPeersUserClientStub{
			contacts: &userpb.Vector_ContactData{Datas: []*mtproto.ContactData{{ContactUserId: 9}}},
		},
		"missing user": &topPeersUserClientStub{
			contacts: &userpb.Vector_ContactData{Datas: []*mtproto.ContactData{{ContactUserId: 9}}},
			users:    &userpb.Vector_ImmutableUser{Datas: []*mtproto.ImmutableUser{topPeersImmutableUser(42, nil)}},
		},
		"nil user row": &topPeersUserClientStub{
			contacts: &userpb.Vector_ContactData{Datas: []*mtproto.ContactData{{ContactUserId: 9}}},
			users:    &userpb.Vector_ImmutableUser{Datas: []*mtproto.ImmutableUser{topPeersImmutableUser(42, nil), nil}},
		},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := newTopPeersTestCore(client).ContactsGetTopPeers(&mtproto.TLContactsGetTopPeers{Correspondents: true})
			if got != nil || (!errors.Is(err, mtproto.ErrInternalServerError) && !errors.Is(err, mtproto.ErrContactIdInvalid)) {
				t.Fatalf("malformed dependency = (%+v, %v), want fail-closed error", got, err)
			}
		})
	}

	if got, err := newTopPeersTestCore(&topPeersUserClientStub{}).ContactsGetTopPeers(nil); got != nil || !errors.Is(err, mtproto.ErrInputRequestInvalid) {
		t.Fatalf("nil request = (%+v, %v), want INPUT_REQUEST_INVALID", got, err)
	}
	if got, err := (&ContactsCore{MD: &metadata.RpcMetadata{UserId: 42}}).ContactsGetTopPeers(&mtproto.TLContactsGetTopPeers{}); got != nil || !errors.Is(err, mtproto.ErrInternalServerError) {
		t.Fatalf("missing DAO = (%+v, %v), want INTERNAL_SERVER_ERROR", got, err)
	}
}
