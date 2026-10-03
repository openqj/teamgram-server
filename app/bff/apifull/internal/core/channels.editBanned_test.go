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
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

type editBannedUserClient struct {
	channelMemberUserClient
	missingID int64
	deletedID int64
}

func (c *editBannedUserClient) UserGetImmutableUserV2(ctx context.Context, in *userpb.TLUserGetImmutableUserV2) (*mtproto.ImmutableUser, error) {
	if in.GetId() == c.missingID {
		return nil, nil
	}
	if in.GetId() == c.deletedID {
		return &mtproto.ImmutableUser{User: &mtproto.UserData{Id: in.GetId(), Deleted: true}}, nil
	}
	return c.channelMemberUserClient.UserGetImmutableUserV2(ctx, in)
}

func TestChannelsEditBannedRoundTrip(t *testing.T) {
	channelID := time.Now().UnixNano()
	owner, admin, member, second, targetAdmin, outsider, deleted := channelID%1_000_000_000+2_100_000_000, channelID%1_000_000_000+2_100_000_001, channelID%1_000_000_000+2_100_000_002, channelID%1_000_000_000+2_100_000_003, channelID%1_000_000_000+2_100_000_004, channelID%1_000_000_000+2_100_000_005, channelID%1_000_000_000+2_100_000_006
	if err := domain.SaveChannel(domain.Channel{ID: channelID, AccessHash: channelID, Creator: owner, Title: "ban-round-trip", Megagroup: true}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := domain.DeleteChannel(owner, channelID); err != nil {
			t.Errorf("cleanup channel: %v", err)
		}
	})
	for _, userID := range []int64{admin, member, second, targetAdmin} {
		if err := domain.JoinChannel(channelID, userID); err != nil {
			t.Fatalf("join %d: %v", userID, err)
		}
	}
	if err := domain.EditChannelAdmin(channelID, owner, admin, &domain.ChannelAdminRights{BanUsers: true}, ""); err != nil {
		t.Fatalf("grant ban_users: %v", err)
	}
	if err := domain.EditChannelAdmin(channelID, owner, targetAdmin, &domain.ChannelAdminRights{InviteUsers: true}, ""); err != nil {
		t.Fatalf("grant target admin rights: %v", err)
	}
	userClient := &editBannedUserClient{missingID: outsider, deletedID: deleted}
	coreFor := func(userID int64) *ApiFullCore {
		return &ApiFullCore{
			ctx:    context.Background(),
			svcCtx: &svc.ServiceContext{Dao: &dao.Dao{UserClient: userClient}},
			MD:     &metadata.RpcMetadata{UserId: userID},
		}
	}
	inputChannel := mtproto.MakeTLInputChannel(&mtproto.InputChannel{ChannelId: channelID, AccessHash: channelID}).To_InputChannel()
	inputPeer := func(userID int64) *mtproto.InputPeer {
		return mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: userID, AccessHash: userID}).To_InputPeer()
	}
	edit := func(actorID, targetID int64, rights *mtproto.ChatBannedRights) (*mtproto.Updates, error) {
		return coreFor(actorID).ChannelsEditBanned(&mtproto.TLChannelsEditBanned{
			Channel: inputChannel, Participant: inputPeer(targetID), BannedRights: rights,
		})
	}
	limited := mtproto.MakeTLChatBannedRights(&mtproto.ChatBannedRights{
		SendMessages: true, UntilDate: int32(time.Now().Add(time.Hour).Unix()),
	}).To_ChatBannedRights()
	updates, err := edit(admin, member, limited)
	if err != nil || updates == nil || len(updates.GetUpdates()) != 1 {
		t.Fatalf("admin restriction: updates=%+v err=%v", updates, err)
	}
	if err = updates.Encode(mtproto.NewEncodeBuf(4096), 229); err != nil {
		t.Fatalf("encode restriction update: %v", err)
	}
	stored, found, err := domain.LoadChannelMember(channelID, member)
	if err != nil || !found || stored.BannedRights == nil || !stored.BannedRights.SendMessages || stored.BannedBy != admin {
		t.Fatalf("stored restriction: member=%+v found=%v err=%v", stored, found, err)
	}
	participant, err := coreFor(owner).ChannelsGetParticipant(&mtproto.TLChannelsGetParticipant{
		Channel: inputChannel, Participant: inputPeer(member),
	})
	if err != nil || participant == nil || participant.GetParticipant().GetPredicateName() != mtproto.Predicate_channelParticipantBanned || !participant.GetParticipant().GetBannedRights().GetSendMessages() {
		t.Fatalf("restricted participant: result=%+v err=%v", participant, err)
	}

	if _, err = edit(outsider, second, limited); !errors.Is(err, mtproto.ErrChatAdminRequired) {
		t.Fatalf("outsider ban: %v", err)
	}
	if _, err = edit(owner, outsider, limited); !errors.Is(err, mtproto.ErrUserIdInvalid) {
		t.Fatalf("unknown participant: %v", err)
	}
	if _, err = edit(owner, deleted, limited); !errors.Is(err, mtproto.ErrInputUserDeactivated) {
		t.Fatalf("deleted participant: %v", err)
	}
	if _, err = edit(admin, targetAdmin, limited); !errors.Is(err, mtproto.ErrChatAdminRequired) {
		t.Fatalf("delegated admin ban: %v", err)
	}
	if _, err = coreFor(owner).ChannelsEditBanned(&mtproto.TLChannelsEditBanned{
		Channel:     mtproto.MakeTLInputChannel(&mtproto.InputChannel{ChannelId: channelID, AccessHash: channelID + 1}).To_InputChannel(),
		Participant: inputPeer(second), BannedRights: limited,
	}); !errors.Is(err, mtproto.ErrChannelInvalid) {
		t.Fatalf("bad access hash: %v", err)
	}
	if _, err = edit(owner, owner, limited); !errors.Is(err, mtproto.ErrUserCreator) {
		t.Fatalf("creator ban: %v", err)
	}
	if _, err = edit(owner, member, mtproto.MakeTLChatBannedRights(&mtproto.ChatBannedRights{}).To_ChatBannedRights()); err != nil {
		t.Fatalf("unrestrict member: %v", err)
	}
	stored, found, err = domain.LoadChannelMember(channelID, member)
	if err != nil || !found || stored.BannedRights != nil || stored.BannedBy != 0 {
		t.Fatalf("member after unrestrict: member=%+v found=%v err=%v", stored, found, err)
	}

	kick := mtproto.MakeTLChatBannedRights(&mtproto.ChatBannedRights{ViewMessages: true}).To_ChatBannedRights()
	updates, err = edit(owner, member, kick)
	if err != nil || updates == nil {
		t.Fatalf("kick member: updates=%+v err=%v", updates, err)
	}
	if memberOf, err := domain.ChannelIsMember(channelID, member); err != nil || memberOf {
		t.Fatalf("kicked membership: member=%v err=%v", memberOf, err)
	}
	participant, err = coreFor(owner).ChannelsGetParticipant(&mtproto.TLChannelsGetParticipant{
		Channel: inputChannel, Participant: inputPeer(member),
	})
	if err != nil || participant == nil || participant.GetParticipant().GetPredicateName() != mtproto.Predicate_channelParticipantBanned || !participant.GetParticipant().GetBannedRights().GetViewMessages() {
		t.Fatalf("kicked participant: result=%+v err=%v", participant, err)
	}
	updates, err = edit(owner, member, mtproto.MakeTLChatBannedRights(&mtproto.ChatBannedRights{}).To_ChatBannedRights())
	if err != nil || updates == nil || updates.GetUpdates()[0].GetNewParticipant_FLAGCHANNELPARTICIPANT().GetPredicateName() != mtproto.Predicate_channelParticipantLeft {
		t.Fatalf("unban kicked member: updates=%+v err=%v", updates, err)
	}
	if _, found, err = domain.LoadChannelMember(channelID, member); err != nil || found {
		t.Fatalf("kicked row after unban: found=%v err=%v", found, err)
	}
	if _, err = edit(owner, member, limited); !errors.Is(err, mtproto.ErrUserNotParticipant) {
		t.Fatalf("restrict exited member: %v", err)
	}

	if _, err = edit(admin, second, limited); err != nil {
		t.Fatalf("admin ban: %v", err)
	}
	expired := mtproto.MakeTLChatBannedRights(&mtproto.ChatBannedRights{
		SendMessages: true, UntilDate: int32(time.Now().Add(-time.Hour).Unix()),
	}).To_ChatBannedRights()
	if _, err = edit(owner, second, expired); err != nil {
		t.Fatalf("expired restriction: %v", err)
	}
	stored, found, err = domain.LoadChannelMember(channelID, second)
	if err != nil || !found || stored.BannedRights != nil {
		t.Fatalf("expired restriction should clear: member=%+v found=%v err=%v", stored, found, err)
	}
}
