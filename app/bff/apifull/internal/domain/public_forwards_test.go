package domain

import (
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

func TestPublicForwardMessagePostgresRoundTrip(t *testing.T) {
	requirePaymentLedgerDB(t)
	channelID := time.Now().UnixNano()
	ownerID := channelID + 1
	if err := SaveChannel(Channel{ID: channelID, AccessHash: channelID + 2, Creator: ownerID, Broadcast: true, Title: "public-forward"}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM apifull_public_forward_message WHERE source_channel_id=$1`, channelID)
		_, _ = db.Exec(`DELETE FROM apifull_channel_message WHERE channel_id=$1`, channelID)
		_, _ = db.Exec(`DELETE FROM apifull_channel WHERE id=$1`, channelID)
	})
	message := mtproto.MakeTLMessage(&mtproto.Message{
		Id:      71,
		Date:    int32(time.Now().Unix()),
		Message: "forwarded",
		PeerId:  mtproto.MakePeerChannel(channelID + 10),
	}).To_Message()
	if err := SavePublicForwardMessage(channelID, 11, message); err != nil {
		t.Fatal(err)
	}
	if err := SavePublicForwardMessage(channelID, 11, message); err != nil {
		t.Fatal(err)
	}
	rows, count, next, err := ListPublicForwardMessages(channelID, 11, "", 10)
	if err != nil || count != 1 || len(rows) != 1 || next != "" {
		t.Fatalf("list = rows=%d count=%d next=%q err=%v", len(rows), count, next, err)
	}
	if rows[0].Message.GetId() != 71 || rows[0].Message.GetMessage() != "forwarded" || rows[0].DestinationPeerType != mtproto.PEER_CHANNEL {
		t.Fatalf("row = %+v", rows[0])
	}
	if _, _, _, err = ListPublicForwardMessages(channelID, 11, "bad-cursor", 10); err == nil {
		t.Fatal("invalid cursor unexpectedly accepted")
	}
	if err = SavePublicForwardMessage(channelID, 11, nil); err == nil {
		t.Fatal("nil message unexpectedly accepted")
	}
}

func TestPublicForwardStoryPostgresRoundTrip(t *testing.T) {
	requirePaymentLedgerDB(t)
	ownerID := time.Now().UnixNano()
	peer := mtproto.MakePeerChannel(ownerID + 10)
	story := mtproto.MakeTLStoryItem(&mtproto.StoryItem{Id: 19, Date: int32(time.Now().Unix()), Out: true}).To_StoryItem()
	if err := SavePublicForwardStory(ownerID, 7, peer, story); err != nil {
		t.Fatal(err)
	}
	if err := SavePublicForwardStory(ownerID, 7, peer, story); err != nil {
		t.Fatal(err)
	}
	rows, count, next, err := ListPublicForwardStories(ownerID, 7, "", 10)
	if err != nil || count != 1 || len(rows) != 1 || next != "" {
		t.Fatalf("list story = rows=%d count=%d next=%q err=%v", len(rows), count, next, err)
	}
	if rows[0].Story.GetId() != story.GetId() || rows[0].Peer.GetChannelId() != peer.GetChannelId() {
		t.Fatalf("story row = %+v", rows[0])
	}
	if _, _, _, err = ListPublicForwardStories(ownerID, 7, "bad-cursor", 10); err == nil {
		t.Fatal("invalid story cursor unexpectedly accepted")
	}
}
