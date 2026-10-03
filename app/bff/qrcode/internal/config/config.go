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

package config

import (
	kafka "github.com/teamgram/marmota/pkg/mq"
	"github.com/zeromicro/go-zero/core/stores/kv"
	"github.com/zeromicro/go-zero/zrpc"
)

type Config struct {
	zrpc.RpcServerConf
	DcId              int32   `json:",optional"`
	KnownDcIds        []int32 `json:",optional"`
	KV                kv.KvConf
	MysqlDSN          string `json:",optional"`
	UserClient        zrpc.RpcClientConf
	AuthSessionClient zrpc.RpcClientConf
	SyncClient        *kafka.KafkaProducerConf
}

// SupportsDc reports whether this instance is configured to serve a DC. With
// no KnownDcIds configured, only the local DC is supported.
func (c Config) SupportsDc(dcID int32) bool {
	if dcID <= 0 || c.DcId <= 0 {
		return false
	}
	if dcID == c.DcId {
		return true
	}
	for _, known := range c.KnownDcIds {
		if known == dcID {
			return true
		}
	}
	return false
}
