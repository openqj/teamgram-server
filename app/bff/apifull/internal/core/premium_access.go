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

package core

import (
	"time"

	"github.com/teamgram/proto/mtproto"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// requirePremium checks the caller's authoritative user record before a
// Telegram Business method can read or mutate Business state. Premium remains
// true for some records after expiry, so a non-zero expiry is checked too.
func (c *ApiFullCore) requirePremium() error {
	uid, err := c.requireUserId()
	if err != nil {
		return err
	}
	d := c.apifullDao()
	if d == nil || d.UserClient == nil {
		return mtproto.ErrMethodNotImpl
	}
	profile, err := d.UserClient.UserGetImmutableUser(callContext(c), &userpb.TLUserGetImmutableUser{Id: uid})
	if err != nil {
		return err
	}
	if profile == nil || profile.GetUser() == nil || profile.GetUser().GetId() != uid {
		return mtproto.ErrInternalServerError
	}
	data := profile.GetUser()
	if !data.GetPremium() {
		return mtproto.ErrPremiumAccountRequired
	}
	if expiry := data.GetPremiumExpireDate(); expiry != nil && expiry.GetValue() > 0 && expiry.GetValue() <= time.Now().Unix() {
		return mtproto.ErrPremiumAccountRequired
	}
	return nil
}
