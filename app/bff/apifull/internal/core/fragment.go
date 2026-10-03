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

import "github.com/teamgram/proto/mtproto"

// RPCFragmentServer: Layer 229 methods previously returned ERR_ENTERPRISE_IS_BLOCKED.
// Username / slug is stored locally under frag:<userId>. No Fragment HTTP call.

func fragBool(c *ApiFullCore, in any) (*mtproto.Bool, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	return nil, mtproto.ErrMethodNotImpl
}

func (c *ApiFullCore) AccountReorderUsernames(in *mtproto.TLAccountReorderUsernames) (*mtproto.Bool, error) {
	return fragBool(c, in)
}

func (c *ApiFullCore) AccountToggleUsername(in *mtproto.TLAccountToggleUsername) (*mtproto.Bool, error) {
	return fragBool(c, in)
}

func (c *ApiFullCore) ChannelsReorderUsernames(in *mtproto.TLChannelsReorderUsernames) (*mtproto.Bool, error) {
	return fragBool(c, in)
}

func (c *ApiFullCore) ChannelsToggleUsername(in *mtproto.TLChannelsToggleUsername) (*mtproto.Bool, error) {
	return fragBool(c, in)
}

func (c *ApiFullCore) ChannelsDeactivateAllUsernames(in *mtproto.TLChannelsDeactivateAllUsernames) (*mtproto.Bool, error) {
	return fragBool(c, in)
}

func (c *ApiFullCore) BotsReorderUsernames(in *mtproto.TLBotsReorderUsernames) (*mtproto.Bool, error) {
	return fragBool(c, in)
}

func (c *ApiFullCore) BotsToggleUsername(in *mtproto.TLBotsToggleUsername) (*mtproto.Bool, error) {
	return fragBool(c, in)
}

func (c *ApiFullCore) FragmentGetCollectibleInfo(in *mtproto.TLFragmentGetCollectibleInfo) (*mtproto.Fragment_CollectibleInfo, error) {
	if _, err := c.requireUserId(); err != nil {
		return nil, err
	}
	return nil, mtproto.ErrMethodNotImpl
}
