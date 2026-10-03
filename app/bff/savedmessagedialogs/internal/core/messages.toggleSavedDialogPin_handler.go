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
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// MessagesToggleSavedDialogPin
// messages.toggleSavedDialogPin#ac81bbde flags:# pinned:flags.0?true peer:InputDialogPeer = Bool;
func (c *SavedMessageDialogsCore) MessagesToggleSavedDialogPin(in *mtproto.TLMessagesToggleSavedDialogPin) (*mtproto.Bool, error) {
	if c == nil || c.MD == nil || c.MD.UserId == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if in == nil || in.GetPeer() == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil {
		return nil, mtproto.ErrInternalServerError
	}

	var (
		peer *mtproto.PeerUtil
	)

	switch in.GetPeer().GetPredicateName() {
	case mtproto.Predicate_inputDialogPeer:
		if in.GetPeer().GetPeer() == nil {
			return nil, mtproto.ErrInputRequestInvalid
		}
		peer = mtproto.FromInputPeer2(c.MD.UserId, in.GetPeer().GetPeer())
	case mtproto.Predicate_inputDialogPeerFolder:
		// error
		c.Logger.Errorf("messages.toggleSavedDialogPin - error: client not send inputDialogPeerFolder: %v", in.GetPeer())
		return mtproto.BoolFalse, nil
	default:
		err := mtproto.ErrInputConstructorInvalid
		c.Logger.Errorf("messages.toggleSavedDialogPin - error: %v", err)
		return nil, err
	}
	if peer == nil || peer.PeerType == mtproto.PEER_EMPTY || peer.PeerId == 0 {
		return nil, mtproto.ErrPeerIdInvalid
	}
	if c.svcCtx.Dao.DialogClient == nil || c.svcCtx.Dao.SyncClient == nil {
		return nil, mtproto.ErrInternalServerError
	}
	switch peer.PeerType {
	case mtproto.PEER_USER:
		if c.svcCtx.Dao.UserClient == nil {
			return nil, mtproto.ErrInternalServerError
		}
	case mtproto.PEER_CHAT:
		if c.svcCtx.Dao.ChatClient == nil {
			return nil, mtproto.ErrInternalServerError
		}
	}

	saved, err := c.svcCtx.Dao.DialogClient.DialogToggleSavedDialogPin(c.ctx, &dialog.TLDialogToggleSavedDialogPin{
		UserId: c.MD.UserId,
		Peer:   peer,
		Pinned: mtproto.ToBool(in.Pinned),
	})
	if err != nil {
		c.Logger.Errorf("messages.toggleSavedDialogPin - error: %v", err)
		return nil, err
	}
	if saved == nil {
		return nil, mtproto.ErrInternalServerError
	}
	if !mtproto.FromBool(saved) {
		return saved, nil
	}

	var (
		idHelper    = mtproto.NewIDListHelper(c.MD.UserId)
		syncUpdates = mtproto.MakeUpdatesByUpdates(mtproto.MakeTLUpdateSavedDialogPinned(&mtproto.Update{
			Pinned: in.GetPinned(),
			Peer_DIALOGPEER: mtproto.MakeTLDialogPeer(&mtproto.DialogPeer{
				Peer: peer.ToPeer(),
			}).To_DialogPeer(),
		}).To_Update())
	)

	idHelper.PickByPeerUtil(peer.PeerType, peer.PeerId)
	idHelper.Visit(
		func(userIdList []int64) {
			if c.svcCtx.Dao.UserClient == nil {
				return
			}
			users, _ := c.svcCtx.Dao.UserClient.UserGetMutableUsers(c.ctx,
				&userpb.TLUserGetMutableUsers{
					Id: userIdList,
				})
			if users != nil {
				syncUpdates.PushUser(users.GetUserListByIdList(c.MD.UserId, userIdList...)...)
			}
		},
		func(chatIdList []int64) {
			if c.svcCtx.Dao.ChatClient == nil {
				return
			}
			chats, _ := c.svcCtx.Dao.ChatClient.ChatGetChatListByIdList(c.ctx,
				&chatpb.TLChatGetChatListByIdList{
					IdList: chatIdList,
				})
			if chats != nil {
				syncUpdates.PushChat(chats.GetChatListByIdList(c.MD.UserId, chatIdList...)...)
			}
		},
		func(channelIdList []int64) {
			if c.channelChatsByID != nil {
				syncUpdates.PushChat(c.channelChatsByID(c.MD.UserId, channelIdList)...)
			}
		})
	synced, err := c.svcCtx.Dao.SyncClient.SyncUpdatesNotMe(c.ctx, &sync.TLSyncUpdatesNotMe{
		UserId:        c.MD.UserId,
		PermAuthKeyId: c.MD.PermAuthKeyId,
		Updates:       syncUpdates,
	})
	if err != nil {
		c.Logger.Errorf("messages.toggleSavedDialogPin - sync error: %v", err)
		return nil, err
	}
	if synced == nil {
		return nil, mtproto.ErrInternalServerError
	}

	return mtproto.BoolTrue, nil
}
