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
	"sync"

	"github.com/teamgram/marmota/pkg/container2/linkedmap"
	"github.com/teamgram/proto/mtproto"
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	"github.com/teamgram/teamgram-server/app/service/biz/message/message"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// MessagesGetSavedDialogs
// messages.getSavedDialogs#5381d21a flags:# exclude_pinned:flags.0?true offset_date:int offset_id:int offset_peer:InputPeer limit:int hash:long = messages.SavedDialogs;
func (c *DialogsCore) MessagesGetSavedDialogs(in *mtproto.TLMessagesGetSavedDialogs) (*mtproto.Messages_SavedDialogs, error) {
	if c == nil || c.MD == nil || c.MD.UserId <= 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if in == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.DialogClient == nil || c.svcCtx.Dao.MessageClient == nil {
		return nil, mtproto.ErrInputRequestInvalid
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
	}

	seenChannel := map[int64]struct{}{}
	var hydrationErr error
	var hydrationErrMu sync.Mutex
	recordHydrationErr := func(err error) {
		if err == nil {
			return
		}
		hydrationErrMu.Lock()
		if hydrationErr == nil {
			hydrationErr = err
		}
		hydrationErrMu.Unlock()
	}
	appendChannels := func(ids []int64) {
		var missing []int64
		for _, id := range ids {
			if id == 0 {
				continue
			}
			if _, ok := seenChannel[id]; ok {
				continue
			}
			seenChannel[id] = struct{}{}
			missing = append(missing, id)
		}
		if len(missing) == 0 {
			return
		}
		if c.svcCtx.Plugin == nil {
			recordHydrationErr(mtproto.ErrMethodNotImpl)
			return
		}
		channels := c.svcCtx.Plugin.GetChannelListByIdList(c.ctx, c.MD.UserId, missing...)
		if len(channels) != len(missing) {
			recordHydrationErr(mtproto.ErrInternalServerError)
			return
		}
		for _, channel := range channels {
			if channel == nil {
				recordHydrationErr(mtproto.ErrInternalServerError)
				return
			}
		}
		rValues.Chats = append(rValues.Chats, channels...)
	}

	if boxList != nil {
		boxList.Visit(c.MD.UserId,
			func(messageList []*mtproto.Message) {
				for _, msg := range messageList {
					if msg == nil || msg.GetId() <= 0 {
						recordHydrationErr(mtproto.ErrInternalServerError)
						continue
					}
					rMessages.Add(msg.Id, msg)
				}
				for i := rMessages.First(); i != nil; i = i.Next() {
					rValues.Messages = append(rValues.Messages, i.Value().(*mtproto.Message))
				}
			},
			func(userIdList []int64) {
				if c.svcCtx.Dao.UserClient == nil {
					recordHydrationErr(mtproto.ErrInternalServerError)
					return
				}
				mUsers, err := c.svcCtx.Dao.UserClient.UserGetMutableUsers(
					c.ctx,
					&userpb.TLUserGetMutableUsers{
						Id: userIdList,
					})
				if err != nil {
					recordHydrationErr(err)
					return
				}
				if mUsers == nil {
					recordHydrationErr(mtproto.ErrInternalServerError)
					return
				}
				hydrated := mUsers.GetUserListByIdList(c.MD.UserId, userIdList...)
				if len(hydrated) != len(userIdList) {
					recordHydrationErr(mtproto.ErrUserIdInvalid)
					return
				}
				rValues.Users = append(rValues.Users, hydrated...)
			},
			func(chatIdList []int64) {
				if c.svcCtx.Dao.ChatClient == nil {
					recordHydrationErr(mtproto.ErrInternalServerError)
					return
				}
				mChats, err := c.svcCtx.Dao.ChatClient.ChatGetChatListByIdList(
					c.ctx,
					&chatpb.TLChatGetChatListByIdList{
						IdList: chatIdList,
					})
				if err != nil {
					recordHydrationErr(err)
					return
				}
				if mChats == nil {
					recordHydrationErr(mtproto.ErrInternalServerError)
					return
				}
				hydrated := mChats.GetChatListByIdList(c.MD.UserId, chatIdList...)
				if len(hydrated) != len(chatIdList) {
					recordHydrationErr(mtproto.ErrChatIdInvalid)
					return
				}
				rValues.Chats = append(rValues.Chats, hydrated...)
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
	hydrationErrMu.Lock()
	err = hydrationErr
	hydrationErrMu.Unlock()
	if err != nil {
		c.Logger.Errorf("messages.getSavedDialogs - entity hydration error: %v", err)
		return nil, err
	}

	return rValues, nil
}
