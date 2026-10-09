package core

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/messenger/msg/inbox/inbox"
	"github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

func (c *MsgCore) sendUserOutgoingPostgres(ctx context.Context, senderID, authKeyID, recipientID int64, messages []*msg.OutboxMessage, users *userpb.Vector_ImmutableUser, userIDs []int64) (*mtproto.Updates, error) {
	if users == nil {
		return nil, mtproto.ErrInternalServerError
	}
	recipients := []*inbox.TLInboxSendUserMessageToInboxV2{{
		UserId: senderID, Out: true, FromId: senderID, FromAuthKeyId: authKeyID,
		PeerType: mtproto.PEER_USER, PeerId: recipientID,
		Users: users.GetUserListByIdList(senderID, userIDs...),
	}}
	if senderID != recipientID {
		recipients = append(recipients, &inbox.TLInboxSendUserMessageToInboxV2{
			UserId: recipientID, FromId: senderID, FromAuthKeyId: authKeyID,
			PeerType: mtproto.PEER_USER, PeerId: recipientID,
			Users: users.GetUserListByIdList(recipientID, userIDs...),
		})
	}
	boxes, err := c.svcCtx.Dao.SendMessagesWithDeliveries(ctx, senderID, mtproto.MakePeerUtil(mtproto.PEER_USER, recipientID), messages, recipients)
	if err != nil {
		return nil, err
	}
	return mtproto.MakeReplyUpdates(
		func(ids []int64) []*mtproto.User { return users.GetUserListByIdList(senderID, ids...) },
		func([]int64) []*mtproto.Chat { return nil },
		func([]int64) []*mtproto.Chat { return nil },
		newMessageUpdates(boxes)...), nil
}

func (c *MsgCore) sendChatOutgoingPostgres(ctx context.Context, senderID, authKeyID, chatID int64, messages []*msg.OutboxMessage, chat *mtproto.MutableChat, users *mtproto.MutableUsers) (*mtproto.Updates, error) {
	if chat == nil || chat.GetChat() == nil || users == nil {
		return nil, mtproto.ErrInternalServerError
	}
	var recipients []*inbox.TLInboxSendUserMessageToInboxV2
	chat.Walk(func(userID int64, participant *mtproto.ImmutableChatParticipant) error {
		if !participant.IsChatMemberStateNormal() && userID != senderID {
			return nil
		}
		toUsers := make([]*mtproto.User, 0, users.Length())
		users.Visit(func(user *mtproto.ImmutableUser) { toUsers = append(toUsers, user.ToUser(userID)) })
		recipients = append(recipients, &inbox.TLInboxSendUserMessageToInboxV2{
			UserId: userID, Out: userID == senderID, FromId: senderID, FromAuthKeyId: authKeyID,
			PeerType: mtproto.PEER_CHAT, PeerId: chatID, Users: toUsers,
			Chats: []*mtproto.Chat{chat.ToUnsafeChat(userID)},
		})
		return nil
	})
	boxes, err := c.svcCtx.Dao.SendMessagesWithDeliveries(ctx, senderID, mtproto.MakePeerUtil(mtproto.PEER_CHAT, chatID), messages, recipients)
	if err != nil {
		return nil, err
	}
	return mtproto.MakeReplyUpdates(
		func(ids []int64) []*mtproto.User { return users.GetUserListByIdList(senderID, ids...) },
		func([]int64) []*mtproto.Chat { return []*mtproto.Chat{chat.ToUnsafeChat(senderID)} },
		func([]int64) []*mtproto.Chat { return nil },
		newMessageUpdates(boxes)...), nil
}

func newMessageUpdates(boxes []*mtproto.MessageBox) []*mtproto.Update {
	updates := make([]*mtproto.Update, 0, len(boxes))
	for _, box := range boxes {
		updates = append(updates, mtproto.MakeTLUpdateNewMessage(&mtproto.Update{
			Pts_INT32: box.Pts, PtsCount: box.PtsCount, RandomId: box.RandomId, Message_MESSAGE: box.Message,
		}).To_Update())
	}
	return updates
}
