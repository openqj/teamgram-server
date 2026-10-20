package persist

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestSponsoredInventoryPostgresRoundTrip(t *testing.T) {
	dsn := os.Getenv("APIFULL_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("APIFULL_POSTGRES_DSN must point to an isolated PostgreSQL 18 database")
	}
	previous := Default
	if err := OpenPostgresReadOnly(dsn); err != nil {
		t.Skipf("PostgreSQL schema unavailable: %v", err)
	}
	const (
		queryKey  = "persist-inventory-test"
		peerKey   = "inputPeerSelf:0:0:0-persist-inventory-test"
		messageID = int32(117)
	)
	t.Cleanup(func() {
		_, _ = Default.(*postgresStore).db.Exec(`DELETE FROM apifull_sponsored_inventory WHERE query = $1 OR peer_key = $2`, queryKey, peerKey)
		_ = ClosePostgres()
		Default = previous
	})

	peers := mtproto.MakeTLContactsSponsoredPeers(&mtproto.Contacts_SponsoredPeers{
		Peers: []*mtproto.SponsoredPeer{{RandomId: []byte("peer")}},
	}).To_Contacts_SponsoredPeers()
	peerPayload, err := json.Marshal(peers)
	if err != nil {
		t.Fatal(err)
	}
	messages := mtproto.MakeTLMessagesSponsoredMessages(&mtproto.Messages_SponsoredMessages{
		Messages: []*mtproto.SponsoredMessage{{RandomId: []byte("message"), Message: "ad"}},
	}).To_Messages_SponsoredMessages()
	db := Default.(*postgresStore).db
	if _, err = db.Exec(`INSERT INTO apifull_sponsored_inventory (inventory_type, query, payload)
		VALUES ('peers', $1, $2::jsonb)`, queryKey, string(peerPayload)); err != nil {
		t.Fatal(err)
	}
	if err = StoreSponsoredMessages(t.Context(), peerKey, 0, messages); err != nil {
		t.Fatal(err)
	}
	contextMessages := mtproto.MakeTLMessagesSponsoredMessages(&mtproto.Messages_SponsoredMessages{
		Messages: []*mtproto.SponsoredMessage{{RandomId: []byte("context-message"), Message: "context ad"}},
	}).To_Messages_SponsoredMessages()
	if err = StoreSponsoredMessages(t.Context(), peerKey, messageID, contextMessages); err != nil {
		t.Fatal(err)
	}

	gotPeers, found, err := LoadSponsoredPeers(t.Context(), queryKey)
	if err != nil || !found || gotPeers == nil || gotPeers.GetPredicateName() != "contacts_sponsoredPeers" || len(gotPeers.GetPeers()) != 1 {
		t.Fatalf("peers = (%v, %v, %v)", gotPeers, found, err)
	}
	gotMessages, found, err := LoadSponsoredMessages(t.Context(), peerKey, 0)
	if err != nil || !found || gotMessages == nil || gotMessages.GetPredicateName() != "messages_sponsoredMessages" || len(gotMessages.GetMessages()) != 1 {
		t.Fatalf("messages = (%v, %v, %v)", gotMessages, found, err)
	}
	gotContextMessages, found, err := LoadSponsoredMessages(t.Context(), peerKey, messageID)
	if err != nil || !found || gotContextMessages == nil || len(gotContextMessages.GetMessages()) != 1 || gotContextMessages.GetMessages()[0].GetMessage() != "context ad" {
		t.Fatalf("context messages = (%v, %v, %v)", gotContextMessages, found, err)
	}
	if _, found, err = LoadSponsoredMessages(t.Context(), peerKey, messageID+1); err != nil || found {
		t.Fatalf("missing context messages = (%v, %v), want no row", err, found)
	}
	if _, found, err = LoadSponsoredMessages(t.Context(), "missing", 0); err != nil || found {
		t.Fatalf("missing messages = (%v, %v), want no row", err, found)
	}
}
