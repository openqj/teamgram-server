package core

import (
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/channelview"
	msgpb "github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
	"github.com/teamgram/teamgram-server/app/service/biz/message/message"
)

func (c *MessagesCore) forwardTexts(from *mtproto.PeerUtil, ids []int32) ([]string, error) {
	if from == nil {
		return nil, mtproto.ErrPeerIdInvalid
	}
	if from.PeerType == mtproto.PEER_CHANNEL {
		texts, err := channelview.Texts(c.MD.UserId, from.PeerId, ids)
		if err != nil {
			return nil, err
		}
		for _, text := range texts {
			if text == "" {
				return nil, mtproto.ErrMessageEmpty
			}
		}
		return texts, nil
	}
	boxes, err := c.svcCtx.Dao.MessageClient.MessageGetUserMessageList(c.ctx, &message.TLMessageGetUserMessageList{
		UserId: c.MD.UserId,
		IdList: ids,
	})
	if err != nil {
		return nil, err
	}
	if err = validateForwardMessageBoxes(c.MD.UserId, from, ids, boxes); err != nil {
		return nil, err
	}
	if err = validateForwardTextOnlyBoxes(boxes); err != nil {
		return nil, err
	}
	byID := make(map[int32]string, len(ids))
	for _, box := range boxes.GetDatas() {
		byID[box.GetMessageId()] = box.GetMessage().GetMessage()
	}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		text, ok := byID[id]
		if !ok {
			return nil, mtproto.ErrMessageIdInvalid
		}
		out = append(out, text)
	}
	return out, nil
}

func validateForwardMessageBoxes(userID int64, from *mtproto.PeerUtil, ids []int32, boxes *message.Vector_MessageBox) error {
	if from == nil || len(ids) == 0 {
		return mtproto.ErrInputRequestInvalid
	}
	if boxes == nil {
		return mtproto.ErrInternalServerError
	}
	wanted := make(map[int32]struct{}, len(ids))
	for _, id := range ids {
		if id <= 0 {
			return mtproto.ErrMessageIdInvalid
		}
		if _, exists := wanted[id]; exists {
			return mtproto.ErrInputRequestInvalid
		}
		wanted[id] = struct{}{}
	}
	expectedPeer := from.ToPeer()
	if expectedPeer == nil {
		return mtproto.ErrPeerIdInvalid
	}
	found := make(map[int32]struct{}, len(ids))
	for _, box := range boxes.GetDatas() {
		if box == nil || box.GetMessage() == nil {
			return mtproto.ErrInternalServerError
		}
		id := box.GetMessageId()
		if id <= 0 || box.GetMessage().GetId() != id || box.GetUserId() != userID {
			return mtproto.ErrMessageIdInvalid
		}
		if _, ok := wanted[id]; !ok || !boxInPeer(from, box, userID) || !messagePeersMatch(box.GetMessage().GetPeerId(), expectedPeer) {
			return mtproto.ErrMessageIdInvalid
		}
		if _, exists := found[id]; exists {
			return mtproto.ErrMessageIdInvalid
		}
		found[id] = struct{}{}
	}
	if len(found) != len(wanted) {
		return mtproto.ErrMessageIdInvalid
	}
	return nil
}

func validateForwardTextOnlyBoxes(boxes *message.Vector_MessageBox) error {
	if boxes == nil {
		return mtproto.ErrInternalServerError
	}
	for _, box := range boxes.GetDatas() {
		if box == nil || box.GetMessage() == nil {
			return mtproto.ErrInternalServerError
		}
		m := box.GetMessage()
		media := m.GetMedia()
		if media != nil && media.GetPredicateName() != mtproto.Predicate_messageMediaEmpty {
			return mtproto.ErrMethodNotImpl
		}
		if m.GetNoforwards() {
			return mtproto.ErrChatForwardsRestricted
		}
		if len(m.GetEntities()) > 0 || m.GetReplyMarkup() != nil || m.GetReplyTo() != nil || m.GetGroupedId() != nil ||
			m.GetViaBotId() != nil || m.GetFactcheck() != nil {
			return mtproto.ErrMethodNotImpl
		}
		if m.GetMessage() == "" {
			return mtproto.ErrMessageEmpty
		}
	}
	return nil
}

func (c *MessagesCore) forwardPlain(toPeer *mtproto.PeerUtil, randomIDs []int64, texts []string) (*mtproto.Updates, error) {
	if toPeer == nil || toPeer.IsChannel() || !toPeer.IsChatOrUser() {
		return nil, mtproto.ErrPeerIdInvalid
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.MsgClient == nil {
		return nil, mtproto.ErrInternalServerError
	}
	now := int32(time.Now().Unix())
	outbox := make([]*msgpb.OutboxMessage, 0, len(texts))
	for i, text := range texts {
		var rid int64
		if i < len(randomIDs) {
			rid = randomIDs[i]
		}
		outbox = append(outbox, msgpb.MakeTLOutboxMessage(&msgpb.OutboxMessage{
			RandomId: rid,
			Message: mtproto.MakeTLMessage(&mtproto.Message{
				Out:     true,
				FromId:  mtproto.MakePeerUser(c.MD.UserId),
				PeerId:  toPeer.ToPeer(),
				Date:    now,
				Message: text,
			}).To_Message(),
		}).To_OutboxMessage())
	}
	updates, err := c.svcCtx.Dao.MsgClient.MsgSendMessageV2(c.ctx, &msgpb.TLMsgSendMessageV2{
		UserId:    c.MD.UserId,
		AuthKeyId: c.MD.PermAuthKeyId,
		PeerType:  toPeer.PeerType,
		PeerId:    toPeer.PeerId,
		Message:   outbox,
	})
	if err != nil {
		return nil, err
	}
	if updates == nil {
		return nil, mtproto.ErrInternalServerError
	}
	return updates, nil
}
