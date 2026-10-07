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
	passkeyhelper "github.com/teamgram/teamgram-server/app/bff/passkey"
	qrcodehelper "github.com/teamgram/teamgram-server/app/bff/qrcode"
	"github.com/teamgram/teamgram-server/pkg/code/conf"
	"github.com/zeromicro/go-zero/core/stores/kv"
	"github.com/zeromicro/go-zero/zrpc"
)

type Config struct {
	zrpc.RpcServerConf
	DcId                          int32   `json:",optional"`
	KnownDcIds                    []int32 `json:",optional"`
	KV                            kv.KvConf
	Code                          *conf.SmsVerifyCodeConfig
	BizServiceClient              zrpc.RpcClientConf
	AuthSessionClient             zrpc.RpcClientConf
	MediaClient                   zrpc.RpcClientConf
	IdgenClient                   zrpc.RpcClientConf
	MsgClient                     zrpc.RpcClientConf
	SyncClient                    *kafka.KafkaProducerConf
	DfsClient                     zrpc.RpcClientConf
	StatusClient                  zrpc.RpcClientConf
	MysqlDSN                      string                       `json:",optional"`
	PaymentProviderEndpoint       string                       `json:",optional"`
	PaymentProviderKey            string                       `json:",optional"`
	PaymentProviderSigningKey     string                       `json:",optional"`
	PaymentProviderTimeoutSeconds int                          `json:",optional"`
	TurnHost                      string                       `json:",optional"`
	TurnPort                      int32                        `json:",optional"`
	TurnUsername                  string                       `json:",optional"`
	TurnPassword                  string                       `json:",optional"`
	TurnSharedSecret              string                       `json:",optional"`
	TurnCredentialTTLSeconds      int                          `json:",optional"`
	Passkey                       passkeyhelper.ProviderConfig `json:",optional"`
	QrCode                        qrcodehelper.ProviderConfig  `json:",optional"`
	SearchPostsFlood              SearchPostsFloodConfig       `json:",optional"`
	SignInServiceNotification     []conf.MessageEntityConfig   `json:",optional"`
	SignInMessage                 []conf.MessageEntityConfig   `json:",optional"`
}

type SearchPostsFloodConfig struct {
	TotalDaily int `json:",optional"`
}
