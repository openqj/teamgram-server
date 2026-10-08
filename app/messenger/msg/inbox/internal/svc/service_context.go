/*
 * Created from 'scheme.tl' by 'mtprotoc'
 *
 * Copyright (c) 2021-present,  Teamgram Studio (https://teamgram.io).
 *  All rights reserved.
 *
 * Author: teamgramio (teamgram.io@gmail.com)
 */

package svc

import (
	"errors"

	kafka "github.com/teamgram/marmota/pkg/mq"
	"github.com/teamgram/marmota/pkg/net/rpcx"
	"github.com/teamgram/teamgram-server/app/messenger/msg/inbox/internal/config"
	"github.com/teamgram/teamgram-server/app/messenger/msg/internal/dao"
	sync_client "github.com/teamgram/teamgram-server/app/messenger/sync/client"
	// channel_client "github.com/teamgram/teamgram-server/app/service/biz/channel/client"
	chat_client "github.com/teamgram/teamgram-server/app/service/biz/chat/client"
	dialog_client "github.com/teamgram/teamgram-server/app/service/biz/dialog/client"
	user_client "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	idgen_client "github.com/teamgram/teamgram-server/app/service/idgen/client"
)

type ServiceContext struct {
	Config config.Config
	*dao.Dao
}

func NewServiceContext(c config.Config) *ServiceContext {
	if c.Postgres.DSN == "" {
		panic(errors.New("messenger/msg/inbox: Postgres.DSN is required"))
	}
	daoStore := &dao.Dao{
		IDGenClient2: idgen_client.NewIDGenClient2(rpcx.GetCachedRpcClient(c.IdgenClient)),
		UserClient:   user_client.NewUserClient(rpcx.GetCachedRpcClient(c.UserClient)),
		ChatClient:   chat_client.NewChatClient(rpcx.GetCachedRpcClient(c.ChatClient)),
		SyncClient:   sync_client.NewSyncMqClient(kafka.GetCachedMQClient(c.SyncClient)),
		DialogClient: dialog_client.NewDialogClient(rpcx.GetCachedRpcClient(c.DialogClient)),
	}
	pg, err := dao.NewPostgres(c.Postgres)
	if err != nil {
		panic(err)
	}
	daoStore.Postgres = pg
	if c.BotSyncClient != nil {
		daoStore.BotSyncClient = sync_client.NewSyncMqClient(kafka.GetCachedMQClient(c.BotSyncClient))
	}

	return &ServiceContext{
		Config: c,
		Dao:    daoStore,
	}
}
