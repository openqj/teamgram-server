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
	"github.com/teamgram/teamgram-server/app/bff/apifull/paymentrecovery"
	usernames_client "github.com/teamgram/teamgram-server/app/bff/usernames/client"
	"github.com/teamgram/teamgram-server/app/botapi/store"
	"github.com/teamgram/teamgram-server/app/interface/httpserver/internal/config"
	"github.com/teamgram/teamgram-server/app/interface/httpserver/internal/dao"
	msg_client "github.com/teamgram/teamgram-server/app/messenger/msg/msg/client"
	authsession_client "github.com/teamgram/teamgram-server/app/service/authsession/client"
	chat_client "github.com/teamgram/teamgram-server/app/service/biz/chat/client"
	user_client "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	dfs_client "github.com/teamgram/teamgram-server/app/service/dfs/client"
	media_client "github.com/teamgram/teamgram-server/app/service/media/client"
)

type ServiceContext struct {
	Config config.Config
	*dao.Dao
	BotAPIStore                *store.Store
	AuthsessionClient          authsession_client.AuthsessionClient
	AIMemoryTokenExchanger     AIMemoryTokenExchanger
	UserClient                 user_client.UserClient
	BotRegistryClient          user_client.BotRegistryClient
	ChatClient                 chat_client.ChatClient
	MsgClient                  msg_client.MsgClient
	InlineBotClient            mtproto.RPCInlineBotClient
	PreparedInlineClient       mtproto.RPCPreparedInlineMessagesClient
	EphemeralClient            mtproto.RPCEphemeralClient
	UserChannelProfilesClient  mtproto.RPCUserChannelProfilesClient
	UsernamesClient            usernames_client.UsernamesClient
	BotAdminRightClient        mtproto.RPCBotAdminRightClient
	ChatInvitesClient          mtproto.RPCChatInvitesClient
	ForumsClient               mtproto.RPCForumsClient
	DialogsClient              mtproto.RPCDialogsClient
	GamesClient                mtproto.RPCGamesClient
	BusinessBotsClient         mtproto.RPCBusinessConnectedBotsClient
	MiniBotAppsClient          mtproto.RPCMiniBotAppsClient
	PaymentsClient             mtproto.RPCPaymentsClient
	StarsClient                mtproto.RPCStarsClient
	StarSubscriptionsClient    mtproto.RPCStarSubscriptionsClient
	GiftsClient                mtproto.RPCGiftsClient
	BotsClient                 mtproto.RPCBotsClient
	BotVerificationIconsClient mtproto.RPCBotVerificationIconsClient
	BoostsClient               mtproto.RPCBoostsClient
	PassportClient             mtproto.RPCPassportClient
	EmojiStatusClient          mtproto.RPCEmojiStatusClient
	MessagesClient             mtproto.RPCMessagesClient
	FilesClient                mtproto.RPCFilesClient
	TodoListsClient            mtproto.RPCTodoListsClient
	CustomEmojisClient         mtproto.RPCCustomEmojisClient
	StickersClient             mtproto.RPCStickersClient
	ReactionsClient            mtproto.RPCReactionsClient
	ChatsClient                mtproto.RPCChatsClient
	ChannelsClient             mtproto.RPCChannelsClient
	StoriesClient              mtproto.RPCStoriesClient
	SuggestedPostsClient       mtproto.RPCSuggestedPostsClient
	DfsClient                  dfs_client.DfsClient
	MediaClient                media_client.MediaClient
}

func NewServiceContext(c config.Config) (*ServiceContext, error) {
	if strings.TrimSpace(c.Postgres.DSN) == "" {
		return nil, errors.New("httpserver: Postgres.DSN is required")
	}
	botAPIStore, err := store.New(context.Background(), c.Postgres)
	if err != nil {
		return nil, err
	}
	if err = paymentrecovery.OpenPostgresReadOnly(c.Postgres.DSN); err != nil {
		botAPIStore.Close()
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
	if strings.TrimSpace(c.IdentityBaseURL) != "" || strings.TrimSpace(c.IdentityInternalServiceToken) != "" {
		if strings.TrimSpace(c.IdentityBaseURL) == "" || strings.TrimSpace(c.IdentityInternalServiceToken) == "" {
			service.Close()
			return nil, errors.New("httpserver: IdentityBaseURL and IdentityInternalServiceToken must be configured together")
		}
		if c.AuthsessionClient.Etcd.Key == "" && len(c.AuthsessionClient.Endpoints) == 0 {
			service.Close()
			return nil, errors.New("httpserver: AuthsessionClient is required when Identity integration is enabled")
		}
		service.AuthsessionClient = authsession_client.NewAuthsessionClient(rpcx.GetCachedRpcClient(c.AuthsessionClient))
		exchanger, err := NewIdentityTokenExchanger(c.IdentityBaseURL, c.IdentityInternalServiceToken, c.IdentityRequestTimeoutSeconds)
		if err != nil {
			service.Close()
			return nil, err
		}
		service.AIMemoryTokenExchanger = exchanger
	}
	if c.BffClient.Etcd.Key != "" || len(c.BffClient.Endpoints) > 0 {
		bffConn := rpcx.GetCachedRpcClient(c.BffClient).Conn()
		service.InlineBotClient = mtproto.NewRPCInlineBotClient(bffConn)
		service.PreparedInlineClient = mtproto.NewRPCPreparedInlineMessagesClient(bffConn)
		service.EphemeralClient = mtproto.NewRPCEphemeralClient(bffConn)
		service.UserChannelProfilesClient = mtproto.NewRPCUserChannelProfilesClient(bffConn)
		service.UsernamesClient = usernames_client.NewUsernamesClient(rpcx.GetCachedRpcClient(c.BffClient))
		service.BotAdminRightClient = mtproto.NewRPCBotAdminRightClient(bffConn)
		service.ChatInvitesClient = mtproto.NewRPCChatInvitesClient(bffConn)
		service.ForumsClient = mtproto.NewRPCForumsClient(bffConn)
		service.DialogsClient = mtproto.NewRPCDialogsClient(bffConn)
		service.GamesClient = mtproto.NewRPCGamesClient(bffConn)
		service.BusinessBotsClient = mtproto.NewRPCBusinessConnectedBotsClient(bffConn)
		service.MiniBotAppsClient = mtproto.NewRPCMiniBotAppsClient(bffConn)
		service.PaymentsClient = mtproto.NewRPCPaymentsClient(bffConn)
		service.StarsClient = mtproto.NewRPCStarsClient(bffConn)
		service.StarSubscriptionsClient = mtproto.NewRPCStarSubscriptionsClient(bffConn)
		service.GiftsClient = mtproto.NewRPCGiftsClient(bffConn)
		service.BotsClient = mtproto.NewRPCBotsClient(bffConn)
		service.BotVerificationIconsClient = mtproto.NewRPCBotVerificationIconsClient(bffConn)
		service.BoostsClient = mtproto.NewRPCBoostsClient(bffConn)
		service.PassportClient = mtproto.NewRPCPassportClient(bffConn)
		service.EmojiStatusClient = mtproto.NewRPCEmojiStatusClient(bffConn)
		service.MessagesClient = mtproto.NewRPCMessagesClient(bffConn)
		service.FilesClient = mtproto.NewRPCFilesClient(bffConn)
		service.TodoListsClient = mtproto.NewRPCTodoListsClient(bffConn)
		service.CustomEmojisClient = mtproto.NewRPCCustomEmojisClient(bffConn)
		service.StickersClient = mtproto.NewRPCStickersClient(bffConn)
		service.ReactionsClient = mtproto.NewRPCReactionsClient(bffConn)
		service.ChatsClient = mtproto.NewRPCChatsClient(bffConn)
		service.ChannelsClient = mtproto.NewRPCChannelsClient(bffConn)
		service.StoriesClient = mtproto.NewRPCStoriesClient(bffConn)
		service.SuggestedPostsClient = mtproto.NewRPCSuggestedPostsClient(bffConn)
		service.MediaClient = media_client.NewMediaClient(rpcx.GetCachedRpcClient(c.BffClient))
	}
	if c.DfsClient.Etcd.Key != "" || len(c.DfsClient.Endpoints) > 0 {
		service.DfsClient = dfs_client.NewDfsClient(rpcx.GetCachedRpcClient(c.DfsClient))
	}
	return service, nil
}

func (s *ServiceContext) Close() {
	if s != nil {
		if s.BotAPIStore != nil {
			s.BotAPIStore.Close()
		}
		_ = paymentrecovery.ClosePostgres()
	}
}
