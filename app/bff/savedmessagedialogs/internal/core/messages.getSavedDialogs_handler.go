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
	"github.com/teamgram/marmota/pkg/container2/linkedmap"
	"github.com/teamgram/proto/mtproto"
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	"github.com/teamgram/teamgram-server/app/service/biz/message/message"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// MessagesGetSavedDialogs
// messages.getSavedDialogs#5381d21a flags:# exclude_pinned:flags.0?true offset_date:int offset_id:int offset_peer:InputPeer limit:int hash:long = messages.SavedDialogs;
func (c *SavedMessageDialogsCore) MessagesGetSavedDialogs(in *mtproto.TLMessagesGetSavedDialogs) (*mtproto.Messages_SavedDialogs, error) {
	if c == nil || c.MD == nil || c.MD.UserId <= 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.DialogClient == nil || c.svcCtx.Dao.MessageClient == nil {
		return nil, mtproto.ErrInternalServerError
	}
	var (
		offsetPeer = mtproto.FromInputPeer2(c.MD.UserId, in.OffsetPeer)
		limit      = in.Limit
		msgIdList  []int32
		rMessages  = linkedmap.New()
		rValues    = mtproto.MakeTLMessagesSavedDialogsSlice(&mtproto.Messages_SavedDialogs{
			Dialogs:  []*mtproto.SavedDialog{},
			Messages: []*mtproto.Message{},
			Chats:    []*mtproto.Chat{},
			Users:    []*mtproto.User{},
			Count:    0,
		}).To_Messages_SavedDialogs()
	)

	if limit > 500 {
		limit = 500
	}
	if limit < 0 {
		return nil, mtproto.ErrLimitInvalid
	}

	dialogs, err := c.svcCtx.Dao.DialogClient.DialogGetSavedDialogs(c.ctx, &dialog.TLDialogGetSavedDialogs{
		UserId:        c.MD.UserId,
		ExcludePinned: mtproto.ToBool(in.ExcludePinned),
		OffsetDate:    in.GetOffsetDate(),
		OffsetId:      in.OffsetId,
		OffsetPeer:    offsetPeer,
		Limit:         limit,
	})
	if err != nil {
		c.Logger.Errorf("messages.getDialogs - error: %v", err)
		return nil, err
	}
	if dialogs == nil {
		return nil, mtproto.ErrInternalServerError
	}
	if len(dialogs.Dialogs) == 0 {
		return rValues, nil
	}

	rValues.Dialogs = dialogs.Dialogs
	rValues.Count = dialogs.Count

	for _, saved := range rValues.Dialogs {
		if saved == nil {
			return nil, mtproto.ErrInternalServerError
		}
		if saved.TopMessage > 0 {
			msgIdList = append(msgIdList, saved.TopMessage)
		}
	}

	var boxList *message.Vector_MessageBox
	if len(msgIdList) > 0 {
		boxList, err = c.svcCtx.Dao.MessageClient.MessageGetUserMessageList(
			c.ctx,
			&message.TLMessageGetUserMessageList{
				UserId: c.MD.UserId,
				IdList: msgIdList,
			})
		if err != nil {
			c.Logger.Errorf("messages.getSavedDialogs - message lookup error: %v", err)
			return nil, err
		}
		if boxList == nil {
			return nil, mtproto.ErrInternalServerError
		}
	}

	seenChannel := map[int64]struct{}{}
	var hydrateErr error
	appendChannels := func(ids []int64) {
		for _, id := range ids {
			if id == 0 {
				continue
			}
			if _, ok := seenChannel[id]; ok {
				continue
			}
			seenChannel[id] = struct{}{}
			if c.channelChatsByID == nil {
				hydrateErr = mtproto.ErrMethodNotImpl
				return
			}
			channels := c.channelChatsByID(c.MD.UserId, []int64{id})
			if len(channels) != 1 || channels[0] == nil || channels[0].GetId() != id {
				hydrateErr = mtproto.ErrInternalServerError
				return
			}
			rValues.Chats = append(rValues.Chats, channels[0])
		}
	}

	if boxList != nil {
		boxList.Visit(c.MD.UserId,
			func(messageList []*mtproto.Message) {
				for _, msg := range messageList {
					if msg == nil || msg.GetId() <= 0 {
						hydrateErr = mtproto.ErrInternalServerError
						return
					}
					rMessages.Add(msg.Id, msg)
				}
				for i := rMessages.First(); i != nil; i = i.Next() {
					rValues.Messages = append(rValues.Messages, i.Value().(*mtproto.Message))
				}
			},
			func(userIdList []int64) {
				if c.svcCtx.Dao.UserClient == nil {
					hydrateErr = mtproto.ErrInternalServerError
					return
				}
				mUsers, userErr := c.svcCtx.Dao.UserClient.UserGetMutableUsers(
					c.ctx,
					&userpb.TLUserGetMutableUsers{
						Id: userIdList,
					})
				if userErr != nil {
					hydrateErr = userErr
					return
				}
				if mUsers == nil {
					hydrateErr = mtproto.ErrInternalServerError
					return
				}
				rValues.Users = append(rValues.Users, mUsers.GetUserListByIdList(c.MD.UserId, userIdList...)...)
			},
			func(chatIdList []int64) {
				if c.svcCtx.Dao.ChatClient == nil {
					hydrateErr = mtproto.ErrInternalServerError
					return
				}
				mChats, chatErr := c.svcCtx.Dao.ChatClient.ChatGetChatListByIdList(
					c.ctx,
					&chatpb.TLChatGetChatListByIdList{
						IdList: chatIdList,
					})
				if chatErr != nil {
					hydrateErr = chatErr
					return
				}
				if mChats == nil {
					hydrateErr = mtproto.ErrInternalServerError
					return
				}
				rValues.Chats = append(rValues.Chats, mChats.GetChatListByIdList(c.MD.UserId, chatIdList...)...)
			},
			func(channelIdList []int64) {
				appendChannels(channelIdList)
			})
	}

	var peerChannels []int64
	for _, saved := range rValues.Dialogs {
		peer := saved.GetPeer()
		if peer != nil && peer.GetPredicateName() == mtproto.Predicate_peerChannel {
			peerChannels = append(peerChannels, peer.GetChannelId())
		}
	}
	appendChannels(peerChannels)
	if hydrateErr != nil {
		c.Logger.Errorf("messages.getSavedDialogs - entity hydration error: %v", hydrateErr)
		return nil, hydrateErr
	}

	return rValues, nil
}
