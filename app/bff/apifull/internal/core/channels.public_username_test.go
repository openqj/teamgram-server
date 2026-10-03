package core

import (
	"context"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	apifullDao "github.com/teamgram/teamgram-server/app/bff/apifull/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/svc"
	user_client "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

type publicUsernameJoinUserClient struct {
	user_client.UserClient
	username  *userpb.UsernameData
	channelID int64
}

func (c *publicUsernameJoinUserClient) UserGetChannelUsername(_ context.Context, in *userpb.TLUserGetChannelUsername) (*userpb.UsernameData, error) {
	c.channelID = in.GetChannelId()
	return c.username, nil
}

func TestChannelsJoinChannelUsesActiveUsernameServiceRecord(t *testing.T) {
	channelID := time.Now().UnixNano()
	const owner int64 = 97001
	const joiner int64 = 97002
	if err := domain.SaveChannel(domain.Channel{
		ID: channelID, AccessHash: channelID, Creator: owner, Title: "username-service-public-channel", Broadcast: true,
	}); err != nil {
		t.Fatal("save channel fixture:", err)
	}
	t.Cleanup(func() {
		if err := domain.DeleteChannel(owner, channelID); err != nil {
			t.Errorf("clean up channel fixture: %v", err)
		}
	})

	client := &publicUsernameJoinUserClient{username: &userpb.UsernameData{
		Username: "username_service_public",
		Peer:     mtproto.MakePeerChannel(channelID),
		Active:   true,
	}}
	core := &ApiFullCore{
		ctx: context.Background(),
		MD:  &metadata.RpcMetadata{UserId: joiner},
		svcCtx: &svc.ServiceContext{Dao: &apifullDao.Dao{
			UserClient: client,
		}},
	}
	input := mtproto.MakeTLInputChannel(&mtproto.InputChannel{ChannelId: channelID, AccessHash: channelID}).To_InputChannel()
	if _, err := core.ChannelsJoinChannel24B524C5(&mtproto.TLChannelsJoinChannel24B524C5{Channel: input}); err != nil {
		t.Fatal("join public channel through username service:", err)
	}
	if client.channelID != channelID {
		t.Fatalf("username lookup channel ID = %d, want %d", client.channelID, channelID)
	}
	if _, found, err := domain.LoadChannelMember(channelID, joiner); err != nil || !found {
		t.Fatalf("joined member persisted = found:%v err:%v", found, err)
	}
}
