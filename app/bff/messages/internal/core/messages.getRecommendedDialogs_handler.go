//go:build recommendation_experimental

/*
 * Copyright (c) 2024-present,  Teamgram Authors.
 *  All rights reserved.
 *
 * Author: teamgramio (teamgram.io@gmail.com)
 */

package core

import (
	"context"
	"errors"
	"sync"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/channelview"
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	"github.com/teamgram/teamgram-server/app/service/biz/message/message"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// MessagesGetRecommendedDialogs
// messages.getRecommendedDialogs#a1b2c3d4 offset:int limit:int = messages.Dialogs;
func (c *MessagesCore) MessagesGetRecommendedDialogs(in *TLMessagesGetRecommendedDialogs) (*mtproto.Messages_Dialogs, error) {
	if c == nil || c.MD == nil || c.MD.UserId <= 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if in == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil ||
		c.svcCtx.Dao.DialogClient == nil || c.svcCtx.Dao.MessageClient == nil ||
		c.svcCtx.Dao.UserClient == nil || c.svcCtx.Dao.ChatClient == nil ||
		c.svcCtx.Dao.ChatClient.Client() == nil {
		return nil, mtproto.ErrInternalServerError
	}

	var (
		offset = in.GetOffset()
		limit  = in.GetLimit()
	)

	if offset < 0 || offset > 10000 {
		return nil, mtproto.ErrOffsetInvalid
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	// 1. 从 Dialog 服务获取推荐列表
	recommendations, err := c.svcCtx.Dao.DialogClient.DialogGetRecommendedDialogs(c.ctx, &dialog.TLDialogGetRecommendedDialogs{
		UserId: c.MD.UserId,
		Offset: offset,
		Limit:  limit,
	})

	if err != nil {
		c.Logger.Errorf("DialogGetRecommendedDialogs error: %v", err)
		return nil, err
	}

	// 2. 如果没有推荐，返回空列表
	if recommendations == nil || len(recommendations.Dialogs) == 0 {
		return mtproto.MakeTLMessagesDialogsSlice(&mtproto.Messages_Dialogs{
			Dialogs:  []*mtproto.Dialog{},
			Messages: []*mtproto.Message{},
			Users:    []*mtproto.User{},
			Chats:    []*mtproto.Chat{},
			Count:    0,
		}).To_Messages_Dialogs(), nil
	}

	// 3. 为每个推荐的对话获取完整的对话信息。
	dialogExtList := make(dialog.DialogExtList, 0, len(recommendations.Dialogs))
	seen := make(map[recommendationPeer]struct{}, len(recommendations.Dialogs))

	for _, rec := range recommendations.Dialogs {
		if rec == nil || rec.GetPeerId() <= 0 || rec.GetPeerType() < 1 || rec.GetPeerType() > 3 {
			continue
		}
		peer := recommendationPeer{peerType: rec.GetPeerType(), peerID: rec.GetPeerId()}
		if _, ok := seen[peer]; ok {
			continue
		}
		seen[peer] = struct{}{}

		dialogExt, err := c.svcCtx.Dao.DialogClient.DialogGetDialogById(c.ctx, &dialog.TLDialogGetDialogById{
			UserId:   c.MD.UserId,
			PeerType: peer.peerType,
			PeerId:   peer.peerID,
		})
		if err != nil {
			// A recommendation may outlive a deleted dialog. Omit that stale row;
			// transport and storage errors still fail the request.
			if errors.Is(err, mtproto.ErrPeerIdInvalid) {
				continue
			}
			return nil, err
		}
		if dialogExt == nil || dialogExt.GetDialog() == nil || dialogExt.GetDialog().GetPeer() == nil {
			continue
		}
		dialogExtList = append(dialogExtList, dialogExt)
	}

	if len(dialogExtList) == 0 {
		return mtproto.MakeTLMessagesDialogsSlice(&mtproto.Messages_Dialogs{
			Dialogs:  []*mtproto.Dialog{},
			Messages: []*mtproto.Message{},
			Users:    []*mtproto.User{},
			Chats:    []*mtproto.Chat{},
			Count:    0,
		}).To_Messages_Dialogs(), nil
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

	messageDialogs := dialogExtList.DoGetMessagesDialogs(
		c.ctx,
		c.MD.UserId,
		func(ctx context.Context, selfUserID int64, ids ...dialog.TopMessageId) []*mtproto.Message {
			msgIDs := make([]int32, 0, len(ids))
			for _, id := range ids {
				if id.Peer != nil && !id.Peer.IsChannel() && id.TopMessage > 0 {
					msgIDs = append(msgIDs, id.TopMessage)
				}
			}
			if len(msgIDs) == 0 {
				return nil
			}
			boxList, err := c.svcCtx.Dao.MessageClient.MessageGetUserMessageList(ctx, &message.TLMessageGetUserMessageList{
				UserId: selfUserID,
				IdList: msgIDs,
			})
			if err != nil {
				recordLoadErr(err)
				return nil
			}
			if boxList == nil {
				recordLoadErr(mtproto.ErrInternalServerError)
				return nil
			}
			messages := make([]*mtproto.Message, 0, len(boxList.GetDatas()))
			boxList.Walk(func(_ int, box *mtproto.MessageBox) {
				if box != nil {
					messages = append(messages, box.ToMessage(selfUserID))
				}
			})
			return messages
		},
		func(ctx context.Context, selfUserID int64, ids ...int64) []*mtproto.User {
			users, err := c.svcCtx.Dao.UserClient.UserGetMutableUsersV2(ctx, &userpb.TLUserGetMutableUsersV2{
				Id:      ids,
				Privacy: true,
				HasTo:   true,
				To:      []int64{selfUserID},
			})
			if err != nil {
				recordLoadErr(err)
				return nil
			}
			if users == nil {
				recordLoadErr(mtproto.ErrInternalServerError)
				return nil
			}
			return users.GetUserListByIdList(selfUserID, ids...)
		},
		func(ctx context.Context, _ int64, ids ...int64) []*mtproto.Chat {
			chats, err := c.svcCtx.Dao.ChatClient.Client().ChatGetChatListByIdList(ctx, &chatpb.TLChatGetChatListByIdList{
				IdList: ids,
			})
			if err != nil {
				recordLoadErr(err)
				return nil
			}
			if chats == nil {
				recordLoadErr(mtproto.ErrInternalServerError)
				return nil
			}
			return chats.GetChatListByIdList(c.MD.UserId, ids...)
		},
		func(_ context.Context, selfUserID int64, ids ...int64) []*mtproto.Chat {
			return channelview.ChatsByID(selfUserID, ids)
		})

	loadErrMu.Lock()
	err = loadErr
	loadErrMu.Unlock()
	if err != nil {
		c.Logger.Errorf("MessagesGetRecommendedDialogs entity hydration error: %v", err)
		return nil, err
	}

	return messageDialogs.ToMessagesDialogs(int32(len(dialogExtList))), nil
}
