/*
 * Created from 'scheme.tl' by 'mtprotoc'
 *
 * Copyright 2022 Teamgram Authors
 *  All rights reserved.
 *
 * Author: teamgramio (teamgram.io@gmail.com)
 */

package config

import (
	kafka "github.com/teamgram/marmota/pkg/mq"
	"github.com/teamgram/teamgram-server/pkg/storage/postgres"

	"github.com/zeromicro/go-zero/core/stores/cache"
	"github.com/zeromicro/go-zero/core/stores/kv"
	"github.com/zeromicro/go-zero/zrpc"
)

type Config struct {
	zrpc.RpcServerConf
	InboxConsumer   kafka.KafkaConsumerConf
	Postgres        postgres.Config
	Cache           cache.CacheConf
	KV              kv.KvConf
	IdgenClient     zrpc.RpcClientConf
	UserClient      zrpc.RpcClientConf
	ChatClient      zrpc.RpcClientConf
	DialogClient    zrpc.RpcClientConf
	SyncClient      *kafka.KafkaProducerConf
	BotSyncClient   *kafka.KafkaProducerConf `json:",optional"`
	MessageSharding int                      `json:",default=1"`
}
