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

	"github.com/teamgram/teamgram-server/app/messenger/sync/internal/config"
	"github.com/teamgram/teamgram-server/app/messenger/sync/internal/server/mq"
	"github.com/teamgram/teamgram-server/app/messenger/sync/internal/svc"
	"github.com/teamgram/teamgram-server/pkg/mqconsumer"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/logx"
)

var configFile = flag.String("f", "etc/sync.yaml", "the config file")

type Server struct {
	// grpcSrv *zrpc.RpcServer
	mq  *mqconsumer.Consumer
	ctx *svc.ServiceContext
}

func New() *Server {
	return new(Server)
}

func (s *Server) Initialize() error {
	var c config.Config
	conf.MustLoad(*configFile, &c, conf.UseEnv())
	if strings.TrimSpace(c.Postgres.DSN) == "" {
		return errors.New("messenger sync: Postgres.DSN is required")
	}
	logx.Infof("messenger sync config loaded")

	if err := logx.SetUp(c.Log); err != nil {
		return err
	}

	ctx, err := svc.NewServiceContext(c)
	if err != nil {
		return err
	}
	s.ctx = ctx
	// s.grpcSrv = grpc.New(ctx, c.RpcServerConf)
	s.mq, err = mq.New(ctx, c.SyncConsumer)
	if err != nil {
		ctx.Dao.Close()
		return err
	}

	// go s.grpcSrv.Start()
	go s.mq.Start()

	return nil
}

func (s *Server) RunLoop() {
}

func (s *Server) Destroy() {
	// s.grpcSrv.Stop()
	if s.mq != nil {
		s.mq.Stop()
	}
	if s.ctx != nil && s.ctx.Dao != nil {
		s.ctx.Dao.Close()
	}
}
