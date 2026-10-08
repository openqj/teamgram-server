/*
 * WARNING! All changes made in this file will be lost!
 * Created from 'scheme.tl' by 'mtprotoc'
 *
 * Copyright (c) 2021-present,  Teamgram Studio (https://teamgram.io).
 *  All rights reserved.
 *
 * Author: teamgramio (teamgram.io@gmail.com)
 */

package msg_helper

import (
	"github.com/teamgram/teamgram-server/app/messenger/msg/msg/internal/config"
	"github.com/teamgram/teamgram-server/app/messenger/msg/msg/internal/server/grpc/service"
	"github.com/teamgram/teamgram-server/app/messenger/msg/msg/internal/svc"
	"github.com/teamgram/teamgram-server/app/messenger/msg/msg/plugin"
)

type (
	Config = config.Config
)

func New(c Config, plugin plugin.MsgPlugin) *service.Service {
	return service.New(svc.NewServiceContext(c, plugin))
}

// NewWithClose constructs a message service and returns a lifecycle callback
// for the PostgreSQL pool owned by its service context.
func NewWithClose(c Config, plugin plugin.MsgPlugin) (*service.Service, func()) {
	ctx := svc.NewServiceContext(c, plugin)
	closeContext := func() {
		if ctx.Dao != nil && ctx.Dao.Postgres != nil {
			ctx.Dao.Postgres.Close()
		}
	}
	return service.New(ctx), closeContext
}
