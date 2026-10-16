package channelview

import (
	"errors"
	"os"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
)

func TestRecentLocationsForInputPeerUsesGeoHistoryAndReadAuthorization(t *testing.T) {
	dsn := os.Getenv("APIFULL_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("APIFULL_POSTGRES_DSN is not configured")
	}
	if err := domain.OpenPostgresReadOnly(dsn); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = domain.Close() })
	cleanupDB, err := persist.OpenPostgresDB(dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cleanupDB.Close() })

	channelID := time.Now().UnixNano()
	owner, member, outsider := channelID+1, channelID+2, channelID+3
	if err = domain.SaveChannel(domain.Channel{
		ID: channelID, AccessHash: channelID, Creator: owner, Title: "recent-locations-test", Broadcast: true,
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := domain.DeleteChannel(owner, channelID); err != nil {
			t.Errorf("clean recent locations fixture: %v", err)
		}
	})
	if err = domain.JoinChannel(channelID, member); err != nil {
		t.Fatal(err)
	}

	geo := mtproto.MakeTLMessageMediaGeo(&mtproto.MessageMedia{}).To_MessageMedia()
	geoRow, err := domain.InsertChannelMessageWithContentAndRandomID(channelID, owner, 0, "geo", 0, 0, 0,
		domain.ChannelMessageContent{Media: geo}, "")
	if err != nil {
		t.Fatal(err)
	}
	for i := int32(0); i < recentLocationsPageSize; i++ {
		if _, err = domain.InsertChannelMessage(channelID, owner, 0, "plain"); err != nil {
			t.Fatal(err)
		}
	}

	peer := mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{
		ChannelId: channelID, AccessHash: channelID,
	}).To_InputPeer()
	got, err := RecentLocationsForInputPeer(member, peer, 1)
	if err != nil || got == nil || len(got.GetMessages()) != 1 || got.GetMessages()[0].GetId() != geoRow.MessageID ||
		got.GetMessages()[0].GetMedia().GetPredicateName() != mtproto.Predicate_messageMediaGeo {
		t.Fatalf("member recent locations: result=%+v err=%v, want geo message %d", got, err, geoRow.MessageID)
	}
	if len(got.GetChats()) != 1 || got.GetChats()[0].GetId() != channelID || got.GetChats()[0].GetLeft() {
		t.Fatalf("member recent locations chat: %+v", got.GetChats())
	}

	if _, err = RecentLocationsForInputPeer(outsider, peer, 1); !errors.Is(err, mtproto.ErrChannelPrivate) {
		t.Fatalf("private channel outsider: got %v", err)
	}
	if err = domain.SaveChannel(domain.Channel{
		ID: channelID, AccessHash: channelID, Creator: owner, Title: "recent-locations-test", Broadcast: true,
		Username: "recent-locations-public",
	}); err != nil {
		t.Fatal(err)
	}
	public, err := RecentLocationsForInputPeer(outsider, peer, 1)
	if err != nil || public == nil || len(public.GetMessages()) != 1 || len(public.GetChats()) != 1 || !public.GetChats()[0].GetLeft() {
		t.Fatalf("public channel outsider: result=%+v err=%v", public, err)
	}
	if err = domain.EditChannelBanned(channelID, owner, outsider, &domain.ChannelBannedRights{ViewMessages: true}, time.Now().Unix()); err != nil {
		t.Fatal(err)
	}
	if _, err = RecentLocationsForInputPeer(outsider, peer, 1); !errors.Is(err, mtproto.ErrUserBannedInChannel) {
		t.Fatalf("banned public channel outsider: got %v", err)
	}
}
