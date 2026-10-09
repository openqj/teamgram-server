package core

import (
	"errors"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
)

func TestChannelsGetFullCommunityLayer229RoundTrip(t *testing.T) {
	if !domain.Ready() {
		t.Skip("APIFULL_POSTGRES_DSN is not configured")
	}
	communityID := time.Now().UnixNano()
	communityOwner := communityID + 1
	linkedChannelID := communityID + 2
	memberID := communityID + 3
	community := domain.Channel{
		ID: communityID, AccessHash: communityID + 10, Creator: communityOwner,
		Title: "community-test", Megagroup: true, CreatedAt: time.Now().Unix(),
	}
	linkedChannel := domain.Channel{
		ID: linkedChannelID, AccessHash: linkedChannelID + 10, Creator: communityOwner,
		Title: "linked-test", Megagroup: true, CreatedAt: time.Now().Unix(),
	}
	if err := domain.SaveChannel(linkedChannel); err != nil {
		t.Fatal(err)
	}
	if err := domain.InviteChannelMembers(linkedChannelID, communityOwner, []int64{memberID}); err != nil {
		t.Fatal(err)
	}
	if err := domain.CreateCommunityWithPeer(community, false, domain.CommunityPeerChannel,
		linkedChannelID, linkedChannel.AccessHash); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := domain.DeleteChannel(communityOwner, communityID); err != nil {
			t.Errorf("delete community fixture: %v", err)
		}
		if err := domain.DeleteChannel(communityOwner, linkedChannelID); err != nil {
			t.Errorf("delete linked channel fixture: %v", err)
		}
	})

	input := mtproto.MakeTLInputChannel(&mtproto.InputChannel{
		ChannelId: communityID, AccessHash: community.AccessHash,
	}).To_InputChannel()
	viewer := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: memberID}}
	full, err := viewer.ChannelsGetFullChannel(&mtproto.TLChannelsGetFullChannel{Channel: input})
	if err != nil || full == nil || full.GetFullChat().GetPredicateName() != mtproto.Predicate_communityFull {
		t.Fatalf("get community full: result=%+v err=%v", full, err)
	}
	if len(full.GetChats()) != 2 || full.GetChats()[0].GetPredicateName() != mtproto.Predicate_channel ||
		full.GetChats()[1].GetPredicateName() != mtproto.Predicate_community {
		t.Fatalf("community chats = %+v", full.GetChats())
	}
	if len(full.GetFullChat().GetLinkedPeers()) != 1 || full.GetFullChat().GetLinkedPeers()[0].GetPeer().GetChannelId() != linkedChannelID {
		t.Fatalf("community linked peers = %+v", full.GetFullChat().GetLinkedPeers())
	}
	if err = full.Encode(mtproto.NewEncodeBuf(4096), 229); err != nil {
		t.Fatalf("encode community full: %v", err)
	}

	stranger := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: memberID + 1}}
	if _, err = stranger.ChannelsGetFullChannel(&mtproto.TLChannelsGetFullChannel{Channel: input}); !errors.Is(err, mtproto.ErrUserNotParticipant) {
		t.Fatalf("unauthorized community full error = %v", err)
	}
}
