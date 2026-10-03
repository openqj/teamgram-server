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
	"github.com/zeromicro/go-zero/core/logx"
)

func TestChannelsEditAdminRoundTrip(t *testing.T) {
	channelID := time.Now().UnixNano()
	owner, member, second, outsider := channelID%1_000_000_000+2_000_000_000, channelID%1_000_000_000+2_000_000_001, channelID%1_000_000_000+2_000_000_002, channelID%1_000_000_000+2_000_000_003
	if err := domain.SaveChannel(domain.Channel{ID: channelID, AccessHash: channelID, Creator: owner, Title: "admin-round-trip", Megagroup: true}); err != nil {
		t.Fatal(err)
	}
	userClient := &channelMemberUserClient{}
	coreFor := func(uid int64) *ApiFullCore {
		return &ApiFullCore{
			ctx:    context.Background(),
			svcCtx: &svc.ServiceContext{Dao: &dao.Dao{UserClient: userClient}},
			MD:     &metadata.RpcMetadata{UserId: uid},
			Logger: logx.WithContext(context.Background()),
		}
	}
	ownerCore := coreFor(owner)
	inputChannel := mtproto.MakeTLInputChannel(&mtproto.InputChannel{ChannelId: channelID, AccessHash: channelID}).To_InputChannel()
	inputUser := func(id int64) *mtproto.InputUser {
		return mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: id, AccessHash: id}).To_InputUser()
	}
	if _, err := ownerCore.ChannelsInviteToChannelC9E33D54(&mtproto.TLChannelsInviteToChannelC9E33D54{Channel: inputChannel, Users: []*mtproto.InputUser{inputUser(member), inputUser(second)}}); err != nil {
		t.Fatalf("invite members: %v", err)
	}
	rights := mtproto.MakeTLChatAdminRights(&mtproto.ChatAdminRights{InviteUsers: true, AddAdmins: true, ManageTopics: true}).To_ChatAdminRights()
	updates, err := ownerCore.ChannelsEditAdmin(&mtproto.TLChannelsEditAdmin{
		Channel: inputChannel, UserId: inputUser(member), AdminRights: rights, Rank_STRING: "moderator",
	})
	if err != nil || updates == nil || len(updates.GetUpdates()) != 1 {
		t.Fatalf("promote member: updates=%+v err=%v", updates, err)
	}
	if err = updates.Encode(mtproto.NewEncodeBuf(4096), 229); err != nil {
		t.Fatalf("encode participant update: %v", err)
	}
	update := updates.GetUpdates()[0]
	if update.GetPredicateName() != mtproto.Predicate_updateChannelParticipant || update.GetNewParticipant_FLAGCHANNELPARTICIPANT() == nil || update.GetNewParticipant_FLAGCHANNELPARTICIPANT().GetAdminRights() == nil || !update.GetNewParticipant_FLAGCHANNELPARTICIPANT().GetAdminRights().GetAddAdmins() {
		t.Fatalf("participant update: %+v", update)
	}
	stored, found, err := domain.LoadChannelMember(channelID, member)
	if err != nil || !found || stored.AdminRights == nil || !stored.AdminRights.AddAdmins || stored.Rank != "moderator" {
		t.Fatalf("stored rights: member=%+v found=%v err=%v", stored, found, err)
	}

	adminCore := coreFor(member)
	adminUpdates, err := adminCore.ChannelsEditAdmin(&mtproto.TLChannelsEditAdmin{
		Channel: inputChannel, UserId: inputUser(second), AdminRights: mtproto.MakeTLChatAdminRights(&mtproto.ChatAdminRights{BanUsers: true}).To_ChatAdminRights(),
	})
	if err != nil || adminUpdates == nil || len(adminUpdates.GetUpdates()) != 1 {
		t.Fatalf("promoted admin edit: updates=%+v err=%v", adminUpdates, err)
	}
	secondStored, found, err := domain.LoadChannelMember(channelID, second)
	if err != nil || !found || secondStored.AdminRights == nil || !secondStored.AdminRights.BanUsers {
		t.Fatalf("promoted target rights: member=%+v found=%v err=%v", secondStored, found, err)
	}

	admins, err := ownerCore.ChannelsGetParticipants(&mtproto.TLChannelsGetParticipants{
		Channel: inputChannel,
		Filter:  mtproto.MakeTLChannelParticipantsAdmins(nil).To_ChannelParticipantsFilter(),
		Limit:   50,
	})
	if err != nil || admins == nil || admins.GetCount() != 3 || len(admins.GetParticipants()) != 3 {
		t.Fatalf("admin listing: result=%+v err=%v", admins, err)
	}
	if len(admins.GetUsers()) != 3 || admins.GetUsers()[0].GetFirstName().GetValue() != "hydrated" {
		t.Fatalf("hydrated admin users: %+v", admins.GetUsers())
	}
	lookup, err := ownerCore.ChannelsGetParticipant(&mtproto.TLChannelsGetParticipant{
		Channel: inputChannel,
		Participant: mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{
			UserId: member, AccessHash: member,
		}).To_InputPeer(),
	})
	if err != nil || lookup == nil || len(lookup.GetUsers()) != 1 || lookup.GetUsers()[0].GetFirstName().GetValue() != "hydrated" {
		t.Fatalf("hydrated participant: result=%+v err=%v", lookup, err)
	}

	if _, err = coreFor(outsider).ChannelsEditAdmin(&mtproto.TLChannelsEditAdmin{
		Channel: inputChannel, UserId: inputUser(member), AdminRights: rights,
	}); !errors.Is(err, mtproto.ErrChatAdminRequired) {
		t.Fatalf("outsider edit: %v", err)
	}
	if _, err = ownerCore.ChannelsEditAdmin(&mtproto.TLChannelsEditAdmin{
		Channel: inputChannel, UserId: inputUser(outsider), AdminRights: rights,
	}); !errors.Is(err, mtproto.ErrUserNotParticipant) {
		t.Fatalf("nonmember target: %v", err)
	}
	if _, err = ownerCore.ChannelsEditAdmin(&mtproto.TLChannelsEditAdmin{
		Channel: inputChannel, UserId: inputUser(member), AdminRights: mtproto.MakeTLChatAdminRights(&mtproto.ChatAdminRights{}).To_ChatAdminRights(),
	}); err != nil {
		t.Fatalf("demote member: %v", err)
	}
	stored, found, err = domain.LoadChannelMember(channelID, member)
	if err != nil || !found || stored.AdminRights != nil || stored.Rank != "" {
		t.Fatalf("demoted rights: member=%+v found=%v err=%v", stored, found, err)
	}
	if err = domain.DeleteChannel(owner, channelID); err != nil {
		t.Fatalf("cleanup channel: %v", err)
	}
}
