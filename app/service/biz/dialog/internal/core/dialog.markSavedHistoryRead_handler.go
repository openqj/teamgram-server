package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
)

// DialogMarkSavedHistoryRead persists the caller's read cursor for an existing saved dialog.
func (c *DialogCore) DialogMarkSavedHistoryRead(in *dialog.TLDialogInsertOrUpdateDialog) (*mtproto.Bool, error) {
	if in == nil || in.GetUserId() <= 0 || in.GetPeerId() <= 0 {
		return nil, mtproto.ErrPeerIdInvalid
	}

	readMaxId := in.GetReadInboxMaxId()
	if readMaxId == nil || readMaxId.GetValue() < 0 {
		return nil, mtproto.ErrMessageIdInvalid
	}
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.SavedDialogsDAO == nil {
		return nil, mtproto.ErrMethodNotImpl
	}

	peerTypes := []int32{in.GetPeerType()}
	switch in.GetPeerType() {
	case mtproto.PEER_SELF:
		peerTypes = append(peerTypes, mtproto.PEER_USER)
	case mtproto.PEER_USER:
		if in.GetPeerId() == in.GetUserId() {
			peerTypes = append(peerTypes, mtproto.PEER_SELF)
		}
	case mtproto.PEER_CHAT, mtproto.PEER_CHANNEL:
	default:
		return nil, mtproto.ErrPeerIdInvalid
	}

	found := false
	for _, peerType := range peerTypes {
		savedDialog, err := c.svcCtx.Dao.SavedDialogsDAO.Select(c.ctx, in.GetUserId(), peerType, in.GetPeerId())
		if err != nil {
			return nil, err
		}
		if savedDialog == nil {
			continue
		}
		found = true
		if _, err = c.svcCtx.Dao.SavedDialogsDAO.UpdateReadMaxId(
			c.ctx,
			readMaxId.GetValue(),
			in.GetUserId(),
			peerType,
			in.GetPeerId(),
		); err != nil {
			return nil, err
		}
	}
	if !found {
		return nil, mtproto.ErrPeerIdInvalid
	}

	return mtproto.BoolTrue, nil
}
