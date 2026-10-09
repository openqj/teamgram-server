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
	"github.com/teamgram/teamgram-server/app/service/biz/biz/internal/config"
	chat_helper "github.com/teamgram/teamgram-server/app/service/biz/chat"
	"github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	code_helper "github.com/teamgram/teamgram-server/app/service/biz/code"
	"github.com/teamgram/teamgram-server/app/service/biz/code/code"
	dialog_helper "github.com/teamgram/teamgram-server/app/service/biz/dialog"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	message_helper "github.com/teamgram/teamgram-server/app/service/biz/message"
	"github.com/teamgram/teamgram-server/app/service/biz/message/message"
	updates_helper "github.com/teamgram/teamgram-server/app/service/biz/updates"
	"github.com/teamgram/teamgram-server/app/service/biz/updates/updates"
	user_helper "github.com/teamgram/teamgram-server/app/service/biz/user"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/zrpc"
	"google.golang.org/grpc"
)

var configFile = flag.String("f", "etc/biz.yaml", "the config file")

type Server struct {
	grpcSrv *zrpc.RpcServer
	closers []func()
}

func New() *Server {
	return new(Server)
}

func (s *Server) Initialize() error {
	var c config.Config
	conf.MustLoad(*configFile, &c, conf.UseEnv())
	if c.Postgres.DSN == "" {
		return errors.New("biz: Postgres.DSN is required")
	}

	logx.Infof("biz config loaded")
	// ctx := svc.NewServiceContext(c)
	// s.grpcSrv = grpc.New(ctx, c.RpcServerConf)

	s.grpcSrv = zrpc.MustNewServer(c.RpcServerConf, func(grpcServer *grpc.Server) {
		// chat_helper
		chatService := chat_helper.New(
			chat_helper.Config{
				RpcServerConf: c.RpcServerConf,
				Postgres:      c.Postgres,
				Cache:         c.Cache,
				MediaClient:   c.MediaClient,
			},
			nil)
		chat.RegisterRPCChatServer(grpcServer, chatService)
		s.closers = append(s.closers, func() { chatService.GetServiceContext().Dao.Postgres.Close() })

		// code_helper
		code.RegisterRPCCodeServer(
			grpcServer,
			code_helper.New(code_helper.Config{
				RpcServerConf: c.RpcServerConf,
				Cache:         c.Cache,
				KV:            c.KV,
			}))

		// dialog_helper
		dialogService := dialog_helper.New(dialog_helper.Config{
			RpcServerConf: c.RpcServerConf,
			Postgres:      c.Postgres,
			Cache:         c.Cache,
		})
		dialog.RegisterRPCDialogServer(grpcServer, dialogService)
		s.closers = append(s.closers, func() { dialogService.GetServiceContext().Dao.Postgres.Close() })

		// message_helper
		messageService := message_helper.New(
			message_helper.Config{
				RpcServerConf:   c.RpcServerConf,
				Postgres:        c.Postgres,
				Cache:           c.Cache,
				MessageSharding: c.MessageSharding,
			},
			nil)
		message.RegisterRPCMessageServer(grpcServer, messageService)
		s.closers = append(s.closers, func() { messageService.GetServiceContext().Dao.Postgres.Close() })

		// updates_helper
		updatesService := updates_helper.New(updates_helper.Config{
			RpcServerConf: c.RpcServerConf,
			Postgres:      c.Postgres,
			KV:            c.KV,
			IdgenClient:   c.IdgenClient,
		})
		updates.RegisterRPCUpdatesServer(grpcServer, updatesService)
		s.closers = append(s.closers, func() { updatesService.GetServiceContext().Dao.Postgres.Close() })

		// user_helper
		userService := user_helper.New(user_helper.Config{
			RpcServerConf: c.RpcServerConf,
			Postgres:      c.Postgres,
			Cache:         c.Cache,
			MediaClient:   c.MediaClient,
		})
		user.RegisterRPCUserServer(grpcServer, userService)
		s.closers = append(s.closers, func() { userService.GetServiceContext().Dao.Postgres.Close() })
	})

	// logx.Must(err)
	go func() {
		s.grpcSrv.Start()
	}()
	return nil
}

func (s *Server) RunLoop() {
}

func (s *Server) Destroy() {
	if s.grpcSrv != nil {
		s.grpcSrv.Stop()
	}
	for _, closePool := range s.closers {
		closePool()
	}
	s.closers = nil
}
