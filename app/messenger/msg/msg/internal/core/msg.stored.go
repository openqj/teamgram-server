package core

import (
	"google.golang.org/grpc/status"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/channelview"
	"github.com/teamgram/teamgram-server/app/bff/apifull/schedstore"
	"github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
)

func (c *MsgCore) openStored() error {
	if err := channelview.Open(c.svcCtx.Config.Mysql.DSN); err != nil {
		c.Logger.Errorf("msg stored mysql is not open")
		return mtproto.ErrInternalServerError
	}
	return nil
}

func asStored(err error) error {
	if err == nil {
		return nil
	}
	if _, ok := status.FromError(err); ok {
		return err
	}
	return mtproto.ErrInternalServerError
}

func joinUpdates(base, extra *mtproto.Updates) *mtproto.Updates {
	if base == nil {
		return extra
	}
	if extra == nil {
		return base
	}
	base.Updates = append(base.Updates, extra.Updates...)
	return base
}

func (c *MsgCore) persistScheduled(in *msg.TLMsgSendMessageV2) (*mtproto.Updates, bool, error) {
	var first int32
	scheduled := false
	for _, outBox := range in.GetMessage() {
		if v := outBox.GetScheduleDate().GetValue(); v != 0 {
			scheduled = true
			if first == 0 {
				first = v
			}
		}
	}
	if !scheduled {
		return nil, false, nil
	}
	if err := c.openStored(); err != nil {
		return nil, true, err
	}
	peer := mtproto.MakePeerUtil(in.PeerType, in.PeerId).ToPeer()
	var combined *mtproto.Updates
	for _, outBox := range in.GetMessage() {
		date := outBox.GetScheduleDate().GetValue()
		if date == 0 {
			date = first
		}
		text := ""
		if m := outBox.GetMessage(); m != nil {
			text = m.GetMessage()
		}
		ups, err := schedstore.Append(in.UserId, peer, text, date)
		if err != nil {
			c.Logger.Errorf("msg.sendMessageV2 - schedule persist failed")
			return nil, true, asStored(err)
		}
		combined = joinUpdates(combined, ups)
	}
	return combined, true, nil
}

func (c *MsgCore) persistChannelSend(in *msg.TLMsgSendMessageV2) (*mtproto.Updates, error) {
	if err := c.openStored(); err != nil {
		return nil, err
	}
	boxes := in.GetMessage()
	if len(boxes) == 1 {
		var when int64
		text := ""
		if m := boxes[0].GetMessage(); m != nil {
			when = int64(m.GetDate())
			text = m.GetMessage()
		}
		ups, err := channelview.Post(in.UserId, in.PeerId, text, when)
		if err != nil {
			c.Logger.Errorf("msg.sendMessageV2 - channel persist failed")
			return nil, asStored(err)
		}
		return ups, nil
	}
	texts := make([]string, 0, len(boxes))
	var when int64
	for _, box := range boxes {
		text := ""
		if m := box.GetMessage(); m != nil {
			text = m.GetMessage()
			if when == 0 {
				when = int64(m.GetDate())
			}
		}
		texts = append(texts, text)
	}
	ups, err := channelview.PostAll(in.UserId, in.PeerId, texts, when)
	if err != nil {
		c.Logger.Errorf("msg.sendMessageV2 - channel persist failed")
		return nil, asStored(err)
	}
	return ups, nil
}

func (c *MsgCore) persistChannelEdit(in *msg.TLMsgEditMessageV2) (*mtproto.Updates, error) {
	if in.NewMessage == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if err := c.openStored(); err != nil {
		return nil, err
	}
	id := in.DstMessage.GetMessageId()
	if id == 0 {
		id = in.DstMessage.GetMessage().GetId()
	}
	if id == 0 {
		return nil, mtproto.ErrMessageIdInvalid
	}
	text := ""
	if m := in.NewMessage.GetMessage(); m != nil {
		text = m.GetMessage()
	}
	ups, err := channelview.Edit(in.UserId, in.PeerId, id, text)
	if err != nil {
		c.Logger.Errorf("msg.editMessageV2 - channel persist failed")
		return nil, asStored(err)
	}
	return ups, nil
}

func (c *MsgCore) deleteChatHistory(in *msg.TLMsgDeleteChatHistory) (*mtproto.Bool, error) {
	if in.ChatId == 0 || in.DeleteUserId == 0 {
		c.Logger.Errorf("msg.deleteChatHistory - invalid request")
		return nil, mtproto.ErrInputRequestInvalid
	}
	chat, err := c.svcCtx.Dao.ChatClient.ChatGetMutableChat(c.ctx, &chatpb.TLChatGetMutableChat{
		ChatId: in.ChatId,
	})
	if err != nil {
		c.Logger.Errorf("msg.deleteChatHistory - chat lookup failed")
		return nil, asStored(err)
	}
	if chat == nil {
		return nil, mtproto.ErrPeerIdInvalid
	}
	did := mtproto.MakeDialogId(0, mtproto.PEER_CHAT, in.ChatId)
	var walkErr error
	chat.Walk(func(userId int64, participant *mtproto.ImmutableChatParticipant) error {
		if walkErr != nil {
			return nil
		}
		rows, err := c.svcCtx.Dao.MessagesDAO.SelectDialogMessageIdList(c.ctx, userId, did.A, did.B)
		if err != nil {
			walkErr = err
			return nil
		}
		ids := make([]int32, 0)
		for _, row := range rows {
			if row.SenderUserId == in.DeleteUserId {
				ids = append(ids, row.UserMessageBoxId)
			}
		}
		if len(ids) == 0 {
			return nil
		}
		if _, err = c.svcCtx.Dao.MessagesDAO.DeleteMessagesByMessageIdList(c.ctx, userId, ids); err != nil {
			walkErr = err
		}
		return nil
	})
	if walkErr != nil {
		c.Logger.Errorf("msg.deleteChatHistory - delete failed")
		return nil, asStored(walkErr)
	}
	return mtproto.BoolTrue, nil
}
