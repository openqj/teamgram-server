package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
)

// MsgReadMentions clears unread mention markers without advancing the dialog
// read cursor or clearing media-unread state.
func (c *MsgCore) MsgReadMentions(in *msg.TLMsgReadMentions) (*mtproto.Messages_AffectedHistory, error) {
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if in.GetPeerId() <= 0 {
		return nil, mtproto.ErrPeerIdInvalid
	}
	if in.GetTopMsgId() != nil && in.GetTopMsgId().GetValue() <= 0 {
		return nil, mtproto.ErrMessageIdInvalid
	}
	if in.GetPeerType() != mtproto.PEER_CHAT {
		return nil, mtproto.ErrPeerIdInvalid
	}

	topMsgID := in.GetTopMsgId()
	var topMsgIDValue int32
	if topMsgID != nil {
		topMsgIDValue = topMsgID.GetValue()
	}
	if c.svcCtx.Dao.Postgres != nil {
		updates, pts, err := c.svcCtx.Dao.ReadMentionsState(c.ctx, in.UserId, in.PeerId, topMsgIDValue, topMsgID != nil)
		if err != nil {
			return nil, err
		}
		var count int32
		for _, update := range updates {
			count += update.PtsCount
		}
		return mtproto.MakeTLMessagesAffectedHistory(&mtproto.Messages_AffectedHistory{Pts: pts, PtsCount: count}).To_Messages_AffectedHistory(), nil
	}
	cleared, err := c.svcCtx.Dao.ClearMentions(
		c.ctx,
		in.GetUserId(),
		in.GetPeerId(),
		topMsgIDValue,
		topMsgID != nil,
	)
	if err != nil {
		c.Logger.Errorf("msg.readMentions - error: %v", err)
		return nil, err
	}
	pts := c.svcCtx.Dao.IDGenClient2.CurrentPtsId(c.ctx, in.GetUserId())
	if cleared > 0 {
		pts = c.svcCtx.Dao.IDGenClient2.NextNPtsId(c.ctx, in.GetUserId(), int(cleared))
	}
	return mtproto.MakeTLMessagesAffectedHistory(&mtproto.Messages_AffectedHistory{
		Pts:      pts,
		PtsCount: cleared,
		Offset:   0,
	}).To_Messages_AffectedHistory(), nil
}
