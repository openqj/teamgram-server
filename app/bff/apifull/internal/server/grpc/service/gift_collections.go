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

func (s *Service) PaymentsCreateStarGiftCollection(ctx context.Context, request *mtproto.TLPaymentsCreateStarGiftCollection) (*mtproto.StarGiftCollection, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PaymentsCreateStarGiftCollection - request: %s", request)
	r, err := c.PaymentsCreateStarGiftCollection(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PaymentsCreateStarGiftCollection - reply: %s", r)
	return r, nil
}

func (s *Service) PaymentsUpdateStarGiftCollection(ctx context.Context, request *mtproto.TLPaymentsUpdateStarGiftCollection) (*mtproto.StarGiftCollection, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PaymentsUpdateStarGiftCollection - request: %s", request)
	r, err := c.PaymentsUpdateStarGiftCollection(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PaymentsUpdateStarGiftCollection - reply: %s", r)
	return r, nil
}

func (s *Service) PaymentsReorderStarGiftCollections(ctx context.Context, request *mtproto.TLPaymentsReorderStarGiftCollections) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PaymentsReorderStarGiftCollections - request: %s", request)
	r, err := c.PaymentsReorderStarGiftCollections(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PaymentsReorderStarGiftCollections - reply: %s", r)
	return r, nil
}

func (s *Service) PaymentsDeleteStarGiftCollection(ctx context.Context, request *mtproto.TLPaymentsDeleteStarGiftCollection) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PaymentsDeleteStarGiftCollection - request: %s", request)
	r, err := c.PaymentsDeleteStarGiftCollection(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PaymentsDeleteStarGiftCollection - reply: %s", r)
	return r, nil
}

func (s *Service) PaymentsGetStarGiftCollections(ctx context.Context, request *mtproto.TLPaymentsGetStarGiftCollections) (*mtproto.Payments_StarGiftCollections, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("PaymentsGetStarGiftCollections - request: %s", request)
	r, err := c.PaymentsGetStarGiftCollections(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("PaymentsGetStarGiftCollections - reply: %s", r)
	return r, nil
}
