// Copyright 2025 Teamgram Authors
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

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/apifull/persist"
)

// ChannelsSetMainProfileTab
// channels.setMainProfileTab#3583fcb1 channel:InputChannel tab:ProfileTab = Bool;
func (c *UserChannelProfilesCore) ChannelsSetMainProfileTab(in *mtproto.TLChannelsSetMainProfileTab) (*mtproto.Bool, error) {
	channelID, err := c.requireChannelAdmin(in.GetChannel())
	if err != nil {
		c.Logger.Errorf("channels.setMainProfileTab - error: %v", err)
		return nil, err
	}
	if in.GetTab() == nil {
		c.Logger.Errorf("channels.setMainProfileTab - error: empty tab")
		return nil, mtproto.ErrChannelInvalid
	}
	raw, err := json.Marshal(in.GetTab())
	if err != nil {
		c.Logger.Errorf("channels.setMainProfileTab - error: %v", err)
		return nil, err
	}
	if err = persist.Default.Set(profileTabKey(channelID), string(raw)); err != nil {
		c.Logger.Errorf("channels.setMainProfileTab - error: %v", err)
		return nil, err
	}
	return mtproto.BoolTrue, nil
}
