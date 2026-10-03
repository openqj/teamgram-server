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

// RPCBusinessOpeningHoursServer: Layer 229 methods previously returned ERR_ENTERPRISE_IS_BLOCKED.

type weeklyOpenJSON struct {
	Start int32 `json:"start"`
	End   int32 `json:"end"`
}

func hoursKey(userId int64) string {
	return "bhours:" + strconv.FormatInt(userId, 10)
}

func (c *ApiFullCore) AccountUpdateBusinessWorkHours(in *mtproto.TLAccountUpdateBusinessWorkHours) (*mtproto.Bool, error) {
	if c.MD == nil || c.MD.UserId == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	key := hoursKey(c.MD.UserId)
	if in == nil || in.GetBusinessWorkHours() == nil {
		if err := persist.Default.Set(key, ""); err != nil {
			return nil, err
		}
		return mtproto.BoolTrue, nil
	}
	h := in.GetBusinessWorkHours()
	weekly := make([]weeklyOpenJSON, 0, len(h.GetWeeklyOpen()))
	for _, w := range h.GetWeeklyOpen() {
		if w == nil {
			continue
		}
		weekly = append(weekly, weeklyOpenJSON{Start: w.GetStartMinute(), End: w.GetEndMinute()})
	}
	blob, err := json.Marshal(struct {
		OpenNow  bool             `json:"open_now"`
		Timezone string           `json:"timezone_id"`
		Weekly   []weeklyOpenJSON `json:"weekly_open"`
	}{h.GetOpenNow(), h.GetTimezoneId(), weekly})
	if err != nil {
		return nil, err
	}
	if err := persist.Default.Set(key, string(blob)); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}
