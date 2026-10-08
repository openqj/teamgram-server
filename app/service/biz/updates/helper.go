/*
 * WARNING! All changes made in this file will be lost!
 * Created from 'scheme.tl' by 'mtprotoc'
 *
 * Copyright (c) 2021-present,  Teamgram Studio (https://teamgram.io).
 *  All rights reserved.
 *
 * Author: teamgramio (teamgram.io@gmail.com)
 */

package updates_helper

import (
	"github.com/teamgram/teamgram-server/app/service/biz/updates/internal/config"
	"github.com/teamgram/teamgram-server/app/service/biz/updates/internal/dal/dao/postgres_dao"
	"github.com/teamgram/teamgram-server/app/service/biz/updates/internal/dal/dataobject"
	"github.com/teamgram/teamgram-server/app/service/biz/updates/internal/server/grpc/service"
	"github.com/teamgram/teamgram-server/app/service/biz/updates/internal/svc"
)

type (
	Config                    = config.Config
	PostgresDB                = postgres_dao.DB
	PostgresUserPtsUpdatesDAO = postgres_dao.UserPtsUpdatesDAO
	PostgresAuthSeqUpdatesDAO = postgres_dao.AuthSeqUpdatesDAO
	PostgresUserPtsUpdatesDO  = dataobject.UserPtsUpdatesDO
	PostgresAuthSeqUpdatesDO  = dataobject.AuthSeqUpdatesDO
)

var (
	NewPostgresUserPtsUpdatesDAO = postgres_dao.NewUserPtsUpdatesDAO
	NewPostgresAuthSeqUpdatesDAO = postgres_dao.NewAuthSeqUpdatesDAO
)

func New(c Config) *service.Service {
	return service.New(svc.NewServiceContext(c))
}
