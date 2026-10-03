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
	"time"

	"github.com/teamgram/proto/mtproto"
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	"github.com/teamgram/teamgram-server/app/service/biz/message/message"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// MessagesGetExtendedMedia
// messages.getExtendedMedia#84f80814 peer:InputPeer id:Vector<int> = Updates;
func (c *MessagesCore) MessagesGetExtendedMedia(in *mtproto.TLMessagesGetExtendedMedia) (*mtproto.Updates, error) {
	empty := mtproto.MakeTLUpdates(&mtproto.Updates{
		Updates: []*mtproto.Update{},
		Users:   []*mtproto.User{},
		Chats:   []*mtproto.Chat{},
		Date:    int32(time.Now().Unix()),
	}).To_Updates()

	if in.GetPeer() == nil {
		c.Logger.Errorf("messages.getExtendedMedia - error: %v", mtproto.ErrPeerIdInvalid)
		return nil, mtproto.ErrPeerIdInvalid
	}
	peer := mtproto.FromInputPeer2(c.MD.UserId, in.Peer)
	if peer.IsChannel() {
		c.Logger.Errorf("messages.getExtendedMedia - error: %v", mtproto.ErrChannelInvalid)
		return nil, mtproto.ErrChannelInvalid
	}
	if !peer.IsChatOrUser() {
		c.Logger.Errorf("messages.getExtendedMedia - error: %v", mtproto.ErrPeerIdInvalid)
		return nil, mtproto.ErrPeerIdInvalid
	}
	if len(in.GetId()) == 0 {
		return empty, nil
	}

	boxes, err := c.svcCtx.Dao.MessageClient.MessageGetUserMessageList(c.ctx, &message.TLMessageGetUserMessageList{
		UserId: c.MD.UserId,
		IdList: in.GetId(),
	})
	if err != nil {
		c.Logger.Errorf("messages.getExtendedMedia - error: %v", err)
		return nil, err
	}

	updates := make([]*mtproto.Update, 0)
	matched := make([]*mtproto.MessageBox, 0)
	for _, box := range boxes.GetDatas() {
		if box == nil || box.GetMessage() == nil || !boxInPeer(peer, box, c.MD.UserId) {
			continue
		}
		media := box.GetMessage().GetMedia()
		ext := media.GetExtendedMedia_VECTORMESSAGEEXTENDEDMEDIA()
		if len(ext) == 0 {
			if one := media.GetExtendedMedia_FLAGMESSAGEEXTENDEDMEDIA(); one != nil {
				ext = []*mtproto.MessageExtendedMedia{one}
			}
		}
		if len(ext) == 0 {
			continue
		}
		matched = append(matched, box)
		updates = append(updates, mtproto.MakeTLUpdateMessageExtendedMedia(&mtproto.Update{
			Peer_PEER:                                peer.ToPeer(),
			MsgId_INT32:                              box.GetMessageId(),
			ExtendedMedia_VECTORMESSAGEEXTENDEDMEDIA: ext,
		}).To_Update())
	}
	if len(updates) == 0 {
		return empty, nil
	}

	var (
		users []*mtproto.User
		chats []*mtproto.Chat
	)
	(&mtproto.MessageBoxList{BoxList: matched}).Visit(c.MD.UserId,
		func(messageList []*mtproto.Message) {},
		func(userIdList []int64) {
			mUsers, _ := c.svcCtx.Dao.UserClient.UserGetMutableUsers(c.ctx, &userpb.TLUserGetMutableUsers{
				Id: userIdList,
			})
			users = mUsers.GetUserListByIdList(c.MD.UserId, userIdList...)
		},
		func(chatIdList []int64) {
			mChats, _ := c.svcCtx.Dao.ChatClient.Client().ChatGetChatListByIdList(c.ctx, &chatpb.TLChatGetChatListByIdList{
				IdList: chatIdList,
			})
			chats = mChats.GetChatListByIdList(c.MD.UserId, chatIdList...)
		},
		func(channelIdList []int64) {})
	if users == nil {
		users = []*mtproto.User{}
	}
	if chats == nil {
		chats = []*mtproto.Chat{}
	}
	return mtproto.MakeTLUpdates(&mtproto.Updates{
		Updates: updates,
		Users:   users,
		Chats:   chats,
		Date:    int32(time.Now().Unix()),
	}).To_Updates(), nil
}
