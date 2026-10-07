// Copyright 2022 Teamgram Authors
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
	"github.com/teamgram/proto/mtproto"
	msgpb "github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	"github.com/teamgram/teamgram-server/app/service/biz/message/message"
)

// MessagesDeleteHistory
// messages.deleteHistory#b08f922a flags:# just_clear:flags.0?true revoke:flags.1?true peer:InputPeer max_id:int min_date:flags.2?int max_date:flags.3?int = messages.AffectedHistory;
func (c *MessagesCore) MessagesDeleteHistory(in *mtproto.TLMessagesDeleteHistory) (*mtproto.Messages_AffectedHistory, error) {
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	var (
		peer = mtproto.FromInputPeer2(c.MD.UserId, in.Peer)
	)

	if peer.IsChannel() {
		c.Logger.Errorf("messages.deleteHistory - error: %v", mtproto.ErrChannelInvalid)
		return nil, mtproto.ErrChannelInvalid
	}

	if !peer.IsChatOrUser() {
		err := mtproto.ErrPeerIdInvalid
		c.Logger.Errorf("messages.deleteHistory - error: %v", err)
		return nil, err
	}

	if hasNonZeroDateFilter(in) {
		return c.deleteHistoryByDate(peer, in)
	}

	affectedHistory, err := c.svcCtx.Dao.MsgClient.MsgDeleteHistory(c.ctx, &msgpb.TLMsgDeleteHistory{
		UserId:    c.MD.UserId,
		AuthKeyId: c.MD.PermAuthKeyId,
		PeerType:  peer.PeerType,
		PeerId:    peer.PeerId,
		JustClear: in.GetJustClear(),
		Revoke:    in.Revoke,
		MaxId:     in.MaxId,
	})

	if err != nil {
		c.Logger.Errorf("messages.deleteHistory - error: %v", err)
		return nil, err
	}
	if affectedHistory == nil {
		return nil, mtproto.ErrInternalServerError
	}

	if !in.GetJustClear() {
		if peer.IsUser() {
			if _, err = c.svcCtx.Dao.DialogDeleteDialog(c.ctx, &dialog.TLDialogDeleteDialog{
				UserId:   c.MD.UserId,
				PeerType: peer.PeerType,
				PeerId:   peer.PeerId,
			}); err != nil {
				c.Logger.Errorf("messages.deleteHistory - delete local dialog: %v", err)
				return nil, err
			}
			if in.Revoke && !peer.IsSelf() {
				if _, err = c.svcCtx.Dao.DialogDeleteDialog(c.ctx, &dialog.TLDialogDeleteDialog{
					UserId:   peer.PeerId,
					PeerType: peer.PeerType,
					PeerId:   c.MD.UserId,
				}); err != nil {
					c.Logger.Errorf("messages.deleteHistory - delete peer dialog: %v", err)
					return nil, err
				}
			}
		}
	}

	return affectedHistory, nil
}

// GramJS may encode optional min/max date fields as empty wrappers when the
// caller sends zero. Zero has the protocol's "not set" meaning and must use
// the normal peer-aware history deletion path.
func hasNonZeroDateFilter(in *mtproto.TLMessagesDeleteHistory) bool {
	return (in.GetMinDate() != nil && in.GetMinDate().GetValue() != 0) ||
		(in.GetMaxDate() != nil && in.GetMaxDate().GetValue() != 0)
}

func (c *MessagesCore) deleteHistoryByDate(peer *mtproto.PeerUtil, in *mtproto.TLMessagesDeleteHistory) (*mtproto.Messages_AffectedHistory, error) {
	peerType := peer.PeerType
	peerID := peer.PeerId
	if peerType == mtproto.PEER_SELF {
		peerType = mtproto.PEER_USER
		peerID = c.MD.UserId
	}

	var minDate, maxDate int32
	if w := in.GetMinDate(); w != nil {
		minDate = w.GetValue()
	}
	if w := in.GetMaxDate(); w != nil {
		maxDate = w.GetValue()
	}

	const (
		pageSize int32 = 100
		maxPages       = 20
	)
	offsetID := int32(0)
	ids := make([]int32, 0)
	exhausted := false
	for page := 0; page < maxPages; page++ {
		boxList, err := c.svcCtx.Dao.MessageClient.MessageGetHistoryMessages(c.ctx, &message.TLMessageGetHistoryMessages{
			UserId:   c.MD.UserId,
			PeerType: peerType,
			PeerId:   peerID,
			OffsetId: offsetID,
			Limit:    pageSize,
		})
		if err != nil {
			c.Logger.Errorf("messages.deleteHistory - error: %v", err)
			return nil, err
		}
		var datas []*mtproto.MessageBox
		if boxList != nil {
			datas = boxList.GetDatas()
		}
		if len(datas) == 0 {
			exhausted = true
			break
		}
		oldestID := int32(0)
		oldestDate := int32(0)
		for _, box := range datas {
			if box == nil || box.GetMessage() == nil {
				continue
			}
			id := box.GetMessageId()
			date := box.GetMessage().GetDate()
			if oldestID == 0 || id < oldestID {
				oldestID = id
				oldestDate = date
			}
			if minDate != 0 && date < minDate {
				continue
			}
			if maxDate != 0 && date > maxDate {
				continue
			}
			if id == 0 {
				continue
			}
			ids = append(ids, id)
		}
		if oldestID == 0 || oldestID == offsetID {
			exhausted = true
			break
		}
		offsetID = oldestID
		if int32(len(datas)) < pageSize || (minDate != 0 && oldestDate < minDate) {
			exhausted = true
			break
		}
	}

	if len(ids) == 0 {
		pts := c.svcCtx.Dao.IDGenClient2.CurrentPtsId(c.ctx, c.MD.UserId)
		return mtproto.MakeTLMessagesAffectedHistory(&mtproto.Messages_AffectedHistory{
			Pts:      pts,
			PtsCount: 0,
			Offset:   0,
		}).To_Messages_AffectedHistory(), nil
	}

	offset := int32(0)
	if !exhausted || len(ids) > int(pageSize) {
		offset = 1
	}
	if len(ids) > int(pageSize) {
		ids = ids[:pageSize]
	}
	affected, err := c.svcCtx.Dao.MsgClient.MsgDeleteMessages(c.ctx, &msgpb.TLMsgDeleteMessages{
		UserId:    c.MD.UserId,
		AuthKeyId: c.MD.PermAuthKeyId,
		PeerType:  peerType,
		PeerId:    peerID,
		Revoke:    in.Revoke,
		Id:        ids,
	})
	if err != nil {
		c.Logger.Errorf("messages.deleteHistory - error: %v", err)
		return nil, err
	}
	if affected == nil {
		return nil, mtproto.ErrInternalServerError
	}
	return mtproto.MakeTLMessagesAffectedHistory(&mtproto.Messages_AffectedHistory{
		Pts:      affected.GetPts(),
		PtsCount: affected.GetPtsCount(),
		Offset:   offset,
	}).To_Messages_AffectedHistory(), nil
}
