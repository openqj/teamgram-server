/*
 * Created from 'scheme.tl' by 'mtprotoc'
 *
 * Copyright (c) 2021-present,  Teamgram Studio (https://teamgram.io).
 *  All rights reserved.
 *
 * Author: teamgramio (teamgram.io@gmail.com)
 */

package server

import (
	"errors"
	"flag"

	"github.com/teamgram/teamgram-server/app/service/idgen/internal/config"
	"github.com/teamgram/teamgram-server/app/service/idgen/internal/server/grpc"
	"github.com/teamgram/teamgram-server/app/service/idgen/internal/svc"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/zrpc"
)

var configFile = flag.String("f", "etc/idgen.yaml", "the config file")

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
	if c.Postgres.DSN == "" {
		return errors.New("idgen: Postgres.DSN is required")
	}

	logx.Info("idgen service config loaded")
	ctx := svc.NewServiceContext(c)
	s.ctx = ctx
	s.grpcSrv = grpc.New(ctx, c.RpcServerConf)

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
	if s.ctx != nil {
		s.ctx.Dao.Close()
	}
}
