/*
 * Created from 'scheme.tl' by 'mtprotoc'
 *
 * Copyright (c) 2024-present, Teamgram Studio (https://teamgram.io).
 *  All rights reserved.
 *
 * Author: teamgramio (teamgram.io@gmail.com)
 */

package config

import (
	"github.com/teamgram/teamgram-server/pkg/storage/postgres"
	"github.com/zeromicro/go-zero/core/stores/kv"
	"github.com/zeromicro/go-zero/rest"
	"github.com/zeromicro/go-zero/zrpc"
)

type Config struct {
	Http                               rest.RestConf
	Session                            zrpc.RpcClientConf
	AuthsessionClient                  zrpc.RpcClientConf `json:",optional"`
	UserClient                         zrpc.RpcClientConf
	ChatClient                         zrpc.RpcClientConf
	MsgClient                          zrpc.RpcClientConf
	BffClient                          zrpc.RpcClientConf `json:",optional"`
	DfsClient                          zrpc.RpcClientConf `json:",optional"`
	Postgres                           postgres.Config
	PaymentProviderName                string    `json:",optional"`
	PaymentProviderWebhookSigningKey   string    `json:",optional"`
	PaymentProviderWebhookMaxBodyBytes int64     `json:",optional"`
	IdentityBaseURL                    string    `json:",optional"`
	IdentityTeamgramServiceToken       string    `json:",optional"`
	IdentityTeamgramInstanceID         string    `json:",optional"`
	IdentityReplayKV                   kv.KvConf `json:",optional"`
	IdentityReplayTTLSeconds           int       `json:",optional"`
	IdentityRequestTimeoutSeconds      int       `json:",optional"`
	KeyFile                            string
	KeyFingerprint                     string
}
