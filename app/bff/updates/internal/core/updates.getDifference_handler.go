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
	"errors"
	"time"

	"github.com/teamgram/proto/mtproto"
	updatesdao "github.com/teamgram/teamgram-server/app/bff/updates/internal/dao"
	"github.com/teamgram/teamgram-server/app/service/authsession/authsession"
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	"github.com/teamgram/teamgram-server/app/service/biz/updates/updates"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// UpdatesGetDifference
// updates.getDifference#19c2f763 flags:# pts:int pts_limit:flags.1?int pts_total_limit:flags.0?int date:int qts:int qts_limit:flags.2?int = updates.Difference;
func (c *UpdatesCore) UpdatesGetDifference(in *mtproto.TLUpdatesGetDifference) (*mtproto.Updates_Difference, error) {
	qtsLimit := int32(0)
	if in.GetQtsLimit() != nil {
		qtsLimit = in.GetQtsLimit().GetValue()
	}
	secretDiff, err := c.svcCtx.Dao.GetSecretDifference(c.ctx, c.MD.UserId, in.GetQts(), qtsLimit)
	if err != nil {
		if errors.Is(err, updatesdao.ErrMaxQTSInvalid) {
			return nil, mtproto.ErrMaxQtsInvalid
		}
		c.Logger.Errorf("updates.getDifference - secret difference error: %v", err)
		return nil, err
	}

	keyId, err := c.svcCtx.Dao.AuthsessionClient.AuthsessionGetPermAuthKeyId(c.ctx, &authsession.TLAuthsessionGetPermAuthKeyId{
		AuthKeyId: c.MD.PermAuthKeyId,
	})
	if err != nil {
		c.Logger.Errorf("updates.getDifference - error: %v", err)
		return nil, err
	}
	c.Logger.Infof("updates.getDifference - keyId: %v", keyId)

	updatesDiff, err := c.svcCtx.Dao.UpdatesClient.UpdatesGetDifferenceV2(c.ctx, &updates.TLUpdatesGetDifferenceV2{
		AuthKeyId:     keyId.GetV(),
		UserId:        c.MD.UserId,
		Pts:           in.Pts,
		PtsTotalLimit: in.PtsTotalLimit,
		Date:          int64(in.Date),
	})
	if err != nil {
		c.Logger.Errorf("updates.getDifference - error: %v", err)
		return nil, err
	}

	var (
		idHelper     = mtproto.NewIDListHelper(c.MD.UserId)
		rDifference  *mtproto.Updates_Difference
		state        *mtproto.Updates_State
		newMessages  []*mtproto.Message
		otherUpdates []*mtproto.Update
		normalSlice  bool
		normalEmpty  bool
	)

	switch updatesDiff.GetPredicateName() {
	case updates.Predicate_differenceEmpty:
		normalEmpty = true
		state = updatesDiff.GetState()
	case updates.Predicate_difference:
		// TODO: fix date
		updatesDiff.State.Date = int32(time.Now().Unix())
		state = updatesDiff.GetState()
		newMessages = updatesDiff.GetNewMessages()
		otherUpdates = updatesDiff.GetOtherUpdates()
	case updates.Predicate_differenceSlice:
		normalSlice = true
		state = updatesDiff.GetIntermediateState()
		newMessages = updatesDiff.GetNewMessages()
		otherUpdates = updatesDiff.GetOtherUpdates()
	case updates.Predicate_differenceTooLong:
		// TODO: iOS
		return mtproto.MakeTLUpdatesDifferenceTooLong(&mtproto.Updates_Difference{
			Pts: updatesDiff.Pts,
		}).To_Updates_Difference(), nil
	default:
		return nil, mtproto.ErrInternalServerError
	}

	if state == nil {
		state = mtproto.MakeTLUpdatesState(&mtproto.Updates_State{}).To_Updates_State()
	}
	state.Qts = secretDiff.CurrentQTS
	encryptedMessages := make([]*mtproto.EncryptedMessage, 0, len(secretDiff.Messages))
	for _, message := range secretDiff.Messages {
		encryptedMessages = append(encryptedMessages, secretEncryptedMessage(message))
	}
	if secretDiff.HasMore && len(secretDiff.Messages) > 0 {
		state.Qts = secretDiff.Messages[len(secretDiff.Messages)-1].QTS
	} else if secretDiff.HasMore {
		secretDiff.HasMore = false
	}

	if normalEmpty && len(encryptedMessages) == 0 && in.GetQts() == secretDiff.CurrentQTS {
		return mtproto.MakeTLUpdatesDifferenceEmpty(&mtproto.Updates_Difference{
			Date: state.GetDate(),
			Seq:  state.GetSeq(),
		}).To_Updates_Difference(), nil
	}

	data := &mtproto.Updates_Difference{
		NewMessages:          newMessages,
		NewEncryptedMessages: encryptedMessages,
		OtherUpdates:         otherUpdates,
		Chats:                nil,
		Users:                nil,
	}
	if normalSlice || secretDiff.HasMore {
		data.IntermediateState = state
		rDifference = mtproto.MakeTLUpdatesDifferenceSlice(data).To_Updates_Difference()
	} else {
		data.State = state
		rDifference = mtproto.MakeTLUpdatesDifference(data).To_Updates_Difference()
	}

	idHelper.PickByMessages(rDifference.NewMessages...)
	idHelper.PickByUpdates(rDifference.OtherUpdates...)

	idHelper.Visit(
		func(userIdList []int64) {
			users, _ := c.svcCtx.Dao.UserClient.UserGetMutableUsers(c.ctx,
				&userpb.TLUserGetMutableUsers{
					Id: append(userIdList, c.MD.UserId),
				})
			rDifference.Users = users.GetUserListByIdList(c.MD.UserId, userIdList...)
		},
		func(chatIdList []int64) {
			chats, _ := c.svcCtx.Dao.ChatClient.ChatGetChatListByIdList(c.ctx,
				&chatpb.TLChatGetChatListByIdList{
					IdList: chatIdList,
				})
			rDifference.Chats = chats.GetChatListByIdList(c.MD.UserId, chatIdList...)
		},
		func(channelIdList []int64) {
		})

	return rDifference, nil
}

func secretEncryptedMessage(message updatesdao.SecretMessage) *mtproto.EncryptedMessage {
	data := &mtproto.EncryptedMessage{
		RandomId: message.RandomID,
		ChatId:   message.ChatID,
		Date:     message.Date,
		Bytes:    message.Data,
	}
	if message.Service {
		return mtproto.MakeTLEncryptedMessageService(data).To_EncryptedMessage()
	}
	if message.File == nil {
		data.File = mtproto.MakeTLEncryptedFileEmpty(nil).To_EncryptedFile()
	} else {
		data.File = mtproto.MakeTLEncryptedFile(&mtproto.EncryptedFile{
			Id:             message.File.ID,
			AccessHash:     message.File.AccessHash,
			Size2_INT32:    int32(message.File.Size),
			Size2_INT64:    message.File.Size,
			DcId:           message.File.DCID,
			KeyFingerprint: message.File.KeyFingerprint,
		}).To_EncryptedFile()
	}
	return mtproto.MakeTLEncryptedMessage(data).To_EncryptedMessage()
}
