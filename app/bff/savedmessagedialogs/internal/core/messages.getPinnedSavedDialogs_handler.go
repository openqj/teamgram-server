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

// MessagesGetPinnedSavedDialogs
// messages.getPinnedSavedDialogs#d63d94e0 = messages.SavedDialogs;
func (c *SavedMessageDialogsCore) MessagesGetPinnedSavedDialogs(in *mtproto.TLMessagesGetPinnedSavedDialogs) (*mtproto.Messages_SavedDialogs, error) {
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
		msgIdList []int32
		rMessages = linkedmap.New()
		rValues   = mtproto.MakeTLMessagesSavedDialogs(&mtproto.Messages_SavedDialogs{
			Dialogs:  []*mtproto.SavedDialog{},
			Messages: []*mtproto.Message{},
			Chats:    []*mtproto.Chat{},
			Users:    []*mtproto.User{},
		}).To_Messages_SavedDialogs()
	)

	dialogs, err := c.svcCtx.Dao.DialogClient.DialogGetPinnedSavedDialogs(c.ctx, &dialog.TLDialogGetPinnedSavedDialogs{
		UserId: c.MD.UserId,
	})
	if err != nil {
		c.Logger.Errorf("messages.getPinnedSavedDialogs - error: %v", err)
		return nil, err
	}
	if dialogs == nil {
		return nil, mtproto.ErrInternalServerError
	}
	if len(dialogs.Dialogs) == 0 {
		return rValues, nil
	}

	rValues.Dialogs = dialogs.Dialogs

	for _, dialog := range rValues.Dialogs {
		if dialog == nil {
			return nil, mtproto.ErrInternalServerError
		}
		if dialog.TopMessage > 0 {
			msgIdList = append(msgIdList, dialog.TopMessage)
		}
	}

	var boxList *message.Vector_MessageBox
	if len(msgIdList) > 0 {
		var err error
		boxList, err = c.svcCtx.Dao.MessageClient.MessageGetUserMessageList(
			c.ctx,
			&message.TLMessageGetUserMessageList{
				UserId: c.MD.UserId,
				IdList: msgIdList,
			})
		if err != nil {
			c.Logger.Errorf("messages.getPinnedSavedDialogs - message lookup error: %v", err)
			return nil, err
		}
		if boxList == nil {
			return nil, mtproto.ErrInternalServerError
		}
	}

	seenChannel := map[int64]struct{}{}
	var hydrateErr error
	appendChannels := func(ids []int64) {
		if hydrateErr != nil {
			return
		}
		if c.channelChatsByID == nil {
			hydrateErr = mtproto.ErrMethodNotImpl
			return
		}
		for _, id := range ids {
			if id <= 0 {
				hydrateErr = mtproto.ErrInternalServerError
				continue
			}
			if _, ok := seenChannel[id]; ok {
				continue
			}
			seenChannel[id] = struct{}{}
			channels := c.channelChatsByID(c.MD.UserId, []int64{id})
			if len(channels) != 1 || channels[0] == nil || channels[0].GetId() != id {
				hydrateErr = mtproto.ErrInternalServerError
				return
			}
			rValues.Chats = append(rValues.Chats, channels[0])
		}
	}

	for _, saved := range rValues.Dialogs {
		peer := saved.GetPeer()
		if peer != nil && peer.GetPredicateName() == mtproto.Predicate_peerChannel {
			appendChannels([]int64{peer.GetChannelId()})
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
					hydrateErr = mtproto.ErrMethodNotImpl
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
					hydrateErr = mtproto.ErrMethodNotImpl
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
	if hydrateErr != nil {
		c.Logger.Errorf("messages.getPinnedSavedDialogs - entity hydration error: %v", hydrateErr)
		return nil, hydrateErr
	}

	return rValues, nil
}
