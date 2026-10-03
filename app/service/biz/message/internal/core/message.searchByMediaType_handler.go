/*
 * Created from 'scheme.tl' by 'mtprotoc'
 *
 * Copyright (c) 2021-present,  Teamgram Studio (https://teamgram.io).
 *  All rights reserved.
 *
 * Author: teamgramio (teamgram.io@gmail.com)
 */

package core

import (
	"math"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/message/internal/dal/dataobject"
	"github.com/teamgram/teamgram-server/app/service/biz/message/message"
)

// MessageSearchByMediaType
// message.searchByMediaType user_id:long peer:PeerUtil media_type:int offset:int limit:int = Vector<MessageBox>;
func (c *MessageCore) MessageSearchByMediaType(in *message.TLMessageSearchByMediaType) (*mtproto.MessageBoxList, error) {
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if in.Limit < 0 {
		return nil, mtproto.ErrLimitInvalid
	}
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.Mysql == nil {
		return nil, mtproto.ErrInternalServerError
	}
	var (
		boxList []*mtproto.MessageBox
		err     error
		offset  = in.Offset
		limit   = in.Limit
	)
	if offset == 0 {
		offset = math.MaxInt32
	}
	if limit > 50 {
		limit = 50
	}

	switch in.GetMediaType() {
	case mtproto.MEDIA_PHONE_CALL:
		if in.PeerType == mtproto.PEER_UNKNOWN && in.PeerId == 0 {
			boxList, err = c.searchSentByMediaType(in.UserId, in.MediaType, offset, limit)
		} else {
			boxList, err = c.searchByPhoneCall(in.UserId, offset, limit)
		}
	case mtproto.MEDIA_PHOTOVIDEO:
		if in.PeerType == mtproto.PEER_UNKNOWN && in.PeerId == 0 {
			boxList, err = c.searchSentByMediaType(in.UserId, in.MediaType, offset, limit)
		} else {
			boxList, err = c.searchByPhotoVideoMediaType(in.UserId, in.PeerType, in.PeerId, in.MediaType, offset, limit)
		}
	default:
		if in.PeerType == mtproto.PEER_UNKNOWN && in.PeerId == 0 {
			boxList, err = c.searchSentByMediaType(in.UserId, in.MediaType, offset, limit)
		} else {
			boxList, err = c.searchByMediaType(in.UserId, in.PeerType, in.PeerId, in.MediaType, offset, limit)
		}
	}
	if err != nil {
		return nil, err
	}

	return mtproto.MakeTLMessageBoxList(&mtproto.MessageBoxList{
		BoxList: boxList,
	}).To_MessageBoxList(), nil
}

func (c *MessageCore) searchSentByMediaType(userId int64, mediaType int32, offset, limit int32) ([]*mtproto.MessageBox, error) {
	var boxList []*mtproto.MessageBox
	_, err := c.svcCtx.Dao.MessagesDAO.SelectSentByMediaTypeWithCB(
		c.ctx,
		userId,
		mediaType,
		offset,
		limit,
		func(sz, i int, v *dataobject.MessagesDO) {
			boxList = append(boxList, c.svcCtx.Dao.MakeMessageBox(c.ctx, userId, v))
		},
	)
	if err != nil {
		return nil, err
	}
	if boxList == nil {
		boxList = []*mtproto.MessageBox{}
	}
	return boxList, nil
}

func (c *MessageCore) searchByPhotoVideoMediaType(
	userId int64,
	peerType int32,
	peerId int64,
	mediaType int32,
	offset, limit int32) ([]*mtproto.MessageBox, error) {

	var (
		dialogId = mtproto.MakeDialogId(userId, peerType, peerId)
		boxList  []*mtproto.MessageBox
	)

	_, err := c.svcCtx.Dao.MessagesDAO.SelectByPhotoVideoMediaTypeWithCB(
		c.ctx,
		userId,
		dialogId.A,
		dialogId.B,
		mediaType,
		offset,
		limit,
		func(sz, i int, v *dataobject.MessagesDO) {
			boxList = append(boxList, c.svcCtx.Dao.MakeMessageBox(c.ctx, userId, v))
		})
	if err != nil {
		return nil, err
	}

	if boxList == nil {
		boxList = []*mtproto.MessageBox{}
	}

	return boxList, nil
}

func (c *MessageCore) searchByMediaType(
	userId int64,
	peerType int32,
	peerId int64,
	mediaType int32,
	offset, limit int32) ([]*mtproto.MessageBox, error) {

	var (
		dialogId = mtproto.MakeDialogId(userId, peerType, peerId)
		boxList  []*mtproto.MessageBox
	)

	_, err := c.svcCtx.Dao.MessagesDAO.SelectByMediaTypeWithCB(
		c.ctx,
		userId,
		dialogId.A,
		dialogId.B,
		mediaType,
		offset,
		limit,
		func(sz, i int, v *dataobject.MessagesDO) {
			boxList = append(boxList, c.svcCtx.Dao.MakeMessageBox(c.ctx, userId, v))
		})
	if err != nil {
		return nil, err
	}

	if boxList == nil {
		boxList = []*mtproto.MessageBox{}
	}

	return boxList, nil
}

func (c *MessageCore) searchByPhoneCall(userId int64, offset, limit int32) ([]*mtproto.MessageBox, error) {
	var boxList []*mtproto.MessageBox
	_, err := c.svcCtx.Dao.MessagesDAO.SelectPhoneCallListWithCB(
		c.ctx,
		userId,
		mtproto.MEDIA_PHONE_CALL,
		offset,
		limit,
		func(sz, i int, v *dataobject.MessagesDO) {
			boxList = append(boxList, c.svcCtx.Dao.MakeMessageBox(c.ctx, userId, v))
		})
	if err != nil {
		return nil, err
	}

	if boxList == nil {
		boxList = []*mtproto.MessageBox{}
	}

	return boxList, nil
}
