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

// RPCBoostsServer: Layer 229 methods previously returned ERR_ENTERPRISE_IS_BLOCKED.

func (c *ApiFullCore) boostsUnavailable() error {
	if _, err := c.requireUserId(); err != nil {
		return err
	}
	return mtproto.ErrMethodNotImpl
}

func (c *ApiFullCore) ChannelsSetBoostsToUnblockRestrictions(in *mtproto.TLChannelsSetBoostsToUnblockRestrictions) (*mtproto.Updates, error) {
	_ = in
	return nil, c.boostsUnavailable()
}

func (c *ApiFullCore) PremiumGetBoostsList(in *mtproto.TLPremiumGetBoostsList) (*mtproto.Premium_BoostsList, error) {
	_ = in
	return nil, c.boostsUnavailable()
}

func (c *ApiFullCore) PremiumGetMyBoosts(in *mtproto.TLPremiumGetMyBoosts) (*mtproto.Premium_MyBoosts, error) {
	_ = in
	return nil, c.boostsUnavailable()
}

func (c *ApiFullCore) PremiumApplyBoost(in *mtproto.TLPremiumApplyBoost) (*mtproto.Premium_MyBoosts, error) {
	_ = in
	return nil, c.boostsUnavailable()
}

func (c *ApiFullCore) PremiumGetBoostsStatus(in *mtproto.TLPremiumGetBoostsStatus) (*mtproto.Premium_BoostsStatus, error) {
	_ = in
	return nil, c.boostsUnavailable()
}

func (c *ApiFullCore) PremiumGetUserBoosts(in *mtproto.TLPremiumGetUserBoosts) (*mtproto.Premium_BoostsList, error) {
	_ = in
	return nil, c.boostsUnavailable()
}

func (c *ApiFullCore) StoriesGetBoostsStatus(in *mtproto.TLStoriesGetBoostsStatus) (*mtproto.Stories_BoostsStatus, error) {
	_ = in
	return nil, c.boostsUnavailable()
}

func (c *ApiFullCore) StoriesGetBoostersList(in *mtproto.TLStoriesGetBoostersList) (*mtproto.Stories_BoostersList, error) {
	_ = in
	return nil, c.boostsUnavailable()
}

func (c *ApiFullCore) StoriesCanApplyBoost(in *mtproto.TLStoriesCanApplyBoost) (*mtproto.Stories_CanApplyBoostResult, error) {
	_ = in
	return nil, c.boostsUnavailable()
}

func (c *ApiFullCore) StoriesApplyBoost(in *mtproto.TLStoriesApplyBoost) (*mtproto.Bool, error) {
	_ = in
	return nil, c.boostsUnavailable()
}
