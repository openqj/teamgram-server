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
	"flag"
	"time"

	"github.com/teamgram/teamgram-server/app/interface/session/internal/config"
	"github.com/teamgram/teamgram-server/app/interface/session/internal/server/grpc"
	"github.com/teamgram/teamgram-server/app/interface/session/internal/svc"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/zrpc"
)

var configFile = flag.String("f", "etc/session.yaml", "the config file")

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

	logx.Infof("session config loaded")

	ctx, err := svc.NewServiceContext(c)
	if err != nil {
		return err
	}
	s.svcCtx = ctx
	s.grpcSrv = grpc.New(s.svcCtx, c.RpcServerConf)

	go func() {
		s.grpcSrv.Start()
	}()
	return nil
}

func (s *Server) RunLoop() {
}

func (s *Server) Destroy() {
	// 优雅排空：先等待进行中的 RPC 请求处理完毕（最多 30s），再停止 gRPC 服务
	if s.svcCtx == nil {
		return
	}
	logx.Infof("session server destroying, draining auth wrappers...")
	if s.svcCtx.MainAuthMgr != nil {
		s.svcCtx.MainAuthMgr.Drain(30 * time.Second)
	}

	if s.grpcSrv != nil {
		s.grpcSrv.Stop()
	}
	if s.svcCtx != nil && s.svcCtx.Dao != nil {
		_ = s.svcCtx.Dao.Close()
	}
}
