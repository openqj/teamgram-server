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
	"github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	"time"
)

// MessagesEditChatDefaultBannedRights
// messages.editChatDefaultBannedRights#a5866b41 peer:InputPeer banned_rights:ChatBannedRights = Updates;
func (c *ChatsCore) MessagesEditChatDefaultBannedRights(in *mtproto.TLMessagesEditChatDefaultBannedRights) (*mtproto.Updates, error) {
	if c == nil || c.MD == nil {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if in == nil || in.GetPeer() == nil || in.GetBannedRights() == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	var (
		peer     = mtproto.FromInputPeer2(c.MD.UserId, in.Peer)
		rUpdates *mtproto.Updates
		date     = time.Now().Unix()
	)

	switch peer.PeerType {
	case mtproto.PEER_CHAT, mtproto.PEER_CHANNEL:
		chat, err := c.svcCtx.Dao.ChatClient.Client().ChatEditChatDefaultBannedRights(c.ctx, &chatpb.TLChatEditChatDefaultBannedRights{
			ChatId:       peer.PeerId,
			OperatorId:   c.MD.UserId,
			BannedRights: in.BannedRights,
		})
		if err != nil {
			c.Logger.Errorf("messages.editChatDefaultBannedRights - error: %v", err)
			return nil, err
		}
		if chat == nil || chat.GetChat() == nil {
			return nil, mtproto.ErrInternalServerError
		}

		defaultBannedUpdates := mtproto.MakeTLUpdateShort(&mtproto.Updates{
			Update: mtproto.MakeTLUpdateChatDefaultBannedRights(&mtproto.Update{
				Peer_PEER:           mtproto.MakePeerChat(peer.PeerId),
				DefaultBannedRights: chat.Chat.DefaultBannedRights,
				Version:             chat.Chat.Version,
			}).To_Update(),
			Date: int32(date),
		}).To_Updates()

		if reply, syncErr := c.svcCtx.Dao.SyncClient.SyncUpdatesNotMe(c.ctx, &sync.TLSyncUpdatesNotMe{
			UserId:        c.MD.UserId,
			PermAuthKeyId: c.MD.PermAuthKeyId,
			Updates:       defaultBannedUpdates,
		}); syncErr != nil {
			return nil, syncErr
		} else if reply == nil {
			return nil, mtproto.ErrInternalServerError
		}
		var firstErr error
		chat.Walk(func(userId int64, participant *mtproto.ImmutableChatParticipant) error {
			if firstErr != nil {
				return firstErr
			}
			if participant == nil {
				firstErr = mtproto.ErrInternalServerError
				return firstErr
			}
			if userId != c.MD.UserId {
				if reply, pushErr := c.svcCtx.Dao.SyncClient.SyncPushUpdates(c.ctx, &sync.TLSyncPushUpdates{
					UserId:  userId,
					Updates: defaultBannedUpdates,
				}); pushErr != nil {
					firstErr = pushErr
					return firstErr
				} else if reply == nil {
					firstErr = mtproto.ErrInternalServerError
					return firstErr
				}
			}
			return nil
		})
		if firstErr != nil {
			return nil, firstErr
		}

		rUpdates = mtproto.MakeTLUpdates(&mtproto.Updates{
			Updates: []*mtproto.Update{},
			Users:   []*mtproto.User{},
			Chats:   []*mtproto.Chat{chat.ToUnsafeChat(c.MD.UserId)},
			Date:    int32(date),
			Seq:     0,
		}).To_Updates()
	default:
		err := mtproto.ErrPeerIdInvalid
		c.Logger.Errorf("invalid peer type: {%v}")
		return nil, err
	}

	return rUpdates, nil
}
