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

// RPCBusinessLocationServer: Layer 229 methods previously returned ERR_ENTERPRISE_IS_BLOCKED.

func locationKey(userId int64) string {
	return "bloc:" + strconv.FormatInt(userId, 10)
}

func (c *ApiFullCore) AccountUpdateBusinessLocation(in *mtproto.TLAccountUpdateBusinessLocation) (*mtproto.Bool, error) {
	if c.MD == nil || c.MD.UserId == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	key := locationKey(c.MD.UserId)
	if in == nil || (in.GetGeoPoint() == nil && in.GetAddress() == nil) {
		if err := persist.Default.Set(key, ""); err != nil {
			return nil, err
		}
		return mtproto.BoolTrue, nil
	}
	var lat, long float64
	var address string
	if in != nil {
		if g := in.GetGeoPoint(); g != nil {
			lat, long = g.GetLat(), g.GetLong()
		}
		if a := in.GetAddress(); a != nil {
			address = a.GetValue()
		}
	}
	blob, err := json.Marshal(struct {
		Lat     float64 `json:"lat"`
		Long    float64 `json:"long"`
		Address string  `json:"address"`
	}{lat, long, address})
	if err != nil {
		return nil, err
	}
	if err := persist.Default.Set(key, string(blob)); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}
