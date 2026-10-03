// Copyright 2026 Teamgram Authors
//  All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// Author: teamgramio (teamgram.io@gmail.com)

package core

import "github.com/teamgram/proto/mtproto"

// RPCAffiliateProgramsServer: Layer 229 methods previously returned ERR_ENTERPRISE_IS_BLOCKED.

func (c *ApiFullCore) affiliateProgramsUnavailable() error {
	if _, err := c.requireUserId(); err != nil {
		return err
	}
	return mtproto.ErrMethodNotImpl
}

func (c *ApiFullCore) BotsUpdateStarRefProgram(in *mtproto.TLBotsUpdateStarRefProgram) (*mtproto.StarRefProgram, error) {
	_ = in
	return nil, c.affiliateProgramsUnavailable()
}

func (c *ApiFullCore) PaymentsGetConnectedStarRefBots(in *mtproto.TLPaymentsGetConnectedStarRefBots) (*mtproto.Payments_ConnectedStarRefBots, error) {
	_ = in
	return nil, c.affiliateProgramsUnavailable()
}

func (c *ApiFullCore) PaymentsGetConnectedStarRefBot(in *mtproto.TLPaymentsGetConnectedStarRefBot) (*mtproto.Payments_ConnectedStarRefBots, error) {
	_ = in
	return nil, c.affiliateProgramsUnavailable()
}

func (c *ApiFullCore) PaymentsGetSuggestedStarRefBots(in *mtproto.TLPaymentsGetSuggestedStarRefBots) (*mtproto.Payments_SuggestedStarRefBots, error) {
	_ = in
	return nil, c.affiliateProgramsUnavailable()
}

func (c *ApiFullCore) PaymentsConnectStarRefBot(in *mtproto.TLPaymentsConnectStarRefBot) (*mtproto.Payments_ConnectedStarRefBots, error) {
	_ = in
	return nil, c.affiliateProgramsUnavailable()
}

func (c *ApiFullCore) PaymentsEditConnectedStarRefBot(in *mtproto.TLPaymentsEditConnectedStarRefBot) (*mtproto.Payments_ConnectedStarRefBots, error) {
	_ = in
	return nil, c.affiliateProgramsUnavailable()
}
