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

package server

import (
	"context"
	"errors"
	"flag"
	"os"
	"strconv"
	"strings"

	"github.com/teamgram/proto/mtproto"
	account_helper "github.com/teamgram/teamgram-server/app/bff/account"
	apifull_helper "github.com/teamgram/teamgram-server/app/bff/apifull"
	"github.com/teamgram/teamgram-server/app/bff/apifull/channelview"
	authorization_helper "github.com/teamgram/teamgram-server/app/bff/authorization"
	autodownload_helper "github.com/teamgram/teamgram-server/app/bff/autodownload"
	"github.com/teamgram/teamgram-server/app/bff/bff/internal/config"
	chatinvites_helper "github.com/teamgram/teamgram-server/app/bff/chatinvites"
	chats_helper "github.com/teamgram/teamgram-server/app/bff/chats"
	configuration_helper "github.com/teamgram/teamgram-server/app/bff/configuration"
	contacts_helper "github.com/teamgram/teamgram-server/app/bff/contacts"
	dialogs_helper "github.com/teamgram/teamgram-server/app/bff/dialogs"
	drafts_helper "github.com/teamgram/teamgram-server/app/bff/drafts"
	files_helper "github.com/teamgram/teamgram-server/app/bff/files"
	messages_helper "github.com/teamgram/teamgram-server/app/bff/messages"
	miscellaneous_helper "github.com/teamgram/teamgram-server/app/bff/miscellaneous"
	notification_helper "github.com/teamgram/teamgram-server/app/bff/notification"
	nsfw_helper "github.com/teamgram/teamgram-server/app/bff/nsfw"
	passkeyhelper "github.com/teamgram/teamgram-server/app/bff/passkey"
	passport_helper "github.com/teamgram/teamgram-server/app/bff/passport"
	premium_helper "github.com/teamgram/teamgram-server/app/bff/premium"
	privacysettingshelper "github.com/teamgram/teamgram-server/app/bff/privacysettings"
	qrcode_helper "github.com/teamgram/teamgram-server/app/bff/qrcode"
	savedmessagedialogshelper "github.com/teamgram/teamgram-server/app/bff/savedmessagedialogs"
	sponsoredmessages_helper "github.com/teamgram/teamgram-server/app/bff/sponsoredmessages"
	tos_helper "github.com/teamgram/teamgram-server/app/bff/tos"
	updates_helper "github.com/teamgram/teamgram-server/app/bff/updates"
	userchannelprofileshelper "github.com/teamgram/teamgram-server/app/bff/userchannelprofiles"
	usernames_helper "github.com/teamgram/teamgram-server/app/bff/usernames"
	users_helper "github.com/teamgram/teamgram-server/app/bff/users"
	webbrowserhelper "github.com/teamgram/teamgram-server/app/bff/webbrowser"
	dialogpb "github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	codeconf "github.com/teamgram/teamgram-server/pkg/code/conf"
	"github.com/teamgram/teamgram-server/pkg/twofa"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/zrpc"
	"google.golang.org/grpc"
)

var configFile = flag.String("f", "etc/bff.yaml", "the config file")

func applyTurnEnvironment(c *config.Config) error {
	for _, override := range []struct {
		name   string
		target *string
	}{
		{name: "TEAMGRAM_TURN_HOST", target: &c.TurnHost},
		{name: "TEAMGRAM_TURN_USERNAME", target: &c.TurnUsername},
		{name: "TEAMGRAM_TURN_PASSWORD", target: &c.TurnPassword},
		{name: "TEAMGRAM_TURN_SHARED_SECRET", target: &c.TurnSharedSecret},
	} {
		if value, ok := os.LookupEnv(override.name); ok {
			if override.name == "TEAMGRAM_TURN_HOST" {
				value = strings.TrimSpace(value)
			}
			*override.target = value
		}
	}
	if value, ok := os.LookupEnv("TEAMGRAM_TURN_PORT"); ok {
		port, err := strconv.ParseInt(strings.TrimSpace(value), 10, 32)
		if err != nil || port < 1 || port > 65535 {
			return errors.New("TEAMGRAM_TURN_PORT must be an integer from 1 to 65535")
		}
		c.TurnPort = int32(port)
	}
	if value, ok := os.LookupEnv("TEAMGRAM_TURN_CREDENTIAL_TTL_SECONDS"); ok {
		ttl, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || ttl < 0 || ttl > 86400 {
			return errors.New("TEAMGRAM_TURN_CREDENTIAL_TTL_SECONDS must be an integer from 0 to 86400")
		}
		c.TurnCredentialTTLSeconds = ttl
	}
	return nil
}

func applyProviderEnvironment(c *config.Config) error {
	if value, ok := os.LookupEnv("TEAMGRAM_PAYMENT_PROVIDER_ENDPOINT"); ok {
		c.PaymentProviderEndpoint = strings.TrimSpace(value)
	}
	if value, ok := os.LookupEnv("TEAMGRAM_PAYMENT_PROVIDER_KEY"); ok {
		c.PaymentProviderKey = value
	}
	if value, ok := os.LookupEnv("TEAMGRAM_PAYMENT_PROVIDER_SIGNING_KEY"); ok {
		c.PaymentProviderSigningKey = value
	}
	if value, ok := os.LookupEnv("TEAMGRAM_PAYMENT_PROVIDER_TIMEOUT_SECONDS"); ok {
		timeout, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || timeout < 0 || timeout > 300 {
			return errors.New("TEAMGRAM_PAYMENT_PROVIDER_TIMEOUT_SECONDS must be an integer from 0 to 300")
		}
		c.PaymentProviderTimeoutSeconds = timeout
	}
	for _, override := range []struct {
		name   string
		target *string
	}{
		{name: "TEAMGRAM_AUTH_PROVIDER_ENDPOINT", target: &c.AuthProviderEndpoint},
		{name: "TEAMGRAM_AUTH_PROVIDER_KEY", target: &c.AuthProviderKey},
		{name: "TEAMGRAM_AUTH_PROVIDER_SIGNING_KEY", target: &c.AuthProviderSigningKey},
	} {
		if value, ok := os.LookupEnv(override.name); ok {
			if override.name == "TEAMGRAM_AUTH_PROVIDER_ENDPOINT" {
				value = strings.TrimSpace(value)
			}
			*override.target = value
		}
	}
	if value, ok := os.LookupEnv("TEAMGRAM_AUTH_PROVIDER_TIMEOUT_SECONDS"); ok {
		timeout, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || timeout < 0 || timeout > 300 {
			return errors.New("TEAMGRAM_AUTH_PROVIDER_TIMEOUT_SECONDS must be an integer from 0 to 300")
		}
		c.AuthProviderTimeoutSeconds = timeout
	}
	for _, override := range []struct {
		name   string
		target *string
	}{
		{name: "TEAMGRAM_GROUP_CALL_MEDIA_ENDPOINT", target: &c.GroupCallMediaEndpoint},
		{name: "TEAMGRAM_GROUP_CALL_MEDIA_API_KEY", target: &c.GroupCallMediaAPIKey},
		{name: "TEAMGRAM_GROUP_CALL_MEDIA_SIGNING_KEY", target: &c.GroupCallMediaSigningKey},
		{name: "TEAMGRAM_GROUP_CALL_MEDIA_RTMP_HOST", target: &c.GroupCallMediaRTMPHost},
	} {
		if value, ok := os.LookupEnv(override.name); ok {
			if override.name == "TEAMGRAM_GROUP_CALL_MEDIA_ENDPOINT" || override.name == "TEAMGRAM_GROUP_CALL_MEDIA_RTMP_HOST" {
				value = strings.TrimSpace(value)
			}
			*override.target = value
		}
	}
	if value, ok := os.LookupEnv("TEAMGRAM_GROUP_CALL_MEDIA_TIMEOUT_SECONDS"); ok {
		timeout, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || timeout < 0 || timeout > 300 {
			return errors.New("TEAMGRAM_GROUP_CALL_MEDIA_TIMEOUT_SECONDS must be an integer from 0 to 300")
		}
		c.GroupCallMediaTimeoutSeconds = timeout
	}

	if c.Code == nil {
		c.Code = &codeconf.SmsVerifyCodeConfig{}
	}
	for _, override := range []struct {
		name   string
		target *string
	}{
		{name: "TEAMGRAM_CODE_SMS_PROVIDER", target: &c.Code.SMSProvider},
		{name: "TEAMGRAM_CODE_EMAIL_PROVIDER", target: &c.Code.EmailProvider},
		{name: "TEAMGRAM_CODE_SEND_URL", target: &c.Code.SendCodeUrl},
		{name: "TEAMGRAM_CODE_EMAIL_SEND_URL", target: &c.Code.EmailSendCodeUrl},
		{name: "TEAMGRAM_CODE_REPORT_MISSING_URL", target: &c.Code.ReportMissingCodeUrl},
		{name: "TEAMGRAM_CODE_VERIFY_URL", target: &c.Code.VerifyCodeUrl},
		{name: "TEAMGRAM_CODE_KEY", target: &c.Code.Key},
		{name: "TEAMGRAM_CODE_SECRET", target: &c.Code.Secret},
		{name: "TEAMGRAM_CODE_CHALLENGE_SECRET", target: &c.Code.ChallengeSecret},
		{name: "TEAMGRAM_CODE_REGION_ID", target: &c.Code.RegionId},
	} {
		if value, ok := os.LookupEnv(override.name); ok {
			*override.target = value
		}
	}
	if value, ok := os.LookupEnv("TEAMGRAM_CODE_PROVIDER_TIMEOUT_SECONDS"); ok {
		timeout, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || timeout < 0 || timeout > 300 {
			return errors.New("TEAMGRAM_CODE_PROVIDER_TIMEOUT_SECONDS must be an integer from 0 to 300")
		}
		c.Code.ProviderTimeoutSeconds = timeout
	}
	if value, ok := os.LookupEnv("TEAMGRAM_CODE_PROVIDER_RETRY_COUNT"); ok {
		retries, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || retries < 0 || retries > 10 {
			return errors.New("TEAMGRAM_CODE_PROVIDER_RETRY_COUNT must be an integer from 0 to 10")
		}
		c.Code.ProviderRetryCount = retries
	}
	if value, ok := os.LookupEnv("TEAMGRAM_CODE_CHALLENGE_TTL_SECONDS"); ok {
		ttl, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || ttl < 0 || ttl > 86400 {
			return errors.New("TEAMGRAM_CODE_CHALLENGE_TTL_SECONDS must be an integer from 0 to 86400")
		}
		c.Code.ChallengeTTLSeconds = ttl
	}
	if value, ok := os.LookupEnv("TEAMGRAM_CODE_RATE_LIMIT"); ok {
		limit, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || limit < 0 || limit > 100000 {
			return errors.New("TEAMGRAM_CODE_RATE_LIMIT must be an integer from 0 to 100000")
		}
		c.Code.RateLimit = limit
	}
	if value, ok := os.LookupEnv("TEAMGRAM_CODE_RATE_WINDOW_SECONDS"); ok {
		window, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || window < 0 || window > 86400 {
			return errors.New("TEAMGRAM_CODE_RATE_WINDOW_SECONDS must be an integer from 0 to 86400")
		}
		c.Code.RateWindowSeconds = window
	}
	if value, ok := os.LookupEnv("TEAMGRAM_CODE_MAX_ATTEMPTS"); ok {
		attempts, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || attempts < 0 || attempts > 1000 {
			return errors.New("TEAMGRAM_CODE_MAX_ATTEMPTS must be an integer from 0 to 1000")
		}
		c.Code.MaxAttempts = attempts
	}
	return nil
}

type Server struct {
	grpcSrv         *zrpc.RpcServer
	postgresClosers []func() error
	apiFullWorkers  interface {
		StopWorkers()
	}
}

type contactsChannelPlugin struct{}

func (contactsChannelPlugin) GetChannelListByIdList(_ context.Context, selfID int64, ids ...int64) []*mtproto.Chat {
	return channelview.ChatsByID(selfID, ids)
}

func (contactsChannelPlugin) GetChannelDialogById(context.Context, int64, int64) (*dialogpb.DialogExt, error) {
	return nil, mtproto.ErrMethodNotImpl
}

func (contactsChannelPlugin) GetChannelMessage(context.Context, int64, int64, int32) (*mtproto.MessageBox, error) {
	return nil, mtproto.ErrMethodNotImpl
}

func (contactsChannelPlugin) GetChannelTypingRecipients(ctx context.Context, selfID int64, peer *mtproto.InputPeer) ([]int64, error) {
	return channelview.TypingRecipients(selfID, peer)
}

func New() *Server {
	return new(Server)
}

func (s *Server) Initialize() error {
	var c config.Config
	conf.MustLoad(*configFile, &c, conf.UseEnv())
	if err := applyTurnEnvironment(&c); err != nil {
		return err
	}
	if err := applyProviderEnvironment(&c); err != nil {
		return err
	}
	if strings.TrimSpace(c.PostgresDSN) == "" {
		return errors.New("bff: PostgresDSN is required")
	}

	logx.Infof("bff configuration loaded for DC %d", c.DcId)
	// ctx := svc.NewServiceContext(c)
	// s.grpcSrv = grpc.New(ctx, c.RpcServerConf)

	s.grpcSrv = zrpc.MustNewServer(c.RpcServerConf, func(grpcServer *grpc.Server) {
		// tos_helper
		mtproto.RegisterRPCTosServer(
			grpcServer,
			tos_helper.New(tos_helper.Config{
				RpcServerConf: c.RpcServerConf,
				PostgresDSN:   c.PostgresDSN,
			}))

		// configuration_helper
		mtproto.RegisterRPCConfigurationServer(
			grpcServer,
			configuration_helper.New(configuration_helper.Config{
				RpcServerConf: c.RpcServerConf,
				PostgresDSN:   c.PostgresDSN,
			}))

		// qrcode_helper
		mtproto.RegisterRPCQrCodeServer(
			grpcServer,
			qrcode_helper.New(
				qrcode_helper.Config{
					RpcServerConf:     c.RpcServerConf,
					DcId:              c.DcId,
					KnownDcIds:        c.KnownDcIds,
					TrustedApps:       c.QrCode.TrustedApps,
					KV:                c.KV,
					PostgresDSN:       c.PostgresDSN,
					UserClient:        c.BizServiceClient,
					AuthSessionClient: c.AuthSessionClient,
					SyncClient:        c.SyncClient,
				},
				nil))

		// miscellaneous_helper
		mtproto.RegisterRPCMiscellaneousServer(
			grpcServer,
			miscellaneous_helper.New(miscellaneous_helper.Config{
				RpcServerConf: c.RpcServerConf,
				PostgresDSN:   c.PostgresDSN,
			}))

		// authorization_helper
		mtproto.RegisterRPCAuthorizationServer(
			grpcServer,
			authorization_helper.New(
				authorization_helper.Config{
					RpcServerConf:              c.RpcServerConf,
					DcId:                       c.DcId,
					KnownDcIds:                 c.KnownDcIds,
					KV:                         c.KV,
					PostgresDSN:                c.PostgresDSN,
					Code:                       c.Code,
					AuthProviderEndpoint:       c.AuthProviderEndpoint,
					AuthProviderKey:            c.AuthProviderKey,
					AuthProviderSigningKey:     c.AuthProviderSigningKey,
					AuthProviderTimeoutSeconds: c.AuthProviderTimeoutSeconds,
					UserClient:                 c.BizServiceClient,
					AuthsessionClient:          c.AuthSessionClient,
					ChatClient:                 c.BizServiceClient,
					StatusClient:               c.StatusClient,
					SyncClient:                 c.SyncClient,
					MsgClient:                  c.MsgClient,
					SignInMessage:              c.SignInMessage,
					SignInServiceNotification:  c.SignInServiceNotification,
				},
				nil,
				nil))

		// premium_helper
		mtproto.RegisterRPCPremiumServer(
			grpcServer,
			premium_helper.New(premium_helper.Config{
				RpcServerConf: c.RpcServerConf,
			}))

		// chatinvites_helper
		mtproto.RegisterRPCChatInvitesServer(
			grpcServer,
			chatinvites_helper.New(chatinvites_helper.Config{
				RpcServerConf: c.RpcServerConf,
				PostgresDSN:   c.PostgresDSN,
				UserClient:    c.BizServiceClient,
				ChatClient:    c.BizServiceClient,
				MsgClient:     c.MsgClient,
				SyncClient:    c.SyncClient,
			}))

		// chats_helper
		mtproto.RegisterRPCChatsServer(
			grpcServer,
			chats_helper.New(chats_helper.Config{
				RpcServerConf:     c.RpcServerConf,
				PostgresDSN:       c.PostgresDSN,
				UserClient:        c.BizServiceClient,
				ChatClient:        c.BizServiceClient,
				MsgClient:         c.MsgClient,
				DialogClient:      c.BizServiceClient,
				SyncClient:        c.SyncClient,
				MediaClient:       c.MediaClient,
				AuthsessionClient: c.AuthSessionClient,
				IdgenClient:       c.IdgenClient,
				MessageClient:     c.BizServiceClient,
			}))

		// files_helper
		mtproto.RegisterRPCFilesServer(
			grpcServer,
			files_helper.New(files_helper.Config{
				RpcServerConf: c.RpcServerConf,
				DcId:          c.DcId,
				DfsClient:     c.DfsClient,
				UserClient:    c.BizServiceClient,
				MediaClient:   c.MediaClient,
			}, nil, nil))

		// passport_helper
		mtproto.RegisterRPCPassportServer(
			grpcServer,
			passport_helper.New(passport_helper.Config{
				RpcServerConf:     c.RpcServerConf,
				PostgresDSN:       c.PostgresDSN,
				AuthsessionClient: c.AuthSessionClient,
				UserClient:        c.BizServiceClient,
			}))

		// updates_helper
		updatesService := updates_helper.New(updates_helper.Config{
			RpcServerConf:     c.RpcServerConf,
			PostgresDSN:       c.PostgresDSN,
			UpdatesClient:     c.BizServiceClient,
			UserClient:        c.BizServiceClient,
			ChatClient:        c.BizServiceClient,
			AuthsessionClient: c.AuthSessionClient,
		})
		s.postgresClosers = append(s.postgresClosers, updatesService.ClosePostgres)
		mtproto.RegisterRPCUpdatesServer(grpcServer, updatesService)

		// contacts_helper
		mtproto.RegisterRPCContactsServer(
			grpcServer,
			contacts_helper.New(
				contacts_helper.Config{
					RpcServerConf: c.RpcServerConf,
					PostgresDSN:   c.PostgresDSN,
					UserClient:    c.BizServiceClient,
					ChatClient:    c.BizServiceClient,
					MessageClient: c.BizServiceClient,
					SyncClient:    c.SyncClient,
				},
				contactsChannelPlugin{}))

		// dialogs_helper
		mtproto.RegisterRPCDialogsServer(
			grpcServer,
			dialogs_helper.New(dialogs_helper.Config{
				RpcServerConf: c.RpcServerConf,
				UpdatesClient: c.BizServiceClient,
				UserClient:    c.BizServiceClient,
				ChatClient:    c.BizServiceClient,
				DialogClient:  c.BizServiceClient,
				SyncClient:    c.SyncClient,
				MessageClient: c.BizServiceClient,
			}, contactsChannelPlugin{}))

		// drafts_helper
		mtproto.RegisterRPCDraftsServer(
			grpcServer,
			drafts_helper.New(drafts_helper.Config{
				RpcServerConf: c.RpcServerConf,
				PostgresDSN:   c.PostgresDSN,
				DialogClient:  c.BizServiceClient,
				UserClient:    c.BizServiceClient,
				SyncClient:    c.SyncClient,
				ChatClient:    c.BizServiceClient,
			}, nil))

		// autodownload_helper
		mtproto.RegisterRPCAutoDownloadServer(
			grpcServer,
			autodownload_helper.New(autodownload_helper.Config{
				RpcServerConf: c.RpcServerConf,
				PostgresDSN:   c.PostgresDSN,
			}))

		// messages_helper
		messagesService := messages_helper.New(messages_helper.Config{
			RpcServerConf: c.RpcServerConf,
			PostgresDSN:   c.PostgresDSN,
			KV:            c.KV,
			SearchPostsFlood: messages_helper.SearchPostsFloodConfig{
				TotalDaily: c.SearchPostsFlood.TotalDaily,
			},
			UserClient:    c.BizServiceClient,
			ChatClient:    c.BizServiceClient,
			MsgClient:     c.MsgClient,
			DialogClient:  c.BizServiceClient,
			IdgenClient:   c.IdgenClient,
			MessageClient: c.BizServiceClient,
			MediaClient:   c.MediaClient,
			SyncClient:    c.SyncClient,
		}, nil)
		s.postgresClosers = append(s.postgresClosers, messagesService.ClosePostgres)
		mtproto.RegisterRPCMessagesServer(grpcServer, messagesService)

		// notification_helper
		mtproto.RegisterRPCNotificationServer(
			grpcServer,
			notification_helper.New(notification_helper.Config{
				RpcServerConf: c.RpcServerConf,
				PostgresDSN:   c.PostgresDSN,
				UserClient:    c.BizServiceClient,
				ChatClient:    c.BizServiceClient,
				SyncClient:    c.SyncClient,
			}, nil))

		// users_helper
		mtproto.RegisterRPCUsersServer(
			grpcServer,
			users_helper.New(
				users_helper.Config{
					RpcServerConf: c.RpcServerConf,
					UserClient:    c.BizServiceClient,
					ChatClient:    c.BizServiceClient,
					DialogClient:  c.BizServiceClient,
				},
				nil,
				nil,
				nil))

		// nsfw_helper
		mtproto.RegisterRPCNsfwServer(
			grpcServer,
			nsfw_helper.New(nsfw_helper.Config{
				RpcServerConf: c.RpcServerConf,
				UserClient:    c.BizServiceClient,
			}))

		// sponsoredmessages_helper
		mtproto.RegisterRPCSponsoredMessagesServer(
			grpcServer,
			sponsoredmessages_helper.New(sponsoredmessages_helper.Config{
				RpcServerConf: c.RpcServerConf,
				PostgresDSN:   c.PostgresDSN,
			}))

		// account_helper
		mtproto.RegisterRPCAccountServer(
			grpcServer,
			account_helper.New(
				account_helper.Config{
					RpcServerConf:     c.RpcServerConf,
					PostgresDSN:       c.PostgresDSN,
					KV:                c.KV,
					UserClient:        c.BizServiceClient,
					AuthsessionClient: c.AuthSessionClient,
					ChatClient:        c.BizServiceClient,
					SyncClient:        c.SyncClient,
				},
				nil,
				nil))
		// usernames_helper
		mtproto.RegisterRPCUsernamesServer(
			grpcServer,
			usernames_helper.New(usernames_helper.Config{
				RpcServerConf: c.RpcServerConf,
				UserClient:    c.BizServiceClient,
				ChatClient:    c.BizServiceClient,
				SyncClient:    c.SyncClient,
			}, nil))

		// privacysettingshelper
		mtproto.RegisterRPCPrivacySettingsServer(
			grpcServer,
			privacysettingshelper.New(privacysettingshelper.Config{
				RpcServerConf:     c.RpcServerConf,
				UserClient:        c.BizServiceClient,
				AuthsessionClient: c.AuthSessionClient,
				ChatClient:        c.BizServiceClient,
				SyncClient:        c.SyncClient,
			}))

		// savedmessagedialogshelper
		mtproto.RegisterRPCSavedMessageDialogsServer(
			grpcServer,
			savedmessagedialogshelper.New(savedmessagedialogshelper.Config{
				RpcServerConf: c.RpcServerConf,
				UpdatesClient: c.BizServiceClient,
				UserClient:    c.BizServiceClient,
				ChatClient:    c.BizServiceClient,
				DialogClient:  c.BizServiceClient,
				SyncClient:    c.SyncClient,
				MessageClient: c.BizServiceClient,
			}))

		// userchannelprofileshelper
		mtproto.RegisterRPCUserChannelProfilesServer(
			grpcServer,
			userchannelprofileshelper.New(userchannelprofileshelper.Config{
				RpcServerConf: c.RpcServerConf,
				PostgresDSN:   c.PostgresDSN,
				MediaClient:   c.MediaClient,
				UserClient:    c.BizServiceClient,
				SyncClient:    c.SyncClient,
			}))

		passkeyService := passkeyhelper.New(passkeyhelper.Config{
			RpcServerConf:     c.RpcServerConf,
			Provider:          c.Passkey,
			PostgresDSN:       c.PostgresDSN,
			DcId:              c.DcId,
			UserClient:        c.BizServiceClient,
			AuthSessionClient: c.AuthSessionClient,
		})
		s.postgresClosers = append(s.postgresClosers, passkeyService.ClosePostgres)
		mtproto.RegisterRPCPasskeyServer(grpcServer, passkeyService)

		mtproto.RegisterRPCWebBrowserServer(
			grpcServer,
			webbrowserhelper.New(webbrowserhelper.Config{
				RpcServerConf: c.RpcServerConf,
				PostgresDSN:   c.PostgresDSN,
			}))

		apiFull := apifull_helper.New(apifull_helper.Config{
			RpcServerConf:                 c.RpcServerConf,
			DialogClient:                  c.BizServiceClient,
			UserClient:                    c.BizServiceClient,
			ChatClient:                    c.BizServiceClient,
			MessageClient:                 c.BizServiceClient,
			MsgClient:                     c.MsgClient,
			DfsClient:                     c.DfsClient,
			SyncClient:                    c.SyncClient,
			KV:                            c.KV,
			Code:                          c.Code,
			PostgresDSN:                   c.PostgresDSN,
			PaymentProviderEndpoint:       c.PaymentProviderEndpoint,
			PaymentProviderKey:            c.PaymentProviderKey,
			PaymentProviderSigningKey:     c.PaymentProviderSigningKey,
			PaymentProviderTimeoutSeconds: c.PaymentProviderTimeoutSeconds,
			GroupCallMediaEndpoint:        c.GroupCallMediaEndpoint,
			GroupCallMediaAPIKey:          c.GroupCallMediaAPIKey,
			GroupCallMediaSigningKey:      c.GroupCallMediaSigningKey,
			GroupCallMediaRTMPHost:        c.GroupCallMediaRTMPHost,
			GroupCallMediaTimeoutSeconds:  c.GroupCallMediaTimeoutSeconds,
			TurnHost:                      c.TurnHost,
			TurnPort:                      c.TurnPort,
			TurnUsername:                  c.TurnUsername,
			TurnPassword:                  c.TurnPassword,
			TurnSharedSecret:              c.TurnSharedSecret,
			TurnCredentialTTLSeconds:      c.TurnCredentialTTLSeconds,
		})
		s.apiFullWorkers = apiFull
		apiFull.StartWorkers()
		mtproto.RegisterRPCAccentColorsServer(grpcServer, apiFull)
		mtproto.RegisterRPCAffiliateProgramsServer(grpcServer, apiFull)
		mtproto.RegisterRPCAiComposeToneServer(grpcServer, apiFull)
		mtproto.RegisterRPCAntiSpamServer(grpcServer, apiFull)
		mtproto.RegisterRPCAutosaveServer(grpcServer, apiFull)
		mtproto.RegisterRPCBoostsServer(grpcServer, apiFull)
		mtproto.RegisterRPCBotAdminRightServer(grpcServer, apiFull)
		mtproto.RegisterRPCBotMenuServer(grpcServer, apiFull)
		mtproto.RegisterRPCBotMenuButtonServer(grpcServer, apiFull)
		mtproto.RegisterRPCBotVerificationIconsServer(grpcServer, apiFull)
		mtproto.RegisterRPCBotsServer(grpcServer, apiFull)
		mtproto.RegisterRPCBusinessChatLinksServer(grpcServer, apiFull)
		mtproto.RegisterRPCBusinessConnectedBotsServer(grpcServer, apiFull)
		mtproto.RegisterRPCBusinessGreetingServer(grpcServer, apiFull)
		mtproto.RegisterRPCBusinessIntroServer(grpcServer, apiFull)
		mtproto.RegisterRPCBusinessLocationServer(grpcServer, apiFull)
		mtproto.RegisterRPCBusinessOpeningHoursServer(grpcServer, apiFull)
		mtproto.RegisterRPCBusinessQuickReplyServer(grpcServer, apiFull)
		mtproto.RegisterRPCChannelAdRevenueServer(grpcServer, apiFull)
		mtproto.RegisterRPCChannelRecommendationsServer(grpcServer, apiFull)
		mtproto.RegisterRPCChannelsServer(grpcServer, apiFull)
		mtproto.RegisterRPCCommunitiesServer(grpcServer, apiFull)
		mtproto.RegisterRPCConferenceCallsServer(grpcServer, apiFull)
		mtproto.RegisterRPCCustomEmojisServer(grpcServer, apiFull)
		mtproto.RegisterRPCDeepLinksServer(grpcServer, apiFull)
		mtproto.RegisterRPCEmojiServer(grpcServer, apiFull)
		mtproto.RegisterRPCEmojiCategoriesServer(grpcServer, apiFull)
		mtproto.RegisterRPCEmojiStatusServer(grpcServer, apiFull)
		mtproto.RegisterRPCEphemeralServer(grpcServer, apiFull)
		mtproto.RegisterRPCFactChecksServer(grpcServer, apiFull)
		mtproto.RegisterRPCFolderTagsServer(grpcServer, apiFull)
		mtproto.RegisterRPCFoldersServer(grpcServer, apiFull)
		mtproto.RegisterRPCForumsServer(grpcServer, apiFull)
		mtproto.RegisterRPCFragmentServer(grpcServer, apiFull)
		mtproto.RegisterRPCFragmentCollectiblesServer(grpcServer, apiFull)
		mtproto.RegisterRPCGamesServer(grpcServer, apiFull)
		mtproto.RegisterRPCGatewayVerificationMessagesServer(grpcServer, apiFull)
		mtproto.RegisterRPCGifsServer(grpcServer, apiFull)
		mtproto.RegisterRPCGiftCodesServer(grpcServer, apiFull)
		mtproto.RegisterRPCGiftCollectionsServer(grpcServer, apiFull)
		mtproto.RegisterRPCGiftsServer(grpcServer, apiFull)
		mtproto.RegisterRPCGiveawaysServer(grpcServer, apiFull)
		mtproto.RegisterRPCGroupCallsServer(grpcServer, apiFull)
		mtproto.RegisterRPCImportedChatsServer(grpcServer, apiFull)
		mtproto.RegisterRPCInlineBotServer(grpcServer, apiFull)
		mtproto.RegisterRPCInternalBotServer(grpcServer, apiFull)
		mtproto.RegisterRPCLangpackServer(grpcServer, apiFull)
		mtproto.RegisterRPCMainMiniBotAppsServer(grpcServer, apiFull)
		mtproto.RegisterRPCMessageEffectsServer(grpcServer, apiFull)
		mtproto.RegisterRPCMessageThreadsServer(grpcServer, apiFull)
		mtproto.RegisterRPCMiniBotAppsServer(grpcServer, apiFull)
		mtproto.RegisterRPCPaidMediaServer(grpcServer, apiFull)
		mtproto.RegisterRPCPaidMessageServer(grpcServer, apiFull)
		mtproto.RegisterRPCPaymentsServer(grpcServer, apiFull)
		mtproto.RegisterRPCPollsServer(grpcServer, apiFull)
		mtproto.RegisterRPCPredefinedServer(grpcServer, apiFull)
		mtproto.RegisterRPCPreparedInlineMessagesServer(grpcServer, apiFull)
		mtproto.RegisterRPCProfileLinksServer(grpcServer, apiFull)
		mtproto.RegisterRPCPromoDataServer(grpcServer, apiFull)
		mtproto.RegisterRPCReactionNotificationServer(grpcServer, apiFull)
		mtproto.RegisterRPCReactionsServer(grpcServer, apiFull)
		mtproto.RegisterRPCReportsServer(grpcServer, apiFull)
		mtproto.RegisterRPCRingtoneServer(grpcServer, apiFull)
		mtproto.RegisterRPCSavedMessageTagsServer(grpcServer, apiFull)
		mtproto.RegisterRPCScheduledMessagesServer(grpcServer, apiFull)
		mtproto.RegisterRPCSeamlessServer(grpcServer, apiFull)
		mtproto.RegisterRPCSecretChatsServer(grpcServer, apiFull)
		mtproto.RegisterRPCSmsjobsServer(grpcServer, apiFull)
		mtproto.RegisterRPCStarSubscriptionsServer(grpcServer, apiFull)
		mtproto.RegisterRPCStarsServer(grpcServer, apiFull)
		mtproto.RegisterRPCStatisticsServer(grpcServer, apiFull)
		mtproto.RegisterRPCStickersServer(grpcServer, apiFull)
		mtproto.RegisterRPCStoriesServer(grpcServer, apiFull)
		mtproto.RegisterRPCSuggestedPostsServer(grpcServer, apiFull)
		mtproto.RegisterRPCTakeoutServer(grpcServer, apiFull)
		mtproto.RegisterRPCThemesServer(grpcServer, apiFull)
		mtproto.RegisterRPCTimezonesServer(grpcServer, apiFull)
		mtproto.RegisterRPCTodoListsServer(grpcServer, apiFull)
		mtproto.RegisterRPCTranscriptionServer(grpcServer, apiFull)
		mtproto.RegisterRPCTranslationServer(grpcServer, apiFull)
		mtproto.RegisterRPCTsfServer(grpcServer, apiFull)
		mtproto.RegisterRPCTwoFaServer(grpcServer, apiFull)
		mtproto.RegisterRPCVoipCallsServer(grpcServer, apiFull)
		mtproto.RegisterRPCWallpapersServer(grpcServer, apiFull)
		mtproto.RegisterRPCWebPageServer(grpcServer, apiFull)
		mtproto.RegisterRPCBizServer(grpcServer, apiFull)

	})

	// logx.Must(err)

	go func() {
		s.grpcSrv.Start()
	}()
	return nil
}

func (s *Server) RunLoop() {
}

func (s *Server) Destroy() {
	if s.apiFullWorkers != nil {
		s.apiFullWorkers.StopWorkers()
	}
	s.grpcSrv.Stop()
	for _, closePostgres := range s.postgresClosers {
		if err := closePostgres(); err != nil {
			logx.Errorf("close BFF PostgreSQL: %v", err)
		}
	}
	if err := twofa.ClosePostgresProofStores(); err != nil {
		logx.Errorf("close two-factor PostgreSQL: %v", err)
	}
	if err := apifull_helper.ClosePostgres(); err != nil {
		logx.Errorf("close APIFull PostgreSQL: %v", err)
	}
}
