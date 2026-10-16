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

func TestMessageViewsForInputPeerUsesPersistentReadCursors(t *testing.T) {
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
	ownerID, memberID, outsiderID := channelID+1, channelID+2, channelID+3
	if err = domain.SaveChannel(domain.Channel{
		ID: channelID, AccessHash: channelID, Creator: ownerID, Title: "message-views-postgres", Broadcast: true,
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := domain.DeleteChannel(ownerID, channelID); err != nil {
			t.Errorf("clean message views fixture: %v", err)
		}
	})
	if err = domain.JoinChannel(channelID, memberID); err != nil {
		t.Fatal(err)
	}
	first, err := domain.InsertChannelMessage(channelID, ownerID, time.Now().Unix(), "first")
	if err != nil {
		t.Fatal(err)
	}
	second, err := domain.InsertChannelMessage(channelID, ownerID, time.Now().Unix(), "second")
	if err != nil {
		t.Fatal(err)
	}
	ptsBefore, err := domain.ChannelMessagePTS(channelID)
	if err != nil {
		t.Fatal(err)
	}
	var eventsBefore int
	if err = cleanupDB.QueryRow(`SELECT COUNT(*) FROM apifull_channel_event WHERE channel_id=?`, channelID).Scan(&eventsBefore); err != nil {
		t.Fatal(err)
	}
	peer := mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{
		ChannelId: channelID, AccessHash: channelID,
	}).To_InputPeer()

	views, err := MessageViewsForInputPeer(ownerID, peer, []int32{first.MessageID}, false)
	if err != nil || len(views) != 1 || views[0].GetViews() == nil || views[0].GetViews().GetValue() != 0 {
		t.Fatalf("creator initial views = %v, err=%v; want one persisted zero count", views, err)
	}
	if _, err = MessageViewsForInputPeer(memberID, peer, []int32{first.MessageID}, true); err != nil {
		t.Fatalf("member increment: %v", err)
	}
	if readMax, readErr := domain.ChannelReadMaxID(memberID, channelID); readErr != nil || readMax != first.MessageID {
		t.Fatalf("member read cursor = %d, err=%v; want %d", readMax, readErr, first.MessageID)
	}

	views, err = MessageViewsForInputPeer(memberID, peer, []int32{first.MessageID}, true)
	if err != nil || len(views) != 1 || views[0].GetViews() == nil || views[0].GetViews().GetValue() != 1 {
		t.Fatalf("repeated member increment = %v, err=%v; want one deduplicated view", views, err)
	}
	if _, err = MessageViewsForInputPeer(ownerID, peer, []int32{second.MessageID}, true); err != nil {
		t.Fatalf("creator increment: %v", err)
	}
	views, err = MessageViewsForInputPeer(ownerID, peer, []int32{second.MessageID, first.MessageID, second.MessageID}, false)
	if err != nil || len(views) != 3 {
		t.Fatalf("ordered channel views = %v, err=%v; want three entries", views, err)
	}
	for i, view := range views {
		if view == nil || view.GetViews() == nil || view.GetViews().GetValue() != 2 {
			t.Fatalf("ordered channel view[%d] = %v; want deduplicated count 2", i, view)
		}
	}
	ownerReadMax, err := domain.ChannelReadMaxID(ownerID, channelID)
	if err != nil || ownerReadMax != second.MessageID {
		t.Fatalf("creator read cursor = %d, err=%v; want %d", ownerReadMax, err, second.MessageID)
	}
	ptsAfter, err := domain.ChannelMessagePTS(channelID)
	if err != nil || ptsAfter != ptsBefore {
		t.Fatalf("message pts changed from %d to %d (err=%v) during view increments", ptsBefore, ptsAfter, err)
	}
	var eventsAfter int
	if err = cleanupDB.QueryRow(`SELECT COUNT(*) FROM apifull_channel_event WHERE channel_id=?`, channelID).Scan(&eventsAfter); err != nil {
		t.Fatal(err)
	}
	if eventsAfter != eventsBefore {
		t.Fatalf("channel events changed from %d to %d during view increments", eventsBefore, eventsAfter)
	}

	beforeInvalid := ownerReadMax
	if _, err = MessageViewsForInputPeer(ownerID, peer, []int32{second.MessageID + 1}, true); !errors.Is(err, mtproto.ErrMessageIdInvalid) {
		t.Fatalf("missing channel message error = %v, want MESSAGE_ID_INVALID", err)
	}
	afterInvalid, readErr := domain.ChannelReadMaxID(ownerID, channelID)
	if readErr != nil || afterInvalid != beforeInvalid {
		t.Fatalf("invalid increment changed creator cursor from %d to %d (err=%v)", beforeInvalid, afterInvalid, readErr)
	}
	if _, err = MessageViewsForInputPeer(outsiderID, peer, []int32{first.MessageID}, false); !errors.Is(err, mtproto.ErrUserNotParticipant) {
		t.Fatalf("outsider views error = %v, want USER_NOT_PARTICIPANT", err)
	}
	badHash := mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{ChannelId: channelID, AccessHash: channelID + 1}).To_InputPeer()
	if _, err = MessageViewsForInputPeer(memberID, badHash, []int32{first.MessageID}, false); !errors.Is(err, mtproto.ErrChannelInvalid) {
		t.Fatalf("bad access hash error = %v, want CHANNEL_INVALID", err)
	}
}
