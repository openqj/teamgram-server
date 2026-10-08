/*
 * Created from 'scheme.tl' by 'mtprotoc'
 *
 * Copyright (c) 2021-present,  Teamgram Studio (https://teamgram.io).
 *  All rights reserved.
 *
 * Author: teamgramio (teamgram.io@gmail.com)
 */

package svc

import (
	"github.com/teamgram/teamgram-server/app/messenger/sync/internal/config"
	"github.com/teamgram/teamgram-server/app/messenger/sync/internal/dao"
)

type ServiceContext struct {
	Config config.Config
	*dao.Dao
}

func NewServiceContext(c config.Config) (*ServiceContext, error) {
	d, err := dao.New(c)
	if err != nil {
		return nil, err
	}
	return &ServiceContext{
		Config: c,
		Dao:    d,
	}, nil
}
