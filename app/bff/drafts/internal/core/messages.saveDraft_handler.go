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
	"github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// MessagesSaveDraft
// messages.saveDraft#bc39e14b flags:# no_webpage:flags.1?true reply_to_msg_id:flags.0?int peer:InputPeer message:string entities:flags.3?Vector<MessageEntity> = Bool;
func (c *DraftsCore) MessagesSaveDraft(in *mtproto.TLMessagesSaveDraft) (*mtproto.Bool, error) {
	if c == nil || c.MD == nil || c.MD.UserId <= 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if in == nil || in.GetPeer() == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.DialogClient == nil || c.svcCtx.Dao.SyncClient == nil {
		return nil, mtproto.ErrInternalServerError
	}
	var (
		peer                = mtproto.FromInputPeer2(c.MD.UserId, in.Peer)
		draft               *mtproto.DraftMessage
		isDraftMessageEmpty = true
		date                = int32(time.Now().Unix())
	)

	if in.NoWebpage == true {
		isDraftMessageEmpty = false
	} else if in.ReplyToMsgId != nil {
		isDraftMessageEmpty = false
	} else if in.Message != "" {
		isDraftMessageEmpty = false
	} else if in.Entities != nil {
		isDraftMessageEmpty = false
	}

	if isDraftMessageEmpty {
		draft = mtproto.MakeTLDraftMessageEmpty(&mtproto.DraftMessage{
			Date_FLAGINT32: mtproto.MakeFlagsInt32(date),
		}).To_DraftMessage()

		if _, err := c.svcCtx.Dao.DialogClient.DialogClearDraftMessage(c.ctx, &dialog.TLDialogClearDraftMessage{
			UserId:   c.MD.UserId,
			PeerType: peer.PeerType,
			PeerId:   peer.PeerId,
		}); err != nil {
			return nil, err
		}
	} else {
		draft = mtproto.MakeTLDraftMessage(&mtproto.DraftMessage{
			NoWebpage:    in.GetNoWebpage(),
			InvertMedia:  in.GetInvertMedia(),
			ReplyToMsgId: in.GetReplyToMsgId(),
			ReplyTo:      in.GetReplyTo(),
			Message:      in.GetMessage(),
			Entities:     in.GetEntities(),
			Media:        in.GetMedia(),
			Date_INT32:   date,
			Effect:       in.GetEffect(),
		}).To_DraftMessage()

		if _, err := c.svcCtx.Dao.DialogClient.DialogSaveDraftMessage(c.ctx, &dialog.TLDialogSaveDraftMessage{
			UserId:   c.MD.UserId,
			PeerType: peer.PeerType,
			PeerId:   peer.PeerId,
			Message:  draft,
		}); err != nil {
			return nil, err
		}
	}

	if err := saveStoredDraft(c.MD.UserId, peer.PeerType, peer.PeerId, draft, isDraftMessageEmpty); err != nil {
		c.Logger.Errorf("messages.saveDraft - error: %v", err)
		return nil, err
	}

	syncUpdates := mtproto.MakeUpdatesByUpdates(mtproto.MakeTLUpdateDraftMessage(&mtproto.Update{
		Peer_PEER: peer.ToPeer(),
		Draft:     draft,
	}).To_Update())

	switch peer.PeerType {
	case mtproto.PEER_SELF:
		if c.svcCtx.Dao.UserClient == nil {
			return nil, mtproto.ErrInternalServerError
		}
		user, err := c.svcCtx.Dao.UserClient.UserGetImmutableUser(c.ctx, &userpb.TLUserGetImmutableUser{
			Id: c.MD.UserId,
		})
		if err != nil {
			return nil, err
		}
		if user == nil {
			return nil, mtproto.ErrUserIdInvalid
		}
		syncUpdates.PushUser(user.ToSelfUser())
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

	// sync
	if _, err := c.svcCtx.Dao.SyncClient.SyncUpdatesNotMe(c.ctx, &sync.TLSyncUpdatesNotMe{
		UserId:        c.MD.UserId,
		PermAuthKeyId: c.MD.PermAuthKeyId,
		Updates:       syncUpdates,
	}); err != nil {
		return nil, err
	}

	return mtproto.BoolTrue, nil
}
