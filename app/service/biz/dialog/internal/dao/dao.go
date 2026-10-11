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
	"errors"
	"strings"

	"github.com/teamgram/teamgram-server/app/service/biz/dialog/internal/config"
)

// Dao dao.
type Dao struct {
	*Postgres
}

// New new a dao and return.
func New(c config.Config) (dao *Dao) {
	if strings.TrimSpace(c.Postgres.DSN) == "" {
		panic(errors.New("biz/dialog: Postgres.DSN is required"))
	}
	dao = &Dao{}
	pg, err := NewPostgres(c.Postgres)
	if err != nil {
		panic(err)
	}
	dao.Postgres = pg
	return dao
}
