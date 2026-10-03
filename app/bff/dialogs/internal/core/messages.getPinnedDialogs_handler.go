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
	"github.com/teamgram/teamgram-server/app/service/biz/updates/updates"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"

	"github.com/zeromicro/go-zero/core/mr"
)

// MessagesGetPinnedDialogs
// messages.getPinnedDialogs#d6b94df2 folder_id:int = messages.PeerDialogs;
func (c *DialogsCore) MessagesGetPinnedDialogs(in *mtproto.TLMessagesGetPinnedDialogs) (*mtproto.Messages_PeerDialogs, error) {
	if c == nil || c.MD == nil || c.MD.UserId <= 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.UpdatesClient == nil || c.svcCtx.Dao.DialogClient == nil || c.svcCtx.Dao.UserClient == nil || c.svcCtx.Dao.MessageClient == nil || c.svcCtx.Dao.ChatClient == nil {
		return nil, mtproto.ErrInternalServerError
	}
	var (
		err      error
		folderId = in.GetFolderId()
	)

	if folderId != 0 && folderId != 1 {
		err = mtproto.ErrFolderIdInvalid
		c.Logger.Errorf("messages.getPinnedDialogs - error: %v", err)
		return nil, err
	}

	var (
		peers              []*mtproto.PeerUtil
		dialogExtList      dialog.DialogExtList
		state              *mtproto.Updates_State
		notifySettingsList []*userpb.PeerPeerNotifySettings
	)

	err = mr.Finish(
		func() error {
			var err2 error
			state, err2 = c.svcCtx.Dao.UpdatesClient.UpdatesGetStateV2(c.ctx, &updates.TLUpdatesGetStateV2{
				AuthKeyId: c.MD.PermAuthKeyId,
				UserId:    c.MD.UserId,
			})
			if err2 != nil {
				c.Logger.Errorf("messages.getPinnedDialogs - error: %v", err2)
				return mtproto.ErrInternalServerError
			}

			return nil
		},
		func() error {
			if folderId == 0 {
				dialogFolder, err2 := c.svcCtx.Dao.DialogClient.DialogGetDialogFolder(c.ctx, &dialog.TLDialogGetDialogFolder{
					UserId:   c.MD.UserId,
					FolderId: 1,
				})
				if err2 != nil {
					c.Logger.Errorf("messages.getPinnedDialogs - error: %v", err2)
					return err2
				}
				if dialogFolder == nil {
					return mtproto.ErrInternalServerError
				}

				dialogExtList = append(dialogExtList, dialogFolder.GetDatas()...)
			}

			return nil
		},
		func() error {
			dList, err2 := c.svcCtx.Dao.DialogClient.DialogGetPinnedDialogs(c.ctx, &dialog.TLDialogGetPinnedDialogs{
				UserId:   c.MD.UserId,
				FolderId: folderId,
			})
			if err2 != nil {
				c.Logger.Errorf("messages.getPinnedDialogs - error: %v", err2)
				return err2
			}
			if dList == nil {
				return mtproto.ErrInternalServerError
			}
			dialogExtList = append(dialogExtList, dList.GetDatas()...)

			return nil
		},
	)
	if err != nil {
		c.Logger.Errorf("messages.getPinnedDialogs - getPeerDialogs error: %v", err)
		return nil, err
	}
	if state == nil {
		return nil, mtproto.ErrInternalServerError
	}

	for _, dialogEx := range dialogExtList {
		if dialogEx == nil || dialogEx.GetDialog() == nil || dialogEx.GetDialog().GetPeer() == nil {
			return nil, mtproto.ErrInternalServerError
		}
		peer2 := mtproto.FromPeer(dialogEx.GetDialog().GetPeer())
		if peer2 == nil {
			return nil, mtproto.ErrInternalServerError
		}
		peers = append(peers, peer2)
	}

	if len(peers) > 0 {
		settingsList, err2 := c.svcCtx.Dao.UserClient.UserGetNotifySettingsList(c.ctx, &userpb.TLUserGetNotifySettingsList{
			UserId: c.MD.UserId,
			Peers:  peers,
		})
		if err2 != nil {
			c.Logger.Errorf("messages.getPinnedDialogs - error: %v", err2)
			return nil, err2
		}
		if settingsList == nil {
			return nil, mtproto.ErrInternalServerError
		}

		notifySettingsList = settingsList.GetDatas()
	}

	for _, dialogEx := range dialogExtList {
		peer2 := mtproto.FromPeer(dialogEx.GetDialog().GetPeer())
		dialogEx.Dialog.NotifySettings = userpb.FindPeerPeerNotifySettings(notifySettingsList, peer2)
	}
	var (
		loadErr   error
		loadErrMu sync.Mutex
	)
	recordLoadErr := func(err error) {
		if err == nil {
			return
		}
		loadErrMu.Lock()
		if loadErr == nil {
			loadErr = err
		}
		loadErrMu.Unlock()
	}

	sort.Sort(sort.Reverse(dialogExtList))

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
					if v != nil {
						msgList = append(msgList, v.ToMessage(c.MD.UserId))
					}
				})
			}

			return msgList
		},
		func(ctx context.Context, selfUserId int64, id ...int64) []*mtproto.User {
			users, err := c.svcCtx.Dao.UserClient.UserGetMutableUsers(c.ctx,
				&userpb.TLUserGetMutableUsers{
					Id: id,
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
	err = loadErr
	loadErrMu.Unlock()
	if err != nil {
		c.Logger.Errorf("messages.getPinnedDialogs - entity load error: %v", err)
		return nil, err
	}

	return messageDialogs.ToMessagesPeerDialogs(state), nil
}
