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
	"encoding/json"
	"fmt"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/persist"
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// MessagesGetAllDrafts
// messages.getAllDrafts#6a3f8d65 = Updates;
func (c *DraftsCore) MessagesGetAllDrafts(in *mtproto.TLMessagesGetAllDrafts) (*mtproto.Updates, error) {
	if c == nil || c.MD == nil || c.MD.UserId <= 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.DialogClient == nil {
		return nil, mtproto.ErrInternalServerError
	}

	drafts, err := c.svcCtx.Dao.DialogClient.DialogGetAllDrafts(c.ctx, &dialog.TLDialogGetAllDrafts{
		UserId: c.MD.UserId,
	})
	if err != nil {
		c.Logger.Errorf("messages.getAllDrafts - error: %v", err)
		return nil, err
	}
	if drafts == nil {
		return nil, mtproto.ErrInternalServerError
	}

	var (
		rUpdates = mtproto.MakeEmptyUpdates()
		idHelper = mtproto.NewIDListHelper(c.MD.UserId)
	)

	seen := map[string]struct{}{}
	for _, v := range drafts.Datas {
		if v == nil || v.Peer == nil || v.Draft == nil {
			return nil, mtproto.ErrInternalServerError
		}
		rUpdates.PushBackUpdate(mtproto.MakeTLUpdateDraftMessage(&mtproto.Update{
			Peer_PEER: v.Peer,
			Draft:     v.Draft,
		}).To_Update())

		idHelper.PickByPeer(v.Peer)
		if v.Peer != nil {
			p := mtproto.FromPeer(v.Peer)
			seen[fmt.Sprintf("%d:%d", p.PeerType, p.PeerId)] = struct{}{}
		}
	}

	for _, item := range loadStoredDrafts(c.MD.UserId) {
		if item.Draft == nil {
			continue
		}
		key := fmt.Sprintf("%d:%d", item.PeerType, item.PeerId)
		if _, ok := seen[key]; ok {
			continue
		}
		peer := mtproto.MakePeer(item.PeerType, item.PeerId)
		rUpdates.PushBackUpdate(mtproto.MakeTLUpdateDraftMessage(&mtproto.Update{
			Peer_PEER: peer,
			Draft:     item.Draft,
		}).To_Update())
		idHelper.PickByPeer(peer)
	}

	var hydrationErr error
	idHelper.Visit(
		func(userIdList []int64) {
			if c.svcCtx.Dao.UserClient == nil {
				hydrationErr = mtproto.ErrInternalServerError
				return
			}
			users, err := c.svcCtx.Dao.UserClient.UserGetMutableUsers(c.ctx,
				&userpb.TLUserGetMutableUsers{
					Id: userIdList,
				})
			if err != nil {
				hydrationErr = err
				return
			}
			if users == nil {
				hydrationErr = mtproto.ErrInternalServerError
				return
			}
			rUpdates.PushUser(users.GetUserListByIdList(c.MD.UserId, userIdList...)...)
		},
		func(chatIdList []int64) {
			if c.svcCtx.Dao.ChatClient == nil {
				hydrationErr = mtproto.ErrInternalServerError
				return
			}
			chats, err := c.svcCtx.Dao.ChatClient.ChatGetChatListByIdList(c.ctx,
				&chatpb.TLChatGetChatListByIdList{
					IdList: chatIdList,
				})
			if err != nil {
				hydrationErr = err
				return
			}
			if chats == nil {
				hydrationErr = mtproto.ErrInternalServerError
				return
			}
			rUpdates.PushChat(chats.GetChatListByIdList(c.MD.UserId, chatIdList...)...)
		},
		func(channelIdList []int64) {
			if c.svcCtx.Plugin == nil {
				hydrationErr = mtproto.ErrMethodNotImpl
				return
			}
			chats := c.svcCtx.Plugin.GetChannelListByIdList(c.ctx, c.MD.UserId, channelIdList...)
			if len(chats) != len(channelIdList) {
				hydrationErr = mtproto.ErrInternalServerError
				return
			}
			rUpdates.PushChat(chats...)
		})
	if hydrationErr != nil {
		return nil, hydrationErr
	}

	return rUpdates, nil
}

type storedDraft struct {
	PeerType int32                 `json:"peer_type"`
	PeerId   int64                 `json:"peer_id"`
	Draft    *mtproto.DraftMessage `json:"draft"`
}

func draftsStoreKey(userID int64) string {
	return fmt.Sprintf("drafts:%d", userID)
}

func loadStoredDrafts(userID int64) []storedDraft {
	if persist.Default == nil {
		return nil
	}
	raw, err := persist.Default.Get(draftsStoreKey(userID))
	if err != nil || raw == "" {
		return nil
	}
	var items []storedDraft
	if err = json.Unmarshal([]byte(raw), &items); err != nil {
		return nil
	}
	return items
}

func saveStoredDraft(userID int64, peerType int32, peerID int64, draft *mtproto.DraftMessage, empty bool) error {
	if persist.Default == nil {
		return nil
	}
	var next []storedDraft
	for _, item := range loadStoredDrafts(userID) {
		if item.PeerType == peerType && item.PeerId == peerID {
			continue
		}
		next = append(next, item)
	}
	if !empty && draft != nil {
		next = append(next, storedDraft{PeerType: peerType, PeerId: peerID, Draft: draft})
	}
	if len(next) == 0 {
		return persist.Default.Set(draftsStoreKey(userID), "")
	}
	raw, err := json.Marshal(next)
	if err != nil {
		return err
	}
	return persist.Default.Set(draftsStoreKey(userID), string(raw))
}

func takeStoredDrafts(userID int64) []storedDraft {
	items := loadStoredDrafts(userID)
	if persist.Default != nil {
		_ = persist.Default.Set(draftsStoreKey(userID), "")
	}
	return items
}
