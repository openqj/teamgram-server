/*
 * Copyright (c) 2026 The Teamgram Authors.
 * All rights reserved.
 */

package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

func (c *UserCore) UserGetCreatedBots(in *user.BotRegistryGetCreatedBotsRequest) (*user.Vector_ImmutableUser, error) {
	if in == nil {
		return nil, mtproto.ErrInputConstructorInvalid
	}
	if c.MD == nil || c.MD.GetUserId() <= 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	creatorUserId := c.MD.GetUserId()

	botIds, err := c.svcCtx.Dao.SelectBotIdsByCreatorUserID(c.ctx, creatorUserId)
	if err != nil {
		return nil, err
	}

	bots := &user.Vector_ImmutableUser{Datas: make([]*mtproto.ImmutableUser, 0, len(botIds))}
	for _, botId := range botIds {
		bot, err := c.svcCtx.Dao.GetImmutableUser(c.ctx, botId, true, creatorUserId)
		if err != nil {
			return nil, err
		}
		if bot == nil || bot.GetUser() == nil || bot.GetUser().GetBot() == nil {
			return nil, mtproto.ErrInternalServerError
		}
		bots.Datas = append(bots.Datas, bot)
	}
	return bots, nil
}

func (c *UserCore) UserGetAdminedBots(in *user.BotRegistryGetAdminedBotsRequest) (*user.Vector_ImmutableUser, error) {
	if in == nil {
		return nil, mtproto.ErrInputConstructorInvalid
	}
	if c.MD == nil || c.MD.GetUserId() <= 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}

	botIds, err := c.svcCtx.Dao.SelectBotIdsByCreatorUserID(c.ctx, c.MD.GetUserId())
	if err != nil {
		return nil, err
	}

	bots := &user.Vector_ImmutableUser{Datas: make([]*mtproto.ImmutableUser, 0, len(botIds))}
	for _, botId := range botIds {
		bot, err := c.svcCtx.Dao.GetImmutableUser(c.ctx, botId, true, c.MD.GetUserId())
		if err != nil {
			return nil, err
		}
		if bot == nil || bot.GetUser() == nil || bot.GetUser().GetBot() == nil {
			return nil, mtproto.ErrInternalServerError
		}
		bots.Datas = append(bots.Datas, bot)
	}
	return bots, nil
}
