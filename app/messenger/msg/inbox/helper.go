/*
 * WARNING! All changes made in this file will be lost!
 * Created from 'scheme.tl' by 'mtprotoc'
 *
 * Copyright (c) 2021-present,  Teamgram Studio (https://teamgram.io).
 *  All rights reserved.
 *
 * Author: teamgramio (teamgram.io@gmail.com)
 */

package inbox_helper

import (
	kafka "github.com/teamgram/marmota/pkg/mq"
	"github.com/teamgram/teamgram-server/app/messenger/msg/inbox/internal/config"
	"github.com/teamgram/teamgram-server/app/messenger/msg/inbox/internal/core"
	"github.com/teamgram/teamgram-server/app/messenger/msg/inbox/internal/server/mq"
	"github.com/teamgram/teamgram-server/app/messenger/msg/inbox/internal/svc"
)

type (
	Config         = config.Config
	ServiceContext = svc.ServiceContext
	InboxCore      = core.InboxCore
)

var (
	NewServiceContext = svc.NewServiceContext
	NewInboxCore      = core.New
)

func New(c Config) *kafka.ConsumerGroup {
	consumer, _ := NewWithContext(c)
	return consumer
}

// NewWithContext constructs the inbox consumer and returns a lifecycle
// callback so callers that own the process can close the PostgreSQL pool
// during shutdown. New keeps the generated helper's original API for other
// callers.
func NewWithContext(c Config) (*kafka.ConsumerGroup, func()) {
	ctx := svc.NewServiceContext(c)
	closeContext := func() {
		if ctx.Dao != nil && ctx.Dao.Postgres != nil {
			ctx.Dao.Postgres.Close()
		}
	}
	return mq.New(ctx, c.InboxConsumer), closeContext
}
