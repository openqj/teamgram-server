package main

import (
	"fmt"
	"os"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/core"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
)

func main() {
	dsn := os.Getenv("APIFULL_POSTGRES_DSN")
	if err := domain.OpenPostgresReadOnly(dsn); err != nil { panic(err) }
	defer domain.Close()
	owner := int64(81100)
	c := &core.ApiFullCore{MD: &metadata.RpcMetadata{UserId: owner}}
	source := timeNow()
	if err := domain.SaveChannel(domain.Channel{ID: source, AccessHash: source, Creator: owner, Title: "src", Megagroup: true}); err != nil { panic(err) }
	if err := domain.JoinChannel(source, 81101); err != nil { panic(err) }
	created, err := c.CommunitiesCreate(&mtproto.TLCommunitiesCreate{Title: "probe2", Peer: mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{ChannelId: source, AccessHash: source}).To_InputPeer()})
	if err != nil { panic(err) }
	id := created.GetChats()[0].GetId()
	channel := mtproto.MakeTLInputChannel(&mtproto.InputChannel{ChannelId:id, AccessHash:id}).To_InputChannel()
	linked, err := c.CommunitiesGetParticipantJoinedChats(&mtproto.TLCommunitiesGetParticipantJoinedChats{Community: channel, Participant: mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId:81101}).To_InputPeer()})
	fmt.Printf("joined chats creators=%v joined=%v chats=%d users=%d err=%v\n", linked.GetCreatorChatIds(), linked.GetJoinedChatIds(), len(linked.GetChats()), len(linked.GetUsers()), err)
	if err := domain.ToggleCommunityPeer(owner, id, domain.CommunityPeerUser, 81102, 0, nil, false, false, 1); err != nil { panic(err) }
	requests, err := c.CommunitiesGetPeerLinkRequests(&mtproto.TLCommunitiesGetPeerLinkRequests{Community:channel, Limit:10})
	fmt.Printf("requests=%d next=%v err=%v\n", len(requests.GetRequests()), requests.GetNextOffset(), err)
	if _, err := c.CommunitiesTogglePeerLinkRequestApproval(&mtproto.TLCommunitiesTogglePeerLinkRequestApproval{Community:channel, Peer:mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId:81102}).To_InputPeer()}); err != nil { panic(err) }
	if _, err := c.CommunitiesToggleParticipantBanned(&mtproto.TLCommunitiesToggleParticipantBanned{Community:channel, Participant:mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId:81102}).To_InputPeer()}); err != nil { panic(err) }
	if _, err := c.CommunitiesToggleCommunityCollapsedInDialogs(&mtproto.TLCommunitiesToggleCommunityCollapsedInDialogs{Community:channel, Collapsed:true}); err != nil { panic(err) }
	fmt.Println("all methods ok")
}

func timeNow() int64 { return 811000000000000000 }
