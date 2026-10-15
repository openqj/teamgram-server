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
	"strings"
	"sync"

	"github.com/teamgram/marmota/pkg/net/rpcx"
	"github.com/teamgram/teamgram-server/app/messenger/msg/inbox/internal/config"
	"github.com/teamgram/teamgram-server/app/messenger/msg/internal/dao"
	sync_client "github.com/teamgram/teamgram-server/app/messenger/sync/client"
	"github.com/teamgram/teamgram-server/pkg/mqproducer"
	// channel_client "github.com/teamgram/teamgram-server/app/service/biz/channel/client"
	chat_client "github.com/teamgram/teamgram-server/app/service/biz/chat/client"
	dialog_client "github.com/teamgram/teamgram-server/app/service/biz/dialog/client"
	user_client "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	idgen_client "github.com/teamgram/teamgram-server/app/service/idgen/client"
	"github.com/zeromicro/go-zero/core/logx"
)

type ServiceContext struct {
	Config config.Config
	*dao.Dao
	producers []*mqproducer.Producer
	closeOnce sync.Once
}

func NewServiceContext(c config.Config) *ServiceContext {
	if strings.TrimSpace(c.Postgres.DSN) == "" {
		panic(errors.New("messenger/msg/inbox: Postgres.DSN is required"))
	}
	if c.BotSyncClient == nil || strings.TrimSpace(c.BotSyncClient.Topic) == "" || len(c.BotSyncClient.Brokers) == 0 {
		panic(errors.New("messenger/msg/inbox: BotSyncClient with topic and brokers is required"))
	}
	if c.BotFatherSyncClient == nil || strings.TrimSpace(c.BotFatherSyncClient.Topic) == "" || len(c.BotFatherSyncClient.Brokers) == 0 {
		panic(errors.New("messenger/msg/inbox: BotFatherSyncClient with topic and brokers is required"))
	}
	syncProducer := mqproducer.MustNew(c.SyncClient)
	producers := []*mqproducer.Producer{syncProducer}
	daoStore := &dao.Dao{
		IDGenClient2: idgen_client.NewIDGenClient2(rpcx.GetCachedRpcClient(c.IdgenClient)),
		UserClient:   user_client.NewUserClient(rpcx.GetCachedRpcClient(c.UserClient)),
		ChatClient:   chat_client.NewChatClient(rpcx.GetCachedRpcClient(c.ChatClient)),
		SyncClient:   sync_client.NewSyncMqClient(syncProducer),
		DialogClient: dialog_client.NewDialogClient(rpcx.GetCachedRpcClient(c.DialogClient)),
	}
	pg, err := dao.NewPostgres(c.Postgres)
	if err != nil {
		_ = syncProducer.Close()
		panic(err)
	}
	daoStore.Postgres = pg
	if c.BotSyncClient != nil {
		producer := mqproducer.MustNew(c.BotSyncClient)
		producers = append(producers, producer)
		daoStore.BotSyncClient = sync_client.NewSyncMqClient(producer)
	}
	if c.BotFatherSyncClient != nil {
		producer := mqproducer.MustNew(c.BotFatherSyncClient)
		producers = append(producers, producer)
		daoStore.BotFatherSyncClient = sync_client.NewSyncMqClient(producer)
	}

	return &ServiceContext{
		Config:    c,
		Dao:       daoStore,
		producers: producers,
	}
}

func (s *ServiceContext) Close() {
	s.closeOnce.Do(func() {
		if err := mqproducer.CloseAll(s.producers...); err != nil {
			logx.Errorf("messenger/msg/inbox: Kafka shutdown failed: %v", err)
		}
		if s.Dao != nil && s.Dao.Postgres != nil {
			s.Dao.Postgres.Close()
		}
	})
}
