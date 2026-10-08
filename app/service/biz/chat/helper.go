/*
 * WARNING! All changes made in this file will be lost!
 * Created from 'scheme.tl' by 'mtprotoc'
 *
 * Copyright (c) 2021-present,  Teamgram Studio (https://teamgram.io).
 *  All rights reserved.
 *
 * Author: teamgramio (teamgram.io@gmail.com)
 */

package chat_helper

import (
	"github.com/teamgram/teamgram-server/app/service/biz/chat/internal/config"
	"github.com/teamgram/teamgram-server/app/service/biz/chat/internal/dal/dao/mysql_dao"
	"github.com/teamgram/teamgram-server/app/service/biz/chat/internal/dal/dao/postgres_dao"
	"github.com/teamgram/teamgram-server/app/service/biz/chat/internal/dal/dataobject"
	"github.com/teamgram/teamgram-server/app/service/biz/chat/internal/server/grpc/service"
	"github.com/teamgram/teamgram-server/app/service/biz/chat/internal/svc"
	"github.com/teamgram/teamgram-server/app/service/biz/chat/plugin"
)

type (
	Config = config.Config
)

type (
	ChatParticipantsDAO      = mysql_dao.ChatParticipantsDAO
	ChatParticipantsDO       = dataobject.ChatParticipantsDO
	PostgresDB               = postgres_dao.DB
	PostgresChatParticipants = postgres_dao.ChatParticipantsDAO
)

var (
	NewChatParticipantsDAO         = mysql_dao.NewChatParticipantsDAO
	NewPostgresChatParticipantsDAO = postgres_dao.NewChatParticipantsDAO
)

func New(c Config, plugin plugin.ChatPlugin) *service.Service {
	return service.New(svc.NewServiceContext(c, plugin))
}
