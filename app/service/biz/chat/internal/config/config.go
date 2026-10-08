/*
 * Created from 'scheme.tl' by 'mtprotoc'
 *
 * Copyright (c) 2021-present,  Teamgram Studio (https://teamgram.io).
 *  All rights reserved.
 *
 * Author: teamgramio (teamgram.io@gmail.com)
 */

package config

import (
	"github.com/teamgram/teamgram-server/pkg/storage/postgres"
	"github.com/zeromicro/go-zero/core/stores/cache"

	"github.com/zeromicro/go-zero/zrpc"
)

type Config struct {
	zrpc.RpcServerConf
	Postgres    postgres.Config
	Cache       cache.CacheConf
	MediaClient zrpc.RpcClientConf
}
