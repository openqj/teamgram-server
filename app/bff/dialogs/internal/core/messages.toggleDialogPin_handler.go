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
	"fmt"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// MessagesToggleDialogPin
// messages.toggleDialogPin#a731e257 flags:# pinned:flags.0?true peer:InputDialogPeer = Bool;
func (c *DialogsCore) MessagesToggleDialogPin(in *mtproto.TLMessagesToggleDialogPin) (*mtproto.Bool, error) {
	if c == nil || c.MD == nil || c.MD.UserId <= 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if in == nil || in.GetPeer() == nil || in.GetPeer().GetPeer() == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.DialogClient == nil || c.svcCtx.Dao.SyncClient == nil {
		return nil, mtproto.ErrInternalServerError
	}

	var peer *mtproto.PeerUtil

	switch in.GetPeer().GetPredicateName() {
	case mtproto.Predicate_inputDialogPeer:
		peer = mtproto.FromInputPeer2(c.MD.UserId, in.GetPeer().GetPeer())
	case mtproto.Predicate_inputDialogPeerFolder:
		// error
		c.Logger.Errorf("messages.toggleDialogPin - error: client not send inputDialogPeerFolder: %v", in.GetPeer())
		return mtproto.BoolFalse, nil
	default:
		err := mtproto.ErrInputConstructorInvalid
		c.Logger.Errorf("messages.toggleDialogPin - error: %v", err)
		return nil, err
	}
	if peer == nil || peer.PeerId <= 0 {
		return nil, mtproto.ErrPeerIdInvalid
	}

	folderId, err := c.svcCtx.Dao.DialogClient.DialogToggleDialogPin(c.ctx, &dialog.TLDialogToggleDialogPin{
		UserId:   c.MD.UserId,
		PeerType: peer.PeerType,
		PeerId:   peer.PeerId,
		Pinned:   mtproto.ToBool(in.Pinned),
	})
	if err != nil {
		c.Logger.Errorf("messages.toggleDialogPin - error: %v", err)
		return nil, err
	}
	if folderId == nil {
		return nil, mtproto.ErrInternalServerError
	}

	syncUpdates := mtproto.MakeUpdatesByUpdates(mtproto.MakeTLUpdateDialogPinned(&mtproto.Update{
		Pinned:   in.GetPinned(),
		FolderId: mtproto.MakeFlagsInt32(folderId.V),
		Peer_DIALOGPEER: mtproto.MakeTLDialogPeer(&mtproto.DialogPeer{
			Peer: peer.ToPeer(),
		}).To_DialogPeer(),
	}).To_Update())

	switch peer.PeerType {
	case mtproto.PEER_SELF, mtproto.PEER_USER:
		if c.svcCtx.Dao.UserClient == nil {
			return nil, mtproto.ErrInternalServerError
		}
		users, err := c.svcCtx.Dao.UserClient.UserGetMutableUsers(c.ctx, &userpb.TLUserGetMutableUsers{Id: []int64{peer.PeerId}})
		if err != nil {
			return nil, err
		}
		if users == nil || len(users.GetDatas()) == 0 {
			return nil, fmt.Errorf("messages.toggleDialogPin: user.getMutableUsers returned no user %d", peer.PeerId)
		}
		syncUpdates.PushUser(users.GetUserListByIdList(c.MD.UserId, peer.PeerId)...)
	case mtproto.PEER_CHAT:
		if c.svcCtx.Dao.ChatClient == nil {
			return nil, mtproto.ErrInternalServerError
		}
		chats, err := c.svcCtx.Dao.ChatClient.ChatGetChatListByIdList(c.ctx, &chatpb.TLChatGetChatListByIdList{IdList: []int64{peer.PeerId}})
		if err != nil {
			return nil, err
		}
		if chats == nil || len(chats.GetDatas()) == 0 {
			return nil, fmt.Errorf("messages.toggleDialogPin: chat.getChatListByIdList returned no chat %d", peer.PeerId)
		}
		syncUpdates.PushChat(chats.GetChatListByIdList(c.MD.UserId, peer.PeerId)...)
	case mtproto.PEER_CHANNEL:
		if c.svcCtx.Plugin == nil {
			return nil, fmt.Errorf("messages.toggleDialogPin: channel resolver is unavailable")
		}
		channels := c.svcCtx.Plugin.GetChannelListByIdList(c.ctx, c.MD.UserId, peer.PeerId)
		if len(channels) != 1 || channels[0] == nil || channels[0].GetId() != peer.PeerId {
			return nil, fmt.Errorf("messages.toggleDialogPin: channel resolver returned no channel %d", peer.PeerId)
		}
		syncUpdates.PushChat(channels...)
	default:
		return nil, mtproto.ErrPeerIdInvalid
	}
	if _, err := c.svcCtx.Dao.SyncClient.SyncUpdatesNotMe(c.ctx, &sync.TLSyncUpdatesNotMe{
		UserId:        c.MD.UserId,
		PermAuthKeyId: c.MD.PermAuthKeyId,
		Updates:       syncUpdates,
	}); err != nil {
		return nil, err
	}

	return mtproto.BoolTrue, nil
}
