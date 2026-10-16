package core

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/svc"
)

func TestChannelsInviteRejectsStaleInputUserAccessHash(t *testing.T) {
	channelID := time.Now().UnixNano()
	owner, target := channelID%1_000_000_000+2_400_000_000, channelID%1_000_000_000+2_400_000_001
	if err := domain.SaveChannel(domain.Channel{
		ID: channelID, AccessHash: channelID, Creator: owner, Title: "input-user-hash", Megagroup: true,
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = domain.DeleteChannel(owner, channelID) })

	core := &ApiFullCore{
		ctx: context.Background(),
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{
			UserClient: &channelMemberUserClient{},
		}},
		MD: &metadata.RpcMetadata{UserId: owner},
	}
	channel := mtproto.MakeTLInputChannel(&mtproto.InputChannel{
		ChannelId: channelID, AccessHash: channelID,
	}).To_InputChannel()
	staleUser := mtproto.MakeTLInputUser(&mtproto.InputUser{
		UserId: target, AccessHash: target + 1,
	}).To_InputUser()

	got, err := core.ChannelsInviteToChannelC9E33D54(&mtproto.TLChannelsInviteToChannelC9E33D54{
		Channel: channel, Users: []*mtproto.InputUser{staleUser},
	})
	if got != nil || !errors.Is(err, mtproto.ErrUserIdInvalid) {
		t.Fatalf("stale inputUser hash: result=%v err=%v, want USER_ID_INVALID", got, err)
	}
	if _, found, loadErr := domain.LoadChannelMember(channelID, target); loadErr != nil || found {
		t.Fatalf("stale inputUser mutated membership: found=%v err=%v", found, loadErr)
	}
}
