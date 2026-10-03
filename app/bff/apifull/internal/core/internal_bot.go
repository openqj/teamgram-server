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

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
)

func miscSave(uid int64, method string, in any) error {
	raw, err := json.Marshal(in)
	if err != nil {
		raw = []byte("null")
	}
	return persist.Default.Set("misc:"+strconv.FormatInt(uid, 10)+":"+method, string(raw))
}

// RPCInternalBotServer: Layer 229 methods previously returned ERR_ENTERPRISE_IS_BLOCKED.

func (c *ApiFullCore) HelpSetBotUpdatesStatus(in *mtproto.TLHelpSetBotUpdatesStatus) (*mtproto.Bool, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if err = miscSave(uid, "HelpSetBotUpdatesStatus", in); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func (c *ApiFullCore) BotsSendCustomRequest(in *mtproto.TLBotsSendCustomRequest) (*mtproto.DataJSON, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	key := "misc:" + strconv.FormatInt(uid, 10) + ":BotsSendCustomRequest:params"
	data := ""
	if in != nil && in.GetParams() != nil {
		data = in.GetParams().GetData()
	}
	if data == "" {
		stored, err := persist.Default.Get(key)
		if err != nil {
			return nil, err
		}
		if stored == "" {
			stored = "{}"
		}
		return mtproto.MakeTLDataJSON(&mtproto.DataJSON{Data: stored}).To_DataJSON(), nil
	}
	if err = persist.Default.Set(key, data); err != nil {
		return nil, err
	}
	return mtproto.MakeTLDataJSON(&mtproto.DataJSON{Data: data}).To_DataJSON(), nil
}

func (c *ApiFullCore) BotsAnswerWebhookJSONQuery(in *mtproto.TLBotsAnswerWebhookJSONQuery) (*mtproto.Bool, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if err = miscSave(uid, "BotsAnswerWebhookJSONQuery", in); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}
