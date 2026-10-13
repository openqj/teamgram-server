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
	"context"
	"sync"

	kafka "github.com/teamgram/marmota/pkg/mq"
	"github.com/teamgram/marmota/pkg/net/rpcx"
	"github.com/teamgram/proto/mtproto"
	msg_client "github.com/teamgram/teamgram-server/app/messenger/msg/msg/client"
	sync_client "github.com/teamgram/teamgram-server/app/messenger/sync/client"
	"github.com/teamgram/teamgram-server/app/messenger/sync/internal/config"
	chat_client "github.com/teamgram/teamgram-server/app/service/biz/chat/client"
	user_client "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	idgen_client "github.com/teamgram/teamgram-server/app/service/idgen/client"
	status_client "github.com/teamgram/teamgram-server/app/service/status/client"
	"github.com/zeromicro/go-zero/core/stores/kv"
	"github.com/zeromicro/go-zero/zrpc"
)

type Dao struct {
	*Postgres
	kv               kv.Store
	conf             *config.Config
	mu               sync.RWMutex
	sessionServers   map[string]SessionPusher
	useStreamSession bool
	idgen_client.IDGenClient2
	status_client.StatusClient
	chat_client.ChatClient
	user_client.UserClient
	user_client.BotRegistryClient
	msg_client.MsgClient
	PushClient sync_client.SyncClient
}

func New(c config.Config) (*Dao, error) {
	pg, err := newPostgresDao(c)
	if err != nil {
		return nil, err
	}
	d := &Dao{
		Postgres:          pg,
		kv:                kv.NewStore(c.KV),
		conf:              &c,
		sessionServers:    make(map[string]SessionPusher),
		useStreamSession:  c.UseStreamSession,
		IDGenClient2:      idgen_client.NewIDGenClient2(zrpc.MustNewClient(c.IdgenClient)),
		StatusClient:      status_client.NewStatusClient(zrpc.MustNewClient(c.StatusClient)),
		ChatClient:        chat_client.NewChatClient(rpcx.GetCachedRpcClient(c.ChatClient)),
		UserClient:        user_client.NewUserClient(rpcx.GetCachedRpcClient(c.UserClient)),
		BotRegistryClient: user_client.NewBotRegistryClient(rpcx.GetCachedRpcClient(c.UserClient)),
		MsgClient:         msg_client.NewMsgClient(rpcx.GetCachedRpcClient(c.MsgClient)),
	}
	if c.PushClient != nil {
		d.PushClient = sync_client.NewSyncMqClient(kafka.MustKafkaProducer(c.PushClient))
	}
	if d.Postgres != nil && d.Postgres.BotAPI != nil {
		d.Postgres.BotAPI.SetUserLoader(func(ctx context.Context, userID int64) (*mtproto.ImmutableUser, error) {
			return d.UserGetImmutableUser(ctx, &userpb.TLUserGetImmutableUser{Id: userID})
		})
	}

	go d.watch(c.SessionClient)
	return d, nil
}
