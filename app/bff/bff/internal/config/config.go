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
	configuration_helper "github.com/teamgram/teamgram-server/app/bff/configuration"
	passkeyhelper "github.com/teamgram/teamgram-server/app/bff/passkey"
	qrcodehelper "github.com/teamgram/teamgram-server/app/bff/qrcode"
	"github.com/teamgram/teamgram-server/pkg/code/conf"
	"github.com/zeromicro/go-zero/core/stores/kv"
	"github.com/zeromicro/go-zero/zrpc"
)

type Config struct {
	zrpc.RpcServerConf
	DcId                                      int32   `json:",optional"`
	KnownDcIds                                []int32 `json:",optional"`
	KV                                        kv.KvConf
	Code                                      *conf.SmsVerifyCodeConfig
	BizServiceClient                          zrpc.RpcClientConf
	AuthSessionClient                         zrpc.RpcClientConf
	MediaClient                               zrpc.RpcClientConf
	IdgenClient                               zrpc.RpcClientConf
	MsgClient                                 zrpc.RpcClientConf
	SyncClient                                *kafka.KafkaProducerConf
	DfsClient                                 zrpc.RpcClientConf
	StatusClient                              zrpc.RpcClientConf
	PostgresDSN                               string                          `json:",optional"`
	PaymentProviderEndpoint                   string                          `json:",optional"`
	PaymentProviderName                       string                          `json:",optional"`
	PaymentProviderKey                        string                          `json:",optional"`
	PaymentProviderSigningKey                 string                          `json:",optional"`
	PaymentProviderTimeoutSeconds             int                             `json:",optional"`
	PaymentReconciliationIntervalSeconds      int                             `json:",optional"`
	PaymentReconciliationBatchSize            int                             `json:",optional"`
	Sub2APIInternalBaseURL                    string                          `json:",optional"`
	Sub2APIServiceToken                       string                          `json:",optional"`
	Sub2APITimeoutSeconds                     int                             `json:",optional"`
	IdentityBaseURL                           string                          `json:",optional"`
	IdentityTeamgramServiceToken              string                          `json:",optional"`
	IdentityTeamgramInstanceID                string                          `json:",optional"`
	AIProviderEndpoint                        string                          `json:",optional"`
	AIProviderKey                             string                          `json:",optional"`
	AIProviderSigningKey                      string                          `json:",optional"`
	AIProviderModel                           string                          `json:",optional"`
	AIProviderTimeoutSeconds                  int                             `json:",optional"`
	BusinessProviderEndpoint                  string                          `json:",optional"`
	BusinessProviderKey                       string                          `json:",optional"`
	BusinessProviderSigningKey                string                          `json:",optional"`
	BusinessProviderTimeoutSeconds            int                             `json:",optional"`
	SponsoredProviderEndpoint                 string                          `json:",optional"`
	SponsoredProviderKey                      string                          `json:",optional"`
	SponsoredProviderSigningKey               string                          `json:",optional"`
	SponsoredProviderTimeoutSeconds           int                             `json:",optional"`
	BotConnectionProviderEndpoint             string                          `json:",optional"`
	BotConnectionProviderKey                  string                          `json:",optional"`
	BotConnectionProviderSigningKey           string                          `json:",optional"`
	BotConnectionProviderTimeoutSeconds       int                             `json:",optional"`
	AuthProviderEndpoint                      string                          `json:",optional"`
	AuthProviderKey                           string                          `json:",optional"`
	AuthProviderSigningKey                    string                          `json:",optional"`
	AuthProviderTimeoutSeconds                int                             `json:",optional"`
	PassportProviderEndpoint                  string                          `json:",optional"`
	PassportProviderKey                       string                          `json:",optional"`
	PassportProviderSigningKey                string                          `json:",optional"`
	PassportProviderTimeoutSeconds            int                             `json:",optional"`
	ProviderFixtureMode                       bool                            `json:",optional"`
	UserChannelProfilesProviderEndpoint       string                          `json:",optional"`
	UserChannelProfilesProviderKey            string                          `json:",optional"`
	UserChannelProfilesProviderSigningKey     string                          `json:",optional"`
	UserChannelProfilesProviderTimeoutSeconds int                             `json:",optional"`
	CdnProviderEndpoint                       string                          `json:",optional"`
	CdnProviderKey                            string                          `json:",optional"`
	CdnProviderSigningKey                     string                          `json:",optional"`
	CdnProviderTimeoutSeconds                 int                             `json:",optional"`
	GroupCallMediaEndpoint                    string                          `json:",optional"`
	GroupCallMediaAPIKey                      string                          `json:",optional"`
	GroupCallMediaSigningKey                  string                          `json:",optional"`
	GroupCallMediaRTMPHost                    string                          `json:",optional"`
	GroupCallMediaTimeoutSeconds              int                             `json:",optional"`
	TurnHost                                  string                          `json:",optional"`
	TurnPort                                  int32                           `json:",optional"`
	TurnUsername                              string                          `json:",optional"`
	TurnPassword                              string                          `json:",optional"`
	TurnSharedSecret                          string                          `json:",optional"`
	TurnCredentialTTLSeconds                  int                             `json:",optional"`
	Passkey                                   passkeyhelper.ProviderConfig    `json:",optional"`
	QrCode                                    qrcodehelper.ProviderConfig     `json:",optional"`
	SearchPostsFlood                          SearchPostsFloodConfig          `json:",optional"`
	SignInServiceNotification                 []conf.MessageEntityConfig      `json:",optional"`
	SignInMessage                             []conf.MessageEntityConfig      `json:",optional"`
	Help                                      configuration_helper.HelpConfig `json:",optional"`
}

type SearchPostsFloodConfig struct {
	TotalDaily int `json:",optional"`
}
