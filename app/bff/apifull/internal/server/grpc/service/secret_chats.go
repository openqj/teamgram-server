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

package service

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/core"
)

func (s *Service) MessagesGetDhConfig(ctx context.Context, request *mtproto.TLMessagesGetDhConfig) (*mtproto.Messages_DhConfig, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesGetDhConfig - request: %s", request)
	r, err := c.MessagesGetDhConfig(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesGetDhConfig - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesRequestEncryption(ctx context.Context, request *mtproto.TLMessagesRequestEncryption) (*mtproto.EncryptedChat, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesRequestEncryption - request: %s", request)
	r, err := c.MessagesRequestEncryption(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesRequestEncryption - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesAcceptEncryption(ctx context.Context, request *mtproto.TLMessagesAcceptEncryption) (*mtproto.EncryptedChat, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesAcceptEncryption - request: %s", request)
	r, err := c.MessagesAcceptEncryption(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesAcceptEncryption - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesDiscardEncryption(ctx context.Context, request *mtproto.TLMessagesDiscardEncryption) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesDiscardEncryption - request: %s", request)
	r, err := c.MessagesDiscardEncryption(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesDiscardEncryption - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesSetEncryptedTyping(ctx context.Context, request *mtproto.TLMessagesSetEncryptedTyping) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesSetEncryptedTyping - request: %s", request)
	r, err := c.MessagesSetEncryptedTyping(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesSetEncryptedTyping - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesReadEncryptedHistory(ctx context.Context, request *mtproto.TLMessagesReadEncryptedHistory) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesReadEncryptedHistory - request: %s", request)
	r, err := c.MessagesReadEncryptedHistory(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesReadEncryptedHistory - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesSendEncrypted(ctx context.Context, request *mtproto.TLMessagesSendEncrypted) (*mtproto.Messages_SentEncryptedMessage, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesSendEncrypted - request: %s", request)
	r, err := c.MessagesSendEncrypted(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesSendEncrypted - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesSendEncryptedFile(ctx context.Context, request *mtproto.TLMessagesSendEncryptedFile) (*mtproto.Messages_SentEncryptedMessage, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesSendEncryptedFile - request: %s", request)
	r, err := c.MessagesSendEncryptedFile(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesSendEncryptedFile - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesSendEncryptedService(ctx context.Context, request *mtproto.TLMessagesSendEncryptedService) (*mtproto.Messages_SentEncryptedMessage, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesSendEncryptedService - request: %s", request)
	r, err := c.MessagesSendEncryptedService(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesSendEncryptedService - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesReceivedQueue(ctx context.Context, request *mtproto.TLMessagesReceivedQueue) (*mtproto.Vector_Long, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesReceivedQueue - request: %s", request)
	r, err := c.MessagesReceivedQueue(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesReceivedQueue - reply: %s", r)
	return r, nil
}
