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
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// MessagesSetHistoryTTL
// messages.setHistoryTTL#b80e5fe4 peer:InputPeer period:int = Updates;
func (c *DialogsCore) MessagesSetHistoryTTL(in *mtproto.TLMessagesSetHistoryTTL) (*mtproto.Updates, error) {
	if in.GetPeer() == nil {
		c.Logger.Errorf("messages.setHistoryTTL - error: %v", mtproto.ErrPeerIdInvalid)
		return nil, mtproto.ErrPeerIdInvalid
	}
	if in.GetPeriod() < 0 {
		c.Logger.Errorf("messages.setHistoryTTL - error: %v", mtproto.ErrTtlPeriodInvalid)
		return nil, mtproto.ErrTtlPeriodInvalid
	}

	peer := mtproto.FromInputPeer2(c.MD.UserId, in.Peer)
	if !peer.IsUserOrChatOrChannel() || peer.PeerId == 0 {
		c.Logger.Errorf("messages.setHistoryTTL - error: %v", mtproto.ErrPeerIdInvalid)
		return nil, mtproto.ErrPeerIdInvalid
	}

	var chats []*mtproto.Chat
	if peer.IsChat() {
		mChat, err := c.svcCtx.Dao.ChatClient.ChatSetHistoryTTL(c.ctx, &chatpb.TLChatSetHistoryTTL{
			SelfId:    c.MD.UserId,
			ChatId:    peer.PeerId,
			TtlPeriod: in.Period,
		})
		if err != nil {
			c.Logger.Errorf("messages.setHistoryTTL - error: %v", err)
			return nil, err
		}
		if mChat != nil {
			chats = []*mtproto.Chat{mChat.ToUnsafeChat(c.MD.UserId)}
		}
	}

	if _, err := c.svcCtx.Dao.DialogClient.DialogSetHistoryTTL(c.ctx, &dialog.TLDialogSetHistoryTTL{
		UserId:    c.MD.UserId,
		PeerType:  peer.PeerType,
		PeerId:    peer.PeerId,
		TtlPeriod: in.Period,
	}); err != nil {
		c.Logger.Errorf("messages.setHistoryTTL - error: %v", err)
		return nil, err
	}

	upd := mtproto.MakeTLUpdatePeerHistoryTTL(&mtproto.Update{
		Peer_PEER: peer.ToPeer(),
		TtlPeriod: wrapperspb.Int32(in.Period),
	}).To_Update()

	var rUpdates *mtproto.Updates
	if len(chats) > 0 {
		rUpdates = mtproto.MakeUpdatesByUpdatesChats(chats, upd)
	} else {
		rUpdates = mtproto.MakeUpdatesByUpdates(upd)
	}

	if _, syncErr := c.svcCtx.Dao.SyncClient.SyncUpdatesNotMe(c.ctx, &sync.TLSyncUpdatesNotMe{
		UserId:        c.MD.UserId,
		PermAuthKeyId: c.MD.PermAuthKeyId,
		Updates:       rUpdates,
	}); syncErr != nil {
		c.Logger.Errorf("messages.setHistoryTTL - sync update: %v", syncErr)
		return nil, syncErr
	}

	return rUpdates, nil
}
