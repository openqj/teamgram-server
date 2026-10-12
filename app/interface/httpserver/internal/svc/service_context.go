/*
 * Created from 'scheme.tl' by 'mtprotoc'
 *
 * Copyright (c) 2024-present, Teamgram Studio (https://teamgram.io).
 *  All rights reserved.
 *
 * Author: teamgramio (teamgram.io@gmail.com)
 */

package svc

import (
	"context"
	"errors"
	"strings"

	"github.com/teamgram/marmota/pkg/net/rpcx"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/botapi/store"
	"github.com/teamgram/teamgram-server/app/interface/httpserver/internal/config"
	"github.com/teamgram/teamgram-server/app/interface/httpserver/internal/dao"
	msg_client "github.com/teamgram/teamgram-server/app/messenger/msg/msg/client"
	chat_client "github.com/teamgram/teamgram-server/app/service/biz/chat/client"
	user_client "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	dfs_client "github.com/teamgram/teamgram-server/app/service/dfs/client"
)

type ServiceContext struct {
	Config config.Config
	*dao.Dao
	BotAPIStore        *store.Store
	UserClient         user_client.UserClient
	BotRegistryClient  user_client.BotRegistryClient
	ChatClient         chat_client.ChatClient
	MsgClient          msg_client.MsgClient
	InlineBotClient    mtproto.RPCInlineBotClient
	ChatInvitesClient  mtproto.RPCChatInvitesClient
	ForumsClient       mtproto.RPCForumsClient
	DialogsClient      mtproto.RPCDialogsClient
	GamesClient        mtproto.RPCGamesClient
	BusinessBotsClient mtproto.RPCBusinessConnectedBotsClient
	MiniBotAppsClient  mtproto.RPCMiniBotAppsClient
	PaymentsClient     mtproto.RPCPaymentsClient
	MessagesClient     mtproto.RPCMessagesClient
	ReactionsClient    mtproto.RPCReactionsClient
	ChatsClient        mtproto.RPCChatsClient
	ChannelsClient     mtproto.RPCChannelsClient
	DfsClient          dfs_client.DfsClient
}

func NewServiceContext(c config.Config) (*ServiceContext, error) {
	if strings.TrimSpace(c.Postgres.DSN) == "" {
		return nil, errors.New("httpserver: Postgres.DSN is required")
	}
	botAPIStore, err := store.New(context.Background(), c.Postgres)
	if err != nil {
		return nil, err
	}
	service := &ServiceContext{
		Config:            c,
		Dao:               dao.New(c),
		BotAPIStore:       botAPIStore,
		UserClient:        user_client.NewUserClient(rpcx.GetCachedRpcClient(c.UserClient)),
		BotRegistryClient: user_client.NewBotRegistryClient(rpcx.GetCachedRpcClient(c.UserClient)),
		ChatClient:        chat_client.NewChatClient(rpcx.GetCachedRpcClient(c.ChatClient)),
		MsgClient:         msg_client.NewMsgClient(rpcx.GetCachedRpcClient(c.MsgClient)),
	}
	if c.BffClient.Etcd.Key != "" || len(c.BffClient.Endpoints) > 0 {
		bffConn := rpcx.GetCachedRpcClient(c.BffClient).Conn()
		service.InlineBotClient = mtproto.NewRPCInlineBotClient(bffConn)
		service.ChatInvitesClient = mtproto.NewRPCChatInvitesClient(bffConn)
		service.ForumsClient = mtproto.NewRPCForumsClient(bffConn)
		service.DialogsClient = mtproto.NewRPCDialogsClient(bffConn)
		service.GamesClient = mtproto.NewRPCGamesClient(bffConn)
		service.BusinessBotsClient = mtproto.NewRPCBusinessConnectedBotsClient(bffConn)
		service.MiniBotAppsClient = mtproto.NewRPCMiniBotAppsClient(bffConn)
		service.PaymentsClient = mtproto.NewRPCPaymentsClient(bffConn)
		service.MessagesClient = mtproto.NewRPCMessagesClient(bffConn)
		service.ReactionsClient = mtproto.NewRPCReactionsClient(bffConn)
		service.ChatsClient = mtproto.NewRPCChatsClient(bffConn)
		service.ChannelsClient = mtproto.NewRPCChannelsClient(bffConn)
	}
	if c.DfsClient.Etcd.Key != "" || len(c.DfsClient.Endpoints) > 0 {
		service.DfsClient = dfs_client.NewDfsClient(rpcx.GetCachedRpcClient(c.DfsClient))
	}
	return service, nil
}

func (s *ServiceContext) Close() {
	if s != nil && s.BotAPIStore != nil {
		s.BotAPIStore.Close()
	}
}
