package core

import (
	"os"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/persist"
	"github.com/teamgram/teamgram-server/app/bff/sponsoredmessages/internal/svc"
)

func TestSponsoredInventoryHandlersReadPostgres(t *testing.T) {
	dsn := os.Getenv("APIFULL_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("APIFULL_POSTGRES_DSN must point to an isolated PostgreSQL 18 database")
	}
	previous := persist.Default
	if err := persist.OpenPostgresReadOnly(dsn); err != nil {
		t.Skipf("PostgreSQL schema unavailable: %v", err)
	}
	t.Cleanup(func() {
		_ = persist.DeleteSponsoredInventory(t.Context(), "handler-test", "inputPeerSelf:0:0:0")
		_ = persist.ClosePostgres()
		persist.Default = previous
	})
	peers := mtproto.MakeTLContactsSponsoredPeers(&mtproto.Contacts_SponsoredPeers{
		Peers: []*mtproto.SponsoredPeer{{RandomId: []byte("peer")}},
	}).To_Contacts_SponsoredPeers()
	if err := persist.StoreSponsoredPeers(t.Context(), "handler-test", peers); err != nil {
		t.Fatal(err)
	}
	messages := mtproto.MakeTLMessagesSponsoredMessages(&mtproto.Messages_SponsoredMessages{
		Messages: []*mtproto.SponsoredMessage{{RandomId: []byte("message"), Message: "ad"}},
	}).To_Messages_SponsoredMessages()
	if err := persist.StoreSponsoredMessages(t.Context(), "inputPeerSelf:0:0:0", messages); err != nil {
		t.Fatal(err)
	}
	c := New(t.Context(), &svc.ServiceContext{})
	c.MD = &metadata.RpcMetadata{UserId: 42}
	gotPeers, err := c.ContactsGetSponsoredPeers(&mtproto.TLContactsGetSponsoredPeers{Q: "handler-test"})
	if err != nil || gotPeers == nil || len(gotPeers.GetPeers()) != 1 {
		t.Fatalf("peers = (%v, %v)", gotPeers, err)
	}
	peer := mtproto.MakeTLInputPeerSelf(&mtproto.InputPeer{}).To_InputPeer()
	gotMessages, err := c.MessagesGetSponsoredMessages(&mtproto.TLMessagesGetSponsoredMessages{Peer: peer})
	if err != nil || gotMessages == nil || len(gotMessages.GetMessages()) != 1 {
		t.Fatalf("messages = (%v, %v)", gotMessages, err)
	}
	missingPeer := mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 1}).To_InputPeer()
	if _, err = c.MessagesGetSponsoredMessages(&mtproto.TLMessagesGetSponsoredMessages{Peer: missingPeer}); err != mtproto.ErrMethodNotImpl {
		t.Fatalf("missing inventory error = %v, want METHOD_NOT_IMPL", err)
	}
}
