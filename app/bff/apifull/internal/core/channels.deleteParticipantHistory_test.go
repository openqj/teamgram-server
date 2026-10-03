package core

import (
	"errors"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
)

func TestChannelsDeleteParticipantHistoryDeletesStoredMessages(t *testing.T) {
	channelID := time.Now().UnixNano()
	ownerID := channelID%1_000_000_000 + 6_000_000_000
	adminID, memberID := ownerID+1, ownerID+2
	if err := domain.SaveChannel(domain.Channel{
		ID: channelID, AccessHash: channelID, Creator: ownerID, Title: "delete-participant-history-test", Megagroup: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := domain.InviteChannelMembers(channelID, ownerID, []int64{adminID, memberID}); err != nil {
		t.Fatal(err)
	}
	if err := domain.EditChannelAdmin(channelID, ownerID, adminID, &domain.ChannelAdminRights{DeleteMessages: true}, "moderator"); err != nil {
		t.Fatal(err)
	}
	inputChannel := mtproto.MakeTLInputChannel(&mtproto.InputChannel{ChannelId: channelID, AccessHash: channelID}).To_InputChannel()
	participant := mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: ownerID, AccessHash: ownerID}).To_InputPeer()
	coreFor := func(userID int64) *ApiFullCore {
		return &ApiFullCore{MD: &metadata.RpcMetadata{UserId: userID}}
	}
	for i := int64(0); i < 3; i++ {
		if _, err := domain.InsertChannelMessage(channelID, ownerID, 100+i, "participant-history"); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := coreFor(memberID).ChannelsDeleteParticipantHistory(&mtproto.TLChannelsDeleteParticipantHistory{
		Channel: inputChannel, Participant: participant,
	}); !errors.Is(err, mtproto.ErrChatAdminRequired) {
		t.Fatalf("member without delete_messages right: got %v", err)
	}
	rows, err := domain.ListChannelMessages(ownerID, channelID, 0, 20)
	if err != nil || len(rows) != 3 {
		t.Fatalf("history after denied request: rows=%d err=%v", len(rows), err)
	}

	affected, err := coreFor(adminID).ChannelsDeleteParticipantHistory(&mtproto.TLChannelsDeleteParticipantHistory{
		Channel: inputChannel, Participant: participant,
	})
	if err != nil || affected == nil || affected.GetPts() != 6 || affected.GetPtsCount() != 3 || affected.GetOffset() != 0 {
		t.Fatalf("delete participant history: result=%+v err=%v", affected, err)
	}
	if err = affected.To_MessagesAffectedHistory().Encode(mtproto.NewEncodeBuf(512), 229); err != nil {
		t.Fatalf("encode affected history: %v", err)
	}
	rows, err = domain.ListChannelMessages(ownerID, channelID, 0, 20)
	if err != nil || len(rows) != 0 {
		t.Fatalf("history after deletion: rows=%d err=%v", len(rows), err)
	}
	row, err := domain.InsertChannelMessage(channelID, ownerID, 200, "after-participant-history-delete")
	if err != nil || row.MessageID != 4 || row.Pts != 7 {
		t.Fatalf("message sequence after deletion: row=%+v err=%v", row, err)
	}
}
