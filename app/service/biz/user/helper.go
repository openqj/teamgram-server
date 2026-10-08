/*
 * WARNING! All changes made in this file will be lost!
 * Created from 'scheme.tl' by 'mtprotoc'
 *
 * Copyright (c) 2021-present,  Teamgram Studio (https://teamgram.io).
 *  All rights reserved.
 *
 * Author: teamgramio (teamgram.io@gmail.com)
 */

package user_helper

import (
	"github.com/teamgram/teamgram-server/app/service/biz/user/internal/config"
	"github.com/teamgram/teamgram-server/app/service/biz/user/internal/dal/dao/postgres_dao"
	"github.com/teamgram/teamgram-server/app/service/biz/user/internal/dao"
	"github.com/teamgram/teamgram-server/app/service/biz/user/internal/server/grpc/service"
	"github.com/teamgram/teamgram-server/app/service/biz/user/internal/svc"
)

type (
	Dao           = dao.Dao
	CacheUserData = dao.CacheUserData
	PostgresDB    = postgres_dao.DB
	PostgresStore = postgres_dao.Store
)

var (
	GenCacheUserDataCacheKey = dao.GenCacheUserDataCacheKey
	NewPostgresStore         = postgres_dao.NewStore
)

type (
	Config  = config.Config
	Service = service.Service
)

var (
	NewServiceContext = svc.NewServiceContext
)

func New(c Config) *service.Service {
	return service.New(svc.NewServiceContext(c))
}
