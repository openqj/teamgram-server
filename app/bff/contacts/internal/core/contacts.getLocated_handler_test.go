package core

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/persist"
	"github.com/teamgram/teamgram-server/app/bff/contacts/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/contacts/internal/svc"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

type locatedStore struct {
	data map[string]string
}

func (s *locatedStore) Get(key string) (string, error) { return s.data[key], nil }

func (s *locatedStore) Set(key, value string) error {
	s.data[key] = value
	return nil
}

type locatedUserClientStub struct {
	userclient.UserClient
	contacts       *userpb.Vector_ContactData
	contactErr     error
	contactRequest *userpb.TLUserGetContactList
	users          *userpb.Vector_ImmutableUser
	usersErr       error
}

func (s *locatedUserClientStub) UserGetContactList(_ context.Context, in *userpb.TLUserGetContactList) (*userpb.Vector_ContactData, error) {
	s.contactRequest = in
	return s.contacts, s.contactErr
}

func (s *locatedUserClientStub) UserGetMutableUsers(context.Context, *userpb.TLUserGetMutableUsers) (*userpb.Vector_ImmutableUser, error) {
	return s.users, s.usersErr
}

func newLocatedTestCore(userID int64, client userclient.UserClient) *ContactsCore {
	ctx := context.Background()
	return &ContactsCore{
		ctx:    ctx,
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{UserClient: client}},
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: userID},
	}
}

func withLocatedStore(t *testing.T, store *locatedStore) {
	t.Helper()
	previous := persist.Default
	persist.Default = store
	t.Cleanup(func() { persist.Default = previous })
}

func locatedRequest(selfExpires *int32) *mtproto.TLContactsGetLocated {
	return &mtproto.TLContactsGetLocated{
		GeoPoint: mtproto.MakeTLInputGeoPoint(&mtproto.InputGeoPoint{
			Lat:  31.2304,
			Long: 121.4737,
		}).To_InputGeoPoint(),
		SelfExpires: func() *wrapperspb.Int32Value {
			if selfExpires == nil {
				return nil
			}
			return wrapperspb.Int32(*selfExpires)
		}(),
	}
}

func locatedPeerIDs(updates *mtproto.Updates) []int64 {
	ids := []int64{}
	for _, update := range updates.GetUpdates() {
		for _, peer := range update.GetPeers() {
			if peer.GetPeer() != nil {
				ids = append(ids, peer.GetPeer().GetUserId())
			}
		}
	}
	return ids
}

func TestContactsGetLocatedShowsOnlyMutualContacts(t *testing.T) {
	const caller, mutual, oneWay, stranger int64 = 42, 50, 60, 70
	expires := int32(time.Now().Add(time.Hour).Unix())
	store := &locatedStore{data: map[string]string{}}
	withLocatedStore(t, store)
	seed, err := json.Marshal([]locatedFix{
		{UserId: mutual, Lat: 31.24, Long: 121.48, Expires: expires},
		{UserId: oneWay, Lat: 31.25, Long: 121.49, Expires: expires},
		{UserId: stranger, Lat: 31.26, Long: 121.50, Expires: expires},
	})
	if err != nil {
		t.Fatal(err)
	}
	store.data[locatedIndexKey] = string(seed)
	client := &locatedUserClientStub{
		contacts: &userpb.Vector_ContactData{Datas: []*mtproto.ContactData{
			{ContactUserId: mutual, MutualContact: true},
			{ContactUserId: oneWay, MutualContact: false},
		}},
		users: &userpb.Vector_ImmutableUser{},
	}

	updates, err := newLocatedTestCore(caller, client).ContactsGetLocated(locatedRequest(nil))
	if err != nil {
		t.Fatal(err)
	}
	if got := locatedPeerIDs(updates); !reflect.DeepEqual(got, []int64{mutual}) {
		t.Fatalf("located peer IDs = %v, want only mutual contact %d", got, mutual)
	}
	if client.contactRequest == nil || client.contactRequest.GetUserId() != caller {
		t.Fatalf("contact lookup = %+v, want caller %d", client.contactRequest, caller)
	}

	live, err := loadLocatedIndex()
	if err != nil {
		t.Fatal(err)
	}
	if got := []int64{live[0].UserId, live[1].UserId, live[2].UserId}; !reflect.DeepEqual(got, []int64{mutual, oneWay, stranger}) {
		t.Fatalf("live index = %v, want all publishers retained", got)
	}
}

func TestContactsGetLocatedKeepsOwnLocationVisible(t *testing.T) {
	const caller int64 = 42
	store := &locatedStore{data: map[string]string{}}
	withLocatedStore(t, store)
	client := &locatedUserClientStub{
		contacts: &userpb.Vector_ContactData{},
		users:    &userpb.Vector_ImmutableUser{},
	}
	expires := int32(time.Now().Add(time.Hour).Unix())

	updates, err := newLocatedTestCore(caller, client).ContactsGetLocated(locatedRequest(&expires))
	if err != nil {
		t.Fatal(err)
	}
	if len(locatedPeerIDs(updates)) != 0 {
		t.Fatalf("own location returned as a foreign peer: %v", locatedPeerIDs(updates))
	}
	if len(updates.GetUpdates()) != 1 || len(updates.GetUpdates()[0].GetPeers()) != 1 || updates.GetUpdates()[0].GetPeers()[0].GetPeer() != nil {
		t.Fatalf("own location update = %+v, want one peerSelfLocated", updates)
	}

	live, err := loadLocatedIndex()
	if err != nil {
		t.Fatal(err)
	}
	if len(live) != 1 || live[0].UserId != caller || live[0].Expires != expires {
		t.Fatalf("stored own location = %+v, want caller %d with expiry %d", live, caller, expires)
	}
}

func TestContactsGetLocatedFailsClosedWhenContactLookupFails(t *testing.T) {
	store := &locatedStore{data: map[string]string{locatedIndexKey: `[{"user_id":50,"lat":31.24,"long":121.48,"expires":2147483647}]`}}
	withLocatedStore(t, store)
	lookupErr := errors.New("contact lookup failed")
	client := &locatedUserClientStub{contactErr: lookupErr}

	updates, err := newLocatedTestCore(42, client).ContactsGetLocated(locatedRequest(nil))
	if updates != nil || err != lookupErr {
		t.Fatalf("ContactsGetLocated() = (%+v, %v), want nil result and %v", updates, err, lookupErr)
	}
	if got := store.data[locatedIndexKey]; got != `[{"user_id":50,"lat":31.24,"long":121.48,"expires":2147483647}]` {
		t.Fatalf("live index changed after failed contact lookup: %q", got)
	}
}
