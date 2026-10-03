// Copyright 2026 Teamgram Authors
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

package svc

import (
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/config"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/dao"
	verification "github.com/teamgram/teamgram-server/pkg/code"
	"github.com/zeromicro/go-zero/core/stores/kv"
)

type ServiceContext struct {
	Config config.Config
	*dao.Dao
	Challenges *verification.ChallengeService
}

func NewServiceContext(c config.Config) *ServiceContext {
	var challenges *verification.ChallengeService
	if len(c.KV) > 0 {
		store := verification.NewRedisChallengeStore(kv.NewStore(c.KV))
		challenges = verification.NewChallengeService(
			store,
			verification.ChallengeSettingsFromConfig(c.Code),
			verification.NewSMSProvider(c.Code, nil),
			verification.NewEmailProvider(c.Code),
		)
	}
	return &ServiceContext{
		Config:     c,
		Dao:        dao.New(c),
		Challenges: challenges,
	}
}
