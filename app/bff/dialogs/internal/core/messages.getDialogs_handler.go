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
	"sort"
	"sync"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/channelview"
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	"github.com/teamgram/teamgram-server/app/service/biz/message/message"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"

	"github.com/zeromicro/go-zero/core/mr"
)

// MessagesGetDialogs
// messages.getDialogs#a0f4cb4f flags:# exclude_pinned:flags.0?true folder_id:flags.1?int offset_date:int offset_id:int offset_peer:InputPeer limit:int hash:long = messages.Dialogs;
func (c *DialogsCore) MessagesGetDialogs(in *mtproto.TLMessagesGetDialogs) (*mtproto.Messages_Dialogs, error) {
	if c == nil || c.MD == nil || c.MD.UserId <= 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if in.GetOffsetPeer() == nil {
		return nil, mtproto.ErrOffsetPeerIdInvalid
	}
	if in.GetLimit() <= 0 {
		return nil, mtproto.ErrLimitInvalid
	}
	if in.GetOffsetDate() < 0 || in.GetOffsetId() < 0 {
		return nil, mtproto.ErrOffsetInvalid
	}
	if folderID := in.GetFolderId(); folderID != nil && folderID.GetValue() < 0 {
		return nil, mtproto.ErrFolderIdInvalid
	}
	var (
		offsetPeer         = mtproto.FromInputPeer2(c.MD.UserId, in.OffsetPeer)
		folderId           = in.GetFolderId().GetValue()
		limit              = in.Limit
		dialogExtList      dialog.DialogExtList
		notifySettingsList []*userpb.PeerPeerNotifySettings
	)
	switch offsetPeer.PeerType {
	case mtproto.PEER_EMPTY, mtproto.PEER_SELF:
	case mtproto.PEER_USER, mtproto.PEER_CHAT, mtproto.PEER_CHANNEL:
		if offsetPeer.PeerId <= 0 {
			return nil, mtproto.ErrOffsetPeerIdInvalid
		}
		if (offsetPeer.PeerType == mtproto.PEER_USER || offsetPeer.PeerType == mtproto.PEER_CHANNEL) && offsetPeer.AccessHash == 0 {
			return nil, mtproto.ErrOffsetPeerIdInvalid
		}
	default:
		return nil, mtproto.ErrOffsetPeerIdInvalid
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.DialogClient == nil || c.svcCtx.Dao.UserClient == nil {
		return nil, mtproto.ErrInternalServerError
	}

	if limit > 500 {
		limit = 500
	}

	if err := mr.Finish(
		func() error {
			dialogs, err := c.svcCtx.Dao.DialogClient.DialogGetDialogs(c.ctx, &dialog.TLDialogGetDialogs{
				UserId:        c.MD.UserId,
				ExcludePinned: mtproto.ToBool(in.ExcludePinned),
				FolderId:      folderId,
			})
			if err != nil {
				c.Logger.Errorf("messages.getDialogs - error: %v", err)
				return err
			}
			if dialogs == nil {
				return mtproto.ErrInternalServerError
			}
			dialogExtList = dialogs.GetDatas()
			return nil
		},
		func() error {
			settingsList, err := c.svcCtx.Dao.UserClient.UserGetAllNotifySettings(c.ctx, &userpb.TLUserGetAllNotifySettings{
				UserId: c.MD.UserId,
			})
			if err != nil {
				c.Logger.Errorf("messages.getDialogs - error: %v", err)
				return err
			}
			if settingsList == nil {
				return mtproto.ErrInternalServerError
			}
			notifySettingsList = settingsList.GetDatas()
			return nil
		}); err != nil {
		return nil, err
	}

	if len(dialogExtList) == 0 {
		out := mtproto.MakeTLMessagesDialogsSlice(&mtproto.Messages_Dialogs{
			Dialogs:  []*mtproto.Dialog{},
			Messages: []*mtproto.Message{},
			Chats:    []*mtproto.Chat{},
			Users:    []*mtproto.User{},
			Count:    0,
		}).To_Messages_Dialogs()
		if in.OffsetId == 0 && in.OffsetDate == 0 {
			channelview.AppendCreatorDialogs(out, c.MD.UserId, nil)
		}
		return out, nil
	}

	var (
		dialogCount     = int32(dialogExtList.Len())
		countedChannels = map[int64]struct{}{}
	)

	for _, dialogEx := range dialogExtList {
		if dialogEx == nil || dialogEx.GetDialog() == nil || dialogEx.GetDialog().GetPeer() == nil {
			return nil, mtproto.ErrInternalServerError
		}
		peer2 := mtproto.FromPeer(dialogEx.GetDialog().GetPeer())

		if peer2.IsChannel() {
			countedChannels[peer2.PeerId] = struct{}{}
		}

		dialogEx.Dialog.NotifySettings = userpb.FindPeerPeerNotifySettings(notifySettingsList, peer2)
	}

	sort.Sort(sort.Reverse(dialogExtList))

	dialogExtList = dialogExtList.GetDialogsByOffsetLimit(
		in.OffsetDate,
		in.OffsetId,
		offsetPeer,
		limit)

	var (
		loadErr   error
		loadErrMu sync.Mutex
	)
	recordLoadErr := func(err error) {
		loadErrMu.Lock()
		if loadErr == nil {
			loadErr = err
		}
		loadErrMu.Unlock()
	}
	messageDialogs := dialogExtList.DoGetMessagesDialogs(
		c.ctx,
		c.MD.UserId,
		func(ctx context.Context, selfUserId int64, id ...dialog.TopMessageId) []*mtproto.Message {
			var (
				msgList   = make([]*mtproto.Message, 0, len(id))
				msgIdList = make([]int32, 0, len(id))
			)
			for _, id2 := range id {
				if !id2.Peer.IsChannel() {
					msgIdList = append(msgIdList, id2.TopMessage)
				}
			}
			if len(msgIdList) > 0 {
				if c.svcCtx.Dao.MessageClient == nil {
					recordLoadErr(mtproto.ErrInternalServerError)
					return nil
				}
				boxList, err := c.svcCtx.Dao.MessageClient.MessageGetUserMessageList(c.ctx, &message.TLMessageGetUserMessageList{
					UserId: c.MD.UserId,
					IdList: msgIdList,
				})
				if err != nil {
					recordLoadErr(err)
					return nil
				}
				if boxList == nil {
					recordLoadErr(mtproto.ErrInternalServerError)
					return nil
				}
				boxList.Walk(func(idx int, v *mtproto.MessageBox) {
					if v == nil || v.GetMessage() == nil {
						recordLoadErr(mtproto.ErrInternalServerError)
						return
					}
					msgList = append(msgList, v.ToMessage(c.MD.UserId))
				})
			}

			return msgList
		},
		func(ctx context.Context, selfUserId int64, id ...int64) []*mtproto.User {
			if c.svcCtx.Dao.UserClient == nil {
				recordLoadErr(mtproto.ErrInternalServerError)
				return nil
			}
			users, err := c.svcCtx.Dao.UserClient.UserGetMutableUsersV2(c.ctx,
				&userpb.TLUserGetMutableUsersV2{
					Id:      id,
					Privacy: true,
					HasTo:   true,
					To:      []int64{selfUserId},
				})
			if err != nil {
				recordLoadErr(err)
				return nil
			}
			if users == nil {
				recordLoadErr(mtproto.ErrInternalServerError)
				return nil
			}

			return users.GetUserListByIdList(c.MD.UserId, id...)
		},
		func(ctx context.Context, selfUserId int64, id ...int64) []*mtproto.Chat {
			if c.svcCtx.Dao.ChatClient == nil {
				recordLoadErr(mtproto.ErrInternalServerError)
				return nil
			}
			chats, err := c.svcCtx.Dao.ChatClient.ChatGetChatListByIdList(c.ctx,
				&chatpb.TLChatGetChatListByIdList{
					IdList: id,
				})
			if err != nil {
				recordLoadErr(err)
				return nil
			}
			if chats == nil {
				recordLoadErr(mtproto.ErrInternalServerError)
				return nil
			}

			return chats.GetChatListByIdList(c.MD.UserId, id...)
		},
		func(ctx context.Context, selfUserId int64, id ...int64) []*mtproto.Chat {
			return channelview.ChatsByID(selfUserId, id)
		})
	loadErrMu.Lock()
	err := loadErr
	loadErrMu.Unlock()
	if err != nil {
		c.Logger.Errorf("messages.getDialogs - entity load error: %v", err)
		return nil, err
	}

	out := messageDialogs.ToMessagesDialogs(dialogCount)
	if in.OffsetId == 0 && in.OffsetDate == 0 {
		channelview.AppendCreatorDialogs(out, c.MD.UserId, countedChannels)
	}
	return out, nil
}
