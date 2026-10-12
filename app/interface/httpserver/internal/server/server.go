/*
 * Created from 'scheme.tl' by 'mtprotoc'
 *
 * Copyright (c) 2024-present, Teamgram Studio (https://teamgram.io).
 *  All rights reserved.
 *
 * Author: teamgramio (teamgram.io@gmail.com)
 */

package server

import (
	"flag"

	"github.com/pkg/errors"
	"github.com/teamgram/teamgram-server/app/interface/httpserver/internal/config"
	"github.com/teamgram/teamgram-server/app/interface/httpserver/internal/server/http"
	"github.com/teamgram/teamgram-server/app/interface/httpserver/internal/svc"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/rest"
)

var configFile = flag.String("f", "etc/httpserver.yaml", "the config file")

type Server struct {
	httpSrv *rest.Server
	ctx     *svc.ServiceContext
}

func New() *Server {
	return new(Server)
}

func (s *Server) Initialize() error {
	var c config.Config
	conf.MustLoad(*configFile, &c)

	logx.Infov(c)
	ctx, err := svc.NewServiceContext(c)
	if err != nil {
		return errors.Wrap(err, "create httpserver service context")
	}

	s.httpSrv = http.New(ctx, c.Http)
	s.ctx = ctx

	return nil
}

func (s *Server) RunLoop() {
}

func (s *Server) Destroy() {
	if s.httpSrv != nil {
		s.httpSrv.Stop()
	}
	if s.ctx != nil {
		s.ctx.Close()
		s.ctx = nil
	}
}
