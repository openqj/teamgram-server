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
	"github.com/teamgram/teamgram-server/app/service/biz/message/message"
)

// MessageGetSearchCounter
// message.getSearchCounter user_id:long peer_type:int peer_id:long media_type:int = Int32;
func (c *MessageCore) MessageGetSearchCounter(in *message.TLMessageGetSearchCounter) (*mtproto.Int32, error) {
	if in == nil || in.GetUserId() <= 0 || in.GetPeerId() <= 0 {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil {
		return nil, mtproto.ErrInternalServerError
	}
	switch in.GetPeerType() {
	case mtproto.PEER_SELF, mtproto.PEER_USER, mtproto.PEER_CHAT, mtproto.PEER_CHANNEL:
	default:
		return nil, mtproto.ErrPeerIdInvalid
	}
	switch in.GetMediaType() {
	case mtproto.MEDIA_PHOTOVIDEO,
		mtproto.MEDIA_FILE,
		mtproto.MEDIA_AUDIO,
		mtproto.MEDIA_URL,
		mtproto.MEDIA_MUSIC,
		mtproto.MEDIA_PHONE_CALL,
		mtproto.MEDIA_GIF,
		mtproto.MEDIA_PHOTOS_ONLY,
		mtproto.MEDIA_VIDEOS_ONLY,
		mtproto.MEDIA_VOICE_FILE,
		mtproto.MEDIA_CHAT_PHOTO,
		mtproto.MEDIA_ROUND_FILE,
		mtproto.MEDIA_PINNED:
	default:
		return nil, mtproto.ErrMethodNotImpl
	}
	dialogID := mtproto.MakeDialogId(in.UserId, in.PeerType, in.PeerId)
	count, err := c.svcCtx.Dao.CountMessageMedia(c.ctx, in.UserId, dialogID.A, dialogID.B, in.MediaType)
	if err != nil {
		return nil, err
	}
	if count < 0 || count > math.MaxInt32 {
		return nil, mtproto.ErrInternalServerError
	}

	return &mtproto.Int32{
		V: int32(count),
	}, nil
}
