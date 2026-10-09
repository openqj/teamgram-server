package core

import (
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
)

func TestCommunitiesPostgresRPCRoundTrip(t *testing.T) {
	if !domain.Ready() {
		t.Skip("APIFULL_POSTGRES_DSN is not configured")
	}
	owner := time.Now().UnixNano()
	participant := owner + 1
	linkedID := owner + 2
	linked := domain.Channel{ID: linkedID, AccessHash: linkedID + 10, Creator: owner, Title: "community-linked", Megagroup: true, CreatedAt: time.Now().Unix()}
	if err := domain.SaveChannel(linked); err != nil {
		t.Fatal(err)
	}
	if err := domain.InviteChannelMembers(linkedID, owner, []int64{participant}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := domain.DeleteChannel(owner, linkedID); err != nil {
			t.Errorf("delete linked channel: %v", err)
		}
	})

	core := &ApiFullCore{MD: &metadata.RpcMetadata{UserId: owner}}
	created, err := core.CommunitiesCreate(&mtproto.TLCommunitiesCreate{
		Title: "community-rpc",
		Peer:  mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: participant, AccessHash: participant + 10}).To_InputPeer(),
	})
	if err != nil || created == nil || len(created.GetChats()) != 1 {
		t.Fatalf("create community = %#v, %v", created, err)
	}
	communityID := created.GetChats()[0].GetId()
	community, ok, err := domain.LoadChannel(communityID)
	if err != nil || !ok {
		t.Fatalf("load created community = %#v, %v, %v", community, ok, err)
	}
	t.Cleanup(func() {
		if err := domain.DeleteChannel(owner, communityID); err != nil {
			t.Errorf("delete community: %v", err)
		}
	})
	inputCommunity := mtproto.MakeTLInputChannel(&mtproto.InputChannel{ChannelId: communityID, AccessHash: community.AccessHash}).To_InputChannel()

	joined, err := core.CommunitiesGetJoinedCommunities(&mtproto.TLCommunitiesGetJoinedCommunities{})
	if err != nil || joined == nil || len(joined.GetChats()) != 1 || joined.GetChats()[0].GetId() != communityID {
		t.Fatalf("joined communities = %#v, %v", joined, err)
	}
	if _, err = core.CommunitiesToggleCommunityCollapsedInDialogs(&mtproto.TLCommunitiesToggleCommunityCollapsedInDialogs{Community: inputCommunity, Collapsed: true}); err != nil {
		t.Fatalf("collapse community: %v", err)
	}
	joined, err = core.CommunitiesGetJoinedCommunities(&mtproto.TLCommunitiesGetJoinedCommunities{})
	if err != nil || joined.GetChats()[0].GetCollapsedInDialogs() != true {
		t.Fatalf("collapsed community = %#v, %v", joined, err)
	}

	if _, err = core.CommunitiesTogglePeerLink(&mtproto.TLCommunitiesTogglePeerLink{
		Community: inputCommunity,
		Peer:      mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{ChannelId: linkedID, AccessHash: linked.AccessHash}).To_InputPeer(),
		Visible:   true,
	}); err != nil {
		t.Fatalf("link channel: %v", err)
	}
	if _, err = core.CommunitiesGetParticipantJoinedChats(&mtproto.TLCommunitiesGetParticipantJoinedChats{
		Community:   inputCommunity,
		Participant: mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: participant, AccessHash: participant + 10}).To_InputPeer(),
	}); err != nil {
		t.Fatalf("participant joined chats: %v", err)
	}

	pendingID := participant + 10
	if err = domain.ToggleCommunityPeer(owner, communityID, domain.CommunityPeerUser, pendingID, pendingID+10, nil, false, false, time.Now().Unix()); err != nil {
		t.Fatal(err)
	}
	requests, err := core.CommunitiesGetPeerLinkRequests(&mtproto.TLCommunitiesGetPeerLinkRequests{Community: inputCommunity, Limit: 10})
	if err != nil || requests == nil || requests.GetTotalCount() != 1 || len(requests.GetRequests()) != 1 {
		t.Fatalf("peer link requests = %#v, %v", requests, err)
	}
	if _, err = core.CommunitiesTogglePeerLinkRequestApproval(&mtproto.TLCommunitiesTogglePeerLinkRequestApproval{
		Community: inputCommunity,
		Peer:      mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: pendingID, AccessHash: pendingID + 10}).To_InputPeer(),
	}); err != nil {
		t.Fatalf("approve peer link: %v", err)
	}
	if err = domain.ToggleCommunityPeer(owner, communityID, domain.CommunityPeerUser, pendingID+1, pendingID+11, nil, false, false, time.Now().Unix()); err != nil {
		t.Fatal(err)
	}
	if _, err = core.CommunitiesToggleAllPeerLinkRequestApproval(&mtproto.TLCommunitiesToggleAllPeerLinkRequestApproval{Community: inputCommunity}); err != nil {
		t.Fatalf("approve all peer links: %v", err)
	}
	if _, err = core.CommunitiesToggleParticipantBanned(&mtproto.TLCommunitiesToggleParticipantBanned{
		Community:   inputCommunity,
		Participant: mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: pendingID, AccessHash: pendingID + 10}).To_InputPeer(),
	}); err != nil {
		t.Fatalf("ban participant: %v", err)
	}
	if _, err = core.CommunitiesToggleParticipantBanned(&mtproto.TLCommunitiesToggleParticipantBanned{
		Community:   inputCommunity,
		Participant: mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: pendingID, AccessHash: pendingID + 10}).To_InputPeer(),
		Unban:       true,
	}); err != nil {
		t.Fatalf("unban participant: %v", err)
	}
}
