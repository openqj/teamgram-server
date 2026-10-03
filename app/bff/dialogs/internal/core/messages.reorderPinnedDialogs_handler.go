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

// MessagesReorderPinnedDialogs
// messages.reorderPinnedDialogs#3b1adf37 flags:# force:flags.0?true folder_id:int order:Vector<InputDialogPeer> = Bool;
func (c *DialogsCore) MessagesReorderPinnedDialogs(in *mtproto.TLMessagesReorderPinnedDialogs) (*mtproto.Bool, error) {
	if c == nil || c.MD == nil || c.MD.UserId <= 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if in == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.DialogClient == nil || c.svcCtx.Dao.SyncClient == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	var (
		peerDialogIdList []int64
		peerDialogList   = make([]*mtproto.DialogPeer, 0, len(in.GetOrder())+1)
	)

	for _, peer := range in.GetOrder() {
		if peer == nil {
			return nil, mtproto.ErrPeerIdInvalid
		}
		switch peer.PredicateName {
		case mtproto.Predicate_inputDialogPeer:
			if peer.GetPeer() == nil {
				return nil, mtproto.ErrPeerIdInvalid
			}
			p := mtproto.FromInputPeer2(c.MD.UserId, peer.Peer)
			if p == nil || p.PeerId <= 0 || (p.PeerType != mtproto.PEER_SELF && p.PeerType != mtproto.PEER_USER && p.PeerType != mtproto.PEER_CHAT && p.PeerType != mtproto.PEER_CHANNEL) {
				return nil, mtproto.ErrPeerIdInvalid
			}
			peerDialogIdList = append(peerDialogIdList, mtproto.MakePeerDialogId(p.PeerType, p.PeerId))
			peerDialogList = append(peerDialogList, mtproto.MakeTLDialogPeer(&mtproto.DialogPeer{
				Peer: p.ToPeer(),
			}).To_DialogPeer())
		case mtproto.Predicate_inputDialogPeerFolder:
			return nil, mtproto.ErrPeerIdInvalid
		default:
			err := mtproto.ErrPeerIdInvalid
			c.Logger.Errorf("messages.reorderPinnedDialogs - error: %v", err)
			return nil, err
		}
	}

	result, err := c.svcCtx.Dao.DialogClient.DialogReorderPinnedDialogs(c.ctx, &dialog.TLDialogReorderPinnedDialogs{
		UserId:   c.MD.UserId,
		Force:    mtproto.ToBool(in.Force),
		FolderId: in.FolderId,
		IdList:   peerDialogIdList,
	})
	if err != nil {
		c.Logger.Errorf("messages.reorderPinnedDialogs - error: %v", err)
		return nil, err
	}
	if result == nil {
		return nil, mtproto.ErrInternalServerError
	}
	if !mtproto.FromBool(result) {
		return result, nil
	}

	syncUpdates := mtproto.MakeUpdatesByUpdates(mtproto.MakeTLUpdatePinnedDialogs(&mtproto.Update{
		FolderId:                   mtproto.MakeFlagsInt32(in.FolderId),
		Order_FLAGVECTORDIALOGPEER: peerDialogList,
	}).To_Update())
	for _, peer := range peerDialogList {
		p := mtproto.FromPeer(peer.GetPeer())
		if p == nil {
			return nil, mtproto.ErrPeerIdInvalid
		}
		switch p.PeerType {
		case mtproto.PEER_SELF, mtproto.PEER_USER:
			if c.svcCtx.Dao.UserClient == nil {
				return nil, mtproto.ErrInternalServerError
			}
			users, err := c.svcCtx.Dao.UserClient.UserGetMutableUsers(c.ctx, &userpb.TLUserGetMutableUsers{Id: []int64{p.PeerId}})
			if err != nil {
				return nil, err
			}
			if users == nil || len(users.GetDatas()) == 0 {
				return nil, fmt.Errorf("messages.reorderPinnedDialogs: user %d was not hydrated", p.PeerId)
			}
			syncUpdates.PushUser(users.GetUserListByIdList(c.MD.UserId, p.PeerId)...)
		case mtproto.PEER_CHAT:
			if c.svcCtx.Dao.ChatClient == nil {
				return nil, mtproto.ErrInternalServerError
			}
			chats, err := c.svcCtx.Dao.ChatClient.ChatGetChatListByIdList(c.ctx, &chatpb.TLChatGetChatListByIdList{IdList: []int64{p.PeerId}})
			if err != nil {
				return nil, err
			}
			if chats == nil || len(chats.GetDatas()) == 0 {
				return nil, fmt.Errorf("messages.reorderPinnedDialogs: chat %d was not hydrated", p.PeerId)
			}
			syncUpdates.PushChat(chats.GetChatListByIdList(c.MD.UserId, p.PeerId)...)
		case mtproto.PEER_CHANNEL:
			if c.svcCtx.Plugin == nil {
				return nil, fmt.Errorf("messages.reorderPinnedDialogs: channel resolver is unavailable")
			}
			channels := c.svcCtx.Plugin.GetChannelListByIdList(c.ctx, c.MD.UserId, p.PeerId)
			if len(channels) != 1 || channels[0] == nil || channels[0].GetId() != p.PeerId {
				return nil, fmt.Errorf("messages.reorderPinnedDialogs: channel %d was not hydrated", p.PeerId)
			}
			syncUpdates.PushChat(channels...)
		}
	}
	if _, err := c.svcCtx.Dao.SyncClient.SyncUpdatesNotMe(c.ctx, &sync.TLSyncUpdatesNotMe{
		UserId:        c.MD.UserId,
		PermAuthKeyId: c.MD.PermAuthKeyId,
		Updates:       syncUpdates,
	}); err != nil {
		return nil, err
	}
	return result, nil
}
