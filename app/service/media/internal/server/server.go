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
	"strings"

	"github.com/teamgram/teamgram-server/app/service/media/internal/config"
	"github.com/teamgram/teamgram-server/app/service/media/internal/server/grpc"
	"github.com/teamgram/teamgram-server/app/service/media/internal/svc"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/zrpc"
)

var configFile = flag.String("f", "etc/media.yaml", "the config file")

type Server struct {
	grpcSrv *zrpc.RpcServer
	svcCtx  *svc.ServiceContext
}

func New() *Server {
	return new(Server)
}

func (s *Server) Initialize() error {
	var c config.Config
	conf.MustLoad(*configFile, &c, conf.UseEnv())
	if strings.TrimSpace(c.Postgres.DSN) == "" {
		return errors.New("media: Postgres.DSN is required")
	}

	logx.Infof("media config loaded")
	ctx := svc.NewServiceContext(c)
	s.svcCtx = ctx
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
	if s.svcCtx != nil {
		s.svcCtx.Close()
	}
}
