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
	"github.com/teamgram/teamgram-server/app/service/biz/message/message"
)

// MessageGetSavedHistoryMessages
// message.getSavedHistoryMessages user_id:long peer_type:int peer_id:long offset_id:int offset_date:int add_offset:int limit:int max_id:int min_id:int hash:long = MessageBoxList;
func (c *MessageCore) MessageGetSavedHistoryMessages(in *message.TLMessageGetSavedHistoryMessages) (*mtproto.MessageBoxList, error) {
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	hasPostgres := c.svcCtx.Dao.Postgres != nil && c.svcCtx.Dao.Postgres.Store != nil && c.svcCtx.Dao.Postgres.Store.Messages != nil
	if !hasPostgres && !c.svcCtx.Dao.HasLegacyStore() {
		return nil, mtproto.ErrMethodNotImpl
	}
	if in.UserId <= 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if in.Limit < 0 {
		return nil, mtproto.ErrLimitInvalid
	}
	var (
		selfUserId = in.UserId
		peer       = mtproto.MakePeerUtil(in.PeerType, in.PeerId)
		addOffset  = in.AddOffset
		limit      = in.Limit
		offsetId   = in.OffsetId
		minId      = in.MinId
		maxId      = in.MaxId
		hash       = in.Hash
		boxList    []*mtproto.MessageBox
		err        error
	)
	peer = normalizeSavedHistoryPeer(peer, selfUserId)
	if limit > 100 {
		limit = 100
	}
	if peer == nil || peer.PeerId <= 0 {
		return nil, mtproto.ErrPeerIdInvalid
	}

	loadType := loadTypeBackward
	if addOffset >= 0 {
		loadType = loadTypeBackward
	} else if addOffset+limit > 0 {
		loadType = loadTypeFirstAroundDate
	} else {
		loadType = loadTypeForward
	}

	if in.OffsetDate > 0 {
		offsetDate := in.OffsetDate
		switch loadType {
		case loadTypeBackward:
			boxList, err = c.svcCtx.Dao.GetOffsetDateBackwardSavedHistoryMessages(c.ctx, selfUserId, peer, offsetDate, minId, maxId, addOffset+limit, hash)
			if err != nil {
				return nil, err
			}
		case loadTypeFirstAroundDate:
			boxList1, err := c.svcCtx.Dao.GetOffsetDateForwardSavedHistoryMessages(c.ctx, selfUserId, peer, offsetDate, minId, maxId, -addOffset, hash)
			if err != nil {
				return nil, err
			}
			for i, j := 0, len(boxList1)-1; i < j; i, j = i+1, j-1 {
				boxList1[i], boxList1[j] = boxList1[j], boxList1[i]
			}
			boxList = append(boxList, boxList1...)
			boxList2, err := c.svcCtx.Dao.GetOffsetDateBackwardSavedHistoryMessages(c.ctx, selfUserId, peer, offsetDate, minId, maxId, limit+addOffset, hash)
			if err != nil {
				return nil, err
			}
			boxList = append(boxList, boxList2...)
		case loadTypeForward:
			boxList, err = c.svcCtx.Dao.GetOffsetDateForwardSavedHistoryMessages(c.ctx, selfUserId, peer, offsetDate, minId, maxId, -addOffset, hash)
			if err != nil {
				return nil, err
			}
			for i, j := 0, len(boxList)-1; i < j; i, j = i+1, j-1 {
				boxList[i], boxList[j] = boxList[j], boxList[i]
			}
		}
	} else {
		if offsetId == 0 {
			offsetId = math.MaxInt32
		}
		switch loadType {
		case loadTypeBackward:
			boxList, err = c.svcCtx.Dao.GetOffsetIdBackwardSavedHistoryMessages(c.ctx, selfUserId, peer, offsetId, minId, maxId, addOffset+limit, hash)
			if err != nil {
				return nil, err
			}
		case loadTypeFirstAroundDate:
			boxList1, err := c.svcCtx.Dao.GetOffsetIdForwardSavedHistoryMessages(c.ctx, selfUserId, peer, offsetId, minId, maxId, -addOffset, hash)
			if err != nil {
				return nil, err
			}
			for i, j := 0, len(boxList1)-1; i < j; i, j = i+1, j-1 {
				boxList1[i], boxList1[j] = boxList1[j], boxList1[i]
			}
			boxList = append(boxList, boxList1...)
			boxList2, err := c.svcCtx.Dao.GetOffsetIdBackwardSavedHistoryMessages(c.ctx, selfUserId, peer, offsetId, minId, maxId, limit+addOffset, hash)
			if err != nil {
				return nil, err
			}
			boxList = append(boxList, boxList2...)
		case loadTypeForward:
			boxList, err = c.svcCtx.Dao.GetOffsetIdForwardSavedHistoryMessages(c.ctx, selfUserId, peer, offsetId, minId, maxId, -addOffset, hash)
			if err != nil {
				return nil, err
			}
			for i, j := 0, len(boxList)-1; i < j; i, j = i+1, j-1 {
				boxList[i], boxList[j] = boxList[j], boxList[i]
			}
		}
	}

	var (
		count int64
	)
	count, err = c.svcCtx.Dao.CountSavedMessages(c.ctx, selfUserId, peer.PeerType, peer.PeerId)
	if err != nil {
		return nil, err
	}

	return mtproto.MakeTLMessageBoxListSlice(&mtproto.MessageBoxList{
		BoxList: boxList,
		Count:   int32(count),
	}).To_MessageBoxList(), nil
}

func normalizeSavedHistoryPeer(peer *mtproto.PeerUtil, selfUserId int64) *mtproto.PeerUtil {
	if peer != nil && peer.PeerId == selfUserId && (peer.PeerType == mtproto.PEER_SELF || peer.PeerType == mtproto.PEER_USER) {
		return mtproto.MakePeerUtil(mtproto.PEER_USER, selfUserId)
	}
	return peer
}
