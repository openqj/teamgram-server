package core

import (
	"errors"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/channelview"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
)

func TestChannelsGetMessagesAuthorization(t *testing.T) {
	channelID := time.Now().UnixNano()
	owner := channelID%1_000_000_000 + 1_000_000_000
	memberID, outsiderID := owner+1, owner+2
	if err := domain.SaveChannel(domain.Channel{
		ID: channelID, AccessHash: channelID, Creator: owner, Title: "get-messages-auth-test", Megagroup: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := domain.JoinChannel(channelID, memberID); err != nil {
		t.Fatal(err)
	}
	if _, err := channelview.Post(owner, channelID, "member-only message", 0); err != nil {
		t.Fatal(err)
	}
	inputChannel := mtproto.MakeTLInputChannel(&mtproto.InputChannel{
		ChannelId: channelID, AccessHash: channelID,
	}).To_InputChannel()
	request := func(channel *mtproto.InputChannel) *mtproto.TLChannelsGetMessages {
		return &mtproto.TLChannelsGetMessages{
			Channel:        channel,
			Id_VECTORINT32: []int32{1},
		}
	}
	for _, userID := range []int64{owner, memberID} {
		c := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: userID}}
		got, err := c.ChannelsGetMessages(request(inputChannel))
		if err != nil || got == nil || len(got.GetMessages()) != 1 || got.GetMessages()[0].GetMessage() != "member-only message" {
			t.Fatalf("authorized read for user %d: result=%+v err=%v", userID, got, err)
		}
	}
	outsider := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: outsiderID}}
	if _, err := outsider.ChannelsGetMessages(request(inputChannel)); !errors.Is(err, mtproto.ErrUserNotParticipant) {
		t.Fatalf("outsider read: got %v", err)
	}
	badHash := mtproto.MakeTLInputChannel(&mtproto.InputChannel{
		ChannelId: channelID, AccessHash: channelID + 1,
	}).To_InputChannel()
	if _, err := (&ApiFullCore{MD: &metadata.RpcMetadata{UserId: memberID}}).ChannelsGetMessages(request(badHash)); !errors.Is(err, mtproto.ErrChannelInvalid) {
		t.Fatalf("bad-hash read: got %v", err)
	}
}
