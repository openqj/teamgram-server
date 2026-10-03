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
	"github.com/teamgram/teamgram-server/app/bff/apifull/channelview"
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	"github.com/teamgram/teamgram-server/app/service/biz/message/message"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

const recentLocationsHistoryPageSize int32 = 100

// MessagesGetRecentLocations
// messages.getRecentLocations#702a40e0 peer:InputPeer limit:int hash:long = messages.Messages;
func (c *MessagesCore) MessagesGetRecentLocations(in *mtproto.TLMessagesGetRecentLocations) (*mtproto.Messages_Messages, error) {
	if c == nil || c.MD == nil || c.MD.UserId <= 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.MessageClient == nil {
		return nil, mtproto.ErrInternalServerError
	}
	if in == nil || in.GetPeer() == nil {
		c.Logger.Errorf("messages.getRecentLocations - error: %v", mtproto.ErrPeerIdInvalid)
		return nil, mtproto.ErrPeerIdInvalid
	}
	peer := mtproto.FromInputPeer2(c.MD.UserId, in.Peer)
	if peer == nil || !peer.IsUserOrChatOrChannel() || peer.PeerId == 0 {
		c.Logger.Errorf("messages.getRecentLocations - error: %v", mtproto.ErrPeerIdInvalid)
		return nil, mtproto.ErrPeerIdInvalid
	}

	limit := in.GetLimit()
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}

	if peer.IsChannel() {
		return c.getRecentChannelLocations(in.Peer)
	}
	geo, err := c.getRecentLocationMessageBoxes(peer, limit)
	if err != nil {
		c.Logger.Errorf("messages.getRecentLocations - error: %v", err)
		return nil, err
	}

	filtered := &message.Vector_MessageBox{Datas: geo}
	var (
		messages []*mtproto.Message
		users    []*mtproto.User
		chats    []*mtproto.Chat
		visitErr error
	)
	filtered.Visit(c.MD.UserId,
		func(messageList []*mtproto.Message) {
			messages = messageList
		},
		func(userIdList []int64) {
			if visitErr != nil {
				return
			}
			mUsers, err := c.svcCtx.Dao.UserClient.UserGetMutableUsers(c.ctx, &userpb.TLUserGetMutableUsers{Id: userIdList})
			if err != nil {
				visitErr = err
				return
			}
			if mUsers == nil {
				visitErr = mtproto.ErrInternalServerError
				return
			}
			hydrated := mUsers.GetUserListByIdList(c.MD.UserId, userIdList...)
			users = append(users, hydrated...)
		},
		func(chatIdList []int64) {
			if visitErr != nil {
				return
			}
			if c.svcCtx.Dao.ChatClient == nil || c.svcCtx.Dao.ChatClient.Client() == nil {
				visitErr = mtproto.ErrInternalServerError
				return
			}
			mChats, err := c.svcCtx.Dao.ChatClient.Client().ChatGetChatListByIdList(c.ctx, &chatpb.TLChatGetChatListByIdList{
				SelfId: c.MD.UserId,
				IdList: chatIdList,
			})
			if err != nil {
				visitErr = err
				return
			}
			if mChats == nil {
				visitErr = mtproto.ErrInternalServerError
				return
			}
			hydrated := mChats.GetChatListByIdList(c.MD.UserId, chatIdList...)
			chats = append(chats, hydrated...)
		}, nil)
	if visitErr != nil {
		c.Logger.Errorf("messages.getRecentLocations - error: %v", visitErr)
		return nil, visitErr
	}

	return mtproto.MakeTLMessagesMessages(&mtproto.Messages_Messages{
		Messages: messages,
		Users:    mtproto.ToSafeUsers(users),
		Chats:    mtproto.ToSafeChats(chats),
	}).To_Messages_Messages(), nil
}

func (c *MessagesCore) getRecentLocationMessageBoxes(peer *mtproto.PeerUtil, limit int32) ([]*mtproto.MessageBox, error) {
	var (
		geo     []*mtproto.MessageBox
		offset  int32
		lastMin int32
	)
	for {
		page, err := c.svcCtx.Dao.MessageClient.MessageGetHistoryMessages(c.ctx, &message.TLMessageGetHistoryMessages{
			UserId:   c.MD.UserId,
			PeerType: peer.PeerType,
			PeerId:   peer.PeerId,
			OffsetId: offset,
			Limit:    recentLocationsHistoryPageSize,
		})
		if err != nil {
			return nil, err
		}
		if page == nil {
			return nil, mtproto.ErrInternalServerError
		}
		boxes := page.GetDatas()
		if len(boxes) == 0 {
			return geo, nil
		}

		var minID int32
		for _, box := range boxes {
			if box == nil {
				continue
			}
			if box.GetMessageId() > 0 && (minID == 0 || box.GetMessageId() < minID) {
				minID = box.GetMessageId()
			}
			if messageHasGeo(box.ToMessage(c.MD.UserId)) {
				geo = append(geo, box)
				if int32(len(geo)) == limit {
					return geo, nil
				}
			}
		}
		if int32(len(boxes)) < recentLocationsHistoryPageSize {
			return geo, nil
		}
		if minID == 0 || (offset != 0 && minID >= offset) || minID == lastMin {
			return nil, mtproto.ErrInternalServerError
		}
		lastMin = minID
		offset = minID
	}
}

func (c *MessagesCore) getRecentChannelLocations(peer *mtproto.InputPeer) (*mtproto.Messages_Messages, error) {
	if _, err := channelview.HistoryForInputPeer(c.MD.UserId, peer, 0, 1); err != nil {
		c.Logger.Errorf("messages.getRecentLocations - error: %v", err)
		return nil, err
	}
	return nil, mtproto.ErrMethodNotImpl
}
