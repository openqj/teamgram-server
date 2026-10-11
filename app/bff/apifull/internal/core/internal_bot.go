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

package core

import (
	"encoding/json"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/domain"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

func miscSave(uid int64, method string, in any) error {
	raw, err := json.Marshal(in)
	if err != nil {
		raw = []byte("null")
	}
	return persist.Default.Set("misc:"+strconv.FormatInt(uid, 10)+":"+method, string(raw))
}

func validBotDataJSON(value *mtproto.DataJSON) bool {
	return value != nil && strings.TrimSpace(value.GetData()) != "" && json.Valid([]byte(value.GetData()))
}

// RPCInternalBotServer: status reporting is backed by the canonical
// PostgreSQL bot-status row; opaque bot requests remain fail closed until
// their respective providers are wired.

func (c *ApiFullCore) HelpSetBotUpdatesStatus(in *mtproto.TLHelpSetBotUpdatesStatus) (*mtproto.Bool, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	// Preserve the canonical refusal for an absent internal request. This also
	// prevents a nil transport decode from becoming a synthetic status row.
	if in == nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	if in.GetPendingUpdatesCount() < 0 || !utf8.ValidString(in.GetMessage()) ||
		utf8.RuneCountInString(in.GetMessage()) > 4096 || strings.IndexByte(in.GetMessage(), 0) >= 0 {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.UserClient == nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	profile, err := c.svcCtx.Dao.UserGetImmutableUser(c.ctx, &userpb.TLUserGetImmutableUser{Id: uid})
	if err != nil {
		return nil, err
	}
	if profile == nil || profile.GetUser() == nil || profile.Deleted() || !profile.IsBot() {
		return nil, mtproto.ErrBotMethodInvalid
	}
	if err = domain.UpsertBotUpdatesStatus(uid, in.GetPendingUpdatesCount(), in.GetMessage()); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func (c *ApiFullCore) BotsSendCustomRequest(in *mtproto.TLBotsSendCustomRequest) (*mtproto.DataJSON, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	if in == nil || in.GetCustomMethod() == "" || len(in.GetCustomMethod()) > 128 || !validBotDataJSON(in.GetParams()) {
		return nil, mtproto.ErrInputRequestInvalid
	}
	result := new(mtproto.DataJSON)
	if err := c.callBusinessProvider("bots.sendCustomRequest", map[string]any{
		"custom_method": in.GetCustomMethod(), "params": in.GetParams(),
	}, result); err != nil {
		return nil, err
	}
	if !validBotDataJSON(result) {
		return nil, mtproto.ErrInternalServerError
	}
	return result, nil
}

func (c *ApiFullCore) BotsAnswerWebhookJSONQuery(in *mtproto.TLBotsAnswerWebhookJSONQuery) (*mtproto.Bool, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	if in == nil || in.GetQueryId() == 0 || in.GetData() == nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	var result struct {
		Value *bool `json:"value"`
	}
	if err := c.callBusinessProvider("bots.answerWebhookJSONQuery", map[string]any{
		"query_id": in.GetQueryId(), "data": in.GetData(),
	}, &result); err != nil {
		return nil, err
	}
	if result.Value == nil {
		return nil, mtproto.ErrInternalServerError
	}
	if *result.Value {
		return mtproto.BoolTrue, nil
	}
	return mtproto.BoolFalse, nil
}
