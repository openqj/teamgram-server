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

package dao

import (
	"context"

	kafka "github.com/teamgram/marmota/pkg/mq"
	"github.com/teamgram/marmota/pkg/net/rpcx"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/config"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
	msg_client "github.com/teamgram/teamgram-server/app/messenger/msg/msg/client"
	msgpb "github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
	sync_client "github.com/teamgram/teamgram-server/app/messenger/sync/client"
	chat_client "github.com/teamgram/teamgram-server/app/service/biz/chat/client"
	dialog_client "github.com/teamgram/teamgram-server/app/service/biz/dialog/client"
	message_client "github.com/teamgram/teamgram-server/app/service/biz/message/client"
	messagepb "github.com/teamgram/teamgram-server/app/service/biz/message/message"
	user_client "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	dfs_client "github.com/teamgram/teamgram-server/app/service/dfs/client"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/stores/kv"
)

type ScheduledMessageSender interface {
	MsgSendMessageV2(context.Context, *msgpb.TLMsgSendMessageV2) (*mtproto.Updates, error)
}

type MessageMutator interface {
	MsgDeleteMessages(context.Context, *msgpb.TLMsgDeleteMessages) (*mtproto.Messages_AffectedMessages, error)
	MsgDeleteHistory(context.Context, *msgpb.TLMsgDeleteHistory) (*mtproto.Messages_AffectedHistory, error)
}

type PhoneCallHistoryMutator interface {
	MsgDeletePhoneCallHistory(context.Context, *msgpb.TLMsgDeletePhoneCallHistory) (*mtproto.Messages_AffectedFoundMessages, error)
}

type PollMessageReader interface {
	MessageGetUserMessage(context.Context, *messagepb.TLMessageGetUserMessage) (*mtproto.MessageBox, error)
}

// MessageHistoryReader exposes the authoritative message history used by
// thread lookups. Keep this narrower than the generated client so APIFull
// callers cannot accidentally depend on unrelated message RPCs.
type MessageHistoryReader interface {
	MessageGetHistoryMessages(context.Context, *messagepb.TLMessageGetHistoryMessages) (*messagepb.Vector_MessageBox, error)
}

type Dao struct {
	dialog_client.DialogClient
	user_client.UserClient
	BotRegistryClient user_client.BotRegistryClient
	chat_client.ChatClient
	sync_client.SyncClient
	dfs_client.DfsClient
	ScheduledMessageSender
	MessageMutator
	PhoneCallHistoryMutator
	PollMessageReader
	MessageHistoryReader
}

func New(c config.Config) *Dao {
	if c.TurnHost != "" {
		domain.SetRelay(c.TurnHost, c.TurnPort)
		logx.Infof("apifull turn relay %s:%d", c.TurnHost, c.TurnPort)
	}
	domain.SetRelayCredentials(c.TurnUsername, c.TurnPassword)
	domain.SetRelaySharedSecret(c.TurnSharedSecret, c.TurnCredentialTTLSeconds)
	// MySQL is the authoritative domain store, while Redis backs the small
	// process-shared KV records used by drafts, GIFs and other APIFull state.
	// Configure both when both are present so a MySQL deployment does not
	// silently fall back to the in-process memory store on BFF restart.
	if len(c.KV) > 0 {
		persist.Use(kv.NewStore(c.KV))
	}
	if dsn := c.MysqlDSN; dsn != "" {
		if err := domain.Open(dsn); err != nil {
			logx.Errorf("apifull mysql open failed: %v", err)
		} else {
			logx.Info("apifull mysql open")
		}
	}
	messageClient := message_client.NewMessageClient(rpcx.GetCachedRpcClient(c.MessageClient))
	userRpcClient := rpcx.GetCachedRpcClient(c.UserClient)
	return &Dao{
		DialogClient:            dialog_client.NewDialogClient(rpcx.GetCachedRpcClient(c.DialogClient)),
		UserClient:              user_client.NewUserClient(userRpcClient),
		BotRegistryClient:       user_client.NewBotRegistryClient(userRpcClient),
		ChatClient:              chat_client.NewChatClient(rpcx.GetCachedRpcClient(c.ChatClient)),
		DfsClient:               dfs_client.NewDfsClient(rpcx.GetCachedRpcClient(c.DfsClient)),
		ScheduledMessageSender:  msg_client.NewMsgClient(rpcx.GetCachedRpcClient(c.MsgClient)),
		MessageMutator:          msg_client.NewMsgClient(rpcx.GetCachedRpcClient(c.MsgClient)),
		PhoneCallHistoryMutator: msg_client.NewMsgClient(rpcx.GetCachedRpcClient(c.MsgClient)),
		PollMessageReader:       messageClient,
		MessageHistoryReader:    messageClient,
		SyncClient:              sync_client.NewSyncMqClient(kafka.MustKafkaProducer(c.SyncClient)),
	}
}
