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

func (s *Service) LangpackGetLangPack(ctx context.Context, request *mtproto.TLLangpackGetLangPack) (*mtproto.LangPackDifference, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("LangpackGetLangPack - request: %s", request)
	r, err := c.LangpackGetLangPack(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("LangpackGetLangPack - reply: %s", r)
	return r, nil
}

func (s *Service) LangpackGetStrings(ctx context.Context, request *mtproto.TLLangpackGetStrings) (*mtproto.Vector_LangPackString, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("LangpackGetStrings - request: %s", request)
	r, err := c.LangpackGetStrings(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("LangpackGetStrings - reply: %s", r)
	return r, nil
}

func (s *Service) LangpackGetDifference(ctx context.Context, request *mtproto.TLLangpackGetDifference) (*mtproto.LangPackDifference, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("LangpackGetDifference - request: %s", request)
	r, err := c.LangpackGetDifference(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("LangpackGetDifference - reply: %s", r)
	return r, nil
}

func (s *Service) LangpackGetLanguages(ctx context.Context, request *mtproto.TLLangpackGetLanguages) (*mtproto.Vector_LangPackLanguage, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("LangpackGetLanguages - request: %s", request)
	r, err := c.LangpackGetLanguages(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("LangpackGetLanguages - reply: %s", r)
	return r, nil
}

func (s *Service) LangpackGetLanguage(ctx context.Context, request *mtproto.TLLangpackGetLanguage) (*mtproto.LangPackLanguage, error) {
	c := core.New(ctx, s.svcCtx)
	c.Logger.Debugf("LangpackGetLanguage - request: %s", request)
	r, err := c.LangpackGetLanguage(request)
	if err != nil {
		return nil, err
	}
	c.Logger.Debugf("LangpackGetLanguage - reply: %s", r)
	return r, nil
}
