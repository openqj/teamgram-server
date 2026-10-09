/*
 * Created from 'scheme.tl' by 'mtprotoc'
 *
 * Copyright (c) 2021-present,  Teamgram Studio (https://teamgram.io).
 *  All rights reserved.
 *
 * Author: teamgramio (teamgram.io@gmail.com)
 */

package core

import (
	"math/rand"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	"github.com/teamgram/teamgram-server/app/service/biz/chat/internal/dal/dataobject"
)

// ChatCreateChat2
// chat.createChat2 flags:# creator_id:long user_id_list:Vector<long> title:string bots:flags.0?Vector<long> ttl_period:flags.1?int = MutableChat;
func (c *ChatCore) ChatCreateChat2(in *chat.TLChatCreateChat2) (*mtproto.MutableChat, error) {
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.Postgres == nil ||
		c.svcCtx.Dao.Postgres.Pool == nil || c.svcCtx.Dao.Postgres.Store == nil {
		return nil, mtproto.ErrInternalServerError
	}
	var (
		chatsDO    *dataobject.ChatsDO
		err        error
		date       = time.Now().Unix()
		creatorId  = in.CreatorId
		userIdList = in.UserIdList
		title      = in.Title
	)

	// TODO:
	var lastChat *dataobject.ChatsDO
	lastChat, err = c.svcCtx.Dao.Postgres.Store.Chats.SelectLastCreator(c.ctx, creatorId)
	if err != nil {
		c.Logger.Errorf("chat.createChat2 - error: %v", err)
		return nil, err
	} else if lastChat != nil {
		if date-lastChat.Date < createChatFlood {
			err = mtproto.NewErrFloodWaitX(int32(date - lastChat.Date))
			c.Logger.Errorf("createChat error: %v. lastCreate = ", err, lastChat.Date)
			return nil, err
		}
	}

	chatsDO = &dataobject.ChatsDO{
		Id:                     0,
		CreatorUserId:          creatorId,
		AccessHash:             rand.Int63(),
		RandomId:               0,
		ParticipantCount:       int32(1 + len(userIdList)),
		Title:                  title,
		About:                  "",
		PhotoId:                0,
		DefaultBannedRights:    int64(mtproto.MakeDefaultBannedRights().ToBannedRights()),
		MigratedToId:           0,
		MigratedToAccessHash:   0,
		AvailableReactionsType: 0,
		AvailableReactions:     "",
		Deactivated:            false,
		Noforwards:             false,
		TtlPeriod:              in.GetTtlPeriod().GetValue(),
		Version:                1,
		Date:                   date,
	}

	participantDOList := make([]*dataobject.ChatParticipantsDO, 0)
	for i := 0; i < len(userIdList)+1; i++ {
		if i == 0 {
			participantDOList = append(participantDOList, &dataobject.ChatParticipantsDO{
				UserId:          creatorId,
				ParticipantType: mtproto.ChatMemberCreator,
				Link:            chat.GenChatInviteHash(),
				InviterUserId:   0,
				InvitedAt:       date,
				Date2:           date,
				IsBot:           false,
			})
		} else {
			participantDOList = append(participantDOList, &dataobject.ChatParticipantsDO{
				UserId:          userIdList[i-1],
				ParticipantType: mtproto.ChatMemberNormal,
				Link:            "",
				InviterUserId:   creatorId,
				InvitedAt:       date,
				Date2:           date,
				IsBot:           false,
			})
		}
	}

	for _, id := range in.Bots {
		participantDOList = append(participantDOList, &dataobject.ChatParticipantsDO{
			UserId:          id,
			ParticipantType: mtproto.ChatMemberNormal,
			Link:            "",
			InviterUserId:   creatorId,
			InvitedAt:       date,
			Date2:           date,
			IsBot:           true,
		})
	}

	tx, txErr := c.svcCtx.Dao.Postgres.Pool.Begin(c.ctx)
	if txErr != nil {
		return nil, txErr
	}
	defer func() { _ = tx.Rollback(c.ctx) }()
	chatsDO.Id, _, txErr = c.svcCtx.Dao.Postgres.Store.Chats.InsertOn(c.ctx, tx, chatsDO)
	if txErr == nil {
		for i := range participantDOList {
			participantDOList[i].ChatId = chatsDO.Id
		}
		_, _, txErr = c.svcCtx.Dao.Postgres.Store.Participants.InsertBulkOn(c.ctx, tx, participantDOList)
	}
	if txErr == nil {
		_, _, txErr = c.svcCtx.Dao.Postgres.Store.Invites.InsertOn(c.ctx, tx, &dataobject.ChatInvitesDO{
			ChatId: chatsDO.Id, AdminId: creatorId, Link: participantDOList[0].Link,
			Permanent: true, Date2: date,
		})
	}
	if txErr == nil {
		txErr = tx.Commit(c.ctx)
	}
	if txErr != nil {
		c.Logger.Errorf("chat.createChat2 - error: %v", txErr)
		return nil, txErr
	}

	chat2 := mtproto.MakeTLMutableChat(&mtproto.MutableChat{
		Chat:             c.svcCtx.Dao.MakeImmutableChatByDO(chatsDO),
		ChatParticipants: make([]*mtproto.ImmutableChatParticipant, 0, len(participantDOList)),
	}).To_MutableChat()

	for i := 0; i < len(participantDOList); i++ {
		chat2.ChatParticipants = append(chat2.ChatParticipants,
			c.svcCtx.Dao.MakeImmutableChatParticipant(participantDOList[i]))
	}

	chat2.Chat.ParticipantsCount = int32(len(participantDOList))

	// c.svcCtx.Dao.PutMutableChat(c.ctx, chat2)

	return chat2, nil
}
