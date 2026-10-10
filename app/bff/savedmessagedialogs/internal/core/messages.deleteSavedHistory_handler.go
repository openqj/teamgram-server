// Copyright 2024 Teamgram Authors
//  All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// Author: teamgramio (teamgram.io@gmail.com)
//

package core

import (
	"math"

	"github.com/teamgram/proto/mtproto"
	msgpb "github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
	"github.com/teamgram/teamgram-server/app/service/biz/message/message"
)

// MessagesDeleteSavedHistory
// messages.deleteSavedHistory#4dc5085f flags:# parent_peer:flags.0?InputPeer peer:InputPeer max_id:int min_date:flags.2?int max_date:flags.3?int = messages.AffectedHistory;
func (c *SavedMessageDialogsCore) MessagesDeleteSavedHistory(in *mtproto.TLMessagesDeleteSavedHistory) (*mtproto.Messages_AffectedHistory, error) {
	if c == nil || c.MD == nil || c.MD.UserId <= 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if in == nil || in.GetPeer() == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if in.GetMaxId() < 0 {
		return nil, mtproto.ErrMessageIdInvalid
	}
	if parent := in.GetParentPeer(); parent != nil {
		parentPeer := mtproto.FromInputPeer2(c.MD.UserId, parent)
		if parentPeer == nil {
			return nil, mtproto.ErrPeerIdInvalid
		}
		parentKey := savedPeerKeyOf(parentPeer.PeerType, parentPeer.PeerId, c.MD.UserId)
		if parentKey.peerType != mtproto.PEER_USER || parentKey.peerId != c.MD.UserId {
			return nil, mtproto.ErrPeerIdInvalid
		}
	}
	peer := mtproto.FromInputPeer2(c.MD.UserId, in.GetPeer())
	if !savedPeerAllowed(peer) || peer.PeerId <= 0 {
		c.Logger.Errorf("messages.deleteSavedHistory - error: invalid peer")
		return nil, mtproto.ErrPeerIdInvalid
	}
	minDate, maxDate := int32(0), int32(0)
	if in.GetMinDate() != nil {
		minDate = in.GetMinDate().GetValue()
	}
	if in.GetMaxDate() != nil {
		maxDate = in.GetMaxDate().GetValue()
	}
	if minDate < 0 {
		return nil, mtproto.ErrMinDateInvalid
	}
	if maxDate < 0 {
		return nil, mtproto.ErrMaxDateInvalid
	}
	if minDate != 0 && maxDate != 0 && minDate > maxDate {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.MessageClient == nil || c.svcCtx.Dao.MsgClient == nil {
		return nil, mtproto.ErrMethodNotImpl
	}

	// Read and delete in bounded batches. The message service owns the
	// PostgreSQL relation and applies the deletion/pts transaction; the BFF
	// only coordinates the protocol filters and never mutates storage itself.
	const pageSize int32 = 100
	const maxPages = 1000
	var (
		offsetID int32 = math.MaxInt32
		pts      int32
		ptsCount int32
	)
	for page := 0; page < maxPages; page++ {
		boxList, err := c.svcCtx.Dao.MessageGetSavedHistoryMessages(c.ctx, &message.TLMessageGetSavedHistoryMessages{
			UserId:     c.MD.UserId,
			PeerType:   peer.PeerType,
			PeerId:     peer.PeerId,
			OffsetId:   offsetID,
			OffsetDate: 0,
			Limit:      pageSize,
			MaxId:      in.GetMaxId(),
		})
		if err != nil {
			return nil, err
		}
		if boxList == nil || len(boxList.GetBoxList()) == 0 {
			break
		}
		ids := make([]int32, 0, len(boxList.GetBoxList()))
		lastID := offsetID
		for _, box := range boxList.GetBoxList() {
			if box == nil || box.GetMessage() == nil {
				continue
			}
			id := box.GetMessageId()
			if id == 0 && box.GetMessage() != nil {
				id = box.GetMessage().GetId()
			}
			if id <= 0 || id >= offsetID {
				continue
			}
			if id < lastID {
				lastID = id
			}
			if in.GetMaxId() > 0 && id > in.GetMaxId() {
				continue
			}
			date := box.GetMessage().GetDate()
			if date != 0 && minDate != 0 && date < minDate {
				continue
			}
			if date != 0 && maxDate != 0 && date > maxDate {
				continue
			}
			ids = append(ids, id)
		}
		if len(ids) > 0 {
			affected, err := c.svcCtx.Dao.MsgDeleteMessages(c.ctx, &msgpb.TLMsgDeleteMessages{
				UserId:    c.MD.UserId,
				AuthKeyId: c.MD.PermAuthKeyId,
				PeerType:  mtproto.PEER_EMPTY,
				PeerId:    c.MD.UserId,
				Revoke:    false,
				Id:        ids,
			})
			if err != nil {
				return nil, err
			}
			if affected != nil {
				if affected.GetPts() > pts {
					pts = affected.GetPts()
				}
				ptsCount += affected.GetPtsCount()
			}
		}
		if lastID <= offsetID || len(boxList.GetBoxList()) < int(pageSize) {
			break
		}
		offsetID = lastID
	}
	return mtproto.MakeTLMessagesAffectedHistory(&mtproto.Messages_AffectedHistory{
		Pts:      pts,
		PtsCount: ptsCount,
		Offset:   0,
	}).To_Messages_AffectedHistory(), nil
}
