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
	"errors"
	"strings"

	dfs_client "github.com/teamgram/teamgram-server/app/service/dfs/client"
	"github.com/teamgram/teamgram-server/app/service/media/internal/config"

	"github.com/zeromicro/go-zero/zrpc"
)

type Dao struct {
	*Postgres
	dfs_client.DfsClient
}

func New(c config.Config) *Dao {
	if strings.TrimSpace(c.Postgres.DSN) == "" {
		panic(errors.New("media: Postgres.DSN is required"))
	}
	pg := newPostgresDao(c)
	return &Dao{
		Postgres:  pg,
		DfsClient: dfs_client.NewDfsClient(zrpc.MustNewClient(c.Dfs)),
	}
}
