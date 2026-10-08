/*
 * Created from 'scheme.tl' by 'mtprotoc'
 *
 * Copyright (c) 2021-present,  Teamgram Studio (https://teamgram.io).
 *  All rights reserved.
 *
 * Author: teamgramio (teamgram.io@gmail.com)
 */

package dao

import (
	"github.com/teamgram/marmota/pkg/stores/sqlc"
	dfs_client "github.com/teamgram/teamgram-server/app/service/dfs/client"
	"github.com/teamgram/teamgram-server/app/service/media/internal/config"

	"github.com/zeromicro/go-zero/zrpc"
)

type Dao struct {
	// Mysql is kept as a named field for legacy test fixtures. New service
	// instances always populate Postgres instead.
	*Mysql
	*Postgres
	sqlc.CachedConn
	dfs_client.DfsClient
}

func New(c config.Config) *Dao {
	pg := newPostgresDao(c)
	return &Dao{
		Postgres:  pg,
		DfsClient: dfs_client.NewDfsClient(zrpc.MustNewClient(c.Dfs)),
	}
}
