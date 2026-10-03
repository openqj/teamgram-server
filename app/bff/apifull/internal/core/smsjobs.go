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
	"fmt"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/internal/persist"
)

// RPCSmsjobsServer: Layer 229 methods previously returned ERR_ENTERPRISE_IS_BLOCKED.

// smsPut is retained for unrelated local audit records used by translation and
// transcription handlers. SMS jobs themselves do not call it without a provider.
func smsPut(userID int64, op string, v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return persist.Default.Set(fmt.Sprintf("sms:%d:%s", userID, op), string(raw))
}

func (c *ApiFullCore) SmsjobsIsEligibleToJoin(in *mtproto.TLSmsjobsIsEligibleToJoin) (*mtproto.Smsjobs_EligibilityToJoin, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	return nil, mtproto.ErrMethodNotImpl
}

func (c *ApiFullCore) SmsjobsJoin(in *mtproto.TLSmsjobsJoin) (*mtproto.Bool, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	return nil, mtproto.ErrMethodNotImpl
}

func (c *ApiFullCore) SmsjobsLeave(in *mtproto.TLSmsjobsLeave) (*mtproto.Bool, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	return nil, mtproto.ErrMethodNotImpl
}

func (c *ApiFullCore) SmsjobsUpdateSettings(in *mtproto.TLSmsjobsUpdateSettings) (*mtproto.Bool, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	return nil, mtproto.ErrMethodNotImpl
}

func (c *ApiFullCore) SmsjobsGetStatus(in *mtproto.TLSmsjobsGetStatus) (*mtproto.Smsjobs_Status, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	return nil, mtproto.ErrMethodNotImpl
}

func (c *ApiFullCore) SmsjobsGetSmsJob(in *mtproto.TLSmsjobsGetSmsJob) (*mtproto.SmsJob, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	return nil, mtproto.ErrMethodNotImpl
}

func (c *ApiFullCore) SmsjobsFinishJob(in *mtproto.TLSmsjobsFinishJob) (*mtproto.Bool, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	return nil, mtproto.ErrMethodNotImpl
}
