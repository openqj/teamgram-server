package channelview

import (
	"errors"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
)

// ImportMigratedChat projects a deactivated basic chat into the authoritative
// APIFull megagroup tables used by channel and message handlers.
func ImportMigratedChat(chat *mtproto.MutableChat, channelID, accessHash int64) error {
	if chat == nil || chat.GetChat() == nil || chat.Id() <= 0 || channelID <= 0 || accessHash == 0 || chat.Creator() <= 0 {
		return mtproto.ErrChannelInvalid
	}
	createdAt := int64(chat.Date())
	if createdAt <= 0 {
		createdAt = time.Now().Unix()
	}
	members := make([]domain.MigratedChannelMember, 0, len(chat.GetChatParticipants()))
	for _, participant := range chat.GetChatParticipants() {
		if participant == nil || participant.GetUserId() <= 0 ||
			(!participant.IsChatMemberStateNormal() && !participant.IsChatMemberStateMigrated()) {
			continue
		}
		member := domain.MigratedChannelMember{
			UserID:    participant.GetUserId(),
			InvitedBy: participant.GetInviterUserId(),
			JoinedAt:  participant.GetInvitedAt(),
			Rank:      participant.GetRank(),
		}
		if member.JoinedAt <= 0 {
			member.JoinedAt = participant.GetDate()
		}
		if participant.IsChatMemberAdmin() {
			member.AdminRights = migratedAdminRights(participant.GetAdminRights())
		}
		members = append(members, member)
	}
	if err := domain.ImportMigratedChannel(domain.MigratedChannel{
		ChatID:     chat.Id(),
		ChannelID:  channelID,
		AccessHash: accessHash,
		CreatorID:  chat.Creator(),
		Title:      chat.Title(),
		About:      chat.About(),
		CreatedAt:  createdAt,
		Members:    members,
	}); err != nil {
		if errors.Is(err, domain.ErrChannelMigrationConflict) {
			return mtproto.ErrChannelInvalid
		}
		return stored(err)
	}
	return nil
}

func migratedAdminRights(in *mtproto.ChatAdminRights) *domain.ChannelAdminRights {
	if in == nil {
		in = mtproto.MakeDefaultChatAdminRights()
	}
	return &domain.ChannelAdminRights{
		ChangeInfo:           in.GetChangeInfo(),
		PostMessages:         in.GetPostMessages(),
		EditMessages:         in.GetEditMessages(),
		DeleteMessages:       in.GetDeleteMessages(),
		BanUsers:             in.GetBanUsers(),
		InviteUsers:          in.GetInviteUsers(),
		PinMessages:          in.GetPinMessages(),
		AddAdmins:            in.GetAddAdmins(),
		Anonymous:            in.GetAnonymous(),
		ManageCall:           in.GetManageCall(),
		Other:                in.GetOther(),
		ManageTopics:         in.GetManageTopics(),
		PostStories:          in.GetPostStories(),
		EditStories:          in.GetEditStories(),
		DeleteStories:        in.GetDeleteStories(),
		ManageDirectMessages: in.GetManageDirectMessages(),
		ManageRanks:          in.GetManageRanks(),
		ManageLinkedPeers:    in.GetManageLinkedPeers(),
	}
}
