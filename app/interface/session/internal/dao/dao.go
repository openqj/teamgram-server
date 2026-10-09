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
	"errors"
	"strings"
	"sync"

	"github.com/teamgram/marmota/pkg/cache"
	"github.com/teamgram/marmota/pkg/net/ip"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/apifull/layer229"
	bff_proxy_client "github.com/teamgram/teamgram-server/app/bff/bff/client"
	"github.com/teamgram/teamgram-server/app/interface/session/internal/config"
	authsession_client "github.com/teamgram/teamgram-server/app/service/authsession/client"
	status_client "github.com/teamgram/teamgram-server/app/service/status/client"

	"github.com/zeromicro/go-zero/zrpc"
)

type Dao struct {
	cache *cache.LRUCache
	authsession_client.AuthsessionClient
	status_client.StatusClient
	*bff_proxy_client.BFFProxyClient
	gateMu           sync.RWMutex
	eGateServers     map[string]*Gateway
	MyServerId       string
	UseStreamGateway bool
	streamingGateway *StreamingGateway
	*RpcShardingManager
}

func (d *Dao) Close() error {
	if d == nil {
		return nil
	}
	if d.streamingGateway != nil {
		d.streamingGateway.Close()
	}
	return layer229.Close()
}

// New constructs the session DAO and opens the process-owned Layer 229
// PostgreSQL store. Startup must fail before serving RPCs when the DSN is
// missing or the database cannot be reached.
func New(c config.Config) (*Dao, error) {
	if strings.TrimSpace(c.PostgresDSN) == "" {
		return nil, errors.New("session: PostgresDSN is required")
	}
	myServerId := ip.FigureOutListenOn(c.ListenOn)
	d := &Dao{
		cache:              cache.NewLRUCache(1024 * 1024 * 1024),
		AuthsessionClient:  authsession_client.NewAuthsessionClient(zrpc.MustNewClient(c.AuthSession)),
		BFFProxyClient:     bff_proxy_client.NewBFFProxyClients(c.BFFProxyClients.Clients, c.BFFProxyClients.IDMap),
		StatusClient:       status_client.NewStatusClient(zrpc.MustNewClient(c.StatusClient)),
		eGateServers:       make(map[string]*Gateway),
		MyServerId:         myServerId,
		UseStreamGateway:   c.UseStreamGateway,
		RpcShardingManager: NewRpcShardingManager(myServerId, c.Etcd),
	}

	if c.UseStreamGateway {
		d.streamingGateway = NewStreamingGateway()
	}

	d.watchGateway(c.GatewayClient)
	if err := wireLayer229(c, d.AuthsessionClient); err != nil {
		return nil, err
	}

	return d, nil
}

func (d *Dao) InvokeContext(ctx context.Context, rpcMetaData *metadata.RpcMetadata, object mtproto.TLObject) (mtproto.TLObject, error) {
	if reply, ok, err := layer229.Dispatch(ctx, rpcMetaData, object); ok {
		return reply, err
	}
	return d.BFFProxyClient.InvokeContext(ctx, rpcMetaData, object)
}
