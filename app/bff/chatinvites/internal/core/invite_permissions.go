package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/channelview"
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
)

func (c *ChatInvitesCore) requireChannelInviteAdmin(input *mtproto.InputChannel) (bool, error) {
	return channelview.ValidateInputChannelInviteAdmin(c.MD.UserId, input)
}

func (c *ChatInvitesCore) requireInvitePermission(chatID, targetAdminID int64) error {
	chat, err := c.svcCtx.Dao.ChatClient.ChatGetMutableChat(c.ctx, &chatpb.TLChatGetMutableChat{ChatId: chatID})
	if err != nil || chat == nil {
		return mtproto.ErrPeerIdInvalid
	}
	me, _ := chat.GetImmutableChatParticipant(c.MD.UserId)
	if me == nil || me.State != mtproto.ChatMemberStateNormal {
		return mtproto.ErrUserNotParticipant
	}
	if !me.CanInviteUsers() {
		return mtproto.ErrChatAdminRequired
	}
	if targetAdminID != 0 && targetAdminID != c.MD.UserId && !me.CanAdminAddAdmins() {
		return mtproto.ErrChatAdminRequired
	}
	return nil
}
