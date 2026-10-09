/*
 * Created from 'scheme.tl' by 'mtprotoc'
 *
 * Copyright (c) 2021-present,  Teamgram Studio (https://teamgram.io).
 *  All rights reserved.
 *
 * Author: teamgramio (teamgram.io@gmail.com)
 */

package dao

import (
	"context"

	"github.com/bwmarrin/snowflake"
	"github.com/teamgram/teamgram-server/app/service/idgen/counter"
	"github.com/teamgram/teamgram-server/app/service/idgen/internal/config"
	"github.com/teamgram/teamgram-server/pkg/storage/postgres"
)

type Dao struct {
	*snowflake.Node
	*counter.CounterStore
}

func New(c config.Config) *Dao {
	var (
		err error
		d   = new(Dao)
	)

	d.Node, err = snowflake.NewNode(c.NodeId)
	if err != nil {
		panic(err)
	}
	pool, err := postgres.NewPool(context.Background(), c.Postgres)
	if err != nil {
		panic(err)
	}
	if err := postgres.VerifySchema(context.Background(), pool,
		`SELECT key,value,updated_at FROM idgen_counters LIMIT 0`); err != nil {
		pool.Close()
		panic(err)
	}
	d.CounterStore = counter.NewCounterStore(pool)

	return d
}

func (d *Dao) Close() {
	if d != nil && d.CounterStore != nil && d.Pool != nil {
		d.Pool.Close()
	}
}
