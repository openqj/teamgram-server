package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/messenger/msg/inbox/inbox"
)

func (c *InboxCore) editLegacyInboxMessages(fromID int64, peerType int32, peerID int64, recipients []int64, message *mtproto.Message) error {
	if fromID <= 0 || peerID <= 0 || message == nil || message.GetId() <= 0 {
		return mtproto.ErrInputRequestInvalid
	}
	source, err := c.svcCtx.Dao.SelectMessageByID(c.ctx, fromID, message.GetId())
	if err != nil {
		return err
	}
	if source == nil {
		return mtproto.ErrMsgIdInvalid
	}
	if source.SenderUserId != fromID || source.PeerType != peerType || source.PeerId != peerID {
		return mtproto.ErrPeerIdInvalid
	}
	seen := make(map[int64]bool, len(recipients))
	for _, recipientID := range recipients {
		if recipientID == fromID || seen[recipientID] {
			continue
		}
		if recipientID <= 0 {
			return mtproto.ErrInputRequestInvalid
		}
		seen[recipientID] = true
		if err := c.svcCtx.Dao.EditInboxMessageState(c.ctx, &inbox.TLInboxEditMessageToInboxV2{
			UserId: recipientID, FromId: fromID, PeerType: peerType, PeerId: peerID,
			NewMessage: &mtproto.MessageBox{DialogMessageId: source.DialogMessageId, Message: message},
		}); err != nil {
			return err
		}
	}
	return nil
}

func (c *InboxCore) readLegacyInboxMedia(fromID int64, peerType int32, peerID int64, ids []*inbox.InboxMessageId) error {
	if fromID <= 0 || peerID <= 0 {
		return mtproto.ErrInputRequestInvalid
	}
	for _, id := range ids {
		if id == nil || id.GetDialogMessageId() <= 0 {
			return mtproto.ErrInputRequestInvalid
		}
		source, err := c.svcCtx.Dao.SelectMessageByDataID(c.ctx, fromID, id.GetDialogMessageId())
		if err != nil {
			return err
		}
		if source == nil {
			continue
		}
		if source.PeerType != peerType || source.PeerId != peerID || id.GetId() != 0 && id.GetId() != source.UserMessageBoxId {
			return mtproto.ErrPeerIdInvalid
		}
		var users []int64
		if peerType == mtproto.PEER_USER {
			users = []int64{fromID, peerID}
		}
		copies, err := c.svcCtx.Dao.SelectMessagesByDataIDUsers(c.ctx, id.GetDialogMessageId(), users, nil)
		if err != nil {
			return err
		}
		for _, copy := range copies {
			if copy.PeerType != peerType || peerType == mtproto.PEER_CHAT && copy.PeerId != peerID || copy.SenderUserId != source.SenderUserId {
				return mtproto.ErrPeerIdInvalid
			}
			if _, err := c.InboxReadMediaUnreadToInboxV2(&inbox.TLInboxReadMediaUnreadToInboxV2{
				UserId: copy.UserId, PeerType: copy.PeerType, PeerId: copy.PeerId, DialogMessageId: copy.DialogMessageId,
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

func (c *InboxCore) updateLegacyInboxReadHistory(in *inbox.TLInboxUpdateHistoryReaded) error {
	if in.FromId <= 0 || in.PeerId <= 0 || in.MaxId <= 0 || (in.PeerType != mtproto.PEER_USER && in.PeerType != mtproto.PEER_CHAT) {
		return mtproto.ErrInputRequestInvalid
	}
	source, err := c.svcCtx.Dao.SelectMessageByID(c.ctx, in.FromId, in.MaxId)
	if err != nil {
		return err
	}
	if source == nil {
		return nil
	}
	if source.PeerType != in.PeerType || source.PeerId != in.PeerId {
		return mtproto.ErrPeerIdInvalid
	}
	recipientID, peerID := in.PeerId, in.FromId
	if in.PeerType == mtproto.PEER_CHAT {
		if in.Sender <= 0 || source.SenderUserId != in.Sender {
			return mtproto.ErrPeerIdInvalid
		}
		recipientID, peerID = in.Sender, in.PeerId
	}
	return c.svcCtx.Dao.ReadOutboxHistoryState(c.ctx, recipientID, mtproto.MakePeerUtil(in.PeerType, peerID), source.DialogMessageId)
}
