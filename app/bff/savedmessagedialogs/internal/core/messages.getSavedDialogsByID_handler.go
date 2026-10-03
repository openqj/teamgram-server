// Copyright 2025 Teamgram Authors
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

// MessagesGetSavedDialogsByID
// messages.getSavedDialogsByID#6f6f9c96 flags:# parent_peer:flags.1?InputPeer ids:Vector<InputPeer> = messages.SavedDialogs;
func (c *SavedMessageDialogsCore) MessagesGetSavedDialogsByID(in *mtproto.TLMessagesGetSavedDialogsByID) (*mtproto.Messages_SavedDialogs, error) {
	if c == nil || c.MD == nil || c.MD.UserId <= 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.DialogClient == nil || c.svcCtx.Dao.MessageClient == nil {
		return nil, mtproto.ErrInternalServerError
	}
	if parent := in.GetParentPeer(); parent != nil {
		p := mtproto.FromInputPeer2(c.MD.UserId, parent)
		if !savedPeerAllowed(p) {
			return nil, mtproto.ErrPeerIdInvalid
		}
	}

	want := make(map[savedPeerKey]struct{}, len(in.GetIds()))
	order := make([]savedPeerKey, 0, len(in.GetIds()))
	for _, id := range in.GetIds() {
		peer := mtproto.FromInputPeer2(c.MD.UserId, id)
		if !savedPeerAllowed(peer) {
			return nil, mtproto.ErrPeerIdInvalid
		}
		key := savedPeerKeyOf(peer.PeerType, peer.PeerId, c.MD.UserId)
		if _, ok := want[key]; ok {
			continue
		}
		want[key] = struct{}{}
		order = append(order, key)
	}

	rValues := mtproto.MakeTLMessagesSavedDialogs(&mtproto.Messages_SavedDialogs{
		Dialogs:  []*mtproto.SavedDialog{},
		Messages: []*mtproto.Message{},
		Chats:    []*mtproto.Chat{},
		Users:    []*mtproto.User{},
	}).To_Messages_SavedDialogs()
	if len(want) == 0 {
		return rValues, nil
	}

	found := make(map[savedPeerKey]*mtproto.SavedDialog, len(want))
	offsetTop := int32(0)
	for page := 0; page < 20 && len(found) < len(want); page++ {
		dialogs, err := c.svcCtx.Dao.DialogClient.DialogGetSavedDialogs(c.ctx, &dialog.TLDialogGetSavedDialogs{
			UserId:        c.MD.UserId,
			ExcludePinned: mtproto.ToBool(false),
			OffsetId:      offsetTop,
			OffsetPeer:    mtproto.MakePeerUtil(mtproto.PEER_EMPTY, 0),
			Limit:         100,
		})
		if err != nil {
			c.Logger.Errorf("messages.getSavedDialogsByID - error: %v", err)
			return nil, err
		}
		if dialogs == nil {
			return nil, mtproto.ErrInternalServerError
		}
		if len(dialogs.Dialogs) == 0 {
			break
		}

		nextTop := int32(0)
		for _, d := range dialogs.Dialogs {
			if d == nil {
				return nil, mtproto.ErrInternalServerError
			}
			if d.TopMessage > 0 && (nextTop == 0 || d.TopMessage < nextTop) {
				nextTop = d.TopMessage
			}
			key, ok := savedPeerKeyFromPeer(d.Peer, c.MD.UserId)
			if !ok {
				continue
			}
			if _, need := want[key]; need {
				if _, already := found[key]; !already {
					found[key] = d
				}
			}
		}
		if len(dialogs.Dialogs) < 100 || nextTop <= 0 || (offsetTop > 0 && nextTop >= offsetTop) {
			break
		}
		offsetTop = nextTop
	}

	var msgIdList []int32
	for _, key := range order {
		d, ok := found[key]
		if !ok {
			continue
		}
		rValues.Dialogs = append(rValues.Dialogs, d)
		if d.TopMessage > 0 {
			msgIdList = append(msgIdList, d.TopMessage)
		}
	}
	var (
		boxList *message.Vector_MessageBox
		err     error
	)
	if len(msgIdList) > 0 {
		boxList, err = c.svcCtx.Dao.MessageClient.MessageGetUserMessageList(c.ctx, &message.TLMessageGetUserMessageList{
			UserId: c.MD.UserId,
			IdList: msgIdList,
		})
		if err != nil {
			c.Logger.Errorf("messages.getSavedDialogsByID - message lookup error: %v", err)
			return nil, err
		}
		if boxList == nil {
			return nil, mtproto.ErrInternalServerError
		}
	}

	rMessages := linkedmap.New()
	var hydrateErr error
	seenChannels := make(map[int64]struct{})
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
				return
			}
			if _, ok := seenChannels[id]; ok {
				continue
			}
			seenChannels[id] = struct{}{}
			channels := c.channelChatsByID(c.MD.UserId, []int64{id})
			if len(channels) != 1 || channels[0] == nil || channels[0].GetId() != id {
				hydrateErr = mtproto.ErrInternalServerError
				return
			}
			rValues.Chats = append(rValues.Chats, channels[0])
		}
	}

	for _, d := range rValues.Dialogs {
		peer := d.GetPeer()
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
				mUsers, userErr := c.svcCtx.Dao.UserClient.UserGetMutableUsers(c.ctx, &userpb.TLUserGetMutableUsers{
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
				mChats, chatErr := c.svcCtx.Dao.ChatClient.ChatGetChatListByIdList(c.ctx, &chatpb.TLChatGetChatListByIdList{
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
		c.Logger.Errorf("messages.getSavedDialogsByID - entity hydration error: %v", hydrateErr)
		return nil, hydrateErr
	}

	return rValues, nil
}
