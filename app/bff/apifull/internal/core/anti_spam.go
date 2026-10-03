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

// persistSetJSON stores v under key prefix "set:" so it does not collide with autosave keys.
func persistSetJSON(userID int64, method string, v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return persist.Default.Set("set:"+strconv.FormatInt(userID, 10)+":"+method, string(raw))
}

// RPCAntiSpamServer: Layer 229 methods previously returned ERR_ENTERPRISE_IS_BLOCKED.

func (c *ApiFullCore) ChannelsToggleAntiSpam(in *mtproto.TLChannelsToggleAntiSpam) (*mtproto.Updates, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if err := persistSetJSON(uid, "channels.toggleAntiSpam", in); err != nil {
		return nil, err
	}
	return emptyUpdates(), nil
}

func (c *ApiFullCore) ChannelsReportAntiSpamFalsePositive(in *mtproto.TLChannelsReportAntiSpamFalsePositive) (*mtproto.Bool, error) {
	uid, err := c.requireUserId()
	if err != nil {
		return nil, err
	}
	if err := persistSetJSON(uid, "channels.reportAntiSpamFalsePositive", in); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}
