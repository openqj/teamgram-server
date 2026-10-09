package core

import (
	"math/rand"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/messenger/msg/inbox/inbox"
	"github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

func (c *MsgCore) readHistoryPostgres(userID, authID int64, peerType int32, peerID int64, maxID int32) (*mtproto.Messages_AffectedMessages, error) {
	if peerType == mtproto.PEER_SELF {
		peerType, peerID = mtproto.PEER_USER, userID
	}
	if peerType != mtproto.PEER_USER && peerType != mtproto.PEER_CHAT {
		return nil, mtproto.ErrPeerIdInvalid
	}
	updates, pts, _, err := c.svcCtx.Dao.ReadHistoryState(c.ctx, userID, mtproto.MakePeerUtil(peerType, peerID), maxID)
	if err != nil {
		return nil, err
	}
	return mtproto.MakeTLMessagesAffectedMessages(&mtproto.Messages_AffectedMessages{Pts: pts, PtsCount: int32(len(updates))}).To_Messages_AffectedMessages(), nil
}

func (c *MsgCore) pinMessagePostgres(in *msg.TLMsgUpdatePinnedMessage) (*mtproto.Updates, error) {
	peerType, peerID := in.PeerType, in.PeerId
	if peerType == mtproto.PEER_SELF {
		peerType, peerID = mtproto.PEER_USER, in.UserId
	}
	peer := mtproto.MakePeerUtil(peerType, peerID)
	if in.Unpin || in.PmOneside || peer.IsSelfUser(in.UserId) {
		updates, _, err := c.svcCtx.Dao.PinMessageState(c.ctx, in.UserId, peer, in.Id, !in.Unpin, !in.PmOneside)
		if err != nil {
			return nil, err
		}
		return mtproto.MakeUpdatesByUpdates(updates...), nil
	}
	service := &msg.OutboxMessage{RandomId: rand.Int63(), Message: mtproto.MakePinnedMessageService(in.Silent, in.UserId, peer, in.Id)}
	var recipients []*inbox.TLInboxSendUserMessageToInboxV2
	var users []*mtproto.User
	var chats []*mtproto.Chat
	if peerType == mtproto.PEER_USER {
		mutable, err := c.svcCtx.Dao.UserClient.UserGetMutableUsers(c.ctx, &userpb.TLUserGetMutableUsers{Id: []int64{in.UserId, peerID}, To: []int64{in.UserId, peerID}})
		if err != nil || mutable == nil {
			if err == nil {
				err = mtproto.ErrInternalServerError
			}
			return nil, err
		}
		users = mutable.GetUserListByIdList(in.UserId, in.UserId, peerID)
		for _, userID := range []int64{in.UserId, peerID} {
			recipients = append(recipients, &inbox.TLInboxSendUserMessageToInboxV2{UserId: userID, Out: userID == in.UserId, FromId: in.UserId, FromAuthKeyId: in.AuthKeyId, PeerType: peerType, PeerId: peerID, Users: mutable.GetUserListByIdList(userID, in.UserId, peerID)})
		}
	} else if peerType == mtproto.PEER_CHAT {
		chat, err := c.svcCtx.Dao.ChatClient.ChatGetMutableChat(c.ctx, &chatpb.TLChatGetMutableChat{ChatId: peerID})
		if err != nil || chat == nil {
			if err == nil {
				err = mtproto.ErrInternalServerError
			}
			return nil, err
		}
		viewerIDs := []int64{}
		chat.Walk(func(userID int64, p *mtproto.ImmutableChatParticipant) error {
			if p.IsChatMemberStateNormal() {
				viewerIDs = append(viewerIDs, userID)
			}
			return nil
		})
		mutable, err := c.svcCtx.Dao.UserClient.UserGetMutableUsersV2(c.ctx, &userpb.TLUserGetMutableUsersV2{Id: []int64{in.UserId}, Privacy: true, HasTo: true, To: viewerIDs})
		if err != nil || mutable == nil {
			if err == nil {
				err = mtproto.ErrInternalServerError
			}
			return nil, err
		}
		users = mutable.GetUserListByIdList(in.UserId, in.UserId)
		chats = []*mtproto.Chat{chat.ToUnsafeChat(in.UserId)}
		for _, userID := range viewerIDs {
			recipients = append(recipients, &inbox.TLInboxSendUserMessageToInboxV2{UserId: userID, Out: userID == in.UserId, FromId: in.UserId, FromAuthKeyId: in.AuthKeyId, PeerType: peerType, PeerId: peerID, Users: mutable.GetUserListByIdList(userID, in.UserId), Chats: []*mtproto.Chat{chat.ToUnsafeChat(userID)}})
		}
	} else {
		return nil, mtproto.ErrPeerIdInvalid
	}
	updates, _, err := c.svcCtx.Dao.PinMessageWithServiceState(c.ctx, in.UserId, peer, in.Id, service, recipients)
	if err != nil {
		return nil, err
	}
	return mtproto.MakeUpdatesByUpdatesUsersChats(users, chats, updates...), nil
}
