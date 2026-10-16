package core

import (
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/persist"
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

func withTopPeersStore(t *testing.T) *topPeersStore {
	t.Helper()
	store := &topPeersStore{data: make(map[string]string)}
	previous := persist.Default
	persist.Default = store
	t.Cleanup(func() { persist.Default = previous })
	return store
}

func newTopPeersTestCore() *ContactsCore {
	return &ContactsCore{MD: &metadata.RpcMetadata{UserId: 42}}
}

func TestContactsGetTopPeersRequiresAuthentication(t *testing.T) {
	got, err := (&ContactsCore{}).ContactsGetTopPeers(&mtproto.TLContactsGetTopPeers{})
	if got != nil || !errors.Is(err, mtproto.ErrAuthKeyUnregistered) {
		t.Fatalf("ContactsGetTopPeers() = (%+v, %v), want nil result and AUTH_KEY_UNREGISTERED", got, err)
	}
}

func TestContactsGetTopPeersValidatesRequestBoundsBeforeRankingProvider(t *testing.T) {
	withTopPeersStore(t)
	core := newTopPeersTestCore()
	for name, request := range map[string]*mtproto.TLContactsGetTopPeers{
		"nil request":     nil,
		"negative offset": {Offset: -1},
		"negative limit":  {Limit: -1},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := core.ContactsGetTopPeers(request)
			wantErr := mtproto.ErrInputRequestInvalid
			if name == "negative offset" {
				wantErr = mtproto.ErrOffsetInvalid
			} else if name == "negative limit" {
				wantErr = mtproto.ErrLimitInvalid
			}
			if got != nil || !errors.Is(err, wantErr) {
				t.Fatalf("ContactsGetTopPeers(%s) = (%+v, %v), want %v", name, got, err, wantErr)
			}
		})
	}
}

func TestContactsGetTopPeersReturnsDisabledStateWithoutRankingProvider(t *testing.T) {
	store := withTopPeersStore(t)
	if err := store.Set(topPeersKey(42), `{"enabled":false}`); err != nil {
		t.Fatal(err)
	}

	got, err := newTopPeersTestCore().ContactsGetTopPeers(&mtproto.TLContactsGetTopPeers{
		Correspondents: true,
		Groups:         true,
		Channels:       true,
		Offset:         10,
		Limit:          20,
		Hash:           123,
	})
	if err != nil || got == nil || got.GetPredicateName() != mtproto.Predicate_contacts_topPeersDisabled {
		t.Fatalf("disabled ContactsGetTopPeers() = (%+v, %v), want contacts.topPeersDisabled", got, err)
	}
	if len(got.GetCategories()) != 0 || len(got.GetChats()) != 0 || len(got.GetUsers()) != 0 {
		t.Fatalf("disabled top peers = %+v, want empty categories/chats/users", got)
	}
}

func TestContactsGetTopPeersFailsClosedWithoutAuthoritativeRanking(t *testing.T) {
	withTopPeersStore(t)
	core := newTopPeersTestCore()
	for name, request := range map[string]*mtproto.TLContactsGetTopPeers{
		"correspondents": {Correspondents: true, Offset: 0, Limit: 20},
		"all categories with hash": {
			Correspondents: true,
			BotsPm:         true,
			BotsInline:     true,
			PhoneCalls:     true,
			ForwardUsers:   true,
			ForwardChats:   true,
			Groups:         true,
			Channels:       true,
			BotsApp:        true,
			BotsGuestchat:  true,
			Offset:         5,
			Limit:          10,
			Hash:           987654321,
		},
		"no category flags": {Offset: 0, Limit: 0, Hash: 1},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := core.ContactsGetTopPeers(request)
			if got != nil || !errors.Is(err, mtproto.ErrMethodNotImpl) {
				t.Fatalf("ContactsGetTopPeers() = (%+v, %v), want nil result and METHOD_NOT_IMPL", got, err)
			}
		})
	}
}
