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

package server

import (
	"errors"
	"flag"

	inbox_helper "github.com/teamgram/teamgram-server/app/messenger/msg/inbox"
	"github.com/teamgram/teamgram-server/app/messenger/msg/internal/config"
	msg_helper "github.com/teamgram/teamgram-server/app/messenger/msg/msg"
	"github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
	"github.com/teamgram/teamgram-server/pkg/mqconsumer"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/zrpc"
	"google.golang.org/grpc"
)

var configFile = flag.String("f", "etc/msg.yaml", "the config file")

type Server struct {
	grpcSrv    *zrpc.RpcServer
	mq         *mqconsumer.Consumer
	closeMsg   func()
	closeInbox func()
}

func New() *Server {
	return new(Server)
}

func (s *Server) Initialize() error {
	var c config.Config
	conf.MustLoad(*configFile, &c, conf.UseEnv())
	if c.Postgres.DSN == "" {
		return errors.New("msg: Postgres.DSN is required")
	}

	logx.Infof("messenger msg config loaded")

	msgService, closeMsg := msg_helper.NewWithClose(
		msg_helper.Config{
			RpcServerConf:   c.RpcServerConf,
			Postgres:        c.Postgres,
			Cache:           c.Cache,
			KV:              c.KV,
			IdgenClient:     c.IdgenClient,
			UserClient:      c.BizServiceClient,
			ChatClient:      c.BizServiceClient,
			SyncClient:      c.SyncClient,
			InboxClient:     c.InboxClient,
			DialogClient:    c.BizServiceClient,
			MessageSharding: c.MessageSharding,
			Redis2:          c.Redis2,
		}, nil)
	s.closeMsg = closeMsg
	s.grpcSrv = zrpc.MustNewServer(c.RpcServerConf, func(grpcServer *grpc.Server) {
		// msg_helper
		msg.RegisterRPCMsgServer(grpcServer, msgService)
	})

	go func() {
		s.grpcSrv.Start()
	}()

	s.mq, s.closeInbox = inbox_helper.NewWithContext(inbox_helper.Config{
		RpcServerConf:   c.RpcServerConf,
		InboxConsumer:   c.InboxConsumer,
		Postgres:        c.Postgres,
		Cache:           c.Cache,
		KV:              c.KV,
		IdgenClient:     c.IdgenClient,
		UserClient:      c.BizServiceClient,
		ChatClient:      c.BizServiceClient,
		SyncClient:      c.SyncClient,
		BotSyncClient:   c.BotSyncClient,
		DialogClient:    c.BizServiceClient,
		MessageSharding: c.MessageSharding,
	})

	go func() {
		s.mq.Start()
	}()

	return nil
}

func (s *Server) RunLoop() {
}

func (s *Server) Destroy() {
	if s.grpcSrv != nil {
		s.grpcSrv.Stop()
	}
	if s.mq != nil {
		s.mq.Stop()
	}
	if s.closeMsg != nil {
		s.closeMsg()
	}
	if s.closeInbox != nil {
		s.closeInbox()
	}
}
