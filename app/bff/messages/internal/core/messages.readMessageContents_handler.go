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
	"context"

	"github.com/teamgram/marmota/pkg/threading2"
	"github.com/teamgram/proto/mtproto"
	msgpb "github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
	"github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	"github.com/teamgram/teamgram-server/app/service/biz/message/message"
)

type readMessageContentsPeer struct {
	peerType int32
	peerID   int64
}

type readMessageContentsGroup struct {
	peer     readMessageContentsPeer
	contents []*msgpb.ContentMessage
}

func groupReadMessageContents(userID int64, messages []*mtproto.MessageBox) ([]readMessageContentsGroup, error) {
	groups := make([]readMessageContentsGroup, 0)
	groupIndexes := make(map[readMessageContentsPeer]int)
	for _, m := range messages {
		if m == nil {
			return nil, mtproto.ErrPeerIdInvalid
		}

		peer := readMessageContentsPeer{}
		switch m.PeerType {
		case mtproto.PEER_CHAT:
			peer = readMessageContentsPeer{peerType: mtproto.PEER_CHAT, peerID: m.GetMessage().GetPeerId().GetChatId()}
		case mtproto.PEER_USER:
			peerID := m.PeerId
			if m.SenderUserId != userID {
				peerID = m.SenderUserId
			}
			peer = readMessageContentsPeer{peerType: mtproto.PEER_USER, peerID: peerID}
		default:
			return nil, mtproto.ErrPeerIdInvalid
		}
		if peer.peerID <= 0 {
			return nil, mtproto.ErrPeerIdInvalid
		}

		groupIndex, ok := groupIndexes[peer]
		if !ok {
			groupIndex = len(groups)
			groupIndexes[peer] = groupIndex
			groups = append(groups, readMessageContentsGroup{peer: peer})
		}
		group := &groups[groupIndex]
		if m.Message.GetMentioned() {
			group.contents = append(group.contents, &msgpb.ContentMessage{
				Id:              m.MessageId,
				SendUserId:      m.SenderUserId,
				DialogMessageId: m.DialogMessageId,
				Mentioned:       true,
			})
		} else if m.Message.GetMediaUnread() {
			group.contents = append(group.contents, &msgpb.ContentMessage{
				Id:              m.MessageId,
				SendUserId:      m.SenderUserId,
				DialogMessageId: m.DialogMessageId,
				MediaUnread:     true,
			})
		} else if m.GetMessage().GetReactions() != nil {
			group.contents = append(group.contents, &msgpb.ContentMessage{
				Id:              m.MessageId,
				SendUserId:      m.SenderUserId,
				DialogMessageId: m.DialogMessageId,
				Reaction:        true,
			})
		}
	}

	return groups, nil
}

// MessagesReadMessageContents
// messages.readMessageContents#36a73f77 id:Vector<int> = messages.AffectedMessages;
func (c *MessagesCore) MessagesReadMessageContents(in *mtproto.TLMessagesReadMessageContents) (*mtproto.Messages_AffectedMessages, error) {
	if c == nil || c.MD == nil || c.MD.UserId <= 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	for _, id := range in.GetId() {
		if id <= 0 {
			return nil, mtproto.ErrMessageIdInvalid
		}
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.MessageClient == nil || c.svcCtx.Dao.MsgClient == nil || c.svcCtx.Dao.SyncClient == nil {
		return nil, mtproto.ErrInternalServerError
	}

	messages, err := c.svcCtx.Dao.MessageClient.MessageGetUserMessageList(c.ctx, &message.TLMessageGetUserMessageList{
		UserId: c.MD.UserId,
		IdList: in.GetId(),
	})
	if err != nil {
		c.Logger.Errorf("messages.readMessageContents - error: %v", err)
		return nil, err
	} else if messages == nil {
		return nil, mtproto.ErrInternalServerError
	} else if messages.Length() == 0 {
		c.Logger.Errorf("messages.readMessageContents - error: missing messages")
		return mtproto.MakeTLMessagesAffectedMessages(&mtproto.Messages_AffectedMessages{
			Pts:      c.svcCtx.Dao.IDGenClient2.CurrentPtsId(c.ctx, c.MD.UserId),
			PtsCount: 0,
		}).To_Messages_AffectedMessages(), nil
	}

	groups, err := groupReadMessageContents(c.MD.UserId, messages.GetDatas())
	if err != nil {
		c.Logger.Errorf("messages.readMessageContents - invalid peer in message list: %v", err)
		return nil, err
	}
	var (
		affected *mtproto.Messages_AffectedMessages
		ptsCount int32
	)
	for _, group := range groups {
		affected, err = c.svcCtx.Dao.MsgClient.MsgReadMessageContents(c.ctx, &msgpb.TLMsgReadMessageContents{
			UserId:    c.MD.UserId,
			AuthKeyId: c.MD.PermAuthKeyId,
			PeerType:  group.peer.peerType,
			PeerId:    group.peer.peerID,
			Id:        group.contents,
		})
		if err != nil {
			c.Logger.Errorf("messages.readMessageContents - %v", err)
			return nil, err
		}
		if affected == nil {
			return nil, mtproto.ErrInternalServerError
		}
		ptsCount += affected.PtsCount
	}
	result := mtproto.MakeTLMessagesAffectedMessages(&mtproto.Messages_AffectedMessages{
		Pts:      affected.Pts,
		PtsCount: ptsCount,
	}).To_Messages_AffectedMessages()

	return threading2.WrapperGoFunc(
		c.ctx,
		result,
		func(ctx context.Context) {
			c.svcCtx.Dao.SyncClient.SyncUpdatesNotMe(ctx, &sync.TLSyncUpdatesNotMe{
				UserId:        c.MD.UserId,
				PermAuthKeyId: c.MD.PermAuthKeyId,
				Updates: mtproto.MakeUpdatesByUpdates(mtproto.MakeTLUpdateReadMessagesContents(&mtproto.Update{
					Messages:  in.Id,
					Pts_INT32: affected.Pts,
					PtsCount:  ptsCount,
				}).To_Update()),
			})
		}).(*mtproto.Messages_AffectedMessages), nil
}
