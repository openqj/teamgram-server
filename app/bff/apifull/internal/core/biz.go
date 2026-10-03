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

// RPCBizServer: Layer 229 methods previously returned ERR_ENTERPRISE_IS_BLOCKED.

func restPut(userID int64, method string, in any) error {
	raw, err := json.Marshal(in)
	if err != nil {
		raw = []byte("null")
	}
	return persist.Default.Set("rest:"+method+":"+strconv.FormatInt(userID, 10), string(raw))
}

func restUpdates() *mtproto.Updates {
	return mtproto.MakeTLUpdates(&mtproto.Updates{
		Updates: []*mtproto.Update{},
		Users:   []*mtproto.User{},
		Chats:   []*mtproto.Chat{},
	}).To_Updates()
}

func (c *ApiFullCore) BizInvokeBizDataRaw(in *mtproto.TLBizInvokeBizDataRaw) (*mtproto.BizDataRaw, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if err = restPut(uid, "biz.invokeBizDataRaw", in); err != nil {
		return nil, err
	}
	return mtproto.MakeTLBizDataRaw(&mtproto.BizDataRaw{}).To_BizDataRaw(), nil
}
