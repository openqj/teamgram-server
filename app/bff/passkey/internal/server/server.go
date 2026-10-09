// Copyright 2025 Teamgram Authors
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
	"strings"

	"github.com/teamgram/teamgram-server/app/bff/passkey/internal/config"
	"github.com/teamgram/teamgram-server/app/bff/passkey/internal/server/grpc"
	"github.com/teamgram/teamgram-server/app/bff/passkey/internal/svc"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/zrpc"
)

var configFile = flag.String("f", "etc/passkey.yaml", "the config file")

type Server struct {
	grpcSrv *zrpc.RpcServer
	ctx     *svc.ServiceContext
}

func New() *Server {
	return new(Server)
}

func (s *Server) Initialize() error {
	var c config.Config
	conf.MustLoad(*configFile, &c, conf.UseEnv())
	if strings.TrimSpace(c.PostgresDSN) == "" {
		return errors.New("passkey: PostgresDSN is required")
	}

	logx.Infof("passkey config loaded")
	ctx := svc.NewServiceContext(c)
	s.ctx = ctx
	s.grpcSrv = grpc.New(ctx, c.RpcServerConf)

	go func() {
		go s.grpcSrv.Start()
	}()
	return nil
}

func (s *Server) RunLoop() {
}

func (s *Server) Destroy() {
	if s.grpcSrv != nil {
		s.grpcSrv.Stop()
	}
	if s.ctx != nil && s.ctx.Dao != nil {
		s.ctx.Dao.Close()
	}
}
