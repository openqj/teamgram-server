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
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// MessagesClearAllDrafts
// messages.clearAllDrafts#7e58ee9c = Bool;
func (c *DraftsCore) MessagesClearAllDrafts(in *mtproto.TLMessagesClearAllDrafts) (*mtproto.Bool, error) {
	if c == nil || c.MD == nil || c.MD.UserId <= 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.DialogClient == nil || c.svcCtx.Dao.SyncClient == nil {
		return nil, mtproto.ErrInternalServerError
	}
	rValues, err := c.svcCtx.Dao.DialogClient.DialogClearAllDrafts(c.ctx, &dialog.TLDialogClearAllDrafts{
		UserId: c.MD.UserId,
	})
	if err != nil {
		c.Logger.Errorf("messages.clearAllDrafts: %v", err)
		return nil, err
	}
	if rValues == nil {
		return nil, mtproto.ErrInternalServerError
	}

	persisted := takeStoredDrafts(c.MD.UserId)
	seen := map[string]struct{}{}
	for _, v := range rValues.Datas {
		if v == nil || v.Peer == nil {
			continue
		}
		p := mtproto.FromPeer(v.Peer)
		seen[fmt.Sprintf("%d:%d", p.PeerType, p.PeerId)] = struct{}{}
	}
	for _, item := range persisted {
		key := fmt.Sprintf("%d:%d", item.PeerType, item.PeerId)
		if _, ok := seen[key]; ok {
			continue
		}
		rValues.Datas = append(rValues.Datas, dialog.MakeTLUpdateDraftMessage(&dialog.PeerWithDraftMessage{
			Peer: mtproto.MakePeer(item.PeerType, item.PeerId),
			Draft: mtproto.MakeTLDraftMessageEmpty(&mtproto.DraftMessage{
				Date_FLAGINT32: mtproto.MakeFlagsInt32(int32(time.Now().Unix())),
			}).To_DraftMessage(),
		}).To_PeerWithDraftMessage())
	}

	if len(rValues.Datas) == 0 {
		return mtproto.BoolTrue, nil
	}

	// sync
	for _, v := range rValues.Datas {
		if v == nil || v.Peer == nil || v.Draft == nil {
			return nil, mtproto.ErrInternalServerError
		}
		syncUpdates := mtproto.MakeUpdatesByUpdates(mtproto.MakeTLUpdateDraftMessage(&mtproto.Update{
			Peer_PEER: v.Peer,
			Draft:     v.Draft,
		}).To_Update())

		peer := mtproto.FromPeer(v.Peer)
		if peer == nil || peer.PeerId <= 0 {
			return nil, mtproto.ErrPeerIdInvalid
		}
		switch peer.PeerType {
		case mtproto.PEER_USER:
			if c.svcCtx.Dao.UserClient == nil {
				return nil, mtproto.ErrInternalServerError
			}
			users, err := c.svcCtx.Dao.UserClient.UserGetMutableUsers(c.ctx, &userpb.TLUserGetMutableUsers{
				Id: []int64{c.MD.UserId, peer.PeerId},
			})
			if err != nil {
				return nil, err
			}
			if users == nil {
				return nil, mtproto.ErrInternalServerError
			}
			user, err := users.GetUnsafeUser(c.MD.UserId, peer.PeerId)
			if err != nil || user == nil {
				if err != nil {
					return nil, err
				}
				return nil, mtproto.ErrUserIdInvalid
			}

			syncUpdates.AddSafeUser(user)
		case mtproto.PEER_CHAT:
			if c.svcCtx.Dao.ChatClient == nil {
				return nil, mtproto.ErrInternalServerError
			}
			chat, err := c.svcCtx.Dao.ChatClient.ChatGetMutableChat(c.ctx, &chatpb.TLChatGetMutableChat{
				ChatId: peer.PeerId,
			})
			if err != nil {
				return nil, err
			}
			if chat == nil || chat.GetChat() == nil {
				return nil, mtproto.ErrChatIdInvalid
			}
			syncUpdates.AddSafeChat(chat.ToUnsafeChat(c.MD.UserId))
		case mtproto.PEER_CHANNEL:
			if c.svcCtx.Plugin == nil {
				return nil, mtproto.ErrMethodNotImpl
			}
			chats := c.svcCtx.Plugin.GetChannelListByIdList(c.ctx, c.MD.UserId, peer.PeerId)
			if len(chats) != 1 || chats[0] == nil || chats[0].GetId() != peer.PeerId {
				return nil, mtproto.ErrInternalServerError
			}
			syncUpdates.PushChat(chats...)
		}

		if _, err := c.svcCtx.Dao.SyncClient.SyncUpdatesNotMe(c.ctx, &sync.TLSyncUpdatesNotMe{
			UserId:        c.MD.UserId,
			PermAuthKeyId: c.MD.PermAuthKeyId,
			Updates:       syncUpdates,
		}); err != nil {
			return nil, err
		}
	}

	return mtproto.BoolTrue, nil
}
