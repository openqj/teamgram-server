// Copyright 2022 Teamgram Authors
//  All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// Author: teamgramio (teamgram.io@gmail.com)
//

package dao

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	kafka "github.com/teamgram/marmota/pkg/mq"
	"github.com/teamgram/marmota/pkg/net/rpcx"
	"github.com/teamgram/teamgram-server/app/bff/messages/internal/config"
	msg_client "github.com/teamgram/teamgram-server/app/messenger/msg/msg/client"
	sync_client "github.com/teamgram/teamgram-server/app/messenger/sync/client"
	chat_client "github.com/teamgram/teamgram-server/app/service/biz/chat/client"
	dialog_client "github.com/teamgram/teamgram-server/app/service/biz/dialog/client"
	message_client "github.com/teamgram/teamgram-server/app/service/biz/message/client"
	user_client "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	idgen_client "github.com/teamgram/teamgram-server/app/service/idgen/client"
	media_client "github.com/teamgram/teamgram-server/app/service/media/client"
	"github.com/zeromicro/go-zero/core/stores/kv"
)

type Dao struct {
	msg_client.MsgClient
	user_client.UserClient
	ChatClient *chat_client.ChatClientHelper
	media_client.MediaClient
	message_client.MessageClient
	idgen_client.IDGenClient2
	dialog_client.DialogClient
	sync_client.SyncClient
	KV               kv.Store
	ReceivedMessages ReceivedMessagesStore
	postgres         *pgxpool.Pool
}

func New(c config.Config) *Dao {
	pool, err := openPostgres(c.PostgresDSN)
	if err != nil {
		panic(err)
	}
	var floodKV kv.Store
	if len(c.KV) > 0 {
		floodKV = kv.NewStore(c.KV)
	}
	return &Dao{
		MsgClient:        msg_client.NewMsgClient(rpcx.GetCachedRpcClient(c.MsgClient)),
		UserClient:       user_client.NewUserClient(rpcx.GetCachedRpcClient(c.UserClient)),
		ChatClient:       chat_client.NewChatClientHelper(rpcx.GetCachedRpcClient(c.ChatClient)),
		MediaClient:      media_client.NewMediaClient(rpcx.GetCachedRpcClient(c.MediaClient)),
		DialogClient:     dialog_client.NewDialogClient(rpcx.GetCachedRpcClient(c.DialogClient)),
		IDGenClient2:     idgen_client.NewIDGenClient2(rpcx.GetCachedRpcClient(c.IdgenClient)),
		MessageClient:    message_client.NewMessageClient(rpcx.GetCachedRpcClient(c.MessageClient)),
		SyncClient:       sync_client.NewSyncMqClient(kafka.MustKafkaProducer(c.SyncClient)),
		KV:               floodKV,
		ReceivedMessages: NewPostgresReceivedMessagesStore(pool),
		postgres:         pool,
	}
}

// Close releases the PostgreSQL pool owned by this BFF instance.
func (d *Dao) Close() {
	if d != nil && d.postgres != nil {
		d.postgres.Close()
	}
}

func openPostgres(dsn string) (*pgxpool.Pool, error) {
	if strings.TrimSpace(dsn) == "" {
		return nil, errors.New("messages: PostgresDSN is required")
	}
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	config.MaxConns = 8
	config.MinConns = 1
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, err
	}
	if err = pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	var version string
	if err = pool.QueryRow(ctx, `SHOW server_version_num`).Scan(&version); err != nil {
		pool.Close()
		return nil, err
	}
	versionNum, parseErr := strconv.Atoi(strings.TrimSpace(version))
	if parseErr != nil || versionNum < 180000 || versionNum >= 190000 {
		pool.Close()
		return nil, errors.New("messages: PostgreSQL 18 is required")
	}
	if _, err = pool.Exec(ctx, `SELECT user_id, max_id FROM bff_messages_received_message LIMIT 0`); err != nil {
		pool.Close()
		return nil, fmt.Errorf("messages: required PostgreSQL schema is unavailable: %w", err)
	}
	return pool, nil
}
