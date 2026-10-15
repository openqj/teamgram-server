package core

import (
	"errors"
	"os"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
)

func TestPaidReactionPrivacyUsesLocalPostgresStoreWhenBusinessProviderUnset(t *testing.T) {
	previous := persist.Default
	persist.Use(&localPrivacyStore{values: map[string]string{}})
	t.Cleanup(func() { persist.Use(previous) })

	core := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 991}}
	peer := &mtproto.InputPeer{PredicateName: mtproto.Predicate_inputPeerUser, UserId: 992}
	privacy := mtproto.MakeTLPaidReactionPrivacyPeer(&mtproto.PaidReactionPrivacy{Peer: peer}).To_PaidReactionPrivacy()

	result, err := core.MessagesTogglePaidReactionPrivacy(&mtproto.TLMessagesTogglePaidReactionPrivacy{
		Peer: peer, MsgId: 7, Private_PAIDREACTIONPRIVACY: privacy, Private_BOOL: mtproto.BoolFalse,
	})
	if err != nil || result != mtproto.BoolTrue {
		t.Fatalf("toggle local privacy = (%v, %v), want BoolTrue", result, err)
	}

	updates, err := (&ApiFullCore{MD: &metadata.RpcMetadata{UserId: 991}}).MessagesGetPaidReactionPrivacy(&mtproto.TLMessagesGetPaidReactionPrivacy{})
	if err != nil || updates == nil || updates.GetPredicateName() != mtproto.Predicate_updates || len(updates.GetUpdates()) != 1 {
		t.Fatalf("get local privacy = (%v, %v), want one canonical updates item", updates, err)
	}
	item := updates.GetUpdates()[0]
	if item.GetPredicateName() != mtproto.Predicate_updatePaidReactionPrivacy || item.GetPrivate_PAIDREACTIONPRIVACY() == nil || item.GetPrivate_PAIDREACTIONPRIVACY().GetPredicateName() != mtproto.Predicate_paidReactionPrivacyPeer || item.GetPrivate_BOOL() != mtproto.BoolFalse {
		t.Fatalf("privacy update = %v, want peer privacy and BoolFalse", item)
	}
}

func TestPaidReactionPrivacyRejectsInvalidLocalRequestBeforePersistence(t *testing.T) {
	store := &localPrivacyStore{values: map[string]string{}}
	previous := persist.Default
	persist.Use(store)
	t.Cleanup(func() { persist.Use(previous) })

	core := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: 992}}
	if result, err := core.MessagesTogglePaidReactionPrivacy(&mtproto.TLMessagesTogglePaidReactionPrivacy{MsgId: 7}); result != nil || !errors.Is(err, mtproto.ErrInputRequestInvalid) {
		t.Fatalf("invalid toggle = (%v, %v), want INPUT_REQUEST_INVALID", result, err)
	}
	if len(store.values) != 0 {
		t.Fatalf("invalid request wrote local privacy state: %v", store.values)
	}
}

func TestPaidReactionPrivacyUsesDurablePostgresStore(t *testing.T) {
	if os.Getenv("APIFULL_POSTGRES_DSN") == "" || !persist.PostgresEnabled() {
		t.Skip("APIFULL_POSTGRES_DSN with the APIFull PostgreSQL store is required")
	}
	dsn := os.Getenv("APIFULL_POSTGRES_DSN")
	db, err := persist.OpenPostgresDB(dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	const userID int64 = 991991
	key := paidReactionPrivacyKey(userID)
	if _, err = db.Exec(`DELETE FROM apifull_kv WHERE k=$1`, key); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM apifull_kv WHERE k=$1`, key) })

	peer := &mtproto.InputPeer{PredicateName: mtproto.Predicate_inputPeerUser, UserId: 992992}
	privacy := mtproto.MakeTLPaidReactionPrivacyPeer(&mtproto.PaidReactionPrivacy{Peer: peer}).To_PaidReactionPrivacy()
	core := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: userID}}
	if result, err := core.MessagesTogglePaidReactionPrivacy(&mtproto.TLMessagesTogglePaidReactionPrivacy{
		Peer: peer, MsgId: 17, Private_PAIDREACTIONPRIVACY: privacy, Private_BOOL: mtproto.BoolFalse,
	}); err != nil || result != mtproto.BoolTrue {
		t.Fatalf("toggle PostgreSQL privacy = (%v, %v), want BoolTrue", result, err)
	}
	var raw string
	if err = db.QueryRow(`SELECT v FROM apifull_kv WHERE k=$1`, key).Scan(&raw); err != nil {
		t.Fatalf("read durable privacy state: %v", err)
	}
	if raw == "" {
		t.Fatal("durable privacy state is empty")
	}
	updates, err := core.MessagesGetPaidReactionPrivacy(&mtproto.TLMessagesGetPaidReactionPrivacy{})
	if err != nil || updates == nil || updates.GetPredicateName() != mtproto.Predicate_updates || len(updates.GetUpdates()) != 1 {
		t.Fatalf("get PostgreSQL privacy = (%v, %v), want one canonical update", updates, err)
	}
	item := updates.GetUpdates()[0]
	if item.GetPredicateName() != mtproto.Predicate_updatePaidReactionPrivacy || item.GetPrivate_PAIDREACTIONPRIVACY() == nil || item.GetPrivate_PAIDREACTIONPRIVACY().GetPredicateName() != mtproto.Predicate_paidReactionPrivacyPeer || item.GetPrivate_BOOL() != mtproto.BoolFalse {
		t.Fatalf("durable privacy update = %v, want peer privacy and BoolFalse", item)
	}
}

type localPrivacyStore struct{ values map[string]string }

func (s *localPrivacyStore) Get(key string) (string, error) { return s.values[key], nil }

func (s *localPrivacyStore) Set(key, value string) error {
	s.values[key] = value
	return nil
}

func (s *localPrivacyStore) Update(key string, fn func(string) (string, error)) error {
	next, err := fn(s.values[key])
	if err != nil {
		return err
	}
	s.values[key] = next
	return nil
}
