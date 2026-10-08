// Copyright 2022 Teamgram Authors
//  All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// Author: teamgramio (teamgram.io@gmail.com)
//

package dao

import (
	"errors"

	"github.com/teamgram/marmota/pkg/net/rpcx"
	"github.com/teamgram/marmota/pkg/stores/sqlc"
	"github.com/teamgram/teamgram-server/app/service/biz/user/internal/config"
	media_client "github.com/teamgram/teamgram-server/app/service/media/client"
)

// Dao dao.
type Dao struct {
	*Mysql
	*Postgres
	sqlc.CachedConn
	media_client.MediaClient
}

// New new a dao and return.
func New(c config.Config) *Dao {
	if c.Postgres.DSN == "" {
		panic(errors.New("biz/user: Postgres.DSN is required"))
	}
	dao := &Dao{MediaClient: media_client.NewMediaClient(rpcx.GetCachedRpcClient(c.MediaClient))}
	pg, err := NewPostgres(c.Postgres)
	if err != nil {
		panic(err)
	}
	dao.Postgres = pg
	return dao
}
