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

func (s *Service) HelpGetPromoData(ctx context.Context, request *mtproto.TLHelpGetPromoData) (*mtproto.Help_PromoData, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("HelpGetPromoData - request: %s", request)
	r, err := c.HelpGetPromoData(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("HelpGetPromoData - reply: %s", r)
	return r, nil
}

func (s *Service) HelpHidePromoData(ctx context.Context, request *mtproto.TLHelpHidePromoData) (*mtproto.Bool, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("HelpHidePromoData - request: %s", request)
	r, err := c.HelpHidePromoData(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("HelpHidePromoData - reply: %s", r)
	return r, nil
}
