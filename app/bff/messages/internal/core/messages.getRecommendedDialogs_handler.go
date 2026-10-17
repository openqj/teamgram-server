//go:build recommendation_experimental

/*
 * Copyright (c) 2024-present,  Teamgram Authors.
 *  All rights reserved.
 *
 * Author: teamgramio (teamgram.io@gmail.com)
 */

package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
)

// MessagesGetRecommendedDialogs
// messages.getRecommendedDialogs#a1b2c3d4 offset:int limit:int = messages.Dialogs;
func (c *MessagesCore) MessagesGetRecommendedDialogs(in *mtproto.TLMessagesGetRecommendedDialogs) (*mtproto.Messages_Dialogs, error) {
	var (
		offset = in.GetOffset()
		limit  = in.GetLimit()
	)

	if limit <= 0 || limit > 100 {
		limit = 20
	}

	// 1. 从 Dialog 服务获取推荐列表
	recommendations, err := c.svcCtx.Svc.DialogClient().DialogGetRecommendedDialogs(c.ctx, &dialog.TLDialogGetRecommendedDialogs{
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
		return mtproto.MakeTLMessagesDialogs(&mtproto.Messages_Dialogs{
			Dialogs:  []*mtproto.Dialog{},
			Messages: []*mtproto.Message{},
			Users:    []*mtproto.User{},
			Chats:    []*mtproto.Chat{},
		}).To_Messages_Dialogs(), nil
	}

	// 3. 为每个推荐的对话获取完整的对话信息
	var (
		dialogList  []*mtproto.Dialog
		messageList []*mtproto.Message
		userIdList  []int64
		chatIdList  []int64
	)

	for _, rec := range recommendations.Dialogs {
		// 根据 peer_type 获取对话
		// peer_type: 1=user, 2=chat, 3=channel
		switch rec.PeerType {
		case 1: // User
			userIdList = append(userIdList, rec.PeerId)
		case 2, 3: // Chat or Channel
			chatIdList = append(chatIdList, rec.PeerId)
		}

		// TODO: 从 Dialog 服务获取完整的对话信息
		// dialogExt, err := c.svcCtx.Svc.DialogClient().DialogGetDialogById(...)
		// 这里需要根据 peer_type 和 peer_id 构建完整的 Dialog 对象
	}

	// 4. 获取用户和聊天信息
	// TODO: 调用 User 服务获取用户信息
	// TODO: 调用 Chat 服务获取聊天信息

	// 5. 构建返回结果
	result := mtproto.MakeTLMessagesDialogs(&mtproto.Messages_Dialogs{
		Dialogs:  dialogList,
		Messages: messageList,
		Users:    []*mtproto.User{}, // TODO: 填充用户列表
		Chats:    []*mtproto.Chat{}, // TODO: 填充聊天列表
	}).To_Messages_Dialogs()

	return result, nil
}
