// Copyright 2022 Teamgram Authors
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
	"strings"

	"github.com/teamgram/proto/mtproto"
)

// AuthToggleBan
// auth.toggleBan flags:# phone:string predefined:flags.0?true expires:flags.1?int reason:flags.1?string = PredefinedUser;
func (c *AuthorizationCore) AuthToggleBan(in *mtproto.TLAuthToggleBan) (*mtproto.PredefinedUser, error) {
	if !c.MD.IsAdmin {
		c.Logger.Errorf("auth.toggleBan - RIGHT_FORBIDDEN")
		return nil, mtproto.ErrRightForbidden
	}
	if strings.TrimSpace(in.GetPhone()) == "" {
		c.Logger.Errorf("auth.toggleBan - phone empty")
		return nil, mtproto.ErrPhoneNumberInvalid
	}
	// User service has no ban/predefined RPC. Do not ban.
	_ = in.GetPredefined()
	_ = in.GetExpires()
	_ = in.GetReason()
	c.Logger.Errorf("auth.toggleBan - RIGHT_FORBIDDEN")
	return nil, mtproto.ErrRightForbidden
}
