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

func (s *Service) AccountGetWallPapers(ctx context.Context, request *mtproto.TLAccountGetWallPapers) (*mtproto.Account_WallPapers, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("AccountGetWallPapers - request: %s", request)
	r, err := c.AccountGetWallPapers(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("AccountGetWallPapers - reply: %s", r)
	return r, nil
}

func (s *Service) AccountGetWallPaper(ctx context.Context, request *mtproto.TLAccountGetWallPaper) (*mtproto.WallPaper, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("AccountGetWallPaper - request: %s", request)
	r, err := c.AccountGetWallPaper(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("AccountGetWallPaper - reply: %s", r)
	return r, nil
}

func (s *Service) AccountUploadWallPaper(ctx context.Context, request *mtproto.TLAccountUploadWallPaper) (*mtproto.WallPaper, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("AccountUploadWallPaper - request: %s", request)
	r, err := c.AccountUploadWallPaper(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("AccountUploadWallPaper - reply: %s", r)
	return r, nil
}

func (s *Service) AccountSaveWallPaper(ctx context.Context, request *mtproto.TLAccountSaveWallPaper) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("AccountSaveWallPaper - request: %s", request)
	r, err := c.AccountSaveWallPaper(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("AccountSaveWallPaper - reply: %s", r)
	return r, nil
}

func (s *Service) AccountInstallWallPaper(ctx context.Context, request *mtproto.TLAccountInstallWallPaper) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("AccountInstallWallPaper - request: %s", request)
	r, err := c.AccountInstallWallPaper(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("AccountInstallWallPaper - reply: %s", r)
	return r, nil
}

func (s *Service) AccountResetWallPapers(ctx context.Context, request *mtproto.TLAccountResetWallPapers) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("AccountResetWallPapers - request: %s", request)
	r, err := c.AccountResetWallPapers(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("AccountResetWallPapers - reply: %s", r)
	return r, nil
}

func (s *Service) AccountGetMultiWallPapers(ctx context.Context, request *mtproto.TLAccountGetMultiWallPapers) (*mtproto.Vector_WallPaper, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("AccountGetMultiWallPapers - request: %s", request)
	r, err := c.AccountGetMultiWallPapers(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("AccountGetMultiWallPapers - reply: %s", r)
	return r, nil
}

func (s *Service) MessagesSetChatWallPaper(ctx context.Context, request *mtproto.TLMessagesSetChatWallPaper) (*mtproto.Updates, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("MessagesSetChatWallPaper - request: %s", request)
	r, err := c.MessagesSetChatWallPaper(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("MessagesSetChatWallPaper - reply: %s", r)
	return r, nil
}
